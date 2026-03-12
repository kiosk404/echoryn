// Package subagent provides a self-contained module for sub-agent lifecycle
// orchestration. It implements the K8S Controller pattern: watches SubAgentRecord
// state transitions and drives the reconciliation loop (spawn → running → completed → announce).
//
// Architecture overview (v2 — SessionController replaces Announcer):
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                      Manager (Controller)                    │
//	│  Spawn ─► validate ─► record ─► session ─► schedule         │
//	│  Execute ─► RunSubAgent ─► wait ─► readOutput ─► announce   │
//	│  Cancel ─► abort context ─► mark cancelled                   │
//	└─────────┬───────────────────┬──────────────────┬────────────┘
//	          │                   │                  │
//	   ┌──────┴──────┐   ┌──────┴────────────┐   ┌─┴──────────┐
//	   │  Scheduler  │   │ SessionController  │   │  Registry  │
//	   │  (semaphore │   │ (K8s controller    │   │  (store)   │
//	   │   + WG)     │   │  + workqueue)      │   │            │
//	   └─────────────┘   └───────────────────┘   └────────────┘
//
// Key architectural change (v2):
//   - Eliminated the dual-lock problem (sessionRunLock vs SessionWriteLock)
//   - Session writes are NEVER performed outside the runner's executeRun goroutine
//   - SessionController only stores pending announcements; the runner consumes them
//   - Workqueue provides K8s-style dirty/processing dedup for trigger requests
//   - Single-writer semantics: the runner is the sole writer to the session object
//
// Design goals:
//   - Independent module: all subagent logic in one package
//   - Extensible: Executor interface for future golem binding
//   - Bug fixes: timeout, session-history output, trigger pollution, lost updates
package subagent

import (
	"context"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
)

// Manager defines the interface for sub-agent orchestration.
//
// This is the primary API surface consumed by:
//   - Plugin layer (sessions_spawn / sessions_status tool handlers)
//   - AgentRunner (MarkRunActive/Idle, ProcessPending)
//   - Module wiring (module.go)
//
// Design: K8S controller pattern — watches SubAgentRecord state transitions
// and drives reconciliation (spawn → running → completed → announce).
type Manager interface {
	// Spawn starts a sub-agent in a new independent session.
	// Validates depth, concurrency, and agent ID before scheduling async execution.
	Spawn(ctx context.Context, req *entity.SubAgentSpawnRequest) (*entity.SubAgentRecord, error)

	// Cancel cancels a running sub-agent by record ID.
	Cancel(ctx context.Context, recordID string) error

	// CancelByParent cancels all running sub-agents for a parent session.
	CancelByParent(ctx context.Context, parentSessionID string) error

	// Get retrieves a sub-agent record by ID.
	Get(ctx context.Context, recordID string) (*entity.SubAgentRecord, error)

	// ListByParent returns all sub-agent records spawned by a given parent session.
	ListByParent(ctx context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error)

	// CountActiveByParent counts non-terminal sub-agents for a parent session.
	CountActiveByParent(ctx context.Context, parentSessionID string) (int, error)

	// FindByIdentifier performs multi-dimensional fuzzy lookup on sub-agent records.
	// Supports: numeric index, "subagent-N" pattern, label prefix, ID prefix, exact ID.
	FindByIdentifier(ctx context.Context, parentSessionID, identifier string) (*entity.SubAgentRecord, error)

	// SetExecutor wires the AgentExecutor after construction.
	// This breaks the circular dependency between Manager ↔ AgentRunner:
	// the manager is created first, then the runner, then the executor is set.
	// Must be called before any Spawn calls.
	SetExecutor(executor AgentExecutor)

	// Recover restores in-flight sub-agent records after process restart.
	Recover(ctx context.Context) error

	// Cleanup removes completed/failed sub-agent records older than the retention period.
	Cleanup(ctx context.Context) error

	// Stop gracefully shuts down the manager.
	Stop(ctx context.Context) error

	// Controller returns the SessionController for direct access.
	// Used by the runner to manage parent busy state and consume pending announcements.
	//
	// Replaces the previous Announcer() method. The SessionController ensures
	// single-writer semantics: pending announcements are returned to the runner
	// (not written directly to the session), eliminating lost-update bugs.
	Controller() *SessionController
}

// Registry defines the persistence layer for sub-agent records.
// Analogous to K8S Informer/Store pattern.
//
// Defined here (in the subagent package) as an interface to decouple
// from concrete store implementations (boltdb, inmemory).
type Registry interface {
	// Save persists a sub-agent record (create or update).
	Save(ctx context.Context, record *entity.SubAgentRecord) error

	// Get retrieves a sub-agent record by ID.
	Get(ctx context.Context, id string) (*entity.SubAgentRecord, error)

	// ListByParent returns all records for a given parent session.
	ListByParent(ctx context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error)

	// ListNonTerminal returns all records that have not reached a terminal state.
	// Used for recovery after restart.
	ListNonTerminal(ctx context.Context) ([]*entity.SubAgentRecord, error)

	// Delete removes a sub-agent record.
	Delete(ctx context.Context, id string) error
}

// AgentExecutor abstracts the agent execution layer.
//
// This decouples the subagent package from AgentRunner, enabling:
//   - Independent testing with mock executors
//   - Future extension for golem-based execution
//   - Multiple executor strategies (local, remote, hybrid)
//
// The concrete implementation wraps AgentRunner.RunSubAgent/Run.
type AgentExecutor interface {
	// RunSubAgent starts a sub-agent run and returns a streaming event reader.
	// The caller consumes events via sr.Recv() until io.EOF.
	RunSubAgent(ctx context.Context, req *ExecuteRequest) (*schema.StreamReader[*entity.AgentEvent], error)

	// TriggerParentTurn starts a new agent turn on a parent session
	// after a direct announcement. The triggerMessage is NOT saved as
	// a user message — it is a system trigger only.
	//
	// BUG FIX: Previous implementation saved triggerMessage as user message,
	// polluting conversation history. Now uses IsTrigger flag.
	TriggerParentTurn(ctx context.Context, parentSessionID, triggerMessage string)
}

// ExecuteRequest is the input to AgentExecutor.RunSubAgent.
type ExecuteRequest struct {
	AgentID   string
	SessionID string
	Input     string
}
