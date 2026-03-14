package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
	"github.com/kiosk404/echoryn/pkg/cli/tui/input"
	"github.com/kiosk404/echoryn/pkg/cli/tui/render"
	"github.com/kiosk404/echoryn/pkg/cli/tui/terminal"
	"github.com/kiosk404/echoryn/pkg/version"
	"github.com/reeflective/readline"
)

// ChatMessage is the message format exchanged with the Hivemind server.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamCallback is called for each text delta during streaming.
type StreamCallback func(delta string)

// ToolCallCallback is called when a tool call is detected in the stream.
type ToolCallCallback func(name string)

// Client defines the interface the TUI needs from the chat backend.
// This decouples the TUI from the concrete HTTP client implementation,
// making it testable and extensible.
type Client interface {
	// ChatStream sends messages and streams the response.
	// cb is called for each text delta; toolCb for each tool invocation.
	// Returns the full assistant reply.
	ChatStream(ctx context.Context, messages []ChatMessage, cb StreamCallback, toolCb ToolCallCallback) (string, error)

	// Model returns the model name.
	Model() string

	// BaseURL returns the server address.
	BaseURL() string

	// SessionKey returns the session identifier.
	SessionKey() string
}

// TUI is the top-level interactive chat controller.
//
// It owns the terminal, input reader, renderer, command registry, and
// the conversation message history. The [Run] method starts the main
// REPL loop and blocks until the user exits.
type TUI struct {
	client   Client
	term     *terminal.Terminal
	reader   *input.Reader
	commands *command.Registry
	cfg      Config
	messages []ChatMessage

	// Team collaboration state.
	teamState *command.TeamState
	teamAPI   command.TeamAPI
}

// New creates a TUI instance. Call [TUI.Run] to start the interactive loop.
func New(client Client, opts ...Option) *TUI {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &TUI{
		client:  client,
		cfg:     cfg,
		teamAPI: cfg.TeamAPI,
	}
}

// Run starts the interactive TUI main loop.
//
// It initialises all subsystems (terminal, input, commands, rendering),
// displays the welcome banner, and enters the read-eval-print loop.
// Run blocks until the user exits (via /quit, Ctrl-C, or Ctrl-D).
func (t *TUI) Run(ctx context.Context) error {
	// --- Terminal setup ---
	t.term = terminal.New(os.Stdin)
	stopResize := t.term.ListenResize()
	defer stopResize()

	// --- Signal handling ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		t.term.Restore()
		render.PrintGoodbye()
		os.Exit(0)
	}()

	// --- Command registry ---
	t.commands = command.NewRegistry()
	command.RegisterBuiltins(t.commands)
	command.RegisterTeamCommands(t.commands)

	// --- Input reader ---
	inputCfg := input.Config{
		Prompt:          t.cfg.Prompt,
		MultilinePrompt: t.cfg.MultilinePrompt,
		History:         t.cfg.History,
	}
	reader, err := input.NewReader(inputCfg, t.commands.CommandInfos())
	if err != nil {
		return fmt.Errorf("tui: init input reader: %w", err)
	}
	t.reader = reader
	defer t.reader.Close()

	// --- Welcome banner ---
	width := t.term.Width()
	render.PrintWelcomeBanner(render.BannerInfo{
		Version:    version.GitVersion,
		Model:      t.client.Model(),
		ServerAddr: t.client.BaseURL(),
		SessionKey: t.client.SessionKey(),
	}, width)

	// Flush stdout after banner so the terminal emulator finishes rendering
	// the large ASCII art before readline sends its cursor-position query.
	os.Stdout.Sync()

	// --- Main REPL loop ---
	for {
		line, err := t.reader.ReadLine()
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) || errors.Is(err, io.EOF) {
				t.term.Restore()
				render.PrintGoodbye()
				return nil
			}
			return fmt.Errorf("tui: readline: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// --- Slash command handling ---
		if strings.HasPrefix(line, "/") {
			if err := t.handleCommand(ctx, line); err != nil {
				if errors.Is(err, command.ErrQuit) {
					render.PrintGoodbye()
					return nil
				}
				render.PrintError(err.Error())
			}
			continue
		}

		// --- Chat message ---
		if err := t.handleChat(ctx, line); err != nil {
			render.PrintError(err.Error())
		}
	}
}

// handleCommand dispatches a slash command.
func (t *TUI) handleCommand(ctx context.Context, rawInput string) error {
	cmd, args, ok := t.commands.Lookup(rawInput)
	if !ok {
		return fmt.Errorf("unknown command: %s (type /help for available commands)", rawInput)
	}

	env := &command.Env{
		Out:          os.Stdout,
		ClearHistory: func() { t.messages = nil },
		Model:        t.client.Model,
		SessionKey:   t.client.SessionKey,
		SetTeamState: func(state *command.TeamState) { t.teamState = state },
		TeamAPI:      t.teamAPI,
	}

	return cmd.Execute(ctx, env, args)
}

// handleChat sends a user message to the server and streams the response.
func (t *TUI) handleChat(ctx context.Context, message string) error {
	width := t.term.Width()
	// Add to conversation history.
	t.messages = append(t.messages, ChatMessage{Role: "user", Content: message})

	// Prepare the streaming renderer.
	render.PrintAssistantLabel(width)
	renderer := render.NewStreamRenderer(width)
	renderer.StartThinking()

	// Send request with streaming.
	reqCtx, cancel := context.WithTimeout(ctx, t.cfg.RequestTimeout)
	defer cancel()

	// Adapt our ChatMessage slice to the client's expected type.
	fullContent, streamErr := t.client.ChatStream(reqCtx, t.messages,
		func(delta string) {
			renderer.OnDelta(delta)
		},
		func(toolName string) {
			renderer.OnToolCall(toolName)
		},
	)

	if streamErr != nil {
		content := renderer.OnError(streamErr)
		if content != "" {
			t.messages = append(t.messages, ChatMessage{Role: "assistant", Content: content})
		}
		return nil // error already displayed
	}

	// Finish: re-render as markdown.
	content := renderer.Finish()

	// Print "Awaiting" message after response.
	render.PrintAwaitingMessage()
	fmt.Println() // blank line after response

	// Flush stdout so the terminal emulator finishes rendering all of the
	// above output before readline sends its DSR cursor-position query.
	os.Stdout.Sync()

	// Use the server's full content if our local buffer missed something.
	if fullContent != "" && content == "" {
		t.messages[len(t.messages)-1] = ChatMessage{Role: "assistant", Content: fullContent}
	}

	return nil
}
