package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Team panel colours.
var (
	teamOutput = termenv.NewOutput(os.Stdout)

	teamTitleColor  = lipgloss.Color("39")  // blue
	memberRunning   = lipgloss.Color("214") // orange
	memberIdle      = lipgloss.Color("241") // gray
	memberCompleted = lipgloss.Color("82")  // green
	memberFailed    = lipgloss.Color("196") // red
	focusIndicator  = lipgloss.Color("226") // yellow
)

// Team status styles.
var (
	teamTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(teamTitleColor)

	teamStrategyStyle = lipgloss.NewStyle().
				Foreground(subtleColor)

	focusStyle = lipgloss.NewStyle().
			Foreground(focusIndicator).
			Bold(true)
)

// TeamMemberInfo holds info needed for rendering a team member.
type TeamMemberInfo struct {
	Label    string
	Role     string
	Status   string // idle, running, completed, failed
	IsLeader bool
	Focused  bool
	NodeID   string
	Progress string
}

// memberStatusColor returns the colour for a given status.
func memberStatusColor(status string) lipgloss.Color {
	switch status {
	case "running":
		return memberRunning
	case "completed":
		return memberCompleted
	case "failed":
		return memberFailed
	default:
		return memberIdle
	}
}

// memberStatusIcon returns the icon for a given status.
func memberStatusIcon(status string) string {
	switch status {
	case "running":
		return "●"
	case "idle":
		return "○"
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	default:
		return "?"
	}
}

// memberRoleIcon returns the icon for a member's role.
func memberRoleIcon(isLeader bool) string {
	if isLeader {
		return "👑"
	}
	return "🔧"
}

// PrintTeamStatusBar prints a compact one-line team status bar.
// Format: 📋 Team: MyTeam (3 members) [●2 ✓1 ✗0]
func PrintTeamStatusBar(name string, strategy string, members []TeamMemberInfo, width int) {
	if len(members) == 0 {
		return
	}

	// Count statuses.
	counts := map[string]int{"running": 0, "idle": 0, "completed": 0, "failed": 0}
	for _, m := range members {
		counts[m.Status]++
	}

	// Build status summary.
	var parts []string
	if counts["running"] > 0 {
		parts = append(parts, teamOutput.String(
			fmt.Sprintf("●%d", counts["running"])).
			Foreground(teamOutput.Color("214")).String())
	}
	if counts["idle"] > 0 {
		parts = append(parts, teamOutput.String(
			fmt.Sprintf("○%d", counts["idle"])).
			Foreground(teamOutput.Color("241")).String())
	}
	if counts["completed"] > 0 {
		parts = append(parts, teamOutput.String(
			fmt.Sprintf("✓%d", counts["completed"])).
			Foreground(teamOutput.Color("82")).String())
	}
	if counts["failed"] > 0 {
		parts = append(parts, teamOutput.String(
			fmt.Sprintf("✗%d", counts["failed"])).
			Foreground(teamOutput.Color("196")).String())
	}

	statusSummary := strings.Join(parts, " ")

	// Build the bar.
	line := fmt.Sprintf("📋 %s (%s, %d members) [%s]",
		teamTitleStyle.Render(name),
		teamStrategyStyle.Render(strategy),
		len(members),
		statusSummary,
	)

	sep := subtleStyle.Render(strings.Repeat("─", width))
	fmt.Fprintln(os.Stdout, sep)
	fmt.Fprintln(os.Stdout, line)
}

// PrintTeamPanel prints a detailed team member panel.
// This is used by /team status and /agents commands.
func PrintTeamPanel(name, strategy string, members []TeamMemberInfo, width int) {
	sep := subtleStyle.Render(strings.Repeat("─", width))
	fmt.Fprintln(os.Stdout, sep)

	// Header.
	header := fmt.Sprintf("📋 Team: %s", teamTitleStyle.Render(name))
	fmt.Fprintln(os.Stdout, header)
	fmt.Fprintf(os.Stdout, "   Strategy: %s\n", teamStrategyStyle.Render(strategy))
	fmt.Fprintf(os.Stdout, "   Members:  %d\n\n", len(members))

	// Member list.
	for _, m := range members {
		icon := lipgloss.NewStyle().
			Foreground(memberStatusColor(m.Status)).
			Render(memberStatusIcon(m.Status))

		roleIcon := memberRoleIcon(m.IsLeader)

		focusMarker := "  "
		if m.Focused {
			focusMarker = focusStyle.Render("▸ ")
		}

		label := m.Label
		if m.Focused {
			label = focusStyle.Render(label)
		}

		statusText := lipgloss.NewStyle().
			Foreground(memberStatusColor(m.Status)).
			Render(m.Status)

		line := fmt.Sprintf("   %s%s %s %s [%s]",
			focusMarker, icon, roleIcon, label, statusText)

		if m.NodeID != "" {
			nodeInfo := lipgloss.NewStyle().Foreground(subtleColor).Render(
				fmt.Sprintf(" (node: %s)", m.NodeID))
			line += nodeInfo
		}

		if m.Progress != "" {
			progress := lipgloss.NewStyle().Foreground(subtleColor).Render(
				fmt.Sprintf(" — %s", m.Progress))
			line += progress
		}

		fmt.Fprintln(os.Stdout, line)
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, sep)
}
