package subagent

import (
	"context"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/pkg/logger"
	"golang.org/x/sync/semaphore"
)

// Scheduler controls sub-agent execution concurrency.
//
// Modeled after K8S scheduler's parallelism control and OpenClaw's
// CommandLane + maxConcurrent mechanism.
//
// Key behaviors:
//   - Weighted semaphore limits concurrent sub-agent goroutines
//   - WaitGroup tracks all in-flight goroutines for graceful shutdown
//   - Context cancellation propagates to all running sub-agents
type Scheduler struct {
	sem    *semaphore.Weighted
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	max    int64
}

// NewScheduler creates a scheduler with the given concurrency limit.
func NewScheduler(maxConcurrent int) *Scheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrent
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		sem:    semaphore.NewWeighted(int64(maxConcurrent)),
		ctx:    ctx,
		cancel: cancel,
		max:    int64(maxConcurrent),
	}
}

// Submit acquires a semaphore slot and runs fn in a new goroutine.
// Returns ErrConcurrencyLimit if the semaphore cannot be acquired immediately
// (non-blocking TryAcquire).
//
// The fn receives a child context that is cancelled when the scheduler stops.
func (s *Scheduler) Submit(fn func(ctx context.Context)) error {
	if !s.sem.TryAcquire(1) {
		return errno.ErrConcurrencyLimit
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.sem.Release(1)
		fn(s.ctx)
	}()

	return nil
}

// Stop cancels all running sub-agents and waits for them to finish.
func (s *Scheduler) Stop() {
	logger.Info("[subagent/Scheduler] stopping, cancelling all running sub-agents")
	s.cancel()
	s.wg.Wait()
	logger.Info("[subagent/Scheduler] all sub-agents stopped")
}
