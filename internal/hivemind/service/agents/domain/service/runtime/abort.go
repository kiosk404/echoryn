package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// AbortController manages run cancellation and timeout.
//
// It wraps context.WithCancel to provide a way to cancel a run.
// - Explicit Abort() for external cancellation
// - Timeout for automatic cancellation after a specified duration
// - Thread-safe abort state tracking
type AbortController struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	down   bool
	runID  string
}

// NewAbortController creates a new AbortController.
//
// It takes a parent context, a runID, and an optional timeout duration.
// If the timeout is greater than 0, the context will be canceled after the timeout.
// Otherwise, the context will be canceled when Abort() is called.
func NewAbortController(parent context.Context, runID string, timeout time.Duration) *AbortController {
	ctx, cancel := context.WithCancel(parent)

	if timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, timeout)
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	return &AbortController{
		ctx:    ctx,
		cancel: cancel,
		runID:  runID,
	}
}

// Context returns the controlled context.
// Use this context for all downstream operations.
func (ac *AbortController) Context() context.Context {
	return ac.ctx
}

// Abort cancels the run and marks it as aborted.
//
// It is safe to call Abort multiple times.
func (ac *AbortController) Abort() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.down {
		return
	}
	ac.down = true
	ac.cancel()
	logger.Info("[AbortController] Abort run %s", ac.runID)
}

// IsAborted returns true if the run is aborted.
func (ac *AbortController) IsAborted() bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.down {
		return true
	}
	select {
	case <-ac.ctx.Done():
		return true
	default:
		return false
	}
}

// CheckAborted returns errno.ErrAborted if the run is aborted.
func (ac *AbortController) CheckAborted() error {
	if ac.IsAborted() {
		return errno.ErrAborted
	}
	return nil
}

// CleanUp cancels the run and marks it as aborted.
func (ac *AbortController) CleanUp() {
	ac.cancel()
}
