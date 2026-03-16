package bubbletea

import (
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/completion"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/textbuffer"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
)

// InputResult is the outcome of a single input round.
type InputResult struct {
	// Content is the submitted text (empty if user quit).
	Content string

	// Quit is true if the user pressed Ctrl-C / Ctrl-D to exit.
	Quit bool
}

// InputModel is the bubbletea model for the input phase.
// It manages the text buffer, completion, and key bindings.
// It does NOT manage the conversation history or streaming —
// those are handled by the outer TUI loop.
type InputModel struct {
	// === Input State ===
	InputBuffer *textbuffer.Buffer

	// === Completion State ===
	CompletionMgr   *completion.Manager
	Completions     []completion.Completion
	CompletionIndex int
	ShowCompletion  bool
	GhostText       string

	// === UI State ===
	Width  int
	Height int
	Ready  bool

	// === Theme ===
	Theme  *theme.SemanticColors
	Styles *theme.Styles

	// === Result ===
	Result *InputResult

	// === Config ===
	Prompt          string
	MultilinePrompt string
}

// NewInputModel creates a new InputModel for one input round.
func NewInputModel(registry *command.Registry, prompt, multilinePrompt string) InputModel {
	t := theme.GetTheme()
	s := theme.GetStyles()

	buf := textbuffer.NewBuffer()

	// Create completion manager from command registry
	compMgr := completion.NewManager()
	if registry != nil {
		for _, info := range registry.CommandInfos() {
			compMgr.RegisterCommand(completion.Command{
				Name:        info.Name, // without "/" prefix — completion.go adds it
				Description: info.Description,
				Aliases:     info.Aliases, // without "/" prefix
			})
		}
	}

	p := prompt
	if p == "" {
		p = "> "
	}
	mp := multilinePrompt
	if mp == "" {
		mp = "· "
	}

	return InputModel{
		InputBuffer:     buf,
		CompletionMgr:   compMgr,
		Theme:           t,
		Styles:          s,
		Prompt:          p,
		MultilinePrompt: mp,
	}
}
