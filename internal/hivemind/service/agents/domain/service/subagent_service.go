package service

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
)

// SubAgentManager is the interface for sub-agent orchestration.
// It is defined in the runtime package to avoid circular imports,
// and re-exported here for convenience.
type SubAgentManager = runtime.SubAgentManager
