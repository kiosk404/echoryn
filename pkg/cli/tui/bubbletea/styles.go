package bubbletea

import (
	"github.com/charmbracelet/lipgloss"
)

// =============================================================================
// Colors
// =============================================================================

var (
	colorPrimary   = lipgloss.Color("39")  // blue
	colorSecondary = lipgloss.Color("241") // gray
	colorSuccess   = lipgloss.Color("82")  // green
	colorWarning   = lipgloss.Color("214") // orange
	colorError     = lipgloss.Color("196") // red
	colorFaint     = lipgloss.Color("8")   // faint gray
)

// =============================================================================
// Styles
// =============================================================================

var (
	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	styleFaint = lipgloss.NewStyle().
			Foreground(colorFaint)

	styleError = lipgloss.NewStyle().
			Foreground(colorError)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleWarning = lipgloss.NewStyle().
			Foreground(colorWarning)

	styleUserPrompt = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	styleAssistantLabel = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	styleInputBorder = lipgloss.NewStyle().
				Foreground(colorFaint)

	styleTeamTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	styleTeamMember = lipgloss.NewStyle().
			Foreground(colorFaint)

	styleFocusMarker = lipgloss.NewStyle().
				Foreground(lipgloss.Color("226")).
				Bold(true)
)

// =============================================================================
// Helper Functions
// =============================================================================

// statusColor returns the color for a member status.
func statusColor(status MemberStatus) lipgloss.Color {
	switch status {
	case StatusRunning:
		return colorWarning
	case StatusCompleted:
		return colorSuccess
	case StatusFailed:
		return colorError
	default:
		return colorFaint
	}
}

// statusIcon returns the icon for a member status.
func statusIcon(status MemberStatus) string {
	switch status {
	case StatusRunning:
		return "●"
	case StatusIdle:
		return "○"
	case StatusCompleted:
		return "✓"
	case StatusFailed:
		return "✗"
	default:
		return "?"
	}
}

// roleIcon returns the icon for a member role.
func roleIcon(role TeamRole, isLeader bool) string {
	if isLeader {
		return "👑"
	}
	switch role {
	case RoleLead:
		return "👑"
	case RoleWorker:
		return "🔧"
	case RoleReviewer:
		return "👀"
	default:
		return "👤"
	}
}
