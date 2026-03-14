package bubbletea

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/markdown"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/spinner"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
)

// View renders the current model state as a string.
func View(m AppModel) string {
	if m.Width == 0 || m.Height == 0 || !m.Ready {
		return "Initializing..."
	}

	// Calculate layout
	m.CalculateLayout()

	// Build the UI
	var sections []string

	// 1. Header
	sections = append(sections, renderHeader(&m))

	// 2. Main content (with optional team panel)
	if m.ShowTeamPanel && m.Team != nil {
		mainContent := renderMainContent(&m)
		teamPanel := renderTeamPanel(&m)
		separator := m.Styles.InputBorder.Render("│")

		content := lipgloss.JoinHorizontal(lipgloss.Top, mainContent, separator, teamPanel)
		sections = append(sections, content)
	} else {
		sections = append(sections, renderMainContent(&m))
	}

	// 3. Suggestions (if shown)
	if m.ShowCompletion && len(m.Completions) > 0 {
		sections = append(sections, renderSuggestions(&m))
	}

	// 4. Input
	sections = append(sections, renderInput(&m))

	// 5. Footer
	sections = append(sections, renderFooter(&m))

	return lipgloss.JoinVertical(lipgloss.Top, sections...)
}

// =============================================================================
// Header
// =============================================================================

func renderHeader(m *AppModel) string {
	s := m.Styles
	var parts []string

	// Session info
	session := s.HeaderInfo.Render("Session: " + m.ShortSessionID())
	parts = append(parts, session)

	// Agent info
	agent := s.HeaderInfo.Render("Agent: " + m.AgentID)
	parts = append(parts, agent)

	// Model info
	model := s.HeaderInfo.Render("Model: " + m.ModelName)
	parts = append(parts, model)

	// Team info (if active)
	if m.Team != nil && m.Team.Enabled {
		team := s.SuccessMessage.Render(fmt.Sprintf("Team: %s (%d members)", m.Team.Name, len(m.Team.Members)))
		parts = append(parts, team)
	}

	header := strings.Join(parts, " │ ")

	// Add streaming indicator
	status := ""
	switch m.Streaming {
	case StreamingResponding:
		status = " " + m.Spinner.View()
	case StreamingWaitingConfirmation:
		status = s.HeaderStatus.Render(" ⚠ Awaiting confirmation...")
	}

	line := header + status

	// Ensure proper width
	return s.Header.Width(m.Width).Render(line)
}

// =============================================================================
// Main Content
// =============================================================================

func renderMainContent(m *AppModel) string {
	width := m.Layout.MainWidth
	height := m.Layout.ContentHeight

	// Use virtual list for messages
	visibleItems := m.MsgList.VisibleItems()

	var lines []string

	// Render visible messages
	for _, item := range visibleItems {
		msgText := renderMessageItem(item.Item, width)
		lines = append(lines, msgText)

		// Measure and update height
		itemHeight := strings.Count(msgText, "\n") + 1
		m.MsgList.SetItemHeight(item.Item.ID(), itemHeight)
	}

	// Pad if needed
	for len(lines) < height {
		lines = append([]string{""}, lines...)
	}

	// Take only what fits
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Render(content)
}

// renderMessageItem renders a message item for display.
func renderMessageItem(item interface{}, width int) string {
	styles := theme.GetStyles()

	// Type assert to HistoryItem
	switch h := item.(type) {
	case *HistoryItemUser:
		return renderUserMsg(h, width, styles)
	case *HistoryItemAssistant:
		return renderAssistantMsg(h, width, styles)
	case *HistoryItemToolGroup:
		return renderToolGroupMsg(h, width, styles)
	case *HistoryItemInfo:
		return renderInfoMsg(h, width, styles)
	case *HistoryItemTeamMessage:
		from := styles.AssistantLabel.Render("[" + h.From + "]")
		content := styles.AssistantContent.Width(width - len(h.From) - 4).Render(h.Content)
		return from + " " + content
	case interface{ GetID() string }:
		// Fallback for items with GetID method
		return fmt.Sprintf("%v", item)
	}
	return ""
}

func renderUserMsg(m *HistoryItemUser, width int, s *theme.Styles) string {
	prompt := s.UserPrompt.Render("> ")
	content := s.UserContent.Width(width - 2).Render(m.Content)
	return prompt + content
}

func renderAssistantMsg(m *HistoryItemAssistant, width int, s *theme.Styles) string {
	label := s.AssistantLabel.Render("◈ ")

	content := m.Content
	if m.Streaming {
		content += "▌"
	}

	// Use markdown renderer for assistant messages
	md := markdown.NewRenderer(width - 3)
	rendered := md.Render(content)

	return label + rendered
}

func renderToolGroupMsg(g *HistoryItemToolGroup, width int, s *theme.Styles) string {
	var lines []string

	// Status icon
	statusIcon := spinner.StatusIcon(string(g.Status))

	// Header
	header := s.ToolHeader.Render(statusIcon + " Tool Calls")
	lines = append(lines, header)

	// List tool calls
	for _, call := range g.Calls {
		line := s.ToolName.Render("  → " + call.Name)

		if call.Arguments != "" {
			args := call.Arguments
			if len(args) > 50 {
				args = args[:47] + "..."
			}
			line += s.ToolArgs.Render("(" + args + ")")
		}

		lines = append(lines, line)
	}

	// Results
	if len(g.Results) > 0 {
		for _, result := range g.Results {
			if result.Error != nil {
				lines = append(lines, s.ToolError.Render("  Error: "+result.Error.Error()))
			} else {
				res := result.Result
				if len(res) > 100 {
					res = res[:97] + "..."
				}
				lines = append(lines, s.ToolResult.Render("  Result: "+res))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func renderInfoMsg(m *HistoryItemInfo, width int, s *theme.Styles) string {
	icon := spinner.StatusIcon(string(m.Level))

	var style lipgloss.Style
	switch m.Level {
	case InfoSuccess:
		style = s.SuccessMessage
	case InfoWarning:
		style = s.WarningMessage
	case InfoError:
		style = s.ErrorMessage
	default:
		style = s.SystemMessage
	}

	return style.Width(width).Render(icon + " " + m.Content)
}

// =============================================================================
// Team Panel
// =============================================================================

func renderTeamPanel(m *AppModel) string {
	s := m.Styles
	width := m.Layout.TeamPanelWidth
	height := m.Layout.ContentHeight

	if m.Team == nil {
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Render("No team")
	}

	var lines []string

	// Title
	title := s.TeamTitle.Width(width).Render("📋 " + m.Team.Name)
	lines = append(lines, title)

	// Strategy
	strategy := s.TeamStatus.Width(width).Render("Strategy: " + m.Team.Strategy)
	lines = append(lines, strategy)

	// Separator
	lines = append(lines, s.InputBorder.Render(strings.Repeat("─", width)))

	// Members
	for i, member := range m.Team.Members {
		focusMarker := "  "
		if i == m.Team.FocusIndex {
			focusMarker = s.TeamFocus.Render("▸ ")
		}

		icon := spinner.StatusIcon(string(member.Status))
		roleIcon := MemberRoleIcon(member.Role, i == 0)

		t := theme.GetTheme()
		statusColor := memberStatusColor(member.Status, t)

		label := member.Label
		if i == m.Team.FocusIndex {
			label = lipgloss.NewStyle().Bold(true).Render(label)
		}

		line := focusMarker + lipgloss.NewStyle().
			Foreground(statusColor).
			Render(icon) + " " + roleIcon + " " + label

		// Status
		status := s.TeamStatus.Render("[" + string(member.Status) + "]")
		line += " " + status

		lines = append(lines, lipgloss.NewStyle().Width(width).Render(line))

		// Progress (if any)
		if member.Progress != "" {
			progress := s.TeamMessage.Render("    " + member.Progress)
			lines = append(lines, lipgloss.NewStyle().Width(width).Render(progress))
		}
	}

	// Separator
	lines = append(lines, s.InputBorder.Render(strings.Repeat("─", width)))

	// Recent team messages
	lines = append(lines, s.TeamStatus.Render("Messages:"))

	msgCount := 0
	maxMsgs := height - len(lines) - 1
	for i := len(m.Team.Messages) - 1; i >= 0 && msgCount < maxMsgs; i-- {
		msg := m.Team.Messages[i]
		line := s.TeamMessage.Width(width).Render("[" + msg.From + "] " + truncate(msg.Content, width-5))
		lines = append(lines, line)
		msgCount++
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, "")
	}

	return lipgloss.NewStyle().
		Width(width).
		Render(strings.Join(lines, "\n"))
}

func memberStatusColor(status MemberStatus, t *theme.SemanticColors) lipgloss.Color {
	switch status {
	case StatusIdle:
		return t.Text.Secondary
	case StatusRunning:
		return t.Status.Warning
	case StatusCompleted:
		return t.Status.Success
	case StatusFailed:
		return t.Status.Error
	default:
		return t.Text.Secondary
	}
}

// =============================================================================
// Suggestions
// =============================================================================

func renderSuggestions(m *AppModel) string {
	s := m.Styles
	width := m.Layout.MainWidth

	if len(m.Completions) == 0 {
		return ""
	}

	var lines []string

	for i, comp := range m.Completions {
		var style lipgloss.Style
		if i == m.CompletionIndex {
			style = s.SuggestionActive
		} else {
			style = s.SuggestionItem
		}

		line := style.Width(width).Render(comp.Display)
		if comp.Description != "" {
			line += s.SuggestionDesc.Render(" - " + comp.Description)
		}
		lines = append(lines, line)
	}

	// Show count if many
	if len(m.Completions) > 5 {
		count := s.HeaderInfo.Render(fmt.Sprintf("(%d/%d)", m.CompletionIndex+1, len(m.Completions)))
		lines = append(lines, count)
	}

	return strings.Join(lines, "\n")
}

// =============================================================================
// Input
// =============================================================================

func renderInput(m *AppModel) string {
	s := m.Styles
	width := m.Layout.MainWidth

	// Border top
	border := s.InputBorder.Render(strings.Repeat("─", width))

	// Get visual lines from buffer
	visualLines := m.InputBuffer.VisualLines()

	var lines []string
	lines = append(lines, border)

	for _, vl := range visualLines {
		// Prompt
		prompt := s.InputPrompt.Render(m.Config.Prompt)

		// Text before cursor
		text := vl.Text
		cursorCol := vl.CursorCol

		if vl.IsCursor && cursorCol >= 0 {
			// Split at cursor
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

			// Render with cursor
			cursor := s.InputCursor.Render(atCursor)
			textLine := s.InputText.Render(before) + cursor + s.InputText.Render(after)

			// Ghost text (completion preview)
			if m.GhostText != "" {
				ghost := s.InputGhost.Render(m.GhostText)
				textLine += ghost
			}

			lines = append(lines, prompt+textLine)
		} else {
			lines = append(lines, prompt+s.InputText.Render(text))
		}
	}

	return strings.Join(lines, "\n")
}

// =============================================================================
// Footer
// =============================================================================

func renderFooter(m *AppModel) string {
	s := m.Styles

	shortcuts := []string{
		"Ctrl+T: Team",
		"Ctrl+N/P: Focus",
		"/: Command",
		"Ctrl+C: Exit",
	}

	// Add streaming hint
	if m.Streaming == StreamingResponding {
		shortcuts = append([]string{"Esc: Cancel"}, shortcuts...)
	}

	shortcutStr := strings.Join(shortcuts, " │ ")

	// Show error if any
	if m.LastError != "" {
		return s.Footer.Width(m.Width).Render(
			s.ErrorMessage.Render("Error: "+m.LastError) + " │ " + shortcutStr,
		)
	}

	return s.Footer.Width(m.Width).Render(shortcutStr)
}

// =============================================================================
// Helpers
// =============================================================================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
