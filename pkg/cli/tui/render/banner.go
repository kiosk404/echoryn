package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ─────────────────────────────────────────────────────────────────────────────
// Colours & Styles — shared across the entire render package
// ─────────────────────────────────────────────────────────────────────────────

var (
	colorOutput = termenv.NewOutput(os.Stdout)

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
	accentColor    = lipgloss.Color("99")  // brand purple
	subtleColor    = lipgloss.Color("242") // dim gray
	dimColor       = lipgloss.Color("238") // very dim
	userColor      = lipgloss.Color("39")  // blue
	assistantColor = lipgloss.Color("183") // lavender
	successColor   = lipgloss.Color("82")  // green
	warningColor   = lipgloss.Color("214") // orange
	errorColor     = lipgloss.Color("196") // red
	infoColor      = lipgloss.Color("39")  // blue
	borderColor    = lipgloss.Color("63")  // border blue-purple

	// Pre-computed styles.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)

	subtleStyle = lipgloss.NewStyle().
			Foreground(subtleColor)

	dimStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(userColor)

	assistantLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(assistantColor)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(errorColor)

	successStyle = lipgloss.NewStyle().
			Foreground(successColor)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	infoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(infoColor)

	// Border box style for input, code blocks, etc.
	boxBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	// Thicker border for active/focused elements.
	focusBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("99")).
				Padding(0, 1)
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

// PrintWelcomeBanner outputs the startup banner with logo, connection info, tips.
func PrintWelcomeBanner(info BannerInfo, width int) {
	fmt.Println()

	// --- ASCII Art Logo ---
	logo := renderGradientLogo(width)
	if logo != "" {
		fmt.Println(logo)
		fmt.Println()
	}

	// --- Tips ---
	tips := []string{
		"Tips for getting started:",
		"1. Ask questions, edit files, or run commands.",
		"2. Be specific for the best results.",
		"3. " + lipgloss.NewStyle().Bold(true).Render("/help") + " for more information.",
	}
	for _, tip := range tips {
		fmt.Println(subtleStyle.Render(tip))
	}
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// Message Labels
// ─────────────────────────────────────────────────────────────────────────────

// PrintUserMessage displays the user's message with a styled prompt.
func PrintUserMessage(msg string, _ int) {
	prompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(userColor).
		Render("> ")
	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Render(msg)
	fmt.Println(prompt + content)
}

// PrintAssistantLabel outputs the assistant marker before streaming.
func PrintAssistantLabel(_ int) {
	// Responding with model label in italics
	responding := lipgloss.NewStyle().
		Italic(true).
		Foreground(subtleColor).
		Render("  Responding with echoryn")
	fmt.Println(responding)
}

// PrintAssistantContent prints a rendered assistant response line.
func PrintAssistantContent(content string) {
	marker := lipgloss.NewStyle().
		Bold(true).
		Foreground(assistantColor).
		Render("✦ ")
	fmt.Print(marker + content)
}

// PrintError outputs a styled error message.
func PrintError(msg string) {
	fmt.Println(errorStyle.Render("✗ Error: " + msg))
}

// PrintSuccess outputs a styled success message.
func PrintSuccess(msg string) {
	fmt.Println(successStyle.Render("✓ " + msg))
}

// PrintInfo outputs a styled info message.
func PrintInfo(msg string) {
	fmt.Println(infoStyle.Render("ℹ " + msg))
}

// PrintGoodbye outputs a farewell message.
func PrintGoodbye() {
	fmt.Println()
	fmt.Println(subtleStyle.Render("  Goodbye! 👋"))
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// Status Bar
// ─────────────────────────────────────────────────────────────────────────────

// StatusBarInfo holds data for the bottom status bar.
type StatusBarInfo struct {
	WorkDir    string
	SandboxMsg string
	Mode       string // e.g., "auto", "manual"
}

// RenderStatusBar renders a Gemini-CLI style bottom status bar.
func RenderStatusBar(info StatusBarInfo, width int) string {
	left := dimStyle.Render("  " + info.WorkDir)

	center := ""
	if info.SandboxMsg != "" {
		center = lipgloss.NewStyle().
			Foreground(infoColor).
			Bold(true).
			Render(info.SandboxMsg)
	}

	right := ""
	if info.Mode != "" {
		right = dimStyle.Render(info.Mode + "  ")
	}

	// Calculate spacing
	leftLen := lipgloss.Width(left)
	centerLen := lipgloss.Width(center)
	rightLen := lipgloss.Width(right)

	gap1 := max(1, (width-leftLen-centerLen-rightLen)/2-leftLen+leftLen)
	gap2 := max(1, width-leftLen-gap1-centerLen-rightLen)

	return left + strings.Repeat(" ", gap1) + center + strings.Repeat(" ", gap2) + right
}

// ─────────────────────────────────────────────────────────────────────────────
// Input Box
// ─────────────────────────────────────────────────────────────────────────────

// RenderInputBox renders a Gemini-CLI style bordered input box.
func RenderInputBox(placeholder string, width int) string {
	if width <= 4 {
		width = 80
	}

	innerWidth := width - 4 // account for border + padding

	prompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(userColor).
		Render("> ")

	placeholderText := lipgloss.NewStyle().
		Foreground(subtleColor).
		Render(placeholder)

	content := prompt + "█ " + placeholderText

	// Pad to fill width
	contentWidth := lipgloss.Width(content)
	if contentWidth < innerWidth {
		content += strings.Repeat(" ", innerWidth-contentWidth)
	}

	return focusBorderStyle.
		Width(innerWidth).
		Render(content)
}

// ─────────────────────────────────────────────────────────────────────────────
// Separator
// ─────────────────────────────────────────────────────────────────────────────

// PrintSeparator outputs a dim horizontal rule.
func PrintSeparator(_ int) {
	// No-op: Gemini style doesn't use thick separators between messages.
	// Messages flow naturally.
}
