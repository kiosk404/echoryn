package subagent

import (
	"context"
	"fmt"
	"io"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/subagent/observer"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// subAgentExecutor encapsulates the async execution body for a sub-agent.
// Separated from the Manager to keep the Manager lean (orchestration only)
// and make the execution logic independently testable.
//
// v2: Uses SessionController instead of Announcer. The controller's
// EnqueueAnnouncement replaces the old Announce() method.
type subAgentExecutor struct {
	executor      AgentExecutor
	registry      Registry
	sessionRepo   repo.SessionRepository
	controller    *SessionController
	cfg           Config
	lifecycleHook LifecycleHook // optional hook for Team integration.
	emitter       *observer.Emitter
}

// execute is the async execution body for a sub-agent.
//
// BUG FIXES:
//  1. Independent execution timeout (3min default) — prevents hang from unbounded runs
//  2. Reads output from session history instead of EventTextDelta stream — reliable
//  3. Graceful context cancellation handling
func (e *subAgentExecutor) execute(parentCtx context.Context, record *entity.SubAgentRecord) {
	// BUG FIX #1: Independent execution timeout.
	// Previously, SubAgentScheduler used context.Background() with no timeout,
	// so if the underlying Run() hung, executeSubAgent would block forever.
	ctx, cancel := context.WithTimeout(parentCtx, e.cfg.ExecutionTimeout)
	defer cancel()

	// Transition to running.
	record.MarkRunning()
	if err := e.registry.Save(ctx, record); err != nil {
		logger.Warn("[subagent] failed to save running state for %s: %v", record.ID, err)
	}
	e.emitter.Running(record.ID, record.SessionID, record.AgentID, "")

	// Build the sub-agent system prompt (minimal mode).
	subAgentPrompt := buildSystemPrompt(record.Task)

	// Run the agent via AgentExecutor.
	sr, err := e.executor.RunSubAgent(ctx, &ExecuteRequest{
		AgentID:   record.AgentID,
		SessionID: record.SessionID,
		Input:     subAgentPrompt,
	})
	if err != nil {
		record.MarkFailed(fmt.Sprintf("failed to start sub-agent run: %v", err))
		_ = e.registry.Save(context.Background(), record)
		logger.Warn("[subagent] sub-agent %s failed to start: %v", record.ID, err)
		e.emitter.Failed(record.ID, record.AgentID, "", 0, err.Error())
		e.announceResult(record)
		return
	}

	// Consume the stream to wait for completion.
	//
	// BUG FIX #2: We no longer accumulate EventTextDelta into lastAssistantContent.
	// Instead, after the stream is consumed, we read the final output from session
	// history (like OpenClaw's readLatestSubagentOutput). This is reliable because
	// the runner always persists the final assistant message via collectStreamResult → session.
	//
	// Event classification:
	//   - EventError:     informational (model fallback, transient retry, etc.) — NOT fatal
	//   - EventRunStatus: if RunStatusFailed → sub-agent truly failed
	//   - EventDone:      sub-agent completed successfully
	var lastError string
	var runFailed bool
	for {
		event, recvErr := sr.Recv()
		if recvErr != nil {
			if recvErr == io.EOF {
				break
			}
			record.MarkFailed(fmt.Sprintf("stream error: %v", recvErr))
			_ = e.registry.Save(context.Background(), record)
			logger.Warn("[subagent] sub-agent %s stream error: %v", record.ID, recvErr)
			e.emitter.StreamError(record.ID, record.AgentID, recvErr.Error())
			e.emitter.Failed(record.ID, record.AgentID, "", record.Duration(), recvErr.Error())
			e.announceResult(record)
			return
		}

		switch event.Type {
		case entity.EventError:
			// Intermediate error: log but do NOT treat as fatal.
			lastError = event.Error
			logger.Info("[subagent] sub-agent %s intermediate error: %s", record.ID, event.Error)

		case entity.EventRunStatus:
			if event.RunStatus == entity.RunStatusFailed {
				runFailed = true
				if event.Error != "" {
					lastError = event.Error
				}
			}

		case entity.EventDone:
			// Explicit success signal.
		}
	}

	// Determine final outcome after stream is fully consumed.
	if runFailed {
		errMsg := lastError
		if errMsg == "" {
			errMsg = "sub-agent run failed (unknown reason)"
		}
		record.MarkFailed(errMsg)
		_ = e.registry.Save(context.Background(), record)
		logger.Warn("[subagent] sub-agent %s failed: %s", record.ID, errMsg)
		e.emitter.Failed(record.ID, record.AgentID, "", record.Duration(), errMsg)
		e.announceResult(record)
		return
	}

	// BUG FIX #2: Read output from session history (reliable path).
	// The runner's executeRun always persists the final assistant message to session,
	// so this is guaranteed to have the complete output even if EventTextDelta
	// events were lost due to timing/buffering issues.
	assistantContent := e.readSubAgentOutput(record.SessionID)

	// Mark completed.
	record.MarkCompleted(assistantContent)
	if err := e.registry.Save(context.Background(), record); err != nil {
		logger.Warn("[subagent] failed to save completed record %s: %v", record.ID, err)
	}

	logger.Info("[subagent] sub-agent %s completed (duration=%s, result_len=%d)",
		record.ID, record.Duration(), len(assistantContent))
	e.emitter.Completed(record.ID, record.AgentID, "", record.Duration())

	// Announce result to parent session.
	e.announceResult(record)

	// Cleanup session if configured.
	if record.Cleanup == "delete" {
		_ = e.sessionRepo.Delete(context.Background(), record.SessionID)
	}
}

// readSubAgentOutput reads the sub-agent's final output from session history.
//
// BUG FIX: Replaces the previous EventTextDelta accumulation pattern which was
// unreliable (events could be lost due to callback timing, goroutine scheduling,
// or Pipe buffer saturation). Session history is always written by the runner's
// executeRun path (collectStreamResult → session.AppendMessage), making this
// the authoritative source for sub-agent output.
//
// Aligned with OpenClaw's readLatestSubagentOutput in subagent-announce.ts.
func (e *subAgentExecutor) readSubAgentOutput(sessionID string) string {
	session, err := e.sessionRepo.Get(context.Background(), sessionID)
	if err != nil {
		logger.Warn("[subagent] readSubAgentOutput: failed to get session %s: %v", sessionID, err)
		return ""
	}

	// Walk messages in reverse to find the last assistant message.
	for i := len(session.Messages) - 1; i >= 0; i-- {
		msg := session.Messages[i]
		if msg.Role == "assistant" && msg.Content != "" {
			return msg.Content
		}
	}

	logger.Warn("[subagent] readSubAgentOutput: no assistant message found in session %s (messages=%d)",
		sessionID, len(session.Messages))
	return ""
}

// announceResult delivers the sub-agent's result to the parent session.
// Uses the SessionController which handles steer/pending delivery paths.
//
// v2: Replaces the old Announcer.Announce() with SessionController.EnqueueAnnouncement().
// The key difference: pending announcements are NOT written directly to the session.
// Instead, they are stored in memory and consumed by the runner within its own
// session write path, ensuring single-writer semantics.
//
// Bubble-up (grandparent propagation): if the parent session is itself a
// sub-agent session that has already reached a terminal state, the result
// is propagated upward to the grandparent session.
func (e *subAgentExecutor) announceResult(record *entity.SubAgentRecord) {
	ctx := context.Background()

	// Notify lifecycle hook (if registered) - enables Team EventBridge integration.
	// Called before announcement so that the Team system can update member status
	// before the parent session processes the result.
	if e.lifecycleHook != nil {
		e.lifecycleHook.OnSubAgentTerminal(ctx, record)
	}

	// Count remaining active runs for the same parent.
	remainingActive := countActiveFromRegistry(ctx, e.registry, record.ParentSessionID)
	// Subtract 1 because the current record might still be counted.
	if remainingActive > 0 {
		remainingActive--
	}

	e.controller.EnqueueAnnouncement(ctx, record, remainingActive)
	e.emitter.Announced(record.ID, record.ParentSessionID)

	// Bubble-up: grandparent propagation.
	e.bubbleUpIfNeeded(ctx, record)
}

// bubbleUpIfNeeded checks if the announcement needs to propagate further up
// the session hierarchy. It recursively walks up parent sessions, checking if
// each one is a sub-agent session with no more active children.
//
// Guard: max 3 levels of bubbling to prevent infinite loops from orphaned records.
func (e *subAgentExecutor) bubbleUpIfNeeded(ctx context.Context, record *entity.SubAgentRecord) {
	const maxBubbleDepth = 3

	parentSessionID := record.ParentSessionID
	for depth := 0; depth < maxBubbleDepth; depth++ {
		parentSession, err := e.sessionRepo.Get(ctx, parentSessionID)
		if err != nil {
			break
		}

		// Not a sub-agent session → stop bubbling.
		if !parentSession.IsSubAgentSession() {
			break
		}

		grandparentSessionID := parentSession.ParentSessionID

		// Check if the parent session's own sub-agent record has reached terminal state.
		subagentRecordID := parentSession.Metadata["subagent_id"]
		if subagentRecordID == "" {
			break
		}

		parentRecord, err := e.registry.Get(ctx, subagentRecordID)
		if err != nil {
			break
		}

		// Parent record not terminal yet → it will announce itself when done.
		if !parentRecord.Status.IsTerminal() {
			break
		}

		// Parent record already announced → stop.
		if parentRecord.Announced {
			break
		}

		// Count remaining active siblings at the grandparent level.
		grandparentRemaining := countActiveFromRegistry(ctx, e.registry, grandparentSessionID)

		// Announce the parent record to the grandparent.
		e.controller.EnqueueAnnouncement(ctx, parentRecord, grandparentRemaining)

		logger.Info("[subagent] bubble-up: announced record %s to grandparent %s (depth=%d)",
			parentRecord.ID, grandparentSessionID, depth+1)

		// Continue bubbling up.
		parentSessionID = grandparentSessionID
	}
}

// countActiveFromRegistry counts non-terminal sub-agents for a parent session.
func countActiveFromRegistry(ctx context.Context, registry Registry, parentSessionID string) int {
	records, err := registry.ListByParent(ctx, parentSessionID)
	if err != nil {
		return 0
	}
	count := 0
	for _, r := range records {
		if !r.Status.IsTerminal() {
			count++
		}
	}
	return count
}
