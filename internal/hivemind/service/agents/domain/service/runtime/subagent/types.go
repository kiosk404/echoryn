package subagent

import "time"

// Config holds configuration for the sub-agent manager.
//
// Aligned with OpenClaw's subagent configuration:
//   - MaxConcurrent:       global concurrency cap (DEFAULT_SUBAGENT_MAX_CONCURRENT = 8)
//   - MaxChildrenPerAgent: per-session child limit (maxChildrenPerAgent = 5)
//   - MaxSpawnDepth:       nesting depth limit (DEFAULT_SUBAGENT_MAX_SPAWN_DEPTH = 3)
type Config struct {
	// MaxConcurrent is the max number of concurrent sub-agents globally.
	// Default: 8 (matches OpenClaw's DEFAULT_SUBAGENT_MAX_CONCURRENT).
	MaxConcurrent int

	// MaxChildrenPerAgent is the max number of active sub-agents per parent session.
	// Default: 5 (matches OpenClaw's maxChildrenPerAgent).
	MaxChildrenPerAgent int

	// MaxSpawnDepth is the maximum nesting depth for sub-agent spawning.
	// 1 means sub-agents cannot spawn further sub-agents.
	// 3 means up to 3 levels of nesting are allowed.
	// Default: 3 (matches OpenClaw's DEFAULT_SUBAGENT_MAX_SPAWN_DEPTH).
	MaxSpawnDepth int

	// ExecutionTimeout is the maximum duration for a single sub-agent execution.
	// BUG FIX: Previously, SubAgentScheduler used context.Background() with no timeout,
	// causing executeSubAgent to potentially block forever. This adds an independent
	// timeout at the subagent level.
	// Default: 3 minutes.
	ExecutionTimeout time.Duration

	// ArchiveAfterMinutes is how long to keep completed records before cleanup.
	// Default: 60.
	ArchiveAfterMinutes int
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		MaxConcurrent:       DefaultMaxConcurrent,
		MaxChildrenPerAgent: DefaultMaxChildrenPerAgent,
		MaxSpawnDepth:       DefaultMaxSpawnDepth,
		ExecutionTimeout:    DefaultExecutionTimeout,
		ArchiveAfterMinutes: 60,
	}
}

const (
	// DefaultMaxConcurrent matches OpenClaw's DEFAULT_SUBAGENT_MAX_CONCURRENT.
	DefaultMaxConcurrent = 8

	// DefaultMaxChildrenPerAgent matches OpenClaw's maxChildrenPerAgent.
	DefaultMaxChildrenPerAgent = 5

	// DefaultMaxSpawnDepth matches OpenClaw's DEFAULT_SUBAGENT_MAX_SPAWN_DEPTH.
	DefaultMaxSpawnDepth = 3

	// DefaultExecutionTimeout is the default timeout for sub-agent execution.
	// BUG FIX: This ensures sub-agents cannot run forever even if the
	// underlying agent run hangs (e.g., LLM API timeout, ReAct loop divergence).
	DefaultExecutionTimeout = 3 * time.Minute
)
