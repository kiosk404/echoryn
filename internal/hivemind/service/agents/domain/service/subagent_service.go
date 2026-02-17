package service

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
)

// SubAgentManager defines the interface for sub-agent orchestration.
//
// Design overview (based on OpenClaw's sub-agent architecture):
//
//  1. A parent agent can "spawn" a sub-agent via the `sessions_spawn` tool
//  2. The sub-agent runs in an **independent session** with its own context
//  3. Sub-agents CANNOT spawn further sub-agents (max depth = 1)
//  4. On completion, the result is "announced" back to the parent session
//     by injecting a message into the parent's message history
//  5. Sub-agents can use a different (potentially cheaper) model than the parent
//
// Concurrency control:
//   - A semaphore limits the max number of concurrent sub-agents per parent (default: 8)
//   - This prevents runaway resource usage from unbounded spawning
//
// Persistence:
//   - SubAgentRecord entries are stored in SubAgentRegistry (BoltDB-backed)
//   - This allows recovery of in-flight sub-agents after process restart
//
// TODO(subagent): Implement this interface. Key components needed:
//   - SubAgentManager implementation (spawn goroutine + semaphore + announce)
//   - SubAgentRegistry (BoltDB persistence for SubAgentRecord)
//   - `sessions_spawn` tool registration in Plugin Framework
//   - `sessions_send` tool for cross-agent communication
//   - Session.ParentSessionID field for depth checking
//   - Integration with AgentRunner.Run() for sub-agent execution
type SubAgentManager interface {
	// Spawn starts a sub-agent in a new independent session.
	// The sub-agent runs asynchronously; use the returned record ID to track progress.
	// Returns ErrMaxDepthExceeded if the parent is itself a sub-agent (no nesting).
	// Returns ErrConcurrencyLimit if the max concurrent sub-agents limit is reached.
	Spawn(ctx context.Context, req *entity.SubAgentSpawnRequest) (*entity.SubAgentRecord, error)

	// Cancel cancels a running sub-agent.
	Cancel(ctx context.Context, recordID string) error

	// Get retrieves a sub-agent record by ID.
	Get(ctx context.Context, recordID string) (*entity.SubAgentRecord, error)

	// ListByParent returns all sub-agent records spawned by a given parent session.
	ListByParent(ctx context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error)

	// Cleanup removes completed/failed sub-agent records older than the retention period.
	Cleanup(ctx context.Context) error
}

// SubAgentRegistry defines the persistence layer for sub-agent records.
//
// TODO(subagent): Implement BoltDB-backed registry.
type SubAgentRegistry interface {
	// Save persists a sub-agent record (create or update).
	Save(ctx context.Context, record *entity.SubAgentRecord) error

	// Get retrieves a sub-agent record by ID.
	Get(ctx context.Context, id string) (*entity.SubAgentRecord, error)

	// ListByParent returns all records for a given parent session.
	ListByParent(ctx context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error)

	// Delete removes a sub-agent record.
	Delete(ctx context.Context, id string) error
}
