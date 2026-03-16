// Package command provides slash command implementations for the TUI.
// This file adds team-related slash commands for the Team collaboration mode.
package command

import (
	"context"
	"fmt"
	"strings"
)

// TeamState holds the current team state exposed to commands.
// The TUI sets this on the Env before executing team commands.
type TeamState struct {
	// Enabled indicates whether a team is currently active.
	Enabled bool

	// ID is the current team's ID.
	ID string

	// Name is the current team's name.
	Name string

	// Strategy is the team's coordination strategy.
	Strategy string

	// Members lists the current team members and their states.
	Members []TeamMemberState

	// FocusIndex tracks which member is currently focused (-1 = none).
	FocusIndex int
}

// TeamMemberState represents a team member's status for display.
type TeamMemberState struct {
	ID        string
	SessionID string
	Label     string
	Role      string
	Status    string // idle, running, completed, failed
	Progress  string
	IsLeader  bool
	NodeID    string
}

// StatusIcon returns the appropriate icon for the member's status.
func (m *TeamMemberState) StatusIcon() string {
	switch m.Status {
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

// RoleIcon returns the appropriate icon for the member's role.
func (m *TeamMemberState) RoleIcon() string {
	if m.IsLeader {
		return "👑"
	}
	return "🔧"
}

// TeamAPI defines the interface the TUI needs for team operations.
// This decouples the TUI from the concrete HTTP client / server-side implementation.
type TeamAPI interface {
	// ListTemplates returns available team templates.
	ListTemplates(ctx context.Context) ([]TemplateInfo, error)

	// CreateTeam creates a team from a template or ad-hoc.
	CreateTeam(ctx context.Context, req CreateTeamReq) (*TeamState, error)

	// GetTeam returns the current team state.
	GetTeam(ctx context.Context, teamID string) (*TeamState, error)

	// DissolveTeam dissolves the current team.
	DissolveTeam(ctx context.Context, teamID string) error

	// SendMessage sends a message to a specific team member.
	SendMessage(ctx context.Context, teamID, recipientLabel, content string) error

	// Broadcast sends a message to all team members.
	Broadcast(ctx context.Context, teamID, content string) error
}

// TemplateInfo is a summary of a team template for display.
type TemplateInfo struct {
	ID          string
	Name        string
	Description string
	Strategy    string
	MemberCount int
}

// CreateTeamReq is the request for creating a team.
type CreateTeamReq struct {
	TemplateID      string
	Name            string
	TaskDescription string
	Strategy        string
}

// RegisterTeamCommands registers all team-related slash commands.
func RegisterTeamCommands(r *Registry) {
	r.Register(&teamCmd{})
	r.Register(&agentsCmd{})
	r.Register(&msgCmd{})
	r.Register(&broadcastCmd{})
	r.Register(&focusCmd{})
}

// ---------- /team ----------

type teamCmd struct{}

func (c *teamCmd) Name() string        { return "team" }
func (c *teamCmd) Aliases() []string   { return nil }
func (c *teamCmd) Group() CommandGroup { return GroupTeam }
func (c *teamCmd) Description() string {
	return "Team management: /team [create|status|dissolve|templates]"
}

func (c *teamCmd) Execute(ctx context.Context, env *Env, args string) error {
	subcommand := strings.TrimSpace(args)
	if subcommand == "" {
		subcommand = "status"
	}

	parts := strings.SplitN(subcommand, " ", 2)
	sub := strings.ToLower(parts[0])
	subArgs := ""
	if len(parts) > 1 {
		subArgs = strings.TrimSpace(parts[1])
	}

	switch sub {
	case "status":
		return teamStatus(env)
	case "templates", "list-templates":
		return teamTemplates(ctx, env)
	case "create":
		return teamCreate(ctx, env, subArgs)
	case "dissolve":
		return teamDissolve(ctx, env)
	default:
		return fmt.Errorf("unknown team subcommand: %s\n  Usage: /team [status|templates|create <template_id> <task>|dissolve]", sub)
	}
}

func teamStatus(env *Env) error {
	state := env.TeamState
	if state == nil || !state.Enabled {
		fmt.Fprintln(env.Out, "No active team. Use /team create <template_id> <task> to start one.")
		return nil
	}

	fmt.Fprintf(env.Out, "\n📋 Team: %s (%s)\n", state.Name, state.ID)
	fmt.Fprintf(env.Out, "   Strategy: %s\n", state.Strategy)
	fmt.Fprintf(env.Out, "   Members: %d\n\n", len(state.Members))

	for i, m := range state.Members {
		focusMarker := "  "
		if i == state.FocusIndex {
			focusMarker = "▸ "
		}
		fmt.Fprintf(env.Out, "   %s%s %s %s [%s]",
			focusMarker, m.StatusIcon(), m.RoleIcon(), m.Label, m.Status)
		if m.NodeID != "" {
			fmt.Fprintf(env.Out, " (node: %s)", m.NodeID)
		}
		if m.Progress != "" {
			fmt.Fprintf(env.Out, " — %s", m.Progress)
		}
		fmt.Fprintln(env.Out)
	}
	fmt.Fprintln(env.Out)
	return nil
}

func teamTemplates(ctx context.Context, env *Env) error {
	if env.TeamAPI == nil {
		fmt.Fprintln(env.Out, "Team API not available. Connect to a Hivemind server first.")
		return nil
	}

	templates, err := env.TeamAPI.ListTemplates(ctx)
	if err != nil {
		return fmt.Errorf("list templates: %w", err)
	}

	if len(templates) == 0 {
		fmt.Fprintln(env.Out, "No team templates available.")
		return nil
	}

	fmt.Fprintf(env.Out, "\n📋 Available Team Templates:\n\n")
	for _, t := range templates {
		fmt.Fprintf(env.Out, "   %-30s  %s (%d members, %s)\n",
			t.ID, t.Name, t.MemberCount, t.Strategy)
		if t.Description != "" {
			fmt.Fprintf(env.Out, "   %-30s  %s\n", "", t.Description)
		}
	}
	fmt.Fprintln(env.Out)
	return nil
}

func teamCreate(ctx context.Context, env *Env, args string) error {
	if env.TeamAPI == nil {
		fmt.Fprintln(env.Out, "Team API not available. Connect to a Hivemind server first.")
		return nil
	}

	if env.TeamState != nil && env.TeamState.Enabled {
		return fmt.Errorf("a team is already active: %s. Use /team dissolve first", env.TeamState.Name)
	}

	// Parse: /team create <template_id> <task_description>
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 || parts[0] == "" {
		return fmt.Errorf("usage: /team create <template_id> <task_description>")
	}

	templateID := parts[0]
	task := parts[1]

	fmt.Fprintf(env.Out, "Creating team from template '%s'...\n", templateID)

	state, err := env.TeamAPI.CreateTeam(ctx, CreateTeamReq{
		TemplateID:      templateID,
		TaskDescription: task,
	})
	if err != nil {
		return fmt.Errorf("create team: %w", err)
	}

	// Update env with the new team state.
	if env.SetTeamState != nil {
		env.SetTeamState(state)
	}

	fmt.Fprintf(env.Out, "✅ Team '%s' created with %d members.\n", state.Name, len(state.Members))
	return teamStatus(env)
}

func teamDissolve(ctx context.Context, env *Env) error {
	if env.TeamAPI == nil {
		fmt.Fprintln(env.Out, "Team API not available.")
		return nil
	}

	state := env.TeamState
	if state == nil || !state.Enabled {
		fmt.Fprintln(env.Out, "No active team to dissolve.")
		return nil
	}

	fmt.Fprintf(env.Out, "Dissolving team '%s'...\n", state.Name)

	if err := env.TeamAPI.DissolveTeam(ctx, state.ID); err != nil {
		return fmt.Errorf("dissolve team: %w", err)
	}

	if env.SetTeamState != nil {
		env.SetTeamState(nil)
	}

	fmt.Fprintln(env.Out, "✅ Team dissolved.")
	return nil
}

// ---------- /agents ----------

type agentsCmd struct{}

func (c *agentsCmd) Name() string        { return "agents" }
func (c *agentsCmd) Aliases() []string   { return nil }
func (c *agentsCmd) Group() CommandGroup { return GroupTeam }
func (c *agentsCmd) Description() string { return "List team members and their status" }

func (c *agentsCmd) Execute(_ context.Context, env *Env, _ string) error {
	return teamStatus(env)
}

// ---------- /msg ----------

type msgCmd struct{}

func (c *msgCmd) Name() string        { return "msg" }
func (c *msgCmd) Aliases() []string   { return []string{"message"} }
func (c *msgCmd) Group() CommandGroup { return GroupTeam }
func (c *msgCmd) Description() string {
	return "Send message to team member: /msg <member_label> <message>"
}

func (c *msgCmd) Execute(ctx context.Context, env *Env, args string) error {
	if env.TeamAPI == nil {
		fmt.Fprintln(env.Out, "Team API not available.")
		return nil
	}

	state := env.TeamState
	if state == nil || !state.Enabled {
		fmt.Fprintln(env.Out, "No active team. Use /team create first.")
		return nil
	}

	// Parse: /msg <label> <message>
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 || parts[0] == "" {
		return fmt.Errorf("usage: /msg <member_label> <message>")
	}

	recipient := parts[0]
	content := parts[1]

	// Validate recipient exists.
	found := false
	for _, m := range state.Members {
		if strings.EqualFold(m.Label, recipient) || strings.EqualFold(m.ID, recipient) {
			found = true
			recipient = m.Label
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown team member: %s. Use /agents to see members", recipient)
	}

	if err := env.TeamAPI.SendMessage(ctx, state.ID, recipient, content); err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	fmt.Fprintf(env.Out, "📩 Message sent to %s.\n", recipient)
	return nil
}

// ---------- /broadcast ----------

type broadcastCmd struct{}

func (c *broadcastCmd) Name() string        { return "broadcast" }
func (c *broadcastCmd) Aliases() []string   { return []string{"bc"} }
func (c *broadcastCmd) Group() CommandGroup { return GroupTeam }
func (c *broadcastCmd) Description() string {
	return "Broadcast message to all team members: /broadcast <message>"
}

func (c *broadcastCmd) Execute(ctx context.Context, env *Env, args string) error {
	if env.TeamAPI == nil {
		fmt.Fprintln(env.Out, "Team API not available.")
		return nil
	}

	state := env.TeamState
	if state == nil || !state.Enabled {
		fmt.Fprintln(env.Out, "No active team. Use /team create first.")
		return nil
	}

	if args == "" {
		return fmt.Errorf("usage: /broadcast <message>")
	}

	if err := env.TeamAPI.Broadcast(ctx, state.ID, args); err != nil {
		return fmt.Errorf("broadcast: %w", err)
	}

	fmt.Fprintf(env.Out, "📢 Message broadcast to %d members.\n", len(state.Members))
	return nil
}

// ---------- /focus ----------

type focusCmd struct{}

func (c *focusCmd) Name() string        { return "focus" }
func (c *focusCmd) Aliases() []string   { return nil }
func (c *focusCmd) Group() CommandGroup { return GroupTeam }
func (c *focusCmd) Description() string {
	return "Focus on a team member: /focus <member_label|next|prev>"
}

func (c *focusCmd) Execute(_ context.Context, env *Env, args string) error {
	state := env.TeamState
	if state == nil || !state.Enabled {
		fmt.Fprintln(env.Out, "No active team.")
		return nil
	}

	if len(state.Members) == 0 {
		fmt.Fprintln(env.Out, "Team has no members.")
		return nil
	}

	target := strings.TrimSpace(args)
	switch strings.ToLower(target) {
	case "next", "n":
		state.FocusIndex = (state.FocusIndex + 1) % len(state.Members)
	case "prev", "p":
		state.FocusIndex = (state.FocusIndex - 1 + len(state.Members)) % len(state.Members)
	case "":
		// Show current focus.
		if state.FocusIndex >= 0 && state.FocusIndex < len(state.Members) {
			m := state.Members[state.FocusIndex]
			fmt.Fprintf(env.Out, "🔍 Focused on: %s %s (%s) [%s]\n",
				m.RoleIcon(), m.Label, m.Role, m.Status)
		} else {
			fmt.Fprintln(env.Out, "No member focused. Use /focus <label> or /focus next.")
		}
		return nil
	default:
		// Find by label.
		found := false
		for i, m := range state.Members {
			if strings.EqualFold(m.Label, target) || strings.EqualFold(m.ID, target) {
				state.FocusIndex = i
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown member: %s. Use /agents to see members", target)
		}
	}

	// Update the env state.
	if env.SetTeamState != nil {
		env.SetTeamState(state)
	}

	m := state.Members[state.FocusIndex]
	fmt.Fprintf(env.Out, "🔍 Focus → %s %s (%s) [%s]\n",
		m.RoleIcon(), m.Label, m.Role, m.Status)
	return nil
}
