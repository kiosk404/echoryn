// Package team provides EventBridge which connects SubAgent lifecycle events
// to the TeamOrchestrator's NotifyMemberCompleted callback.
package team

import (
	"context"
	"fmt"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// SubAgentEvent represents a lifecycle event from the SubAgent subsystem.
// This is the canonical event format that EventBridge consumes.
type SubAgentEvent struct {
	// RecordID is the SubAgentRecord ID.
	RecordID string

	// SessionID is the SubAgent's session ID.
	SessionID string

	// ParentSessionID is the parent that spawned this SubAgent.
	ParentSessionID string

	// Status is the terminal status (completed, failed, cancelled).
	Status string

	// Output is the SubAgent's final output or error message.
	Output string
}

// EventBridge watches SubAgent lifecycle events and forwards them to
// the TeamOrchestrator for team state management.
//
// It maintains a reverse index from SubAgent session IDs to team member IDs,
// registered when members are spawned via TeamOrchestrator.InstantiateTeam()
// or TeamOrchestrator.AddMember().
//
// Usage:
//
//	bridge := team.NewEventBridge(orchestrator, registry)
//	bridge.RegisterMember(teamID, memberID, sessionID)
//	// ... later, when SubAgent completes:
//	bridge.OnSubAgentCompleted(ctx, event)
type EventBridge struct {
	orchestrator TeamOrchestrator
	registry     TeamRegistry

	mu sync.RWMutex
	// sessionIndex maps SubAgent sessionID → {teamID, memberID}
	sessionIndex map[string]memberRef
}

// memberRef is a reverse reference from session to team member.
type memberRef struct {
	teamID   string
	memberID string
}

// NewEventBridge creates a new EventBridge.
func NewEventBridge(orchestrator TeamOrchestrator, registry TeamRegistry) *EventBridge {
	return &EventBridge{
		orchestrator: orchestrator,
		registry:     registry,
		sessionIndex: make(map[string]memberRef),
	}
}

// RegisterMember registers a SubAgent session as a team member.
// Called when a member is spawned during team instantiation.
func (b *EventBridge) RegisterMember(teamID, memberID, sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessionIndex[sessionID] = memberRef{
		teamID:   teamID,
		memberID: memberID,
	}
	logger.Debug("[EventBridge] registered member: session=%s → team=%s/member=%s", sessionID, teamID, memberID)
}

// UnregisterMember removes a member from the session index.
func (b *EventBridge) UnregisterMember(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessionIndex, sessionID)
}

// UnregisterTeam removes all members of a team from the session index.
func (b *EventBridge) UnregisterTeam(teamID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sid, ref := range b.sessionIndex {
		if ref.teamID == teamID {
			delete(b.sessionIndex, sid)
		}
	}
}

// OnSubAgentCompleted handles a SubAgent terminal event.
// If the SubAgent is a team member, it forwards the notification
// to TeamOrchestrator.NotifyMemberCompleted().
func (b *EventBridge) OnSubAgentCompleted(ctx context.Context, event SubAgentEvent) error {
	b.mu.RLock()
	ref, ok := b.sessionIndex[event.SessionID]
	b.mu.RUnlock()

	if !ok {
		// This SubAgent is not part of any team — ignore.
		return nil
	}

	// Map SubAgent status to TeamMemberStatus.
	var memberStatus TeamMemberStatus
	switch event.Status {
	case "completed":
		memberStatus = TeamMemberStatusCompleted
	case "failed", "cancelled":
		memberStatus = TeamMemberStatusFailed
	default:
		logger.Warn("[EventBridge] unexpected SubAgent status: %s (session=%s)", event.Status, event.SessionID)
		memberStatus = TeamMemberStatusFailed
	}

	logger.Info("[EventBridge] forwarding SubAgent event: session=%s → team=%s/member=%s status=%s",
		event.SessionID, ref.teamID, ref.memberID, memberStatus)

	if err := b.orchestrator.NotifyMemberCompleted(ctx, ref.teamID, ref.memberID, memberStatus, event.Output); err != nil {
		return fmt.Errorf("notify member completed: %w", err)
	}

	// Clean up the index entry.
	b.mu.Lock()
	delete(b.sessionIndex, event.SessionID)
	b.mu.Unlock()

	return nil
}

// HasTeamMember returns true if the session is registered as a team member.
func (b *EventBridge) HasTeamMember(sessionID string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.sessionIndex[sessionID]
	return ok
}

// GetTeamRef returns the team and member ID for a session, if registered.
func (b *EventBridge) GetTeamRef(sessionID string) (teamID, memberID string, ok bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ref, exists := b.sessionIndex[sessionID]
	if !exists {
		return "", "", false
	}
	return ref.teamID, ref.memberID, true
}

// OnSubAgentTerminal implements subagent.LifecycleHook.
// It is called by the SubAgent executor when any SubAgent reaches a terminal state.
// If the SubAgent is a team member, it converts the record into a SubAgentEvent
// and delegates to OnSubAgentCompleted.
//
// This is the actual bridge point that connects SubAgent lifecycle → Team system.
func (b *EventBridge) OnSubAgentTerminal(ctx context.Context, record *entity.SubAgentRecord) {
	// Quick check: is this SubAgent a team member?
	b.mu.RLock()
	_, isTeamMember := b.sessionIndex[record.SessionID]
	b.mu.RUnlock()

	if !isTeamMember {
		return // Not a team member, nothing to do.
	}

	// Convert SubAgentRecord to SubAgentEvent.
	event := SubAgentEvent{
		RecordID:        record.ID,
		SessionID:       record.SessionID,
		ParentSessionID: record.ParentSessionID,
		Status:          string(record.Status),
		Output:          record.Result,
	}
	if record.Status == entity.SubAgentStatusFailed {
		event.Output = record.Error
	}

	if err := b.OnSubAgentCompleted(ctx, event); err != nil {
		logger.Warn("[EventBridge] OnSubAgentTerminal failed: %v", err)
	}
}
