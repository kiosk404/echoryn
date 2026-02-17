package plugin

import (
	"context"
)

// Plugin is the fundamental interface that all plugins must implement.
// Each Plugin has a static definition and registers its capabilities
// via the PluginAPI during the Register phase
type Plugin interface {
	// Name returns the unique identifier of this plugin.
	// Must be DNS-compatible (lowercase, hyphens, no spaces).
	Name() string
}

// InitPlugin is an optional interface for plugins that need initialization
// with access to the pluginAPI, called during framework setup
type InitPlugin interface {
	Plugin

	// Init is called after the plugin is instantiated, allowing it
	// to register Tool/Cli/Hook/Service capabilities via the PluginAPI.
	Init(api PluginAPI) error
}

// LifecyclePlugin is an optional interface for plugins that have
// start/stop lifecycle
type LifecyclePlugin interface {
	Plugin

	// Start is called when the framework is initializing,
	// after all plugins have been registered.
	Start(ctx context.Context) error

	// Stop is called when the framework is shutting down,
	// before plugins are unregistered.
	Stop(ctx context.Context) error
}

// PluginFactory is a function that creates a new instance of a plugin.
// It is called during framework initialization, after all plugins have been registered.
type PluginFactory func(args PluginArgs, handle Handle) (Plugin, error)

// PluginArgs is a map of arguments passed to the PluginFactory.
// These arguments are typically configuration values or dependencies.
type PluginArgs map[string]interface{}

// Definition is the static metadata for a plugin.
// It is used to register plugins into the framework.
type Definition struct {
	ID          string
	Name        string
	Kind        string
	Description string
}

// Handle is the interface that plugins use to access the framework's runtime API.
// It is passed to the PluginFactory during plugin instantiation.
type Handle interface {
	// RuntimeAPI returns the framework's runtime API.
	// This is used to access services, tools, and other plugins.
	RuntimeAPI() RuntimeAPI
}
