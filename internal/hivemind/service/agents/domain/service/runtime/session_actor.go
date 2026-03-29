package runtime

import (
	"sync"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// sessionActorPool manages per-session actors that serialize Run execution.
//
// This replaces sessionRunMutex with an Actor model aligned with OpenClaw's
// session lane pattern (session:<key> lane with maxConcurrent=1):
//
//   - OpenClaw: enqueueCommandInLane("session:<key>", task) → per-session FIFO queue
//   - Echoryn:  sessionActorPool.Submit(sessionID, task)   → per-session goroutine mailbox
//
// Each session gets a dedicated goroutine (actor) that processes tasks sequentially.
// Tasks are submitted via a buffered channel (mailbox). The actor goroutine drains
// the mailbox one task at a time, ensuring that:
//
//  1. Only one Run executes per session at a time (serialization)
//  2. No explicit Lock/Unlock or defer ordering is needed (deadlock-free)
//  3. Different sessions run fully concurrently (per-session granularity)
//
// Lifecycle: actors are created lazily on first Submit and cleaned up when idle
// (no pending tasks and no in-flight task). The cleanup is handled by the actor
// goroutine itself — after draining the mailbox, it attempts to remove itself
// from the pool under a lock. If a new task was enqueued between the drain check
// and the lock acquisition, the actor continues processing instead of exiting.
//
// Why not sync.Mutex? The mutex approach requires careful defer ordering to avoid
// deadlocks (e.g., MarkRunIdle must execute after sessionRelease). The actor model
// eliminates this class of bugs entirely — tasks are just functions in a FIFO queue.
type sessionActorPool struct {
	mu     sync.Mutex
	actors map[string]*sessionActor
}

// sessionActor is a single-goroutine mailbox for one session.
//
// Aligned with OpenClaw's per-session lane (maxConcurrent=1):
//   - mailbox: buffered channel acting as the task queue
//   - The goroutine loops: receive task → execute → receive next
//   - When mailbox is empty and drained, the goroutine exits and the actor
//     is removed from the pool (lazy cleanup)
type sessionActor struct {
	sessionID string
	mailbox   chan func()
	pool      *sessionActorPool
}

// mailboxCapacity is the buffer size for per-session task queues.
// 16 is generous — in practice, a session rarely has more than 2-3 concurrent
// Run requests (user message + trigger from subagent completion).
const mailboxCapacity = 16

// newSessionActorPool creates a new actor pool.
func newSessionActorPool() *sessionActorPool {
	return &sessionActorPool{
		actors: make(map[string]*sessionActor),
	}
}

// Submit enqueues a task for serial execution on the given session's actor.
//
// If no actor exists for the session, one is created and its goroutine is started.
// The task will execute after all previously submitted tasks for this session complete.
//
// This is the Go equivalent of OpenClaw's:
//
//	enqueueCommandInLane("session:<key>", task)
//
// The task function receives no arguments — all context should be captured in the closure.
// Tasks MUST NOT panic; if they do, the actor goroutine will crash and the session's
// mailbox will be permanently stuck. Use safego.Recovery inside the task if needed.
func (p *sessionActorPool) Submit(sessionID string, task func()) {
	p.mu.Lock()
	actor, exists := p.actors[sessionID]
	if !exists {
		actor = &sessionActor{
			sessionID: sessionID,
			mailbox:   make(chan func(), mailboxCapacity),
			pool:      p,
		}
		p.actors[sessionID] = actor
		go actor.run()
	}
	p.mu.Unlock()

	actor.mailbox <- task
}

// run is the actor's main loop. It processes tasks from the mailbox sequentially.
//
// When the mailbox is empty, the actor attempts to clean itself up from the pool.
// If a new task arrives between the empty check and the cleanup lock, the actor
// continues processing (prevents lost tasks).
//
// This is the Go equivalent of OpenClaw's command-queue drainLane pump() function.
func (a *sessionActor) run() {
	for {
		select {
		case task := <-a.mailbox:
			a.executeTask(task)
		default:
			// Mailbox empty — try to clean up.
			if a.tryCleanup() {
				return // Actor exited.
			}
			// Cleanup failed (new task arrived) — continue processing.
		}
	}
}

// executeTask runs a single task with panic recovery.
func (a *sessionActor) executeTask(task func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("[SessionActor] panic in session %s task: %v", a.sessionID, r)
		}
	}()
	task()
}

// tryCleanup attempts to remove this actor from the pool when idle.
//
// Returns true if the actor was removed and should exit its goroutine.
// Returns false if a new task arrived and the actor should continue.
//
// The double-check pattern (check mailbox under lock) prevents a race where:
//   - Actor sees empty mailbox (select default)
//   - Submit() adds a task to mailbox and finds actor already exists (no new goroutine)
//   - Actor removes itself from pool
//   - Task sits in orphaned mailbox forever
//
// By checking len(mailbox) under the pool lock, we ensure Submit() and tryCleanup()
// are mutually exclusive — if Submit() ran between our default case and the lock,
// len(mailbox) > 0 and we continue processing.
func (a *sessionActor) tryCleanup() bool {
	a.pool.mu.Lock()
	defer a.pool.mu.Unlock()

	// Double-check: new task may have been submitted between the select-default
	// and acquiring this lock.
	if len(a.mailbox) > 0 {
		return false // New task arrived, keep running.
	}

	// No pending tasks — remove actor from pool.
	delete(a.pool.actors, a.sessionID)
	logger.Debug("[SessionActor] actor for session %s exited (idle)", a.sessionID)
	return true
}
