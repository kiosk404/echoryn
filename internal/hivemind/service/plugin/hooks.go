package plugin

import (
	"context"

	agentEntity "github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
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

	// HookBeforeCompaction is fired before compaction is about to run.
	// Plugin can perform pre-compaction actions (e.g., LLM-driven memory flush.)
	// Data: {"agent", "session", "window_info", "chat_model"}
	HookBeforeCompaction HookEvent = "before_compaction"

	// HookAfterCompaction is fired after compaction completes successfully.
	// Plugins can perform post-compaction actions (e.g., workspace context refresh.)
	// Data: {"agent", "session", "summary", "compaction_count"}
	HookAfterCompaction HookEvent = "after_compaction"

	// HookAfterTurn is fired after each agent turn completes successfully.
	// Data: {"agent", "session", "run", "token_usage"}
	// Aligned with OpenClaw's ContextEngine.afterTurn() and Claude Code's PostSamplingHook.
	HookAfterTurn HookEvent = "after_turn"
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

// AfterTurnHook is a function called after each successful agent turn.
// It receives the turn context including session, agent, and token usage info.
//
// Aligned with OpenClaw's ContextEngine.afterTurn() and Claude Code's PostSamplingHook.
type AfterTurnHook func(ctx context.Context, data AfterTurnData) error

// AfterTurnData carries context for after-turn hooks.
type AfterTurnData struct {
	AgentID    string
	SessionID  string
	RunID      string
	TokensUsed int
	MaxTokens  int

	// Messages are the session's active messages at turn completion.
	// Populated by AgentRunner.fireAfterTurnHooks from session.ActiveMessages().
	// Used by memory-core's post-turn extraction hook to analyze conversation context.
	Messages []*agentEntity.Message
}
