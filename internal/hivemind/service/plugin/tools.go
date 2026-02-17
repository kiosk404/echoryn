package plugin

import (
	"context"
)

// ToolDefinition describes a tool registered by a plugin.
// Tools have a single Handler function that is called when the tool is invoked.
// Tools are invoked by the Agent to perform actions.
type ToolDefinition struct {
	// Name is the tool's unique name. (e.g. "memory_search")
	Name string
	// Description is a brief description of the tool's purpose.
	Description string
	// Parameters defines the input schema for the tool.
	Parameters []ParameterDef
	// Handler is the function that is called when the tool is invoked.
	Handler ToolHandler
}

// ParameterDef defines a single parameter for a tool.
type ParameterDef struct {
	// Name is the parameter's unique name. (e.g. "query")
	Name string
	// Type is the parameter's data type. (e.g. "string", "number", "object")
	Type string
	// Description is a brief description of the parameter's purpose.
	Description string
	// Required indicates whether the parameter is mandatory.
	Required bool
}

// ToolHandler is the function that is called when the tool is invoked.
// It receives the context and a map of parameter values, and returns the result or an error.
type ToolHandler func(ctx context.Context, params map[string]interface{}) (interface{}, error)

// ToolProvider is an optional plugin interface for plugins that want to
// contribute Tools to the system.
//
// Tools are invoked by the Agent to perform actions.
//
// Tools are registered into the shared ToolRegistry during framework init.
type ToolProvider interface {
	Plugin
	// Tools returns the Tools contributed by this plugin.
	// These are registered into the shared ToolRegistry during framework init.
	Tools() []ToolDefinition
}
