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
}

// Command is the interface that all slash commands must implement.
type Command interface {
	// Name returns the command name without the leading '/'
	Name() string

	// Aliases returns alternative names for the command.
	Aliases() []string

	// Description returns a short help string shown in /help and completion.
	Description() string

	// Execute runs the command. The args string contains everything
	// after the command name (trimmed).
	Execute(ctx context.Context, env *Env, args string) error
}
