package subagent

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// toolDenyList defines tools that sub-agents are NOT allowed to use.
//
// Modeled after OpenClaw's DEFAULT_SUBAGENT_TOOL_DENY:
//   - sessions_spawn: prevent recursive sub-agent spawning
//   - sessions_status: sub-agents don't need orchestration awareness
//   - sessions_list: sub-agents don't need session management
//   - sessions_send: sub-agents don't send cross-session messages
//   - memory_search/read/write/delete: sub-agents must not access long-term memory
//
// K8S equivalent: PodSecurityPolicy / RBAC restricting container capabilities.
var toolDenyList = map[string]bool{
	"sessions_spawn":  true,
	"sessions_status": true,
	"sessions_list":   true,
	"sessions_send":   true,
	"memory_search":   true,
	"memory_read":     true,
	"memory_write":    true,
	"memory_delete":   true,
}

// FilterDeniedTools removes denied tools from the tool list for sub-agent runs.
// Exported so that the runner can call it during tool preparation.
func FilterDeniedTools(tools []tool.BaseTool) []tool.BaseTool {
	var filtered []tool.BaseTool
	var denied []string

	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			filtered = append(filtered, t)
			continue
		}
		if toolDenyList[info.Name] {
			denied = append(denied, info.Name)
			continue
		}
		filtered = append(filtered, t)
	}

	if len(denied) > 0 {
		logger.Debug("[subagent] filtered %d denied tools: %v", len(denied), denied)
	}

	return filtered
}
