// Package builtin registers all in-tree (built-in) plugins.
// This is analogous to K8s scheduler's algorithmprovider package,
// which registers all default scheduling plugins.
//
// New built-in plugins should be added to NewInTreeRegistry().
package builtin

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	feishuchannel "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/channel/feishu"
	telegramchannel "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/channel/telegram"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics"
	golemcluster "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/golem-cluster"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/llmtask"
	memorycore "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory/memory-core"
	skillsplugin "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/skills"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/subagent"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/tracing/langfuse-tracing"
	langsmithtracing "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/tracing/langsmith-tracing"
	webfetch "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/web-fetch"
	ddgsearch "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/web-search/duckduckgo-web-search"
	geminisearch "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/web-search/gemini-web-search"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// NewInTreeRegistry creates the in-tree plugin registry with all
// built-in plugins pre-registered.
//
// Configuration is sourced from PluginsOptions (aligned with OpenClaw's
// plugins.entries[pluginID].config pattern). Each plugin receives its
// config via PluginArgs["config"], resolved from the unified PluginsOptions.
//
// This is the single source of truth for what plugins ship with Echoryn.
// To add a new built-in plugin:
// 1. Create the plugin package under builtin/
// 2. Add a Register call here
func NewInTreeRegistry(opts *genericoptions.PluginsOptions, golemModule *golem.Module) *plugin.InTreeRegistry {
	registry := plugin.NewInTreeRegistry()

	// --- memory-core: default memory system (SQLite + hybrid search) ---
	registry.Register(
		memorycore.PluginDefinition(),
		memorycore.Factory,
		plugin.PluginArgs{
			"config": memorycore.ResolveMemoryConfig(opts),
		},
	)

	// --- diagnostics: observability and metrics collection ---
	registry.Register(
		diagnostics.PluginDefinition(),
		diagnostics.Factory,
		plugin.PluginArgs{
			"config": diagnostics.ResolveDiagnosticsConfig(opts),
		},
	)

	// --- llm-task: generic JSON-only LLM tool ---
	registry.Register(
		llmtask.PluginDefinition(),
		llmtask.Factory,
		plugin.PluginArgs{
			"config": llmtask.ResolveLLMTaskConfig(opts),
		},
	)

	// --- langfuse-tracing: LLM application-level observability via Langfuse ---
	registry.Register(
		langfusetracing.PluginDefinition(),
		langfusetracing.Factory,
		plugin.PluginArgs{
			"config": langfusetracing.ResolveLangfuseConfig(opts),
		},
	)

	// --- langsmith-tracing: LLM application-level observability via LangSmith ---
	// Slot: "tracing" -- mutually exclusive with langsmith-tracing.
	registry.Register(
		langsmithtracing.PluginDefinition(),
		langsmithtracing.Factory,
		plugin.PluginArgs{
			"config": langsmithtracing.ResolveLangSmithConfig(opts),
		})

	// --- subagent: sub-agent orchestration (sessions_spawn/sessions_status) ---
	registry.Register(
		subagent.PluginDefinition(),
		subagent.Factory,
		plugin.PluginArgs{
			"config": subagent.ResolveSubAgentConfig(opts),
		},
	)

	// --- golem: bridges golem subsystem to agent runtime ---
	// Note: registry + dispatcher deps are injected post-init GolemDepsSetter
	// interface probe in server.go (same pattern as channelManagerSetter)
	registry.Register(
		golemcluster.PluginDefinition(),
		golemcluster.Factory,
		plugin.PluginArgs{
			"config":     golemcluster.ResolveGolemClusterConfig(opts),
			"registry":   golemModule.Registry,
			"dispatcher": golemModule.Dispatcher,
			"scheduler":  golemModule.Scheduler,
		})

	// --- skills: bridges file-system skills (SKILL.md) to agent runtime ---
	registry.Register(
		skillsplugin.PluginDefinition(),
		skillsplugin.Factory,
		plugin.PluginArgs{
			"config": skillsplugin.ResolveSkillsConfig(opts),
		})

	// --- web-fetch: HTTP fetch + readable content extraction (HTML + markdown/text) ---
	registry.Register(
		webfetch.PluginDefinition(),
		webfetch.Factory,
		plugin.PluginArgs{
			"config": webfetch.ResolveWebFetchConfig(opts),
		})

	// --- web-search (gemini): web search via Gemini Google search grounding ---
	registry.Register(
		geminisearch.PluginDefinition(),
		geminisearch.Factory,
		plugin.PluginArgs{
			"config": geminisearch.ResolveWebSearchConfig(opts),
		})

	// --- web-search (duckduckgo): privacy-focused web search, no API key required ---
	// Slot: "web-search" — mutually exclusive with gemini-web-search.
	registry.Register(
		ddgsearch.PluginDefinition(),
		ddgsearch.Factory,
		plugin.PluginArgs{
			"config": ddgsearch.ResolveDDGSearchConfig(opts),
		})

	// --- channel-feishu: Feishu/Lark IM integration (websocket/webhook) ---
	registry.Register(
		feishuchannel.PluginDefinition(),
		feishuchannel.Factory,
		plugin.PluginArgs{
			"config": feishuchannel.ResolveFeishuConfig(opts),
		})

	// --- channel-telegram: Telegram Bot integration ---
	registry.Register(
		telegramchannel.PluginDefinition(),
		telegramchannel.Factory,
		plugin.PluginArgs{
			"config": telegramchannel.ResolveTelegramConfig(opts),
		})

	return registry
}
