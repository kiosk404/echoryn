package subagent

import (
	"context"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/workqueue"
)

// SessionController manages the lifecycle of sub-agent announcements
// using a K8s-style controller pattern with a workqueue.
//
// Architecture (replacing the previous Announcer with its steer/queue/direct
// ad-hoc dispatch and dual-lock session writes):
//
//	Producer side:
//	  SubAgentExecutor.announceResult() → controller.EnqueueAnnouncement()
//	  Runner.MarkRunIdleAndDrain()      → controller.ProcessPending() [sync, in-band]
//
//	Consumer side:
//	  Runner.executeRun() end           → controller.ProcessPending()
//	    → reads pending announcements for the session
//	    → appends system messages to the SAME session object the runner holds
//	    → triggers new agent turn if needed (all within runner's session lock)
//
// Key invariant: session writes are NEVER performed outside the runner's
// executeRun goroutine. The controller only stores pending announcements;
// the runner consumes and applies them. This eliminates the dual-lock
// problem (sessionRunLock vs writeLock) that caused lost updates.
//
// Steer path is preserved: when the parent run is active, announcements
// are injected via the steer channel for lowest latency. The pending
// path is used only when steer is not available (channel full or no
// active run).
type SessionController struct {
	registry    Registry
	sessionRepo repo.SessionRepository

	// executor is used for triggering new parent turns after pending delivery.
	// Set via SetExecutor after construction (circular dependency break).
	executor AgentExecutor

	// --- Active Run Tracking ---
	// Tracks which parent sessions currently have an active agent run.
	// When active, announcements use the steer channel for real-time injection.
	runMu      sync.RWMutex
	activeRuns map[string]*SteerChannel

	// --- Pending Announcements ---
	// Stores announcements that could not be delivered via steer.
	// Key: parentSessionID. Consumed by the runner when a run completes.
	//
	// This replaces the old Announcer's pendingQueue + deliverDirect pattern.
	// Critical difference: pending announcements are NOT written to the session
	// by the controller. The runner reads them and writes to its own session
	// object, ensuring single-writer semantics.
	pendingMu sync.Mutex
	pending   map[string][]*PendingAnnouncement

	// --- Trigger Queue ---
	// Workqueue for sessions that need a new agent turn triggered.
	// Dedup ensures that even if multiple sub-agents complete simultaneously,
	// only one trigger per session is processed at a time.
	triggerQueue workqueue.Interface[string]
}

// PendingAnnouncement wraps a formatted announcement event for deferred delivery.
type PendingAnnouncement struct {
	Record *entity.SubAgentRecord
	Event  *entity.TaskCompletionEvent
}

// NewSessionController creates a new session controller.
//
// This replaces the previous NewAnnouncer + SessionWriteLock pattern.
func NewSessionController(
	registry Registry,
	sessionRepo repo.SessionRepository,
) *SessionController {
	return &SessionController{
		registry:     registry,
		sessionRepo:  sessionRepo,
		activeRuns:   make(map[string]*SteerChannel),
		pending:      make(map[string][]*PendingAnnouncement),
		triggerQueue: workqueue.New[string](),
	}
}

// SetExecutor sets the AgentExecutor for triggering parent turns.
// Must be called after AgentRunner is fully wired (circular dependency break).
func (c *SessionController) SetExecutor(executor AgentExecutor) {
	c.executor = executor
}

// Stop shuts down the controller's trigger queue.
func (c *SessionController) Stop() {
	c.triggerQueue.ShutDown()
}

// --- Producer API (called by SubAgentExecutor) ---

// EnqueueAnnouncement records a sub-agent completion and determines the
// delivery path. This is the single entry point for all sub-agent results.
//
// Delivery paths:
//   - Steer: parent has active run → inject via steer channel (non-blocking)
//   - Pending: steer unavailable → store for later delivery by runner
//
// The "direct" path from the old Announcer is eliminated. Instead, pending
// announcements trigger a new agent turn via the triggerQueue, which is
// processed by the trigger worker (a single goroutine that serializes
// TriggerParentTurn calls per session).
func (c *SessionController) EnqueueAnnouncement(
	ctx context.Context,
	record *entity.SubAgentRecord,
	remainingActive int,
) AnnounceDeliveryMode {
	if record == nil || !record.Status.IsTerminal() {
		return ""
	}

	// Already announced — idempotency guard.
	if record.Announced {
		logger.Debug("[SessionController] record %s already announced, skipping", record.ID)
		return ""
	}

	// Build structured event.
	event := entity.NewTaskCompletionEvent(record, remainingActive)

	// Try steer first.
	c.runMu.RLock()
	steer, parentBusy := c.activeRuns[record.ParentSessionID]
	c.runMu.RUnlock()

	if parentBusy && steer != nil && steer.Ch != nil {
		select {
		case steer.Ch <- event.FormatForPrompt():
			c.markAnnounced(ctx, record)
			return AnnounceDeliverySteer
		default:
			// Steer channel full — fall through to pending.
		}
	}

	// Pending path: store for later delivery.
	c.pendingMu.Lock()
	c.pending[record.ParentSessionID] = append(
		c.pending[record.ParentSessionID],
		&PendingAnnouncement{Record: record, Event: event},
	)
	c.pendingMu.Unlock()

	c.markAnnounced(ctx, record)

	logger.Info("[SessionController] enqueued announcement for sub-agent %s to parent %s (remaining=%d, parentBusy=%v)",
		record.ID, record.ParentSessionID, remainingActive, parentBusy)

	// If parent is idle, enqueue a trigger.
	if !parentBusy {
		c.triggerQueue.Add(record.ParentSessionID)
	}

	return AnnounceDeliveryQueue
}

// markAnnounced marks a record as announced and persists it.
func (c *SessionController) markAnnounced(ctx context.Context, record *entity.SubAgentRecord) {
	record.Announced = true
	if err := c.registry.Save(ctx, record); err != nil {
		logger.Warn("[SessionController] failed to save announced record %s: %v", record.ID, err)
	}
}

// --- Active Run Tracking (called by Runner) ---

// MarkRunActive registers a session as having an active agent run and creates
// a steer channel for real-time sub-agent announcement injection.
func (c *SessionController) MarkRunActive(sessionID string) *SteerChannel {
	sc := &SteerChannel{
		Ch: make(chan string, steerChannelBufferSize),
	}
	c.runMu.Lock()
	c.activeRuns[sessionID] = sc
	c.runMu.Unlock()
	logger.Debug("[SessionController] session %s marked as active (steer ready)", sessionID)
	return sc
}

// MarkRunIdle removes a session from the active runs map and closes the
// steer channel. Returns any pending announcements for the session.
//
// CRITICAL: Unlike the old MarkRunIdleAndDrain, this does NOT write to the
// session or trigger new turns. The caller (runner) is responsible for
// processing the returned pending announcements within its own session
// write path, ensuring single-writer semantics.
func (c *SessionController) MarkRunIdle(sessionID string) []*PendingAnnouncement {
	// Step 1: Remove from active runs.
	c.runMu.Lock()
	sc, existed := c.activeRuns[sessionID]
	delete(c.activeRuns, sessionID)
	c.runMu.Unlock()

	if existed && sc != nil && sc.Ch != nil {
		close(sc.Ch)
		// Drain any undelivered steer messages for logging.
		for range sc.Ch {
			// Messages that didn't get delivered via steer are already
			// in the pending queue (or were consumed by the LLM).
		}
	}

	// Step 2: Take all pending announcements.
	c.pendingMu.Lock()
	pending := c.pending[sessionID]
	delete(c.pending, sessionID)
	c.pendingMu.Unlock()

	if len(pending) > 0 {
		logger.Info("[SessionController] session %s idle, returning %d pending announcements",
			sessionID, len(pending))
	}

	return pending
}

// IsParentBusy checks if a parent session currently has an active run.
func (c *SessionController) IsParentBusy(sessionID string) bool {
	c.runMu.RLock()
	_, busy := c.activeRuns[sessionID]
	c.runMu.RUnlock()
	return busy
}

// PendingCount returns the number of queued announcements for a session.
func (c *SessionController) PendingCount(sessionID string) int {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	return len(c.pending[sessionID])
}

// TakePending atomically takes all pending announcements for a session.
// Returns nil if there are no pending announcements.
//
// This is called by the runner during its session persistence phase
// (before session.Update()) to consume announcements that arrived while
// the run was active but couldn't be delivered via steer.
//
// CRITICAL: The runner calls this while it still holds the session run lock
// and before session.Update(). This ensures that:
//  1. The pending announcements are appended to the runner's session object
//  2. They are persisted along with all other session changes in a single Update()
//  3. No other goroutine writes to the session (single-writer invariant)
func (c *SessionController) TakePending(sessionID string) []*PendingAnnouncement {
	c.pendingMu.Lock()
	pending := c.pending[sessionID]
	delete(c.pending, sessionID)
	c.pendingMu.Unlock()
	return pending
}

// ReEnqueuePending puts announcements back into the pending queue.
// This is used when MarkRunIdle returns pending announcements that arrived
// after the runner's session.Update() — they need to be re-queued so the
// next trigger worker run will process them.
func (c *SessionController) ReEnqueuePending(sessionID string, announcements []*PendingAnnouncement) {
	if len(announcements) == 0 {
		return
	}
	c.pendingMu.Lock()
	c.pending[sessionID] = append(c.pending[sessionID], announcements...)
	c.pendingMu.Unlock()

	// Trigger a new run to process these.
	c.triggerQueue.Add(sessionID)
}

// --- Trigger Worker (processes trigger queue) ---

// StartTriggerWorker starts a goroutine that processes the trigger queue.
// Each session in the queue gets a TriggerParentTurn call. The workqueue's
// dirty/processing dedup ensures that:
//   - Only one trigger per session is in-flight at a time
//   - If new announcements arrive while a trigger is running, the session
//     is re-enqueued (via Done → dirty re-add) for another turn
//
// This replaces the old `go TriggerParentTurn()` fire-and-forget pattern
// that had no dedup and no back-pressure.
func (c *SessionController) StartTriggerWorker(ctx context.Context) {
	go func() {
		logger.Info("[SessionController] trigger worker started")
		for {
			sessionID, shutdown := c.triggerQueue.Get()
			if shutdown {
				logger.Info("[SessionController] trigger worker stopped")
				return
			}

			c.processTrigger(ctx, sessionID)
			c.triggerQueue.Done(sessionID)
		}
	}()
}

// processTrigger handles a single trigger for a session.
// It checks if there are pending announcements and triggers a new agent turn.
func (c *SessionController) processTrigger(ctx context.Context, sessionID string) {
	// Check if there are still pending announcements.
	c.pendingMu.Lock()
	hasPending := len(c.pending[sessionID]) > 0
	c.pendingMu.Unlock()

	if !hasPending {
		logger.Debug("[SessionController] trigger for session %s: no pending announcements, skipping", sessionID)
		return
	}

	// Check if parent already has an active run.
	if c.IsParentBusy(sessionID) {
		logger.Debug("[SessionController] trigger for session %s: parent busy, will be processed on run completion", sessionID)
		return
	}

	if c.executor == nil {
		logger.Warn("[SessionController] trigger for session %s: no executor set", sessionID)
		return
	}

	logger.Info("[SessionController] triggering new agent turn for session %s", sessionID)
	c.executor.TriggerParentTurn(ctx, sessionID, "[subagent-announce-trigger] Sub-agent tasks completed")
}

// --- Helpers for the Runner to apply pending announcements ---

// FormatAnnouncementsAsMessages converts pending announcements into
// entity.Message objects (system role) that the runner can append to
// the session. This is a pure transformation — no session writes.
func FormatAnnouncementsAsMessages(announcements []*PendingAnnouncement) []*entity.Message {
	if len(announcements) == 0 {
		return nil
	}

	messages := make([]*entity.Message, 0, len(announcements))
	for _, a := range announcements {
		content := a.Event.FormatForPrompt()
		messages = append(messages, entity.NewSystemMessage(content))
	}
	return messages
}
