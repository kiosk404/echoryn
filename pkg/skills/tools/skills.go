package tools

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/kiosk404/echoryn/pkg/skills"
)

// NewSkillTools creates all skill-related tools for an agent.
// Returns a slice of tools that can be added to the agent's tool list
func NewSkillTools(registry *skills.Registry) []tool.BaseTool {
	return []tool.BaseTool{
		NewListSkillsTool(registry),
		NewViewSkillTool(registry),
	}
}

// ToolNames returns the names of all skill-related tools.
func ToolNames() []string {
	return []string{"view_skill", "list_skills"}
}
