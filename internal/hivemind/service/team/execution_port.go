package team

import (
	"context"
)

// ExecutionPort is Team BC's outbound port to the Execution domain.
// It abstracts away all Execution BC (SubAgent) implementation details.
//
// Design: This interface is defined in the team package (Team BC owns it).
// Concrete implementation is provided by integration.teamExecutionAdapter.
//
// Key principle: Team BC depends on this interface; Execution BC has NO dependency on Team BC.
// This enforces one-way dependency: Team → ExecutionPort → Execution BC.
type ExecutionPort interface {
	// SpawnWorker creates a new execution worker according to the request.
	// Returns a WorkerRef (opaque logical reference) and the agent ID for confirmation.
	SpawnWorker(ctx context.Context, req *SpawnWorkerRequest) (*SpawnWorkerResult, error)

	// CancelWorker terminates an executing worker by its reference.
	CancelWorker(ctx context.Context, ref WorkerRef) error

	// GetWorkerStatus retrieves the current status of a worker.
	GetWorkerStatus(ctx context.Context, ref WorkerRef) (WorkerStatus, error)
}

// SpawnWorkerRequest is the request to spawn an execution worker.
// It contains only Team BC's concern — no Execution BC internal fields.
type SpawnWorkerRequest struct {
	// ParentSessionID is the session that triggered the spawn.
	ParentSessionID string

	// ParentRunID is the run that triggered the spawn.
	ParentRunID string

	// Task is the task description for the worker.
	Task string

	// AgentID is the agent to use.
	AgentID string

	// Label is the display name for the worker.
	Label string

	// Model is the optional LLM model override.
	Model string
}

// SpawnWorkerResult is the response from spawning an execution worker.
type SpawnWorkerResult struct {
	// WorkerRef is the logical reference to the spawned worker.
	WorkerRef WorkerRef

	// AgentID echoes the agent ID for confirmation.
	AgentID string

	// SessionID is the worker's session identifier.
	// Needed by Team BC for mailbox registration and event bridge wiring.
	// This is NOT an Execution BC internal leak — it's a collaboration-level concept
	// that both Team BC and Collaboration BC need for message routing.
	SessionID string
}

// WorkerStatus represents the current status of an executing worker.
type WorkerStatus string

const (
	WorkerStatusPending  WorkerStatus = "pending"
	WorkerStatusRunning  WorkerStatus = "running"
	WorkerStatusComplete WorkerStatus = "complete"
	WorkerStatusFailed   WorkerStatus = "failed"
	WorkerStatusCanceled WorkerStatus = "canceled"
)
