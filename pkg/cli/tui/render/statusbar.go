package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// StatusBar displays token usage, duration, and model info at the bottom
// of the chat area.
//
// Normal completion:
//
//	142/300 tokens │ 3.2s │ deepseek-chat
//
// Aborted:
//
//	── aborted ── │ 48/?? tokens │ 1.1s │ deepseek-chat
type StatusBar struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	Duration         time.Duration
	Model            string
	Aborted          bool
	width            int
}

// NewStatusBar creates a StatusBar with the given terminal width.
func NewStatusBar(width int) *StatusBar {
	return &StatusBar{width: width}
}

// SetFromUsage populates the bar from a TokenUsage and metadata.
func (b *StatusBar) SetFromUsage(usage *TokenUsage, model string, duration time.Duration, aborted bool) {
	b.Model = model
	b.Duration = duration
	b.Aborted = aborted
	if usage != nil {
		b.PromptTokens = usage.PromptTokens
		b.CompletionTokens = usage.CompletionTokens
		b.TotalTokens = usage.TotalTokens
	}
}

// Render produces the status line string.
func (b *StatusBar) Render() string {
	if b.width <= 0 {
		b.width = 80
	}

	var parts []string

	// Left: token count or abort indicator
	if b.Aborted {
		abortStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("208")) // orange
		parts = append(parts, abortStyle.Render("── aborted ──"))
	} else if b.TotalTokens > 0 {
		tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		parts = append(parts, tokenStyle.Render(
			fmt.Sprintf("%d/%d tokens", b.PromptTokens+b.CompletionTokens, b.TotalTokens)))
	}

	// Duration
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	parts = append(parts, dimStyle.Render(fmt.Sprintf("%.1fs", b.Duration.Seconds())))

	// Model
	modelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("249"))
	parts = append(parts, modelStyle.Render(b.Model))

	line := strings.Join(parts, dimStyle.Render(" │ "))

	// Truncate to width if needed
	w := runewidth.StringWidth(line)
	if w > b.width {
		line = runewidth.Truncate(line, b.width-3, "…")
	}

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	topBorder := borderStyle.Render(strings.Repeat("─", b.width))
	return topBorder + "\n" + line
}
