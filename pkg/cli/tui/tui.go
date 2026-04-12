package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	btea "github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea"
	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
	"github.com/kiosk404/echoryn/pkg/cli/tui/render"
	"github.com/kiosk404/echoryn/pkg/cli/tui/terminal"
	"github.com/kiosk404/echoryn/pkg/version"
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

// ChatResult holds the outcome of a streaming chat interaction.
type ChatResult struct {
	// Content is the full assistant reply text.
	Content string

	// Usage is token consumption data (nil if unavailable or aborted early).
	Usage *TokenUsage

	// Aborted is true if the user pressed Escape to interrupt.
	Aborted bool
}

// Client defines the interface the TUI needs from the chat backend.
// This decouples the TUI from the concrete HTTP client implementation,
// making it testable and extensible.
type Client interface {
	// ChatStream sends messages and streams the response.
	// cb is called for each text delta; toolCb for each tool invocation.
	// Returns the full assistant reply.
	ChatStream(ctx context.Context, messages []ChatMessage, cb StreamCallback, toolCb ToolCallCallback) (*ChatResult, error)

	// Abort cancels the currently running agent execution.
	Abort(ctx context.Context) error

	// Model returns the model name.
	Model() string

	// BaseURL returns the server address.
	BaseURL() string

	// SessionKey returns the session identifier.
	SessionKey() string
}

// TUI is the top-level interactive chat controller.
//
// It uses bubbletea in inline mode (no alt-screen) for input capture
// with completion and multiline editing, while streaming AI responses
// directly to stdout for scroll-back friendly output.
type TUI struct {
	client   Client
	term     *terminal.Terminal
	commands *command.Registry
	cfg      Config
	messages []ChatMessage

	// Team collaboration state.
	teamState    *command.TeamState
	teamAPI      command.TeamAPI
	teamEventSub command.TeamEventSubscriber
	eventWatcher *teamEventWatcher
}

// New creates a TUI instance. Call [TUI.Run] to start the interactive loop.
func New(client Client, opts ...Option) *TUI {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	t := &TUI{
		client:       client,
		cfg:          cfg,
		teamAPI:      cfg.TeamAPI,
		teamEventSub: cfg.TeamEventSubscriber,
	}

	// Initialize event watcher if subscriber is available.
	if t.teamEventSub != nil {
		handler := &tuiEventHandler{teamState: &t.teamState}
		t.eventWatcher = newTeamEventWatcher(t.teamEventSub, handler)
	}

	return t
}

// Run starts the interactive TUI main loop.
//
// It initialises all subsystems (terminal, commands, rendering),
// displays the welcome banner, and enters the read-eval-print loop
// powered by bubbletea inline mode.
//
// Each iteration:
//  1. Start a bubbletea Program in inline mode to get user input
//  2. Process the input (slash command or chat message)
//  3. For chat messages: stream the AI response to stdout
//  4. Repeat
//
// Run blocks until the user exits (via /quit, Ctrl-C, or Ctrl-D).
func (t *TUI) Run(ctx context.Context) error {
	// --- Terminal setup ---
	t.term = terminal.New(os.Stdin)
	stopResize := t.term.ListenResize()
	defer stopResize()

	// Ensure event watcher is stopped on exit.
	defer func() {
		if t.eventWatcher != nil {
			t.eventWatcher.Stop()
		}
	}()

	// --- Signal handling ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		t.term.Restore()
		render.PrintGoodbye()
		render.PrintResumeHint(t.cfg.ProgramName, t.client.SessionKey())
		os.Exit(0)
	}()

	// --- Command registry ---
	t.commands = command.NewRegistry()
	command.RegisterBuiltins(t.commands)
	command.RegisterTeamCommands(t.commands)

	// --- Welcome banner ---
	width := t.term.Width()
	render.PrintWelcomeBanner(render.BannerInfo{
		Version:    version.GitVersion,
		Model:      t.client.Model(),
		ServerAddr: t.client.BaseURL(),
		SessionKey: t.client.SessionKey(),
	}, width)

	// Flush stdout after banner
	os.Stdout.Sync()

	// --- Main REPL loop (bubbletea inline mode) ---
	for {
		result, err := t.readInput()
		if err != nil {
			return fmt.Errorf("tui: input: %w", err)
		}

		if result.Quit {
			render.PrintGoodbye()
			render.PrintResumeHint(t.cfg.ProgramName, t.client.SessionKey())
			return nil
		}

		line := strings.TrimSpace(result.Content)
		if line == "" {
			continue
		}

		// Print input border (after input box)
		t.printInputEcho(line)

		// --- Slash command handling ---
		if strings.HasPrefix(line, "/") {
			if err := t.handleCommand(ctx, line); err != nil {
				if errors.Is(err, command.ErrQuit) {
					render.PrintGoodbye()
					render.PrintResumeHint(t.cfg.ProgramName, t.client.SessionKey())
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

// readInput runs a single bubbletea inline Program to capture user input.
func (t *TUI) readInput() (*btea.InputResult, error) {
	model := btea.NewInputModel(
		t.commands,
		t.cfg.Prompt,
		t.cfg.MultilinePrompt,
	)

	p := tea.NewProgram(
		model,
		// Inline mode: do NOT use tea.WithAltScreen() so output
		// stays in the terminal's scroll-back buffer.
	)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m, ok := finalModel.(btea.InputModel)
	if !ok {
		return &btea.InputResult{Quit: true}, nil
	}

	if m.Result != nil {
		return m.Result, nil
	}

	// No result means the program quit without explicit result
	return &btea.InputResult{Quit: true}, nil
}

// printInputEcho prints the user's input in a styled format after
// the bubbletea input box has been cleared.
func (t *TUI) printInputEcho(line string) {
	render.PrintUserMessage(line, t.term.Width())
	fmt.Println()
}

// handleCommand dispatches a slash command.
func (t *TUI) handleCommand(ctx context.Context, rawInput string) error {
	cmd, args, ok := t.commands.Lookup(rawInput)
	if !ok {
		return fmt.Errorf("unknown command: %s (type /help for available commands)", rawInput)
	}

	// Capture team ID before command execution for change detection.
	prevTeamID := ""
	if t.teamState != nil {
		prevTeamID = t.teamState.ID
	}

	env := &command.Env{
		Out:          os.Stdout,
		ClearHistory: func() { t.messages = nil },
		Model:        t.client.Model,
		SessionKey:   t.client.SessionKey,
		TeamState:    t.teamState,
		SetTeamState: func(state *command.TeamState) { t.teamState = state },
		TeamAPI:      t.teamAPI,
	}

	if err := cmd.Execute(ctx, env, args); err != nil {
		return err
	}

	// Detect team state transitions and manage event watcher lifecycle.
	newTeamID := ""
	if t.teamState != nil {
		newTeamID = t.teamState.ID
	}
	if t.eventWatcher != nil {
		switch {
		case newTeamID != "" && newTeamID != prevTeamID:
			// Team created: start watching events.
			t.eventWatcher.Start(ctx, newTeamID)
		case newTeamID == "" && prevTeamID != "":
			// Team dissolved: stop watching events.
			t.eventWatcher.Stop()
		}
	}

	return nil
}

// handleChat sends a user message to the server and streams the response.
// It supports Esc/Ctrl-C to abort the running stream.
func (t *TUI) handleChat(ctx context.Context, message string) error {
	width := t.term.Width()
	startTime := time.Now()
	// Add to conversation history.
	t.messages = append(t.messages, ChatMessage{Role: "user", Content: message})

	// Prepare the streaming renderer.
	render.PrintAssistantLabel(width)
	renderer := render.NewStreamRenderer(width)
	renderer.StartThinking()

	// Create abortable context with extended timeout.
	reqCtx, reqCancel := context.WithTimeout(ctx, t.cfg.RequestTimeout)
	defer reqCancel()

	// Result channel: exactly one value when streaming finishes or is aborted.
	type chatOutcome struct {
		result *ChatResult
		err    error
	}
	outcomeCh := make(chan chatOutcome, 1)

	// Start streaming in a goroutine so we can monitor keyboard input.
	go func() {
		result, streamErr := t.client.ChatStream(reqCtx, t.messages,
			func(delta string) { renderer.OnDelta(delta) },
			func(toolName string) { renderer.OnToolCall(toolName) },
		)
		outcomeCh <- chatOutcome{result: result, err: streamErr}
	}()

	// Monitor keyboard for abort key (Esc) in a separate goroutine.
	// Uses raw mode byte reading — only during streaming, not during bubbletea input.
	abortCh := make(chan struct{})
	go func() {
		buf := [1]byte{}
		for {
			n, err := os.Stdin.Read(buf[:])
			if n > 0 && buf[0] == 0x1B { // ESC key
				close(abortCh)
				return
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for either: stream completion, abort signal, or context cancellation.
	select {
	case outcome := <-outcomeCh:
		// Stream finished normally or errored.
		if outcome.err != nil {
			content := renderer.OnError(outcome.err)
			if content != "" {
				t.messages = append(t.messages, ChatMessage{Role: "assistant", Content: content})
			}
			t.printStatusLine(outcome.result, true, message, startTime)
			return nil
		}
		// Success — finish rendering.
		content := renderer.Finish()
		t.printStatusLine(outcome.result, false, message, startTime)
		render.PrintAwaitingMessage()
		fmt.Println()
		os.Stdout.Sync()

		if outcome.result != nil && outcome.result.Content != "" && content == "" {
			t.messages[len(t.messages)-1] = ChatMessage{Role: "assistant", Content: outcome.result.Content}
		}
		return nil

	case <-abortCh:
		// User pressed Esc — abort!
		reqCancel() // Cancel HTTP request context immediately.

		// Also call server-side abort for graceful shutdown.
		_ = t.client.Abort(reqCtx) // Best-effort; ignore errors.

		// Wait for the stream goroutine to finish (should be quick after cancel).
		<-outcomeCh

		// Stop the renderer without markdown re-render.
		content := renderer.Abort()
		t.printStatusLine(nil, true, message, startTime)
		fmt.Println()
		os.Stdout.Sync()

		if content != "" {
			t.messages = append(t.messages, ChatMessage{Role: "assistant", Content: content})
		}
		return nil

	case <-reqCtx.Done():
		// Context cancelled (timeout or external).
		<-outcomeCh
		renderer.Abort()
		t.printStatusLine(nil, true, message, startTime)
		fmt.Println()
		return nil
	}
}

// printStatusLine renders the token/cost status bar after a chat exchange.
func (t *TUI) printStatusLine(result *ChatResult, aborted bool, userInput string, startTime time.Time) {
	width := t.term.Width()
	bar := render.NewStatusBar(width)

	if result != nil && result.Usage != nil {
		bar.SetFromUsage(result.Usage, t.client.Model(), time.Since(startTime), aborted)
	} else {
		bar.Aborted = aborted
		bar.Model = t.client.Model()
		bar.Duration = time.Since(startTime)
	}

	if result != nil && aborted {
		bar.Aborted = true
		if result.Usage != nil {
			bar.PromptTokens = result.Usage.PromptTokens
			bar.CompletionTokens = result.Usage.CompletionTokens
			bar.TotalTokens = result.Usage.TotalTokens
		}
	}

	fmt.Println(bar.Render())
}
