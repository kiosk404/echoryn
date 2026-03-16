package bubbletea

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
)

// renderInputView renders the inline input area.
// This is the only thing bubbletea manages — the rest goes to stdout.
func renderInputView(m InputModel) string {
	if !m.Ready || m.Width == 0 {
		return ""
	}

	var sections []string

	// 1. Top border
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	topBorder := borderStyle.Render(strings.Repeat("─", m.Width))
	sections = append(sections, topBorder)

	// 2. Input lines with prompt and cursor
	visualLines := m.InputBuffer.VisualLines()
	s := theme.GetStyles()

	for i, vl := range visualLines {
		// Prompt: primary for first line, multiline prompt for rest
		var prompt string
		if i == 0 {
			prompt = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				Render(m.Prompt)
		} else {
			prompt = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Render(m.MultilinePrompt)
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

			// Ghost text (completion preview)
			if m.GhostText != "" && i == 0 {
				ghost := s.InputGhost.Render(m.GhostText)
				textLine += ghost
			}

			sections = append(sections, prompt+textLine)
		} else {
			sections = append(sections, prompt+s.InputText.Render(text))
		}
	}

	// If no visual lines, show empty prompt with cursor
	if len(visualLines) == 0 {
		prompt := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Render(m.Prompt)
		cursor := s.InputCursor.Render(" ")
		sections = append(sections, prompt+cursor)
	}

	// 3. Completion suggestions (if visible)
	if m.ShowCompletion && len(m.Completions) > 0 {
		sections = append(sections, renderCompletions(m))
	}

	// 4. Bottom border
	bottomBorder := borderStyle.Render(strings.Repeat("─", m.Width))
	sections = append(sections, bottomBorder)

	// 5. Status hints
	hints := renderHints(m)
	if hints != "" {
		sections = append(sections, hints)
	}

	return strings.Join(sections, "\n")
}

// renderCompletions renders the completion popup.
func renderCompletions(m InputModel) string {
	if len(m.Completions) == 0 {
		return ""
	}

	s := theme.GetStyles()
	var lines []string

	maxShow := 8
	if len(m.Completions) < maxShow {
		maxShow = len(m.Completions)
	}

	for i := 0; i < maxShow; i++ {
		comp := m.Completions[i]
		var style lipgloss.Style
		if i == m.CompletionIndex {
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

	if len(m.Completions) > maxShow {
		more := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render(fmt.Sprintf("  ... and %d more", len(m.Completions)-maxShow))
		lines = append(lines, more)
	}

	return strings.Join(lines, "\n")
}

// renderHints renders the keyboard shortcut hints below the input.
func renderHints(m InputModel) string {
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	hints := []string{
		"Enter: Send",
		"Alt+Enter: Newline",
		"Tab: Accept",
		"↑↓: Navigate",
		"Ctrl+C: Exit",
	}

	return hintStyle.Render(strings.Join(hints, " │ "))
}
