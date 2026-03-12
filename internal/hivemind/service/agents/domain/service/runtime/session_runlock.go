package runtime

import "sync"

// sessionRunMutex provides per-session mutual exclusion for Run execution.
//
// With this mutex, runs on the same session are serialized. The second run must
// wait for the first to complete (including session.Update()), then loads the
// up-to-date session with all messages from the first run.
//
// Design: identical pattern to subagent.SessionWriteLock but scoped to the
// runner package. Per-session granularity ensures that runs on different sessions
// are fully concurrent — only same-session runs are serialized.
//
// Aligned with OpenClaw's implicit run serialization through the gateway queue.
type sessionRunMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// newSessionRunMutex creates a new sessionRunMutex.
func newSessionRunMutex() *sessionRunMutex {
	return &sessionRunMutex{
		locks: make(map[string]*sync.Mutex),
	}
}

// Acquire returns a release function for the per-session lock.
// The caller MUST call the returned function to release the lock.
//
// Thread-safe: the internal map is protected by a top-level mutex.
// Individual session locks are independent — different sessions don't block each other.
func (m *sessionRunMutex) Acquire(sessionID string) func() {
	m.mu.Lock()
	lock, ok := m.locks[sessionID]
	if !ok {
		lock = &sync.Mutex{}
		m.locks[sessionID] = lock
	}
	m.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}
