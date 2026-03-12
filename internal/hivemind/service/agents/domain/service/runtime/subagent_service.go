package runtime

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/subagent"
)

// SubAgentManager is the interface for subagent orchestration.
// It is defined in the subagent package and re-exported here for convenience.
type SubAgentManager = subagent.Manager
