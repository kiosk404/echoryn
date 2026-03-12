package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Colors used throughout the TUI, defined once for consistency.
var (
	colorOutput = termenv.NewOutput(os.Stdout)

	// Accent colour for the primary brand.
	accentColor = lipgloss.Color("208") // orange

	// Subtle colour for secondary text and separators.
	subtleColor = lipgloss.Color("241") // gray

	// Colours for role labels.
	userColor      = lipgloss.Color("39")  // blue
	assistantColor = lipgloss.Color("212") // pink

	// Error colour.
	errorColor = lipgloss.Color("196") // red
)

// Styles are pre-computed lipgloss styles, shared across the TUI.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)

	subtleStyle = lipgloss.NewStyle().
			Foreground(subtleColor)

	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(userColor)

	assistantLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(assistantColor)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(errorColor)

	tipHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)
)

// BannerInfo holds the data displayed in the welcome banner.
type BannerInfo struct {
	Version    string
	Model      string
	ServerAddr string
	SessionKey string
}

// PrintWelcomeBanner outputs the startup banner with connection info and tips.
func PrintWelcomeBanner(info BannerInfo, width int) {
	sep := subtleStyle.Render(strings.Repeat("─", width))
	fmt.Println(sep)

	title := titleStyle.Render(fmt.Sprintf("Echoryn Chat %s", info.Version))
	fmt.Println(title)
	fmt.Println()

	fmt.Printf("  Model:   %s\n", info.Model)
	fmt.Printf("  Server:  %s\n", info.ServerAddr)
	if info.SessionKey != "" {
		fmt.Printf("  Session: %s\n", info.SessionKey)
	}
	fmt.Println()

	fmt.Println(tipHeaderStyle.Render("Tips:"))
	fmt.Println("  Type a message and press Enter to send")
	fmt.Println("  Shift+Enter or \\+Enter for multi-line input")
	fmt.Println("  /clear  — reset conversation")
	fmt.Println("  /help   — show all commands")
	fmt.Println("  /quit   — exit")
	fmt.Println("  Ctrl+C  — exit")
	fmt.Println(sep)
	fmt.Println()
}

// PrintSeparator outputs a dim horizontal rule.
func PrintSeparator(width int) {
	n := width - 2
	if n < 20 {
		n = 20
	}
	fmt.Println(subtleStyle.Render(strings.Repeat("─", n)))
}

// PrintUserMessage displays the user's message with a role label.
func PrintUserMessage(msg string, width int) {
	PrintSeparator(width)
	fmt.Println(userLabelStyle.Render("you"))
	styled := lipgloss.NewStyle().Foreground(userColor).Render(msg)
	fmt.Println(styled)
}

// PrintAssistantLabel outputs the assistant name label before streaming.
func PrintAssistantLabel(width int) {
	PrintSeparator(width)
	fmt.Println(assistantLabelStyle.Render("echoryn"))
}

// PrintError outputs a styled error message.
func PrintError(msg string) {
	fmt.Println(errorStyle.Render("Error: " + msg))
}

// PrintGoodbye outputs a farewell message.
func PrintGoodbye() {
	fmt.Println()
	fmt.Println(subtleStyle.Render("Goodbye!"))
	fmt.Println()
}
