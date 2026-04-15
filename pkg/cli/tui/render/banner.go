package render

import (
	"fmt"
	"strings"

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
			if ch == ' ' || ch == '╗' || ch == '╝' || ch == '╚' || ch == '╔' || ch == '═' || ch == '║' {
				// Use gradient color even for box-drawing characters.
				colorIdx := i * len(gradientColors) / max(len(runes), 1)
				if colorIdx >= len(gradientColors) {
					colorIdx = len(gradientColors) - 1
				}
				sb.WriteString(lipgloss.NewStyle().
					Foreground(gradientColors[colorIdx]).
					Render(string(ch)))
			} else {
				colorIdx := i * len(gradientColors) / max(len(runes), 1)
				if colorIdx >= len(gradientColors) {
					colorIdx = len(gradientColors) - 1
				}
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

// BannerInfo holds the data displayed in the welcome banner.
type BannerInfo struct {
	Version    string
	Model      string
	ServerAddr string
	SessionKey string
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

	// --- Tips ---
	tips := []string{
		"Tips for getting started:",
		"1. Ask questions, edit files, or run commands.",
		"2. Be specific for the best results.",
		"3. " + lipgloss.NewStyle().Bold(true).Render("/help") + " for more information.",
	}
	for _, tip := range tips {
		sections = append(sections, subtleStyle.Render(tip))
	}
	sections = append(sections, "")

	return strings.Join(sections, "\n")
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
