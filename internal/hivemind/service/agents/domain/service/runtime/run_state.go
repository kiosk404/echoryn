package runtime

import (
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// RunStateMachine manages the lifecycle state transitions of a run.
// State machine: Created -> InProgress -> Completed | Failed | Cancelled
// This is the Echoryn equivalent
type RunStateMachine struct {
	run     *entity.Run
	runRepo repo.RunRepository
}

// NewRunStateMachine creates a new RunStateMachine for the given run.
func NewRunStateMachine(run *entity.Run, runRepo repo.RunRepository) *RunStateMachine {
	return &RunStateMachine{
		run:     run,
		runRepo: runRepo,
	}
}

// TransitionToInProgress transitions the run to the InProgress state.
func (sm *RunStateMachine) TransitionToInProgress() error {
	if sm.run.Status != entity.RunStatusCreated {
		return errno.ErrRunAlreadyDone
	}
	sm.run.Status = entity.RunStatusInProgress
	logger.InfoX(pkg.ModuleName, "[RunState] run %s -> in_progress", "runID", sm.run.ID)
	return nil
}

// TransitionToCompleted transitions the run to the Completed state.
func (sm *RunStateMachine) TransitionToCompleted(output string, usage *entity.TokenUsage) error {
	now := time.Now()
	sm.run.CompletedAt = &now
	sm.run.Status = entity.RunStatusCompleted
	sm.run.Output = output
	sm.run.Usage = usage
	logger.InfoX(pkg.ModuleName, "[RunState] run %s -> completed", "runID", sm.run.ID)
	return nil
}

// TransitionToFailed transitions the run to the Failed state.
func (sm *RunStateMachine) TransitionToFailed(code, message string) {
	now := time.Now()
	sm.run.CompletedAt = &now
	sm.run.Status = entity.RunStatusFailed
	sm.run.Error = &entity.RunError{Code: code, Message: message}
	logger.ErrorX(pkg.ModuleName, "[RunState] run %s -> failed, err: %v", "runID", sm.run.ID, sm.run.Error)
}

// TransitionToCancelled transitions the run to the Cancelled state.
func (sm *RunStateMachine) TransitionToCancelled() {
	now := time.Now()
	sm.run.CompletedAt = &now
	sm.run.Status = entity.RunStatusCancelled
	logger.InfoX(pkg.ModuleName, "[RunState] run %s -> cancelled", "runID", sm.run.ID)
}

// Run returns the current run.
func (sm *RunStateMachine) Run() *entity.Run {
	return sm.run
}
