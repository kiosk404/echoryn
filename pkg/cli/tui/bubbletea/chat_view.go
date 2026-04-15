package bubbletea

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
)

// chatSpinnerFrames — braille dot pattern (same as Claude Code / Gemini CLI).
var chatSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ─────────────────────────────────────────────────────────────────────────────
// View dispatch — Strategy Pattern.
//
// Each phase renders its own "active zone". Past turns live in scroll-back
// (flushed via tea.Println), so View() only renders the current state.
// ─────────────────────────────────────────────────────────────────────────────

// chatView is the main View renderer for ChatModel.
func chatView(m *ChatModel) string {
	if !m.ready || m.width == 0 {
		return ""
	}

	switch m.phase {
	case PhaseInput:
		return viewInput(m)
	case PhaseThinking:
		return viewThinking(m)
	case PhaseStreaming:
		return viewStreaming(m)
	case PhaseRendering:
		return viewRendering(m)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Input
// ─────────────────────────────────────────────────────────────────────────────

func viewInput(m *ChatModel) string {
	s := theme.GetStyles()

	var sections []string

	// Top border.
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	topBorder := borderStyle.Render(strings.Repeat("─", m.width))
	sections = append(sections, topBorder)

	// Input lines with prompt and cursor.
	visualLines := m.inputBuffer.VisualLines()

	for i, vl := range visualLines {
		var prompt string
		if i == 0 {
			prompt = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				Render(m.prompt)
		} else {
			prompt = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Render(m.multilinePrompt)
		}

		text := vl.Text
		cursorCol := vl.CursorCol

		if vl.IsCursor && cursorCol >= 0 {
			runes := []rune(text)
			before := ""
			atCursor := " "
			after := ""

			if cursorCol <= len(runes) {
				before = string(runes[:cursorCol])
				if cursorCol < len(runes) {
					atCursor = string(runes[cursorCol])
					after = string(runes[cursorCol+1:])
				}
			} else {
				before = string(runes)
			}

			cursor := s.InputCursor.Render(atCursor)
			textLine := s.InputText.Render(before) + cursor + s.InputText.Render(after)

			if m.ghostText != "" && i == 0 {
				ghost := s.InputGhost.Render(m.ghostText)
				textLine += ghost
			}

			sections = append(sections, prompt+textLine)
		} else {
			sections = append(sections, prompt+s.InputText.Render(text))
		}
	}

	// Empty prompt with cursor if no lines.
	if len(visualLines) == 0 {
		prompt := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Render(m.prompt)
		cursor := s.InputCursor.Render(" ")
		sections = append(sections, prompt+cursor)
	}

	// Completion suggestions.
	if m.showCompletion && len(m.completions) > 0 {
		sections = append(sections, viewChatCompletions(m))
	}

	// Bottom border.
	bottomBorder := borderStyle.Render(strings.Repeat("─", m.width))
	sections = append(sections, bottomBorder)

	// Hints.
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	hints := []string{
		"Enter: Send",
		"Esc: Abort",
		"Alt+Enter: Newline",
		"Tab: Accept",
		"↑↓: Navigate",
		"Ctrl+C: Exit",
	}
	sections = append(sections, hintStyle.Render(strings.Join(hints, " │ ")))

	return strings.Join(sections, "\n")
}

// viewChatCompletions renders the completion popup.
func viewChatCompletions(m *ChatModel) string {
	if len(m.completions) == 0 {
		return ""
	}

	s := theme.GetStyles()
	var lines []string

	maxShow := 8
	if len(m.completions) < maxShow {
		maxShow = len(m.completions)
	}

	for i := 0; i < maxShow; i++ {
		comp := m.completions[i]
		var style lipgloss.Style
		if i == m.completionIndex {
			style = s.SuggestionActive
		} else {
			style = s.SuggestionItem
		}

		line := style.Render(comp.Display)
		if comp.Description != "" {
			line += s.SuggestionDesc.Render(" — " + comp.Description)
		}
		lines = append(lines, line)
	}

	if len(m.completions) > maxShow {
		more := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render(fmt.Sprintf("  ... and %d more", len(m.completions)-maxShow))
		lines = append(lines, more)
	}

	return strings.Join(lines, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Thinking — animated spinner
// ─────────────────────────────────────────────────────────────────────────────

func viewThinking(m *ChatModel) string {
	t := theme.GetTheme()

	frame := chatSpinnerFrames[m.spinnerFrame%len(chatSpinnerFrames)]
	colorIdx := m.spinnerFrame % len(t.Accent)

	styledFrame := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent[colorIdx]).
		Render(frame)

	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color("242")).
		Render(" Thinking...")

	elapsed := time.Since(time.Unix(0, m.spinnerStart))
	var elapsedText string
	if elapsed > 2*time.Second {
		elapsedText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Render(fmt.Sprintf(" (%.1fs)", elapsed.Seconds()))
	}

	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("  (esc to cancel)")

	return styledFrame + label + elapsedText + hint
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Streaming — raw text (tool calls are flushed inline to scroll-back)
// ─────────────────────────────────────────────────────────────────────────────

func viewStreaming(m *ChatModel) string {
	var sections []string

	// Assistant marker + raw streaming text.
	content := m.streamContent.String()
	if content != "" {
		marker := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("183")).
			Render("✦ ")
		sections = append(sections, marker+content)
	}

	// ESC hint.
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("  (esc to cancel)")
	sections = append(sections, hint)

	return strings.Join(sections, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool call panel — friendly inline display
// ─────────────────────────────────────────────────────────────────────────────

// toolInfo maps tool names to display labels and icons.
var toolDisplayInfo = map[string]struct {
	icon  string
	label string
}{
	"web_search": {"🔍", "Web Search"},
	"web_fetch":  {"🌐", "Web Fetch"},
	"read_file":  {"📄", "Read File"},
	"write_file": {"✏️", "Write File"},
	"edit_file":  {"✏️", "Edit File"},
	"run_shell":  {"⚡", "Run Shell"},
	"list_files": {"📁", "List Files"},
}

// formatToolCallPanel renders a styled tool call indicator.
// Output looks like:
//
//	─ 🔍 Web Search ─────────────────

func formatToolCallPanel(name string, width int) string {
	icon := "⚙️"
	label := name

	if info, ok := toolDisplayInfo[name]; ok {
		icon = info.icon
		label = info.label
	}

	// Calculate available width for the border fill.
	borderColor := lipgloss.Color("63")
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("183"))

	renderedLabel := fmt.Sprintf(" %s %s ", icon, labelStyle.Render(label))
	labelLen := lipgloss.Width(renderedLabel)
	fillLen := width - labelLen - 10 //
	if fillLen < 0 {
		fillLen = 0
	}

	return borderStyle.Render("  ─────") + renderedLabel + borderStyle.Render(strings.Repeat("─", fillLen))
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Rendering — brief indicator
// ─────────────────────────────────────────────────────────────────────────────

func viewRendering(_ *ChatModel) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("242")).
		Render("  Rendering...")
}
