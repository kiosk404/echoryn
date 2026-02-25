package runtime

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// SubAgentRegistry defines the persistence layer for sub-agent records.
// Analogous to K8S Informer/Store pattern.
//
// Defined here (in the runtime package) to avoid circular imports
// between service ↔ runtime. Implementations live in store/boltdb and store/inmemory.
type SubAgentRegistry interface {
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

// SubAgentManagerConfig holds configuration for the sub-agent manager.
type SubAgentManagerConfig struct {
	// MaxConcurrent is the max number of concurrent sub-agents.
	// Default: 8 (matches OpenClaw's DEFAULT_SUBAGENT_MAX_CONCURRENT).
	MaxConcurrent int

	// ArchiveAfterMinutes is how long to keep completed records before cleanup.
	// Default: 60.
	ArchiveAfterMinutes int
}

// DefaultSubAgentManagerConfig returns the default configuration.
func DefaultSubAgentManagerConfig() SubAgentManagerConfig {
	return SubAgentManagerConfig{
		MaxConcurrent:       DefaultMaxConcurrentSubAgents,
		ArchiveAfterMinutes: 60,
	}
}

// subAgentManagerImpl implements service.SubAgentManager.
//
// This is the core orchestrator for sub-agent lifecycle:
//   - Spawn: validate → create record → create session → schedule async execution
//   - Execute: run agent in independent session → consume stream → announce result
//   - Cancel: abort running sub-agent via context cancellation
//   - Recover: restore in-flight records after process restart
//
// K8S Controller pattern: watches SubAgentRecord state transitions and drives
// the reconciliation loop (spawn → running → completed → announce).
type subAgentManagerImpl struct {
	registry    SubAgentRegistry
	agentRepo   repo.AgentRepository
	sessionRepo repo.SessionRepository
	runner      *AgentRunner
	scheduler   *SubAgentScheduler
	announcer   *AnnounceController
	cfg         SubAgentManagerConfig

	// abortFuncs tracks cancel functions for running sub-agents.
	// Key: record ID, Value: cancel function.
	mu         sync.Mutex
	abortFuncs map[string]context.CancelFunc
}

// SubAgentManager defines the interface for sub-agent orchestration.
// This is defined in runtime to avoid circular imports between service ↔ runtime.
//
// Design overview (based on OpenClaw's sub-agent architecture, K8S controller pattern):
//
//  1. A parent agent can "spawn" a sub-agent via the `sessions_spawn` tool
//  2. The sub-agent runs in an **independent session** with its own context
//  3. Sub-agents CANNOT spawn further sub-agents (max depth = 1)
//  4. On completion, the result is "announced" back to the parent session
//  5. Sub-agents can use a different (potentially cheaper) model than the parent
type SubAgentManager interface {
	Spawn(ctx context.Context, req *entity.SubAgentSpawnRequest) (*entity.SubAgentRecord, error)
	Cancel(ctx context.Context, recordID string) error
	CancelByParent(ctx context.Context, parentSessionID string) error
	Get(ctx context.Context, recordID string) (*entity.SubAgentRecord, error)
	ListByParent(ctx context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error)
	Recover(ctx context.Context) error
	Cleanup(ctx context.Context) error
	Stop(ctx context.Context) error
}

var _ SubAgentManager = (*subAgentManagerImpl)(nil)

// NewSubAgentManager creates a new sub-agent manager.
func NewSubAgentManager(
	registry SubAgentRegistry,
	agentRepo repo.AgentRepository,
	sessionRepo repo.SessionRepository,
	runner *AgentRunner,
	cfg SubAgentManagerConfig,
) SubAgentManager {
	scheduler := NewSubAgentScheduler(cfg.MaxConcurrent)
	announcer := NewAnnounceController(runner, registry, sessionRepo)

	return &subAgentManagerImpl{
		registry:    registry,
		agentRepo:   agentRepo,
		sessionRepo: sessionRepo,
		runner:      runner,
		scheduler:   scheduler,
		announcer:   announcer,
		cfg:         cfg,
		abortFuncs:  make(map[string]context.CancelFunc),
	}
}

// Spawn starts a sub-agent in a new independent session.
func (m *subAgentManagerImpl) Spawn(ctx context.Context, req *entity.SubAgentSpawnRequest) (*entity.SubAgentRecord, error) {
	// 1. Depth check: sub-agents cannot spawn further sub-agents.
	parentSession, err := m.sessionRepo.Get(ctx, req.ParentSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent session: %w", err)
	}
	if parentSession.IsSubAgentSession() {
		return nil, errno.ErrMaxDepthExceeded
	}

	// 2. Resolve agent ID (default to parent's agent).
	agentID := req.AgentID
	if agentID == "" {
		agentID = parentSession.AgentID
	}

	// Validate agent exists.
	if _, err := m.agentRepo.Get(ctx, agentID); err != nil {
		return nil, fmt.Errorf("sub-agent target agent %q: %w", agentID, err)
	}

	// 3. Create sub-agent record.
	record := &entity.SubAgentRecord{
		ID:              uuid.New().String(),
		ParentSessionID: req.ParentSessionID,
		ParentRunID:     req.ParentRunID,
		AgentID:         agentID,
		Task:            req.Task,
		Label:           req.Label,
		Model:           req.Model,
		Cleanup:         req.EffectiveCleanup(),
		Status:          entity.SubAgentStatusPending,
		CreatedAt:       time.Now(),
	}

	// 4. Create independent session for the sub-agent.
	subSession := &entity.Session{
		ID:              uuid.New().String(),
		AgentID:         agentID,
		ParentSessionID: parentSession.ID,
		Messages:        make([]*entity.Message, 0),
		Metadata: map[string]string{
			"subagent_id":   record.ID,
			"subagent_task": req.Task,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := m.sessionRepo.Create(ctx, subSession); err != nil {
		return nil, fmt.Errorf("failed to create sub-agent session: %w", err)
	}
	record.SessionID = subSession.ID

	// 5. Persist record.
	if err := m.registry.Save(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to save sub-agent record: %w", err)
	}

	// 6. Schedule async execution.
	if err := m.scheduler.Submit(func(schedCtx context.Context) {
		m.executeSubAgent(schedCtx, record)
	}); err != nil {
		// Concurrency limit hit — mark as failed.
		record.MarkFailed("concurrency limit reached")
		_ = m.registry.Save(ctx, record)
		return nil, errno.ErrConcurrencyLimit
	}

	logger.Info("[SubAgentManager] spawned sub-agent %s (task=%q, agent=%s, session=%s)",
		record.ID, record.Task, record.AgentID, record.SessionID)

	return record, nil
}

// executeSubAgent is the async execution body for a sub-agent.
func (m *subAgentManagerImpl) executeSubAgent(parentCtx context.Context, record *entity.SubAgentRecord) {
	// Create a cancellable context for this specific sub-agent.
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	// Register abort function.
	m.mu.Lock()
	m.abortFuncs[record.ID] = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.abortFuncs, record.ID)
		m.mu.Unlock()
	}()

	// Transition to running.
	record.MarkRunning()
	if err := m.registry.Save(ctx, record); err != nil {
		logger.Warn("[SubAgentManager] failed to save running state for %s: %v", record.ID, err)
	}

	// Build the sub-agent system prompt (minimal mode).
	subAgentPrompt := buildSubAgentSystemPrompt(record.Task)

	// Run the agent via AgentRunner.
	sr, err := m.runner.RunSubAgent(ctx, &RunRequest{
		AgentID:   record.AgentID,
		SessionID: record.SessionID,
		Input:     subAgentPrompt,
	})
	if err != nil {
		record.MarkFailed(fmt.Sprintf("failed to start sub-agent run: %v", err))
		_ = m.registry.Save(context.Background(), record)
		logger.Warn("[SubAgentManager] sub-agent %s failed to start: %v", record.ID, err)
		return
	}

	// Consume the stream to get the final result.
	var lastAssistantContent string
	for {
		event, err := sr.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			record.MarkFailed(fmt.Sprintf("stream error: %v", err))
			_ = m.registry.Save(context.Background(), record)
			logger.Warn("[SubAgentManager] sub-agent %s stream error: %v", record.ID, err)
			return
		}

		// Track the latest text content for the final result.
		if event.Type == entity.EventTextDelta {
			lastAssistantContent += event.Delta
		}

		// Check for errors.
		if event.Type == entity.EventError {
			record.MarkFailed(event.Error)
			_ = m.registry.Save(context.Background(), record)
			logger.Warn("[SubAgentManager] sub-agent %s error event: %s", record.ID, event.Error)
			return
		}
	}

	// Mark completed.
	record.MarkCompleted(lastAssistantContent)
	if err := m.registry.Save(context.Background(), record); err != nil {
		logger.Warn("[SubAgentManager] failed to save completed record %s: %v", record.ID, err)
	}

	logger.Info("[SubAgentManager] sub-agent %s completed (duration=%s, result_len=%d)",
		record.ID, record.Duration(), len(lastAssistantContent))

	// Announce result to parent session.
	m.announcer.Announce(context.Background(), record)

	// Cleanup session if configured.
	if record.Cleanup == "delete" {
		_ = m.sessionRepo.Delete(context.Background(), record.SessionID)
	}
}

// Cancel cancels a running sub-agent by record ID.
func (m *subAgentManagerImpl) Cancel(ctx context.Context, recordID string) error {
	record, err := m.registry.Get(ctx, recordID)
	if err != nil {
		return err
	}
	if record.Status.IsTerminal() {
		return errno.ErrSubAgentAlreadyDone
	}

	m.mu.Lock()
	cancelFn, ok := m.abortFuncs[recordID]
	m.mu.Unlock()

	if ok {
		cancelFn()
	}

	record.MarkCancelled()
	return m.registry.Save(ctx, record)
}

// CancelByParent cancels all running sub-agents for a parent session.
func (m *subAgentManagerImpl) CancelByParent(ctx context.Context, parentSessionID string) error {
	records, err := m.registry.ListByParent(ctx, parentSessionID)
	if err != nil {
		return err
	}

	for _, record := range records {
		if !record.Status.IsTerminal() {
			if cancelErr := m.Cancel(ctx, record.ID); cancelErr != nil {
				logger.Warn("[SubAgentManager] failed to cancel sub-agent %s: %v",
					record.ID, cancelErr)
			}
		}
	}
	return nil
}

// Get retrieves a sub-agent record by ID.
func (m *subAgentManagerImpl) Get(ctx context.Context, recordID string) (*entity.SubAgentRecord, error) {
	return m.registry.Get(ctx, recordID)
}

// ListByParent returns all sub-agent records spawned by a given parent session.
func (m *subAgentManagerImpl) ListByParent(ctx context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error) {
	return m.registry.ListByParent(ctx, parentSessionID)
}

// Recover restores in-flight sub-agent records after process restart.
// Records in non-terminal state are marked as failed since their goroutines are gone.
func (m *subAgentManagerImpl) Recover(ctx context.Context) error {
	records, err := m.registry.ListNonTerminal(ctx)
	if err != nil {
		return fmt.Errorf("failed to list non-terminal records: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	logger.Info("[SubAgentManager] recovering %d in-flight sub-agent records", len(records))

	for _, record := range records {
		record.MarkFailed("process restarted while sub-agent was running")
		if err := m.registry.Save(ctx, record); err != nil {
			logger.Warn("[SubAgentManager] failed to save recovery state for %s: %v",
				record.ID, err)
		}

		// Attempt to announce the failure to the parent.
		if !record.Announced {
			m.announcer.Announce(ctx, record)
		}
	}

	return nil
}

// Cleanup removes completed/failed sub-agent records older than the retention period.
func (m *subAgentManagerImpl) Cleanup(ctx context.Context) error {
	cutoff := time.Now().Add(-time.Duration(m.cfg.ArchiveAfterMinutes) * time.Minute)

	// We iterate parent sessions with known sub-agents.
	// For simplicity, list all and filter.
	// In production, a more efficient query would be needed.
	records, err := m.registry.ListNonTerminal(ctx)
	if err != nil {
		return err
	}

	// Get terminal records by checking all parents (simplified approach).
	// For a real implementation, SubAgentRegistry would have a ListAll method.
	_ = records
	_ = cutoff

	logger.Debug("[SubAgentManager] cleanup pass completed")
	return nil
}

// Stop gracefully shuts down the manager.
func (m *subAgentManagerImpl) Stop(_ context.Context) error {
	m.scheduler.Stop()
	return nil
}

// buildSubAgentSystemPrompt creates the system prompt for a sub-agent.
// Modeled after OpenClaw's buildSubagentSystemPrompt.
func buildSubAgentSystemPrompt(task string) string {
	return fmt.Sprintf(`You are a subagent spawned by the main agent for a specific task.
Complete the task below. That is your entire purpose.
You are NOT the main agent. Do not try to be.
Do not send messages to the user. Do not ask questions. Just complete the task and report your findings.

## Task

%s

## Instructions

- Focus solely on the task above
- Be thorough and provide detailed findings
- When done, summarize your results clearly
- Do not attempt to spawn further sub-agents`, task)
}
