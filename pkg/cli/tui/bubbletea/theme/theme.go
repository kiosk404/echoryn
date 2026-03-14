// Package theme provides semantic theming for the TUI.
// Inspired by Gemini CLI's theme system.
package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// SemanticColors defines semantic color tokens for the UI.
type SemanticColors struct {
	// Text colors
	Text struct {
		Primary   lipgloss.Color
		Secondary lipgloss.Color
		Accent    lipgloss.Color
		Link      lipgloss.Color
		Response  lipgloss.Color
	}

	// Background colors
	Background struct {
		Primary  lipgloss.Color
		Message  lipgloss.Color
		Input    lipgloss.Color
		Focus    lipgloss.Color
		Selected lipgloss.Color
	}

	// Border colors
	Border struct {
		Default lipgloss.Color
		Focus   lipgloss.Color
	}

	// Status colors
	Status struct {
		Error   lipgloss.Color
		Success lipgloss.Color
		Warning lipgloss.Color
		Info    lipgloss.Color
	}

	// Accent colors (for spinner gradient)
	Accent []lipgloss.Color
}

// Theme is the active theme instance.
var currentTheme *SemanticColors

// DefaultDarkTheme returns the default dark theme.
func DefaultDarkTheme() *SemanticColors {
	c := &SemanticColors{}

	// Text colors
	c.Text.Primary = lipgloss.Color("15")    // White
	c.Text.Secondary = lipgloss.Color("241") // Gray
	c.Text.Accent = lipgloss.Color("39")     // Blue
	c.Text.Link = lipgloss.Color("81")       // Light blue
	c.Text.Response = lipgloss.Color("252")  // Light gray

	// Background colors
	c.Background.Primary = lipgloss.Color("235") // Dark gray
	c.Background.Message = lipgloss.Color("236")
	c.Background.Input = lipgloss.Color("234")
	c.Background.Focus = lipgloss.Color("238")
	c.Background.Selected = lipgloss.Color("238")

	// Border colors
	c.Border.Default = lipgloss.Color("238")
	c.Border.Focus = lipgloss.Color("39")

	// Status colors
	c.Status.Error = lipgloss.Color("196")   // Red
	c.Status.Success = lipgloss.Color("82")  // Green
	c.Status.Warning = lipgloss.Color("214") // Orange
	c.Status.Info = lipgloss.Color("39")     // Blue

	// Accent colors (Google brand colors for spinner)
	c.Accent = []lipgloss.Color{
		lipgloss.Color("129"), // Purple
		lipgloss.Color("39"),  // Blue
		lipgloss.Color("44"),  // Cyan
		lipgloss.Color("82"),  // Green
		lipgloss.Color("214"), // Yellow
		lipgloss.Color("196"), // Red
	}

	return c
}

// GetTheme returns the current active theme.
func GetTheme() *SemanticColors {
	if currentTheme == nil {
		currentTheme = DefaultDarkTheme()
	}
	return currentTheme
}

// SetTheme sets the active theme.
func SetTheme(t *SemanticColors) {
	currentTheme = t
}

// Styles provides pre-built styles using the current theme.
type Styles struct {
	// Header styles
	Header       lipgloss.Style
	HeaderInfo   lipgloss.Style
	HeaderStatus lipgloss.Style

	// Message styles
	UserPrompt       lipgloss.Style
	UserContent      lipgloss.Style
	AssistantLabel   lipgloss.Style
	AssistantContent lipgloss.Style
	SystemMessage    lipgloss.Style
	ErrorMessage     lipgloss.Style
	SuccessMessage   lipgloss.Style
	WarningMessage   lipgloss.Style

	// Input styles
	InputBorder lipgloss.Style
	InputPrompt lipgloss.Style
	InputText   lipgloss.Style
	InputGhost  lipgloss.Style
	InputCursor lipgloss.Style

	// Tool styles
	ToolHeader  lipgloss.Style
	ToolName    lipgloss.Style
	ToolArgs    lipgloss.Style
	ToolResult  lipgloss.Style
	ToolError   lipgloss.Style
	ToolConfirm lipgloss.Style

	// Team panel styles
	TeamTitle   lipgloss.Style
	TeamMember  lipgloss.Style
	TeamFocus   lipgloss.Style
	TeamStatus  lipgloss.Style
	TeamMessage lipgloss.Style

	// Suggestion styles
	SuggestionItem   lipgloss.Style
	SuggestionActive lipgloss.Style
	SuggestionDesc   lipgloss.Style

	// Code block styles
	CodeBlock   lipgloss.Style
	CodeHeader  lipgloss.Style
	CodeContent lipgloss.Style
	CodeLineNum lipgloss.Style

	// Footer styles
	Footer lipgloss.Style
}

// GetStyles returns pre-built styles from the current theme.
func GetStyles() *Styles {
	t := GetTheme()
	s := &Styles{}

	// Header
	s.Header = lipgloss.NewStyle().
		Background(t.Background.Primary).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(t.Border.Default).
		Padding(0, 1)

	s.HeaderInfo = lipgloss.NewStyle().
		Foreground(t.Text.Secondary)

	s.HeaderStatus = lipgloss.NewStyle().
		Foreground(t.Status.Warning).
		Bold(true)

	// Messages
	s.UserPrompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)

	s.UserContent = lipgloss.NewStyle().
		Foreground(t.Text.Primary)

	s.AssistantLabel = lipgloss.NewStyle().
		Foreground(t.Text.Accent).
		Bold(true)

	s.AssistantContent = lipgloss.NewStyle().
		Foreground(t.Text.Response)

	s.SystemMessage = lipgloss.NewStyle().
		Foreground(t.Text.Secondary).
		Italic(true)

	s.ErrorMessage = lipgloss.NewStyle().
		Foreground(t.Status.Error)

	s.SuccessMessage = lipgloss.NewStyle().
		Foreground(t.Status.Success)

	s.WarningMessage = lipgloss.NewStyle().
		Foreground(t.Status.Warning)

	// Input
	s.InputBorder = lipgloss.NewStyle().
		Foreground(t.Border.Default)

	s.InputPrompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)

	s.InputText = lipgloss.NewStyle().
		Foreground(t.Text.Primary)

	s.InputGhost = lipgloss.NewStyle().
		Foreground(t.Text.Secondary)

	s.InputCursor = lipgloss.NewStyle().
		Foreground(t.Text.Accent).
		Blink(true)

	// Tool
	s.ToolHeader = lipgloss.NewStyle().
		Foreground(t.Text.Accent).
		Bold(true)

	s.ToolName = lipgloss.NewStyle().
		Foreground(t.Text.Accent)

	s.ToolArgs = lipgloss.NewStyle().
		Foreground(t.Text.Secondary)

	s.ToolResult = lipgloss.NewStyle().
		Foreground(t.Status.Success)

	s.ToolError = lipgloss.NewStyle().
		Foreground(t.Status.Error)

	s.ToolConfirm = lipgloss.NewStyle().
		Foreground(t.Status.Warning).
		Bold(true)

	// Team panel
	s.TeamTitle = lipgloss.NewStyle().
		Foreground(t.Text.Accent).
		Bold(true).
		Padding(0, 1)

	s.TeamMember = lipgloss.NewStyle().
		Foreground(t.Text.Primary)

	s.TeamFocus = lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)

	s.TeamStatus = lipgloss.NewStyle().
		Foreground(t.Text.Secondary)

	s.TeamMessage = lipgloss.NewStyle().
		Foreground(t.Text.Secondary)

	// Suggestions
	s.SuggestionItem = lipgloss.NewStyle().
		Foreground(t.Text.Primary).
		Padding(0, 1)

	s.SuggestionActive = lipgloss.NewStyle().
		Foreground(t.Text.Primary).
		Background(t.Background.Selected).
		Bold(true).
		Padding(0, 1)

	s.SuggestionDesc = lipgloss.NewStyle().
		Foreground(t.Text.Secondary)

	// Code block
	s.CodeBlock = lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Padding(0, 1)

	s.CodeHeader = lipgloss.NewStyle().
		Foreground(t.Text.Secondary).
		Background(lipgloss.Color("236")).
		Padding(0, 1)

	s.CodeContent = lipgloss.NewStyle().
		Foreground(t.Text.Response)

	s.CodeLineNum = lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))

	// Footer
	s.Footer = lipgloss.NewStyle().
		Foreground(t.Text.Secondary).
		Padding(0, 1)

	return s
}
