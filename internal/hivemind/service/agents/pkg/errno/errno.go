package errno

import (
	"errors"
)

var (
	ErrAgentNotFound       = errors.New("agent not found")
	ErrSessionNotFound     = errors.New("session not found")
	ErrRunNotFound         = errors.New("run not found")
	ErrRunAlreadyDone      = errors.New("run already done")
	ErrNoToolsAvailable    = errors.New("no tools available")
	ErrMaxTurnsExceeded    = errors.New("max turns exceeded")
	ErrAborted             = errors.New("run aborted")
	ErrContextOverflow     = errors.New("context overflow")
	ErrModelNotToolCapable = errors.New("model not tool capable")

	// SubAgent errors.
	ErrSubAgentNotFound    = errors.New("sub-agent record not found")
	ErrMaxDepthExceeded    = errors.New("maximum sub-agent spawn depth exceeded")
	ErrConcurrencyLimit    = errors.New("max concurrent sub-agents limit reached")
	ErrSubAgentAlreadyDone = errors.New("sub-agent already in terminal state")
	ErrParentSessionBuse   = errors.New("parent session is currently running an agent turn")
)
