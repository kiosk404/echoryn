package plugin

import (
	"context"
)

// ServiceDefinition describes a background service registered by a plugin.
// Service have a Start/Stop lifecycle managed by the framework.
type ServiceDefinition struct {
	// Name is the service's unique name.
	Name string

	// Start launches the service. It should be non-blocking
	Start func(ctx context.Context) error
	// Stop gracefully shuts down the service. It should be non-blocking.
	Stop func(ctx context.Context) error
}

// ServiceProvider is an optional plugin interface that allows plugins to
// register background services. The framework probes for this interface
// when loading plugins.
type ServiceProvider interface {
	Plugin
	// Services returns the list of background services provided by the plugin.
	Services() []ServiceDefinition
}
