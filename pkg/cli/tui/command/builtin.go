package command

import (
	"context"
	"fmt"
)

// sentinel errors used by the TUI main loop to detect special commands.
var (
	// ErrQuit signals the main loop to exit gracefully.
	ErrQuit = fmt.Errorf("quit")
)

// RegisterBuiltins adds all built-in slash commands to the registry.
func RegisterBuiltins(r *Registry) {
	r.Register(&clearCmd{})
	r.Register(&quitCmd{})
	r.Register(&helpCmd{registry: r})
	r.Register(&modelCmd{})
	r.Register(&sessionCmd{})
}

// ---------- /clear ----------

type clearCmd struct{}

func (c *clearCmd) Name() string        { return "clear" }
func (c *clearCmd) Aliases() []string   { return nil }
func (c *clearCmd) Group() CommandGroup { return GroupSession }
func (c *clearCmd) Description() string { return "Reset conversation history" }

func (c *clearCmd) Execute(_ context.Context, env *Env, _ string) error {
	if env.ClearHistory != nil {
		env.ClearHistory()
	}
	fmt.Fprintln(env.Out, "Conversation cleared.")
	return nil
}

// ---------- /quit ----------

type quitCmd struct{}

func (c *quitCmd) Name() string        { return "quit" }
func (c *quitCmd) Aliases() []string   { return []string{"exit"} }
func (c *quitCmd) Group() CommandGroup { return GroupSession }
func (c *quitCmd) Description() string { return "Exit the chat" }

func (c *quitCmd) Execute(_ context.Context, _ *Env, _ string) error {
	return ErrQuit
}

// ---------- /help ----------

type helpCmd struct {
	registry *Registry
}

func (c *helpCmd) Name() string        { return "help" }
func (c *helpCmd) Aliases() []string   { return []string{"?"} }
func (c *helpCmd) Group() CommandGroup { return GroupSession }
func (c *helpCmd) Description() string { return "Show available commands" }

func (c *helpCmd) Execute(_ context.Context, env *Env, _ string) error {
	fmt.Fprintln(env.Out, c.registry.Help())
	return nil
}

// ---------- /model ----------

type modelCmd struct{}

func (c *modelCmd) Name() string        { return "model" }
func (c *modelCmd) Aliases() []string   { return nil }
func (c *modelCmd) Group() CommandGroup { return GroupSession }
func (c *modelCmd) Description() string { return "Show current model" }

func (c *modelCmd) Execute(_ context.Context, env *Env, _ string) error {
	model := "unknown"
	if env.Model != nil {
		model = env.Model()
	}
	fmt.Fprintf(env.Out, "Current model: %s\n", model)
	return nil
}

// ---------- /session ----------

type sessionCmd struct{}

func (c *sessionCmd) Name() string        { return "session" }
func (c *sessionCmd) Aliases() []string   { return nil }
func (c *sessionCmd) Group() CommandGroup { return GroupSession }
func (c *sessionCmd) Description() string { return "Show current session info" }

func (c *sessionCmd) Execute(_ context.Context, env *Env, _ string) error {
	key := "none"
	if env.SessionKey != nil {
		key = env.SessionKey()
	}
	fmt.Fprintf(env.Out, "Session: %s\n", key)
	return nil
}
