package plugin

import "strings"

// DefaultProfiles defines the built-in tool profile presets.
// Aligned with OpenClaw's minimal/coding/messaging/full profiles.
//
// Profiles control which tools are sent to the LLM at the start of a turn.
// An empty slice means "no tools"; a nil/missing key means "allow all".
//
// To add a custom profile, register it via PluginsOptions.Tools.Profiles.
var DefaultProfiles = map[string][]string{
	// minimal: just tool_search for discovery (extreme minimalism)
	"minimal": {"tool_search"},

	// coding: web + memory + subagent for typical coding assistant scenarios
	"coding": {
		"tool_search",
		"web_search", "web_fetch",
		"search_memory", "update_memory",
		"sessions_spawn", "sessions_status",
	},

	// full: empty = all tools allowed (no profile filtering)
	"full": nil,

	// golem: shell + file ops + skill execution for Golem worker nodes
	"golem": {
		"tool_search",
	},

	// team: coding profile + team collaboration tools
	"team": {
		"tool_search",
		"web_search", "web_fetch",
		"search_memory", "update_memory",
		"sessions_spawn", "sessions_status",
		"send_message", "team_create", "team_dissolve", "team_status",
	},
}

// DefaultGroups defines tool groups for the group:xxx syntax sugar.
// Groups expand to a list of tool names and can be used in allow/deny lists.
//
// Example: {"deny": ["group:cluster"]} denies all Golem cluster tools.
var DefaultGroups = map[string][]string{
	"group:web":      {"web_fetch", "web_search"},
	"group:memory":   {"search_memory", "update_memory", "list_memories"},
	"group:cluster":  {"cluster_list_nodes", "cluster_get_node", "cluster_dispatch_task", "cluster_execute_skill"},
	"group:team":     {"send_message", "team_create", "team_dissolve", "team_status", "team_message"},
	"group:skills":   {"list_skills", "view_skill", "execute_skill"},
	"group:subagent": {"sessions_spawn", "sessions_status"},
}

// ExpandGroups expands group:xxx references in an allow/deny list to individual tool names.
// Non-group entries are passed through unchanged.
//
// Example:
//
//	ExpandGroups([]string{"group:web", "llm_task"})
//	// → ["web_fetch", "web_search", "llm_task"]
func ExpandGroups(names []string) []string {
	if len(names) == 0 {
		return names
	}
	var result []string
	for _, name := range names {
		if strings.HasPrefix(name, "group:") {
			if members, ok := DefaultGroups[name]; ok {
				result = append(result, members...)
			}
			// Unknown group names are silently ignored.
		} else {
			result = append(result, name)
		}
	}
	return result
}
