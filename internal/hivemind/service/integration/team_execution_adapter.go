// Package integration provides cross-bounded-context adapters and module assembly.
// This layer is the ONLY place where Team BC, Execution BC, and Collaboration BC
// are coupled. All other packages maintain strict boundaries.
package integration

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/subagent"
	"github.com/kiosk404/echoryn/internal/hivemind/service/team"
)

// WorkerIndex maintains the bidirectional mapping between WorkerRef and Execution BC internals.
// This is the ONLY place in the codebase where WorkerRef ↔ SubAgentRecordID/SessionID mapping exists.
// Team BC never accesses SubAgentRecordID or SessionID directly.
type WorkerIndex struct {
	// Forward mapping: WorkerRef.ID → SubAgentRecordID + SessionID
	refs map[string]*workerMapping // key: WorkerRef.ID

	// Reverse mappings for fast lookup
	byRecordID  map[string]string // SubAgentRecordID → WorkerRef.ID
	bySessionID map[string]string // SessionID → WorkerRef.ID
}

type workerMapping struct {
	recordID  string
	sessionID string
	agentID   string
}

// NewWorkerIndex creates a new bidirectional mapping index.
func NewWorkerIndex() *WorkerIndex {
	return &WorkerIndex{
		refs:        make(map[string]*workerMapping),
		byRecordID:  make(map[string]string),
		bySessionID: make(map[string]string),
	}
}

// Register adds a mapping from WorkerRef to Execution BC internals.
func (wi *WorkerIndex) Register(ref team.WorkerRef, recordID string, sessionID string, agentID string) error {
	if ref.ID == "" {
		return fmt.Errorf("worker ref ID cannot be empty")
	}
	if recordID == "" {
		return fmt.Errorf("record ID cannot be empty")
	}
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	// Check for duplicates
	if existing, ok := wi.refs[ref.ID]; ok {
		return fmt.Errorf("worker ref %s already registered (recordID=%s)", ref.ID, existing.recordID)
	}

	mapping := &workerMapping{
		recordID:  recordID,
		sessionID: sessionID,
		agentID:   agentID,
	}
	wi.refs[ref.ID] = mapping
	wi.byRecordID[recordID] = ref.ID
	wi.bySessionID[sessionID] = ref.ID

	return nil
}

// GetRecordID retrieves the SubAgentRecordID for a WorkerRef.
func (wi *WorkerIndex) GetRecordID(ref team.WorkerRef) (string, bool) {
	mapping, ok := wi.refs[ref.ID]
	if !ok {
		return "", false
	}
	return mapping.recordID, true
}

// GetSessionID retrieves the SessionID for a WorkerRef.
func (wi *WorkerIndex) GetSessionID(ref team.WorkerRef) (string, bool) {
	mapping, ok := wi.refs[ref.ID]
	if !ok {
		return "", false
	}
	return mapping.sessionID, true
}

// GetAgentID retrieves the AgentID for a WorkerRef.
func (wi *WorkerIndex) GetAgentID(ref team.WorkerRef) (string, bool) {
	mapping, ok := wi.refs[ref.ID]
	if !ok {
		return "", false
	}
	return mapping.agentID, true
}

// FindByRecordID finds the WorkerRef for a given SubAgentRecordID.
func (wi *WorkerIndex) FindByRecordID(recordID string) (team.WorkerRef, bool) {
	refID, ok := wi.byRecordID[recordID]
	if !ok {
		return team.WorkerRef{}, false
	}
	return team.WorkerRef{ID: refID}, true
}

// FindBySessionID finds the WorkerRef for a given SessionID.
func (wi *WorkerIndex) FindBySessionID(sessionID string) (team.WorkerRef, bool) {
	refID, ok := wi.bySessionID[sessionID]
	if !ok {
		return team.WorkerRef{}, false
	}
	return team.WorkerRef{ID: refID}, true
}

// Unregister removes a mapping (called when worker terminates).
func (wi *WorkerIndex) Unregister(ref team.WorkerRef) {
	mapping, ok := wi.refs[ref.ID]
	if !ok {
		return
	}

	delete(wi.refs, ref.ID)
	delete(wi.byRecordID, mapping.recordID)
	delete(wi.bySessionID, mapping.sessionID)
}

// --- Team-Execution Adapter ---

// teamExecutionAdapter implements team.ExecutionPort by adapting Team's SpawnWorker calls
// into SubAgent Manager.Spawn calls. This is the ONLY place where Team BC and
// Execution BC are coupled.
type teamExecutionAdapter struct {
	manager     subagent.Manager
	workerIndex *WorkerIndex
}

// NewTeamExecutionAdapter creates a new adapter that bridges Team BC to Execution BC.
func NewTeamExecutionAdapter(manager subagent.Manager, workerIndex *WorkerIndex) team.ExecutionPort {
	return &teamExecutionAdapter{
		manager:     manager,
		workerIndex: workerIndex,
	}
}

// SpawnWorker implements team.ExecutionPort.SpawnWorker by delegating to subagent.Manager.
func (a *teamExecutionAdapter) SpawnWorker(ctx context.Context, req *team.SpawnWorkerRequest) (*team.SpawnWorkerResult, error) {
	if req == nil {
		return nil, fmt.Errorf("spawn worker request is nil")
	}

	// Adapt Team's request to SubAgent's spawn request
	spawnReq := &entity.SubAgentSpawnRequest{
		ParentSessionID: req.ParentSessionID,
		ParentRunID:     req.ParentRunID,
		Task:            req.Task,
		AgentID:         req.AgentID,
		Label:           req.Label,
		Model:           req.Model,
	}

	// Execute the spawn in the Execution BC
	record, err := a.manager.Spawn(ctx, spawnReq)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn worker: %w", err)
	}

	// Create a logical WorkerRef (Team BC never sees the SubAgentRecordID)
	workerRef := team.WorkerRef{ID: "worker-" + record.ID}

	// Register the mapping
	if err := a.workerIndex.Register(workerRef, record.ID, record.SessionID, record.AgentID); err != nil {
		// Log warning but don't fail — the worker is already spawned
		// TODO: proper logging
		fmt.Printf("warning: failed to register worker index: %v\n", err)
	}

	return &team.SpawnWorkerResult{
		WorkerRef: workerRef,
		AgentID:   record.AgentID,
		SessionID: record.SessionID,
	}, nil
}

// CancelWorker implements team.ExecutionPort.CancelWorker by delegating to subagent.Manager.
func (a *teamExecutionAdapter) CancelWorker(ctx context.Context, ref team.WorkerRef) error {
	if ref.ID == "" {
		return fmt.Errorf("worker ref ID cannot be empty")
	}

	// Resolve WorkerRef to SubAgentRecordID
	recordID, ok := a.workerIndex.GetRecordID(ref)
	if !ok {
		return fmt.Errorf("unknown worker ref: %s", ref.ID)
	}

	// Delegate to SubAgent Manager
	if err := a.manager.Cancel(ctx, recordID); err != nil {
		return fmt.Errorf("failed to cancel worker: %w", err)
	}

	// Clean up the mapping
	a.workerIndex.Unregister(ref)

	return nil
}

// GetWorkerStatus implements team.ExecutionPort.GetWorkerStatus by querying the SubAgent record.
func (a *teamExecutionAdapter) GetWorkerStatus(ctx context.Context, ref team.WorkerRef) (team.WorkerStatus, error) {
	if ref.ID == "" {
		return "", fmt.Errorf("worker ref ID cannot be empty")
	}

	// Resolve WorkerRef to SubAgentRecordID
	recordID, ok := a.workerIndex.GetRecordID(ref)
	if !ok {
		return "", fmt.Errorf("unknown worker ref: %s", ref.ID)
	}

	// Get the SubAgent record from the manager
	record, err := a.manager.Get(ctx, recordID)
	if err != nil {
		return "", fmt.Errorf("failed to get worker status: %w", err)
	}

	// Map SubAgentStatus to WorkerStatus
	status := mapSubAgentStatusToWorkerStatus(record.Status)
	return status, nil
}

// mapSubAgentStatusToWorkerStatus converts entity.SubAgentStatus to team.WorkerStatus.
// This ensures Team BC never directly references SubAgent status values.
func mapSubAgentStatusToWorkerStatus(status entity.SubAgentStatus) team.WorkerStatus {
	switch status {
	case entity.SubAgentStatusPending:
		return team.WorkerStatusPending
	case entity.SubAgentStatusRunning:
		return team.WorkerStatusRunning
	case entity.SubAgentStatusCompleted:
		return team.WorkerStatusComplete
	case entity.SubAgentStatusFailed:
		return team.WorkerStatusFailed
	case entity.SubAgentStatusCancelled:
		return team.WorkerStatusCanceled
	default:
		// Defensive: treat unknown status as pending
		return team.WorkerStatusPending
	}
}
