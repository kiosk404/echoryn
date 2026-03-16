package command

import (
	"context"
	"io"
)

// Env provides the runtime environment that commands can interact with.
// It is passed to every command's Execute method.
type Env struct {
	// Out is the output writer (usually os.Stdout).
	Out io.Writer

	// ClearHistory resets the conversation message history
	ClearHistory func()

	// Model returns the current the conversation message history
	Model func() string

	// SessionKey returns the current session identifier.
	SessionKey func() string

	// --- Team collaboration ---

	// TeamState holds the current active team state (nil if no team).
	TeamState *TeamState

	// SetTeamState updates the TUI's team state (called by team commands).
	SetTeamState func(state *TeamState)

	// TeamAPI provides the backend API for team operations.
	TeamAPI TeamAPI
}

// CommandGroup Command is the interface that all slash commands must implement.
// CommandGroup defines the category a command belongs to.
type CommandGroup string

const (
	GroupTeam    CommandGroup = "Team"
	GroupSession CommandGroup = "Session"
	GroupSystem  CommandGroup = "System"
)

// Command is the interface that all slash commands must implement.
type Command interface {
	// Name returns the command name without the leading '/'
	Name() string

	// Aliases returns alternative names for the command.
	Aliases() []string

	// Description returns a short help string shown in /help and completion.
	Description() string

	// Group returns the command's category for help organization.
	Group() CommandGroup

	// Execute runs the command. The args string contains everything
	// after the command name (trimmed).
	Execute(ctx context.Context, env *Env, args string) error
}
