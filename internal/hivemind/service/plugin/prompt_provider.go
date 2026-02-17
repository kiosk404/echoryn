package plugin

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
)

// PromptProvider is an optional plugin interface for plugins that want to
// contribute PromptSections and/or PromptMutators to the system prompt pipeline.
//
// This is the fifth capability injection channel, alongside:
//   - ToolProvider (tools for Agent to call)
//   - HookProvider (lifecycle hooks)
//   - ServiceProvider (background services)
//   - CLIProvider (CLI commands)
//
// The framework probes for this interface during Init() and auto-registers
// sections/mutators into the shared PromptPipeline.
type PromptProvider interface {
	Plugin

	// PromptSections returns the PromptSections contributed by this plugin.
	// These are registered into the shared Pipeline during framework init.
	PromptSections() []prompt.PromptSection
}

// PromptMutatorProvider is an optional plugin interface for plugins that want to
// contribute PromptMutators to the system prompt pipeline.
//
// PromptMutators returns the PromptMutators contributed by this plugin.
// These are registered into the shared Pipeline during framework init.
type PromptMutatorProvider interface {
	Plugin

	// PromptMutators returns the PromptMutators contributed by this plugin.
	// These are registered into the shared Pipeline during framework init.
	PromptMutators() []prompt.PromptMutator
}
