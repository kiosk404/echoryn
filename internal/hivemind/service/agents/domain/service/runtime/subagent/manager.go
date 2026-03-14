package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/internal/hivemind/service/subagent/observer"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// managerImpl implements the Manager interface.
//
// This is the core orchestrator for sub-agent lifecycle:
//   - Spawn: validate → create record → create session → schedule async execution
//   - Execute: run agent in independent session → wait completion → read output → announce
//   - Cancel: abort running sub-agent via context cancellation
//   - Recover: restore in-flight records after process restart
//
// K8S Controller pattern: watches SubAgentRecord state transitions and drives
// the reconciliation loop (spawn → running → completed → announce).
//
// v2 architecture: Uses SessionController (K8s-style workqueue + single-writer)
// instead of the previous Announcer (ad-hoc steer/queue/direct + dual-lock).
type managerImpl struct {
	registry    Registry
	agentRepo   repo.AgentRepository
	sessionRepo repo.SessionRepository
	scheduler   *Scheduler
	controller  *SessionController
	exec        *subAgentExecutor
	cfg         Config
	emitter     *observer.Emitter

	// abortFuncs tracks cancel functions for running sub-agents.
	// Key: record ID, Value: cancel function.
	mu         sync.Mutex
	abortFuncs map[string]context.CancelFunc

	// spawnMu serializes Spawn() calls per parent session to prevent
	// index assignment race conditions when multiple sub-agents are
	// spawned concurrently (e.g., parallel tool calls from the LLM).
	spawnMu sync.Map // key: parentSessionID (string), value: *sync.Mutex
}

var _ Manager = (*managerImpl)(nil)

// NewManager creates a new sub-agent manager.
//
// The executor parameter provides the agent execution capability.
// After construction, call SetExecutor to wire the AgentExecutor
// (required before any Spawn calls can execute).
//
// The obs parameter is optional: pass nil for a no-op observer.
//
// v2: Uses SessionController instead of Announcer. The SessionController
// ensures single-writer semantics on sessions — pending announcements
// are stored in memory and consumed by the runner within its own session
// write path, eliminating the dual-lock lost-update problem.
func NewManager(
	registry Registry,
	agentRepo repo.AgentRepository,
	sessionRepo repo.SessionRepository,
	cfg Config,
	obs observer.Observer,
) *managerImpl {
	scheduler := NewScheduler(cfg.MaxConcurrent)
	controller := NewSessionController(registry, sessionRepo)
	emitter := observer.NewEmitter(obs)

	mgr := &managerImpl{
		registry:    registry,
		agentRepo:   agentRepo,
		sessionRepo: sessionRepo,
		scheduler:   scheduler,
		controller:  controller,
		cfg:         cfg,
		emitter:     emitter,
		abortFuncs:  make(map[string]context.CancelFunc),
	}

	// Create the executor with all dependencies.
	mgr.exec = &subAgentExecutor{
		registry:    registry,
		sessionRepo: sessionRepo,
		controller:  controller,
		cfg:         cfg,
		emitter:     emitter,
	}

	return mgr
}

// SetExecutor wires the AgentExecutor after construction.
// This breaks the circular dependency between Manager ↔ AgentRunner:
// the manager is created first, then the runner, then the executor is set.
//
// K8S analogy: post-start injection, similar to how controllers are wired
// after the informer factory is created.
func (m *managerImpl) SetExecutor(executor AgentExecutor) {
	if executor == nil {
		panic("subagent.Manager.SetExecutor: executor must not be nil")
	}
	m.exec.executor = executor
	m.controller.SetExecutor(executor)
}

// SetLifecycleHook registers a hook that is called when any SubAgent reaches
// a terminal state. This is the bridge between SubAgent lifecycle and the
// Team module's EventBridge.
func (m *managerImpl) SetLifecycleHook(hook LifecycleHook) {
	m.exec.lifecycleHook = hook
}

// Controller returns the SessionController.
// Used by the runner to manage parent busy state and consume pending announcements.
func (m *managerImpl) Controller() *SessionController {
	return m.controller
}

// parentSpawnMutex returns a per-parent mutex that serializes Spawn() calls
// for the same parent session.
func (m *managerImpl) parentSpawnMutex(parentSessionID string) *sync.Mutex {
	actual, _ := m.spawnMu.LoadOrStore(parentSessionID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// Spawn starts a sub-agent in a new independent session.
//
// Validation pipeline (aligned with OpenClaw's spawnSubagentDirect):
//  1. Depth check: enforce MaxSpawnDepth
//  2. Per-session child limit: enforce MaxChildrenPerAgent
//  3. Agent authorization check
//  4. Create session + record (index assignment is serialized per-parent)
//  5. Schedule async execution
func (m *managerImpl) Spawn(ctx context.Context, req *entity.SubAgentSpawnRequest) (*entity.SubAgentRecord, error) {
	// Serialize per-parent to prevent index race conditions.
	pmu := m.parentSpawnMutex(req.ParentSessionID)
	pmu.Lock()
	defer pmu.Unlock()

	// 1. Depth check.
	// If parent session doesn't exist (e.g. external CLI client with auto-generated session ID),
	// treat it as a root-level call with depth 0.
	parentSession, err := m.sessionRepo.Get(ctx, req.ParentSessionID)
	if err != nil {
		logger.Debug("[subagent] parent session %q not found, treating as root-level (depth 0)", req.ParentSessionID)
		parentSession = nil
	}

	currentDepth := 0
	var parentAgentID string
	if parentSession != nil {
		currentDepth = parentSession.SpawnDepth()
		parentAgentID = parentSession.AgentID
	}

	if currentDepth >= m.cfg.MaxSpawnDepth {
		return nil, fmt.Errorf("%w (current depth: %d, max: %d)",
			errno.ErrMaxDepthExceeded, currentDepth, m.cfg.MaxSpawnDepth)
	}

	// 2. Per-session child limit.
	activeCount, err := m.CountActiveByParent(ctx, req.ParentSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count active sub-agents: %w", err)
	}
	if activeCount >= m.cfg.MaxChildrenPerAgent {
		return nil, fmt.Errorf("%w (active: %d, max per session: %d)",
			errno.ErrConcurrencyLimit, activeCount, m.cfg.MaxChildrenPerAgent)
	}

	// 3. Resolve agent ID (default to parent's agent if available).
	agentID := req.AgentID
	if agentID == "" {
		agentID = parentAgentID
	}

	// Validate agent exists. If the requested agent_id is not found,
	// fall back to the parent's agent rather than failing entirely.
	if _, err := m.agentRepo.Get(ctx, agentID); err != nil {
		// Try fallback to parent's parent's agent if available and different.
		if parentAgentID != "" && agentID != parentAgentID {
			logger.Warn("[subagent] requested agent %q not found, falling back to parent agent %q: %v",
				agentID, parentAgentID, err)
			agentID = parentAgentID
			if _, err := m.agentRepo.Get(ctx, agentID); err != nil {
				return nil, fmt.Errorf("fallback to parent agent %q also failed: %w", agentID, err)
			}
		} else {
			return nil, fmt.Errorf("sub-agent target agent %q: %w", agentID, err)
		}
	}

	// 4. Compute index.
	existingChildren, _ := m.registry.ListByParent(ctx, req.ParentSessionID)
	childIndex := len(existingChildren)

	// 5. Create sub-agent record.
	record := &entity.SubAgentRecord{
		ID:              uuid.New().String(),
		ParentSessionID: req.ParentSessionID,
		ParentRunID:     req.ParentRunID,
		AgentID:         agentID,
		Task:            req.Task,
		Label:           req.Label,
		Model:           req.Model,
		Cleanup:         req.EffectiveCleanup(),
		Index:           childIndex,
		SpawnDepth:      currentDepth + 1,
		Status:          entity.SubAgentStatusPending,
		CreatedAt:       time.Now(),
	}

	// 6. Create independent session for the sub-agent.
	subSession := &entity.Session{
		ID:              fmt.Sprintf("agent:%s:subagent:%s", agentID, uuid.New().String()),
		AgentID:         agentID,
		ParentSessionID: req.ParentSessionID,
		Messages:        make([]*entity.Message, 0),
		Metadata: map[string]string{
			"subagent_id":   record.ID,
			"subagent_task": req.Task,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	subSession.SetSpawnDepth(currentDepth + 1)

	if err := m.sessionRepo.Create(ctx, subSession); err != nil {
		return nil, fmt.Errorf("failed to create sub-agent session: %w", err)
	}
	record.SessionID = subSession.ID

	// 7. Persist record.
	if err := m.registry.Save(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to save sub-agent record: %w", err)
	}

	// 8. Schedule async execution.
	if err := m.scheduler.Submit(func(schedCtx context.Context) {
		// Create a cancellable context for this specific sub-agent.
		ctx, cancel := context.WithCancel(schedCtx)
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

		m.exec.execute(ctx, record)
	}); err != nil {
		record.MarkFailed("global concurrency limit reached")
		_ = m.registry.Save(ctx, record)
		return nil, errno.ErrConcurrencyLimit
	}

	// Emit observer events.
	m.emitter.Spawned(record.ID, record.SessionID, record.ParentSessionID, record.AgentID, "", record.SpawnDepth)
	m.emitter.Scheduled(record.ID, record.SessionID, record.AgentID)

	logger.Info("[subagent] spawned sub-agent %s (task=%q, agent=%s, session=%s, depth=%d, index=%d, parentSessionID=%s, label=%q)",
		record.ID, record.Task, record.AgentID, record.SessionID, record.SpawnDepth, record.Index, record.ParentSessionID, record.Label)

	return record, nil
}

// CountActiveByParent counts non-terminal sub-agents for a parent session.
func (m *managerImpl) CountActiveByParent(ctx context.Context, parentSessionID string) (int, error) {
	return countActiveFromRegistry(ctx, m.registry, parentSessionID), nil
}

// Cancel cancels a running sub-agent by record ID.
func (m *managerImpl) Cancel(ctx context.Context, recordID string) error {
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
	m.emitter.Cancelled(record.ID, record.AgentID, "")
	return m.registry.Save(ctx, record)
}

// CancelByParent cancels all running sub-agents for a parent session.
func (m *managerImpl) CancelByParent(ctx context.Context, parentSessionID string) error {
	records, err := m.registry.ListByParent(ctx, parentSessionID)
	if err != nil {
		return err
	}

	for _, record := range records {
		if !record.Status.IsTerminal() {
			if cancelErr := m.Cancel(ctx, record.ID); cancelErr != nil {
				logger.Warn("[subagent] failed to cancel sub-agent %s: %v",
					record.ID, cancelErr)
			}
		}
	}
	return nil
}

// Get retrieves a sub-agent record by ID.
func (m *managerImpl) Get(ctx context.Context, recordID string) (*entity.SubAgentRecord, error) {
	return m.registry.Get(ctx, recordID)
}

// FindByIdentifier performs multi-dimensional fuzzy lookup.
func (m *managerImpl) FindByIdentifier(ctx context.Context, parentSessionID, identifier string) (*entity.SubAgentRecord, error) {
	return FindByIdentifier(ctx, m.registry, parentSessionID, identifier)
}

// ListByParent returns all sub-agent records spawned by a given parent session.
func (m *managerImpl) ListByParent(ctx context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error) {
	return m.registry.ListByParent(ctx, parentSessionID)
}

// Recover restores in-flight sub-agent records after process restart.
func (m *managerImpl) Recover(ctx context.Context) error {
	records, err := m.registry.ListNonTerminal(ctx)
	if err != nil {
		return fmt.Errorf("failed to list non-terminal records: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	logger.Info("[subagent] recovering %d in-flight sub-agent records", len(records))

	for _, record := range records {
		record.MarkFailed("process restarted while sub-agent was running")
		if err := m.registry.Save(ctx, record); err != nil {
			logger.Warn("[subagent] failed to save recovery state for %s: %v",
				record.ID, err)
		}

		if !record.Announced {
			ctx := context.Background()
			remainingActive := countActiveFromRegistry(ctx, m.registry, record.ParentSessionID)
			m.controller.EnqueueAnnouncement(ctx, record, remainingActive)
		}
	}

	return nil
}

// Cleanup removes completed/failed sub-agent records older than the retention period.
func (m *managerImpl) Cleanup(ctx context.Context) error {
	cutoff := time.Now().Add(-time.Duration(m.cfg.ArchiveAfterMinutes) * time.Minute)
	_ = cutoff // UNIMPLEMENTED: requires ListAll/ListTerminal on Registry
	logger.Debug("[subagent] cleanup pass completed (stub — not yet implemented)")
	return nil
}

// Stop gracefully shuts down the manager.
func (m *managerImpl) Stop(_ context.Context) error {
	m.controller.Stop()
	m.scheduler.Stop()
	return nil
}
