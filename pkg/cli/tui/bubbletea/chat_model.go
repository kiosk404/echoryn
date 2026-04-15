package bubbletea

import (
	"context"
	"strings"

	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/completion"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/textbuffer"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
)

// ─────────────────────────────────────────────────────────────────────────────
// Phase — State Machine core enum.
// ─────────────────────────────────────────────────────────────────────────────

// Phase represents the current phase of the chat lifecycle.
// This is the core of the State Machine pattern — all Update() and View()
// dispatch is driven by the current Phase.
type Phase int

const (
	// PhaseInput is the default phase: the user is typing.
	PhaseInput Phase = iota

	// PhaseThinking is active after the user submits a message,
	// while waiting for the first streaming delta.
	PhaseThinking

	// PhaseStreaming is active while receiving text deltas from the LLM.
	PhaseStreaming

	// PhaseRendering is the brief phase where raw text is re-rendered
	// as markdown before being flushed to scroll-back.
	PhaseRendering
)

// String returns a human-readable phase name (useful for debugging).
func (p Phase) String() string {
	switch p {
	case PhaseInput:
		return "input"
	case PhaseThinking:
		return "thinking"
	case PhaseStreaming:
		return "streaming"
	case PhaseRendering:
		return "rendering"
	default:
		return "unknown"
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ChatClient — interface for backend communication.
// ─────────────────────────────────────────────────────────────────────────────

// ChatClient defines the interface the ChatModel needs from the backend.
// This mirrors tui.Client but lives in the bubbletea package to avoid
// an import cycle. The tui package bridges via clientBridge.
type ChatClient interface {
	// ChatStream sends messages and streams the response.
	ChatStream(ctx context.Context, messages []ChatMessage, cb func(delta string), toolCb func(name string)) (*ChatStreamResult, error)

	// Abort cancels the currently running agent execution.
	Abort(ctx context.Context) error

	// Model returns the model name.
	Model() string

	// BaseURL returns the server address.
	BaseURL() string

	// SessionKey returns the session identifier.
	SessionKey() string
}

// ChatMessage is the message format exchanged with the server.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatStreamResult holds the outcome of a streaming chat interaction.
type ChatStreamResult struct {
	Content string
	Usage   *ChatTokenUsage
}

// ─────────────────────────────────────────────────────────────────────────────
// ChatModel — the single long-lived BubbleTea model.
// ─────────────────────────────────────────────────────────────────────────────

// ChatModel is the single long-lived BubbleTea model that manages
// the entire chat lifecycle: input → thinking → streaming → rendering → input.
//
// Design patterns:
//   - State Machine: Phase enum drives all dispatch
//   - Strategy: Each phase has dedicated update/view handlers
//   - Facade: Single entry point for the TUI, hiding internal complexity
type ChatModel struct {
	// === Phase (State Machine) ===
	phase Phase

	// === Input State (reuses existing components) ===
	inputBuffer     *textbuffer.Buffer
	completionMgr   *completion.Manager
	completions     []completion.Completion
	completionIndex int
	showCompletion  bool
	ghostText       string

	// === Streaming State ===
	streamContent strings.Builder    // accumulates raw text for live View() display
	fullContent   strings.Builder    // accumulates ALL content across tool-call flushes (for history)
	streamCancel  context.CancelFunc // cancels the streaming HTTP request
	toolCalls     []string           // tool names called during this turn

	// === Rendering State ===
	renderedMarkdown string          // glamour-rendered output after stream completes
	lastUsage        *ChatTokenUsage // token usage from the last completed turn
	lastAborted      bool            // whether the last turn was aborted
	lastStartTime    int64           // unix nano, for duration calculation

	// === Conversation History (API messages) ===
	messages []ChatMessage

	// === Spinner State ===
	spinnerFrame int   // current animation frame index
	spinnerStart int64 // unix nano, for elapsed time display

	// === UI State ===
	width  int
	height int
	ready  bool

	// === Theme ===
	theme  *theme.SemanticColors
	styles *theme.Styles

	// === Config ===
	prompt          string
	multilinePrompt string
	bannerText      string // pre-rendered welcome banner to flush in Init()

	// === Dependencies ===
	client   ChatClient
	commands *command.Registry

	// === Team State ===
	teamState    *command.TeamState
	teamAPI      command.TeamAPI
	setTeamState func(state *command.TeamState)
}

// ChatModelConfig holds the configuration for creating a ChatModel.
type ChatModelConfig struct {
	Client          ChatClient
	Commands        *command.Registry
	Prompt          string
	MultilinePrompt string
	TeamAPI         command.TeamAPI
	BannerText      string // pre-rendered welcome banner
}

// NewChatModel creates a new ChatModel. This is called once at startup.
func NewChatModel(cfg ChatModelConfig) *ChatModel {
	t := theme.GetTheme()
	s := theme.GetStyles()

	buf := textbuffer.NewBuffer()

	// Create completion manager from command registry.
	compMgr := completion.NewManager()
	if cfg.Commands != nil {
		for _, info := range cfg.Commands.CommandInfos() {
			compMgr.RegisterCommand(completion.Command{
				Name:        info.Name,
				Description: info.Description,
				Aliases:     info.Aliases,
			})
		}
	}

	p := cfg.Prompt
	if p == "" {
		p = "> "
	}
	mp := cfg.MultilinePrompt
	if mp == "" {
		mp = "· "
	}

	return &ChatModel{
		phase:           PhaseInput,
		inputBuffer:     buf,
		completionMgr:   compMgr,
		theme:           t,
		styles:          s,
		prompt:          p,
		multilinePrompt: mp,
		bannerText:      cfg.BannerText,
		client:          cfg.Client,
		commands:        cfg.Commands,
		teamAPI:         cfg.TeamAPI,
	}
}
