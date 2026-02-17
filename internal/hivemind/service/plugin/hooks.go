package plugin

import (
	"context"
)

// HookEvent identifies a lifecycle event that plugins can subscribe to.
// This corresponds to OpenClaw's typed lifecycle hooks (on()).
type HookEvent string

const (
	// HookServerStart is fired when the hivemind server starts.
	HookServerStart HookEvent = "server_start"

	// HookServerStop is fired during graceful shutdown.
	HookServerStop HookEvent = "server_stop"

	// HookBeforeAgentStart is fired before an Agent session begins.
	// Plugins can inject context (e.g., memory recall) here.
	HookBeforeAgentStart HookEvent = "before_agent_start"

	// HookAgentEnd is fired after an Agent session ends.
	// Plugins can capture/persist data (e.g., memory flush) here.
	HookAgentEnd HookEvent = "agent_end"

	// HookBeforeGenerate is fired before LLM generation.
	HookBeforeGenerate HookEvent = "before_generate"

	// HookAfterGenerate is fired after LLM generation completes.
	HookAfterGenerate HookEvent = "after_generate"
)

// HookHandler is the callback function for lifecycle hooks.
// The data parameter is event-specific; plugins should type-assert as needed.
type HookHandler func(ctx context.Context, data interface{}) error

// HookProvider is an optional plugin interface for plugins that want to
// register hooks declaratively. The framework probes for this interface
// and auto-registers the hooks.
type HookProvider interface {
	Plugin
	// Hooks returns a mapping of events to handlers.
	Hooks() map[HookEvent]HookHandler
}
