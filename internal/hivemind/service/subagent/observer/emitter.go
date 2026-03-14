package observer

import "time"

// Emitter is a convenience wrapper around Observer that provides typed emit
// methods for each event kind. It is nil-safe: if the underlying Observer is
// nil, all methods are no-ops.
//
// Usage:
//
//	emitter := observer.NewEmitter(obs)
//	emitter.Spawned(recordID, sessionID, parentID, agentID, teamID, spawnDepth)
//	emitter.Completed(recordID, agentID, teamID, duration)
//
// This avoids repetitive Event{} construction at each call site.
type Emitter struct {
	obs Observer
}

// NewEmitter creates an Emitter wrapping the given Observer.
// If obs is nil, the Emitter is a no-op (nil-safe).
func NewEmitter(obs Observer) *Emitter {
	return &Emitter{obs: obs}
}

// IsActive returns true if the underlying Observer is non-nil.
func (e *Emitter) IsActive() bool {
	return e != nil && e.obs != nil
}

// Spawned emits an EventSpawned event.
func (e *Emitter) Spawned(recordID, sessionID, parentSessionID, agentID, teamID string, spawnDepth int) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:            EventSpawned,
		Timestamp:       time.Now(),
		RecordID:        recordID,
		SessionID:       sessionID,
		ParentSessionID: parentSessionID,
		AgentID:         agentID,
		TeamID:          teamID,
		SpawnDepth:      spawnDepth,
	})
}

// Scheduled emits an EventScheduled event.
func (e *Emitter) Scheduled(recordID, sessionID, agentID string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventScheduled,
		Timestamp: time.Now(),
		RecordID:  recordID,
		SessionID: sessionID,
		AgentID:   agentID,
	})
}

// Running emits an EventRunning event.
func (e *Emitter) Running(recordID, sessionID, agentID, teamID string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventRunning,
		Timestamp: time.Now(),
		RecordID:  recordID,
		SessionID: sessionID,
		AgentID:   agentID,
		TeamID:    teamID,
	})
}

// Completed emits an EventCompleted event.
func (e *Emitter) Completed(recordID, agentID, teamID string, duration time.Duration) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventCompleted,
		Timestamp: time.Now(),
		RecordID:  recordID,
		AgentID:   agentID,
		TeamID:    teamID,
		Duration:  duration,
	})
}

// Failed emits an EventFailed event.
func (e *Emitter) Failed(recordID, agentID, teamID string, duration time.Duration, errMsg string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventFailed,
		Timestamp: time.Now(),
		RecordID:  recordID,
		AgentID:   agentID,
		TeamID:    teamID,
		Duration:  duration,
		Error:     errMsg,
	})
}

// Cancelled emits an EventCancelled event.
func (e *Emitter) Cancelled(recordID, agentID, teamID string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventCancelled,
		Timestamp: time.Now(),
		RecordID:  recordID,
		AgentID:   agentID,
		TeamID:    teamID,
	})
}

// Timeout emits an EventTimeout event.
func (e *Emitter) Timeout(recordID, agentID, teamID string, duration time.Duration) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventTimeout,
		Timestamp: time.Now(),
		RecordID:  recordID,
		AgentID:   agentID,
		TeamID:    teamID,
		Duration:  duration,
	})
}

// Routed emits an EventRouted event.
func (e *Emitter) Routed(recordID, sessionID string, location ExecutionLocation, strategy, nodeID, teamID string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventRouted,
		Timestamp: time.Now(),
		RecordID:  recordID,
		SessionID: sessionID,
		Location:  location,
		Strategy:  strategy,
		NodeID:    nodeID,
		TeamID:    teamID,
	})
}

// Fallback emits an EventFallback event.
func (e *Emitter) Fallback(recordID, sessionID, strategy, errMsg string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventFallback,
		Timestamp: time.Now(),
		RecordID:  recordID,
		SessionID: sessionID,
		Strategy:  strategy,
		Error:     errMsg,
	})
}

// StreamError emits an EventStreamError event.
func (e *Emitter) StreamError(recordID, agentID, errMsg string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:      EventStreamError,
		Timestamp: time.Now(),
		RecordID:  recordID,
		AgentID:   agentID,
		Error:     errMsg,
	})
}

// Announced emits an EventAnnounced event.
func (e *Emitter) Announced(recordID, parentSessionID string) {
	if !e.IsActive() {
		return
	}
	e.obs.Emit(Event{
		Kind:            EventAnnounced,
		Timestamp:       time.Now(),
		RecordID:        recordID,
		ParentSessionID: parentSessionID,
	})
}
