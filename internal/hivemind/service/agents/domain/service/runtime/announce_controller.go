package runtime

import (
	"context"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// AnnounceController delivers sub-agent results back to parent sessions.
//
// Modeled after OpenClaw's subagent-announce.ts + announce-queue:
//   - When a sub-agent completes, the controller injects its result as a
//     user message into the parent session
//   - It then triggers a new agent run on the parent so the main agent
//     can summarize the result to the user
//
// Delivery strategies (following OpenClaw):
//   - Direct: parent is idle → trigger new run immediately
//   - Queue: parent is busy → enqueue, deliver when parent run ends
//
// K8S equivalent: Job Controller watching Pod completions.
type AnnounceController struct {
	runner      *AgentRunner
	registry    SubAgentRegistry
	sessionRepo repo.SessionRepository

	// pendingQueue holds announcements waiting for the parent to become idle.
	// Key: parentSessionID, Value: list of records to announce.
	mu           sync.Mutex
	pendingQueue map[string][]*entity.SubAgentRecord
}

// NewAnnounceController creates a new announcement controller.
func NewAnnounceController(
	runner *AgentRunner,
	registry SubAgentRegistry,
	sessionRepo repo.SessionRepository,
) *AnnounceController {
	return &AnnounceController{
		runner:       runner,
		registry:     registry,
		sessionRepo:  sessionRepo,
		pendingQueue: make(map[string][]*entity.SubAgentRecord),
	}
}

// Announce delivers a completed sub-agent's result to its parent session.
//
// It injects the announcement message into the parent session and triggers
// a new agent run so the main agent can summarize the result.
func (ac *AnnounceController) Announce(ctx context.Context, record *entity.SubAgentRecord) {
	if record == nil || !record.Status.IsTerminal() {
		return
	}

	parentSession, err := ac.sessionRepo.Get(ctx, record.ParentSessionID)
	if err != nil {
		logger.Warn("[AnnounceController] failed to get parent session %s: %v",
			record.ParentSessionID, err)
		return
	}

	// Build and inject announcement message.
	announcement := record.FormatAnnouncement()
	parentSession.AppendMessage(entity.NewUserMessage(announcement))
	if err := ac.sessionRepo.Update(ctx, parentSession); err != nil {
		logger.Warn("[AnnounceController] failed to update parent session %s: %v",
			record.ParentSessionID, err)
		return
	}

	// Mark as announced.
	record.Announced = true
	if err := ac.registry.Save(ctx, record); err != nil {
		logger.Warn("[AnnounceController] failed to save announced record %s: %v",
			record.ID, err)
	}

	// Trigger a new run on the parent agent to process the announcement.
	sr, err := ac.runner.Run(ctx, &RunRequest{
		AgentID:   parentSession.AgentID,
		SessionID: parentSession.ID,
		Input:     announcement,
	})
	if err != nil {
		logger.Warn("[AnnounceController] failed to trigger parent run for session %s: %v",
			parentSession.ID, err)
		return
	}

	// Drain the stream (we don't need to consume the events here,
	// the client will pick them up via SSE/WebSocket in the future).
	go func() {
		for {
			_, err := sr.Recv()
			if err != nil {
				break
			}
		}
	}()

	logger.Info("[AnnounceController] announced sub-agent %s result to parent session %s",
		record.ID, record.ParentSessionID)
}

// Enqueue adds a sub-agent record to the pending queue for later delivery.
// Used when the parent session is currently busy.
func (ac *AnnounceController) Enqueue(record *entity.SubAgentRecord) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.pendingQueue[record.ParentSessionID] = append(
		ac.pendingQueue[record.ParentSessionID], record)
	logger.Debug("[AnnounceController] enqueued sub-agent %s for parent %s",
		record.ID, record.ParentSessionID)
}

// DrainPending delivers all pending announcements for the given parent session.
// Called when a parent agent run ends (via agent_end hook).
func (ac *AnnounceController) DrainPending(ctx context.Context, parentSessionID string) {
	ac.mu.Lock()
	pending := ac.pendingQueue[parentSessionID]
	delete(ac.pendingQueue, parentSessionID)
	ac.mu.Unlock()

	for _, record := range pending {
		ac.Announce(ctx, record)
	}
}
