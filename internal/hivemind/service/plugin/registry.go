package plugin

import (
	"fmt"
	"sync"

	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/spf13/cobra"
)

// Registry is the central plugin registry that holds all loaded plugins
// and their registered capabilities (tools, CLI, hooks, services).
//
// This follows K8s scheduler's frameworkImpl pattern: the framework
// maintains ordered lists of plugins per extension point.
//
// Thread-safe: all mutations are guarded by a mutex.
type Registry struct {
	mu sync.RWMutex

	// plugins holds all loaded plugins, keyed by plugin name.
	plugins map[string]Plugin

	// pluginOrder preserves the registration order of plugins.
	pluginOrder []string

	// definitions holds static metadata for each plugin.
	definitions map[string]Definition

	// --- Registered capabilities (aggregated from all plugins) ---

	// tools maps tool name → ToolDefinition.
	// Tool names are globally unique; duplicate registration is an error.
	tools map[string]ToolDefinition

	// toolOwners maps tool name → plugin name (for diagnostics).
	toolOwners map[string]string

	// cliRegistrars holds all CLI registrars in registration order.
	cliRegistrars []cliEntry

	// hooks maps event → ordered list of handlers.
	hooks map[HookEvent][]hookEntry

	// services holds all background services in registration order.
	services []serviceEntry

	// --- Slot management ---

	// slots maps kind → active plugin name.
	slots map[string]string
}

// cliEntry tracks which plugin registered a CLI registrar.
type cliEntry struct {
	pluginName string
	registrar  CLIRegistrar
}

// hookEntry tracks which plugin registered a hook handler.
type hookEntry struct {
	pluginName string
	handler    HookHandler
}

// serviceEntry tracks which plugin registered a service.
type serviceEntry struct {
	pluginName string
	service    ServiceDefinition
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins:     make(map[string]Plugin),
		definitions: make(map[string]Definition),
		tools:       make(map[string]ToolDefinition),
		toolOwners:  make(map[string]string),
		hooks:       make(map[HookEvent][]hookEntry),
		slots:       make(map[string]string),
	}
}

// --- Registration methods (called by pluginAPIImpl) ---

func (r *Registry) addTool(pluginName string, tool ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.toolOwners[tool.Name]; ok {
		logger.Warn("[Plugin] tool %q already registered by plugin %q, overriding with %q",
			tool.Name, existing, pluginName)
	}
	r.tools[tool.Name] = tool
	r.toolOwners[tool.Name] = pluginName
}

func (r *Registry) addCLI(pluginName string, registrar CLIRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cliRegistrars = append(r.cliRegistrars, cliEntry{
		pluginName: pluginName,
		registrar:  registrar,
	})
}

func (r *Registry) addHook(pluginName string, event HookEvent, handler HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hooks[event] = append(r.hooks[event], hookEntry{
		pluginName: pluginName,
		handler:    handler,
	})
}

func (r *Registry) addService(pluginName string, svc ServiceDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.services = append(r.services, serviceEntry{
		pluginName: pluginName,
		service:    svc,
	})
}

// --- Query methods ---

// GetPlugin returns a loaded plugin by name.
func (r *Registry) GetPlugin(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// GetTools returns all registered tools.
func (r *Registry) GetTools() map[string]ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]ToolDefinition, len(r.tools))
	for k, v := range r.tools {
		result[k] = v
	}
	return result
}

// GetHooks returns all handlers registered for the given event.
func (r *Registry) GetHooks(event HookEvent) []HookHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := r.hooks[event]
	handlers := make([]HookHandler, 0, len(entries))
	for _, e := range entries {
		handlers = append(handlers, e.handler)
	}
	return handlers
}

// GetServices returns all registered background services.
func (r *Registry) GetServices() []ServiceDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ServiceDefinition, 0, len(r.services))
	for _, e := range r.services {
		result = append(result, e.service)
	}
	return result
}

// RegisterCLICommands registers all plugin-provided CLI subcommands
// into the given cobra parent command.
func (r *Registry) RegisterCLICommands(parent *cobra.Command) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, entry := range r.cliRegistrars {
		entry.registrar.RegisterCommands(parent)
	}
}

// PluginNames returns the names of all loaded plugins in registration order.
func (r *Registry) PluginNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, len(r.pluginOrder))
	copy(result, r.pluginOrder)
	return result
}

// Len returns the number of loaded plugins.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}

// --- Internal registration ---

// registerPlugin adds a plugin to the registry. Called by Framework.
func (r *Registry) registerPlugin(name string, def Definition, p Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %q is already registered", name)
	}

	r.plugins[name] = p
	r.definitions[name] = def
	r.pluginOrder = append(r.pluginOrder, name)
	return nil
}
