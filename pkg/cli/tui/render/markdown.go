package render

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
)

// MarkdownRenderer converts markdown text to ANSI-styled terminal output.
//
// It wraps charmbracelet/glamour with sensible defaults and caches the
// renderer instance for reuse across multiple calls.
type MarkdownRenderer struct {
	width   int
	profile termenv.Profile
}

// NewMarkdownRenderer creates a renderer that word-wraps at the given width.
// The profile controls color depth (TrueColor, ANSI256, ANSI, Ascii).
// Pass 0 for width to use a default of 80.
func NewMarkdownRenderer(width int, profile termenv.Profile) *MarkdownRenderer {
	if width <= 0 {
		width = 80
	}
	return &MarkdownRenderer{
		width:   width,
		profile: profile,
	}
}

// Render converts a markdown string to ANSI-formatted terminal output.
// On error it returns the original content unchanged.
func (m *MarkdownRenderer) Render(content string) string {
	if content == "" {
		return ""
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithColorProfile(m.profile),
		glamour.WithWordWrap(m.width),
	)
	if err != nil {
		return content
	}

	rendered, err := r.Render(content)
	if err != nil {
		return content
	}

	// Trim trailing newlines that glamour likes to add.
	return strings.TrimRight(rendered, "\n")
}

// SetWidth updates the wrap width for subsequent Render calls.
func (m *MarkdownRenderer) SetWidth(width int) {
	if width > 0 {
		m.width = width
	}
}
