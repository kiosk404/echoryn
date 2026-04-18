package tui

import (
	"context"
	"fmt"
	"os"

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
// It creates a single long-lived BubbleTea Program that manages
// the full chat lifecycle (input → thinking → streaming → markdown
// re-render → back to input). Output is inline (no alt-screen) so
// completed turns stay in the terminal's scroll-back buffer.
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
// It creates a single long-lived BubbleTea Program that manages the
// entire chat lifecycle. The Program runs in inline mode (no alt-screen)
// so output stays in the terminal's scroll-back buffer.
//
// Run blocks until the user exits (via /quit, Ctrl-C, or Ctrl-D).
func (t *TUI) Run(_ context.Context) error {
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

	// --- Command registry ---
	t.commands = command.NewRegistry()
	command.RegisterBuiltins(t.commands)
	command.RegisterTeamCommands(t.commands)

	// --- Welcome banner (rendered to string, flushed via tea.Println in Init) ---
	width := t.term.Width()
	bannerText := render.FormatWelcomeBanner(render.BannerInfo{
		Version:    version.GitVersion,
		Model:      t.client.Model(),
		ServerAddr: t.client.BaseURL(),
		SessionKey: t.client.SessionKey(),
		Tools:      t.cfg.BannerTools,
		GolemNodes: t.cfg.BannerNodes,
		Skills:     t.cfg.BannerSkills,
	}, width)

	// --- Create the single long-lived ChatModel ---
	bridge := &clientBridge{inner: t.client}
	model := btea.NewChatModel(btea.ChatModelConfig{
		Client:          bridge,
		Commands:        t.commands,
		Prompt:          t.cfg.Prompt,
		MultilinePrompt: t.cfg.MultilinePrompt,
		TeamAPI:         t.teamAPI,
		BannerText:      bannerText,
	})

	// --- Create and run the BubbleTea Program ---
	p := tea.NewProgram(model)
	btea.SetChatProgram(p)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	render.PrintGoodbye()
	render.PrintResumeHint(t.cfg.ProgramName, t.client.SessionKey())
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// clientBridge adapts tui.Client to btea.ChatClient.
//
// This bridge exists because tui.Client and btea.ChatClient are identical
// in shape but live in different packages. The bridge converts between
// the two ChatMessage types and callback signatures.
// ─────────────────────────────────────────────────────────────────────────────

type clientBridge struct {
	inner Client
}

// Compile-time check.
var _ btea.ChatClient = (*clientBridge)(nil)

func (b *clientBridge) ChatStream(ctx context.Context, messages []btea.ChatMessage, cb func(string), toolCb func(string)) (*btea.ChatStreamResult, error) {
	// Convert btea.ChatMessage → tui.ChatMessage.
	tuiMsgs := make([]ChatMessage, len(messages))
	for i, m := range messages {
		tuiMsgs[i] = ChatMessage{Role: m.Role, Content: m.Content}
	}

	result, err := b.inner.ChatStream(ctx, tuiMsgs,
		StreamCallback(cb),
		ToolCallCallback(toolCb),
	)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	r := &btea.ChatStreamResult{Content: result.Content}
	if result.Usage != nil {
		r.Usage = &btea.ChatTokenUsage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	}
	return r, nil
}

func (b *clientBridge) Abort(ctx context.Context) error { return b.inner.Abort(ctx) }
func (b *clientBridge) Model() string                   { return b.inner.Model() }
func (b *clientBridge) BaseURL() string                 { return b.inner.BaseURL() }
func (b *clientBridge) SessionKey() string              { return b.inner.SessionKey() }
