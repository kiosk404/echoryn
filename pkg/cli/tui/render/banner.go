package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────────────────────────────────────
// Colours & Styles — shared across the entire render package
// ─────────────────────────────────────────────────────────────────────────────

var (
	// Brand gradient palette (blue → purple → pink).
	gradientColors = []lipgloss.Color{
		lipgloss.Color("63"),  // slate blue
		lipgloss.Color("99"),  // medium purple
		lipgloss.Color("135"), // medium orchid
		lipgloss.Color("171"), // orchid
		lipgloss.Color("207"), // hot pink
		lipgloss.Color("213"), // light pink
	}

	// Semantic colours.
	subtleColor    = lipgloss.Color("242") // dim gray
	dimColor       = lipgloss.Color("238") // very dim
	userColor      = lipgloss.Color("39")  // blue
	assistantColor = lipgloss.Color("183") // lavender
	successColor   = lipgloss.Color("82")  // green
	warningColor   = lipgloss.Color("214") // orange
	errorColor     = lipgloss.Color("196") // red
	infoColor      = lipgloss.Color("39")  // blue

	// Pre-computed styles.
	subtleStyle = lipgloss.NewStyle().
			Foreground(subtleColor)

	dimStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(errorColor)

	infoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(infoColor)
)

// ─────────────────────────────────────────────────────────────────────────────
// ASCII Art Logo
// ─────────────────────────────────────────────────────────────────────────────

// echorynLogo is the ASCII art banner (block-style).
var echorynLogo = []string{
	" ███████╗  ██████╗ ██╗  ██╗  ██████╗  ██████╗  ██╗   ██╗ ███╗   ██╗",
	" ██╔════╝ ██╔════╝ ██║  ██║ ██╔═══██╗ ██╔══██╗ ╚██╗ ██╔╝ ████╗  ██║",
	" █████╗   ██║      ███████║ ██║   ██║ ██████╔╝  ╚████╔╝  ██╔██╗ ██║",
	" ██╔══╝   ██║      ██╔══██║ ██║   ██║ ██╔══██╗   ╚██╔╝   ██║╚██╗██║",
	" ███████╗ ╚██████╗ ██║  ██║ ╚██████╔╝ ██║  ██║    ██║    ██║ ╚████║",
	" ╚══════╝  ╚═════╝ ╚═╝  ╚═╝  ╚═════╝  ╚═╝  ╚═╝    ╚═╝    ╚═╝  ╚═══╝",
}

// renderGradientLogo renders the ASCII art logo with a horizontal gradient.
func renderGradientLogo(width int) string {
	var lines []string

	for _, row := range echorynLogo {
		// If terminal is too narrow, skip the logo.
		if width < 40 {
			return ""
		}

		// If terminal is narrower than the logo, truncate.
		displayRow := row
		if len(displayRow) > width {
			displayRow = displayRow[:width]
		}

		// Apply gradient colour per character.
		var sb strings.Builder
		runes := []rune(displayRow)
		for i, ch := range runes {
			colorIdx := i * len(gradientColors) / max(len(runes), 1)
			if colorIdx >= len(gradientColors) {
				colorIdx = len(gradientColors) - 1
			}
			if ch == ' ' || ch == '╗' || ch == '╝' || ch == '╚' || ch == '╔' || ch == '═' || ch == '║' {
				sb.WriteString(lipgloss.NewStyle().
					Foreground(gradientColors[colorIdx]).
					Render(string(ch)))
			} else {
				sb.WriteString(lipgloss.NewStyle().
					Bold(true).
					Foreground(gradientColors[colorIdx]).
					Render(string(ch)))
			}
		}
		lines = append(lines, sb.String())
	}

	return strings.Join(lines, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// Welcome Banner
// ─────────────────────────────────────────────────────────────────────────────

// BannerInfo holds all data for the welcome banner.
type BannerInfo struct {
	Version    string
	Model      string
	ServerAddr string
	SessionKey string

	// Runtime info (fetched from Hivemind at startup).
	Tools      []ToolGroupInfo
	GolemNodes []GolemNodeInfo
	Skills     []SkillGroupInfo
}

// ToolGroupInfo is a group of tools by category.
type ToolGroupInfo struct {
	Category string
	Tools    []string
}

// GolemNodeInfo is a Golem node summary.
type GolemNodeInfo struct {
	ID     string
	Name   string
	Status string
	Labels map[string]string
}

// SkillGroupInfo is a group of skills by source.
type SkillGroupInfo struct {
	Source string
	Skills []string
}

// FormatWelcomeBanner returns the startup banner as a string (for tea.Println).
func FormatWelcomeBanner(info BannerInfo, width int) string {
	var sections []string
	sections = append(sections, "") // leading blank line

	// --- ASCII Art Logo ---
	logo := renderGradientLogo(width)
	if logo != "" {
		sections = append(sections, logo)
		sections = append(sections, "")
	}

	// --- Session Info Line ---
	infoLine := renderSessionInfo(info)
	if infoLine != "" {
		sections = append(sections, infoLine)
		sections = append(sections, "")
	}

	// --- Available Tools ---
	if len(info.Tools) > 0 {
		sections = append(sections, renderToolGroups(info.Tools, width))
	}

	// --- Available Golem Nodes ---
	if len(info.GolemNodes) > 0 {
		sections = append(sections, renderGolemNodes(info.GolemNodes))
	}

	// --- Available Skills ---
	if len(info.Skills) > 0 {
		sections = append(sections, renderSkillGroups(info.Skills, width))
	}

	// --- Summary line ---
	summary := renderSummaryLine(info)
	if summary != "" {
		sections = append(sections, "")
		sections = append(sections, summary)
	}

	// --- Random Tip ---
	sections = append(sections, "")
	sections = append(sections, subtleStyle.Render(randomTip()))
	sections = append(sections, "")

	return strings.Join(sections, "\n")
}

// renderSessionInfo renders the model, server, version, and session info line.
func renderSessionInfo(info BannerInfo) string {
	var parts []string
	if info.Model != "" {
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(assistantColor).Render(info.Model))
	}
	if info.Version != "" {
		parts = append(parts, dimStyle.Render(info.Version))
	}
	if info.ServerAddr != "" {
		parts = append(parts, dimStyle.Render(info.ServerAddr))
	}
	if len(parts) == 0 {
		return ""
	}
	line := "  " + strings.Join(parts, dimStyle.Render(" · "))
	if info.SessionKey != "" {
		line += "\n  " + dimStyle.Render("Session: "+info.SessionKey)
	}
	return line
}

// renderToolGroups renders the tools section grouped by category in a compact
// horizontal layout. Each category gets a colored label, tools are listed as
// comma-separated tags that wrap to the next line when exceeding terminal width.
//
// Example output:
//
//	Available Tools
//	  core: tool_search
//	  cluster: cluster_list_nodes, cluster_get_node, cluster_dispatch_task,
//	           cluster_execute_skill
//	  memory: memory_search, memory_read, memory_write, memory_delete
//	  web: web_fetch, web_search
func renderToolGroups(groups []ToolGroupInfo, width int) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(warningColor).Render("  Available Tools")
	var lines []string
	lines = append(lines, header)

	indent := "    "       // 4 spaces before category label
	wrapIndent := "      " // 6 spaces for continuation lines (aligned after label)

	for _, g := range groups {
		cat := lipgloss.NewStyle().Foreground(infoColor).Render(g.Category) +
			lipgloss.NewStyle().Foreground(infoColor).Render(":")

		// Calculate the visual width of the category prefix.
		catPrefix := indent + g.Category + ": "
		catPrefixLen := len(catPrefix)

		// Build wrapped tool list.
		maxLineLen := width - 4
		if maxLineLen < 30 {
			maxLineLen = 60
		}

		var toolLines []string
		currentLine := ""
		for i, tool := range g.Tools {
			separator := ""
			if i > 0 {
				separator = ", "
			}
			entry := separator + tool
			prefixLen := catPrefixLen
			if len(toolLines) > 0 {
				prefixLen = len(wrapIndent)
			}
			if currentLine != "" && prefixLen+len(currentLine)+len(entry) > maxLineLen {
				toolLines = append(toolLines, currentLine)
				currentLine = tool
			} else {
				currentLine += entry
			}
		}
		if currentLine != "" {
			toolLines = append(toolLines, currentLine)
		}

		// First line: "    category: tool1, tool2, ..."
		if len(toolLines) > 0 {
			lines = append(lines, indent+cat+" "+subtleStyle.Render(toolLines[0]))
			// Continuation lines aligned under the tool list.
			for _, tl := range toolLines[1:] {
				lines = append(lines, wrapIndent+subtleStyle.Render(tl))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// renderGolemNodes renders the golem nodes section with status.
func renderGolemNodes(nodes []GolemNodeInfo) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(successColor).Render("  Available Golem Nodes")
	var lines []string
	lines = append(lines, header)
	for _, n := range nodes {
		name := n.Name
		if name == "" {
			name = n.ID
		}
		statusStyle := lipgloss.NewStyle().Foreground(successColor)
		if n.Status != "NODE_STATUS_ONLINE" {
			statusStyle = lipgloss.NewStyle().Foreground(warningColor)
		}
		status := statusStyle.Render("[" + n.Status + "]")
		lines = append(lines, "    "+subtleStyle.Render(name)+" "+status)
	}
	return strings.Join(lines, "\n")
}

// renderSkillGroups renders the skills section grouped by source.
func renderSkillGroups(groups []SkillGroupInfo, width int) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render("  Available Skills")
	var lines []string
	lines = append(lines, header)
	for _, g := range groups {
		src := lipgloss.NewStyle().Foreground(infoColor).Render(g.Source + ":")
		skillList := strings.Join(g.Skills, ", ")
		maxLen := width - 20
		if maxLen < 10 {
			maxLen = 40
		}
		if len(skillList) > maxLen {
			skillList = skillList[:maxLen-3] + "..."
		}
		lines = append(lines, "    "+src+" "+subtleStyle.Render(skillList))
	}
	return strings.Join(lines, "\n")
}

// renderSummaryLine renders the summary footer: "N tools · M skills · K golems · /help".
func renderSummaryLine(info BannerInfo) string {
	totalTools := 0
	for _, g := range info.Tools {
		totalTools += len(g.Tools)
	}
	totalSkills := 0
	for _, g := range info.Skills {
		totalSkills += len(g.Skills)
	}
	totalNodes := len(info.GolemNodes)

	var parts []string
	if totalTools > 0 {
		parts = append(parts, fmt.Sprintf("%d tools", totalTools))
	}
	if totalSkills > 0 {
		parts = append(parts, fmt.Sprintf("%d skills", totalSkills))
	}
	if totalNodes > 0 {
		parts = append(parts, fmt.Sprintf("%d golems", totalNodes))
	}
	parts = append(parts, lipgloss.NewStyle().Bold(true).Render("/help")+" for commands")

	return "  " + subtleStyle.Render(strings.Join(parts, " · "))
}

// randomTip returns a random tip from a pool of helpful suggestions.
func randomTip() string {
	tips := []string{
		"Tip: Ask questions, edit files, or run commands.",
		"Tip: Be specific for the best results.",
		"Tip: Use /help to see all available commands.",
		"Tip: Press Esc during streaming to abort the response.",
		"Tip: Use Alt+Enter to type multiline input.",
		"Tip: Use --resume to continue a previous conversation.",
	}
	return "  ✦ " + tips[time.Now().UnixNano()%int64(len(tips))]
}

// ─────────────────────────────────────────────────────────────────────────────
// Message Labels
// ─────────────────────────────────────────────────────────────────────────────

// FormatUserMessage returns a styled user message string (for tea.Println).
func FormatUserMessage(msg string) string {
	prompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(userColor).
		Render("> ")
	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Render(msg)
	return prompt + content
}

// PrintError outputs a styled error message.
func PrintError(msg string) {
	fmt.Println(errorStyle.Render("✗ Error: " + msg))
}

// PrintGoodbye outputs a farewell message.
func PrintGoodbye() {
	fmt.Println()
	fmt.Println(subtleStyle.Render("  Goodbye! 👋"))
	fmt.Println()
}

// PrintResumeHint prints the resume command after goodbye.
// This tells the user how to continue this conversation later.
func PrintResumeHint(programName, sessionKey string) {
	if sessionKey == "" {
		return
	}
	hint := lipgloss.NewStyle().
		Foreground(subtleColor).
		Render("  To resume this conversation:")
	cmd := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Render(fmt.Sprintf(" --resume %s", sessionKey))
	fmt.Println(hint)
	fmt.Println(cmd)
	fmt.Println()
}
