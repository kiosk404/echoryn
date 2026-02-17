package plugin

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Framework is the core plugin framework that manages plugin lifecycle.
// It orchestrates: plugin loading → slot resolution → Init → Start → Stop.
//
// This is modeled after K8s scheduler's frameworkImpl, which:
// 1. Holds the plugin registry
// 2. Manages plugin instantiation via factories
// 3. Drives the scheduling cycle (in our case, plugin lifecycle)
//
// The Framework implements the Handle interface, so plugins can access
// shared runtime resources through it.
type Framework struct {
	registry       *Registry
	handle         *handleImpl
	slotConfig     SlotConfig
	factories      map[string]registeredFactory
	promptPipeLine *prompt.Pipeline
}

// registeredFactory pairs a PluginFactory with its Definition and args.
type registeredFactory struct {
	definition Definition
	factory    PluginFactory
	args       PluginArgs
}

// Config holds the configuration for creating a Framework.
// Follows the K8s Config → Complete() → New() pattern.
type Config struct {
	// SlotConfig controls which plugins are active per slot kind.
	SlotConfig SlotConfig

	// RuntimeAPI provides plugins access to core modules.
	RuntimeAPI RuntimeAPI
}

// CompletedConfig is the validated and completed framework configuration.
type CompletedConfig struct {
	*Config
}

// Complete validates and fills in defaults for the framework configuration.
func (c *Config) Complete() CompletedConfig {
	if c.SlotConfig == nil {
		c.SlotConfig = make(SlotConfig)
	}
	return CompletedConfig{c}
}

// New creates a new Framework from the completed configuration.
func (c CompletedConfig) New() *Framework {
	handle := newHandle(c.RuntimeAPI)
	return &Framework{
		registry:   NewRegistry(),
		handle:     handle,
		slotConfig: c.SlotConfig,
		factories:  make(map[string]registeredFactory),
	}
}

// --- Factory Registration (pre-init phase) ---

// RegisterFactory registers a PluginFactory with its Definition and optional args.
// This is analogous to K8s scheduler's WithPlugin() option.
//
// Factories are registered before Init(); the Framework instantiates plugins
// from them during Init().
func (f *Framework) RegisterFactory(def Definition, factory PluginFactory, args PluginArgs) error {
	if _, exists := f.factories[def.ID]; exists {
		return fmt.Errorf("plugin factory %q is already registered", def.ID)
	}
	f.factories[def.ID] = registeredFactory{
		definition: def,
		factory:    factory,
		args:       args,
	}
	return nil
}

// --- Lifecycle ---

// Init instantiates all registered factories, resolves slots, and calls
// Init/Register on each plugin.
//
// This corresponds to the "plugin loading" phase in OpenClaw:
// 1. Iterate factories in registration order
// 2. Resolve slot constraints
// 3. Instantiate plugin via factory
// 4. Call InitPlugin.Init() if implemented (register Tool/CLI/Hook/Service)
// 5. Auto-probe for ToolProvider/HookProvider/ServiceProvider/CLIProvider interfaces
func (f *Framework) Init() error {
	logger.Info("[Plugin] initializing framework with %d plugin factories", len(f.factories))

	activeSlots := make(map[string]string)

	for _, entry := range f.factories {
		def := entry.definition

		// Step 1: Slot resolution.
		if err := ResolveSlot(def, activeSlots, f.slotConfig); err != nil {
			logger.Info("[Plugin] skipping plugin %q: %v", def.ID, err)
			continue
		}

		// Step 2: Instantiate via factory.
		p, err := entry.factory(entry.args, f.handle)
		if err != nil {
			return fmt.Errorf("failed to create plugin %q: %w", def.ID, err)
		}

		// Step 3: Register in registry.
		if err := f.registry.registerPlugin(p.Name(), def, p); err != nil {
			return fmt.Errorf("failed to register plugin %q: %w", def.ID, err)
		}

		// Mark slot as occupied.
		if def.Kind != "" && def.Kind != "general" {
			activeSlots[def.Kind] = def.ID
		}

		// Step 4: Call InitPlugin.Init() if implemented.
		if initP, ok := p.(InitPlugin); ok {
			api := newPluginAPI(f.registry, p.Name())
			if err := initP.Init(api); err != nil {
				return fmt.Errorf("plugin %q Init() failed: %w", def.ID, err)
			}
		}

		// Step 5: Auto-probe interfaces and register capabilities.
		f.probeAndRegister(p)

		logger.Info("[Plugin] loaded plugin %q (kind=%s)", def.ID, def.Kind)
	}

	logger.Info("[Plugin] framework initialized: %d plugins, %d tools, %d services",
		f.registry.Len(), len(f.registry.tools), len(f.registry.services))
	return nil
}

// probeAndRegister checks if a plugin implements optional provider interfaces
// and auto-registers their capabilities. This is the K8s "interface probe" pattern.
func (f *Framework) probeAndRegister(p Plugin) {
	name := p.Name()

	// Probe ToolProvider.
	if tp, ok := p.(ToolProvider); ok {
		for _, tool := range tp.Tools() {
			f.registry.addTool(name, tool)
		}
	}

	// Probe HookProvider.
	if hp, ok := p.(HookProvider); ok {
		for event, handler := range hp.Hooks() {
			f.registry.addHook(name, event, handler)
		}
	}

	// Probe ServiceProvider.
	if sp, ok := p.(ServiceProvider); ok {
		for _, svc := range sp.Services() {
			f.registry.addService(name, svc)
		}
	}

	// Probe CLIProvider.
	if cp, ok := p.(CLIProvider); ok {
		for _, registrar := range cp.CLIRegistrars() {
			f.registry.addCLI(name, registrar)
		}
	}

	// Probe PromptProvider - register sections/mutators into the shared pipeline.
	if f.promptPipeLine != nil {
		if pp, ok := p.(PromptProvider); ok {
			for _, section := range pp.PromptSections() {
				f.promptPipeLine.RegisterSection(section)
			}
		}
		if mp, ok := p.(PromptMutatorProvider); ok {
			for _, mutator := range mp.PromptMutators() {
				f.promptPipeLine.RegisterMutator(mutator)
			}
		}
	}
}

// Start starts all plugin services and fires the ServerStart hook.
func (f *Framework) Start(ctx context.Context) error {
	// Start LifecyclePlugin plugins.
	for _, name := range f.registry.PluginNames() {
		p, _ := f.registry.GetPlugin(name)
		if lp, ok := p.(LifecyclePlugin); ok {
			logger.Info("[Plugin] starting lifecycle plugin %q", name)
			if err := lp.Start(ctx); err != nil {
				return fmt.Errorf("plugin %q Start() failed: %w", name, err)
			}
		}
	}

	// Start registered services.
	services := f.registry.GetServices()
	for _, svc := range services {
		logger.Info("[Plugin] starting service %q", svc.Name)
		if err := svc.Start(ctx); err != nil {
			return fmt.Errorf("service %q Start() failed: %w", svc.Name, err)
		}
	}

	// Fire ServerStart hook.
	if err := FireHooks(ctx, f.registry, HookServerStart, nil); err != nil {
		logger.Warn("[Plugin] server_start hook error: %v", err)
	}

	return nil
}

// Stop stops all plugin services and fires the ServerStop hook.
func (f *Framework) Stop(ctx context.Context) error {
	// Fire ServerStop hook.
	if err := FireHooks(ctx, f.registry, HookServerStop, nil); err != nil {
		logger.Warn("[Plugin] server_stop hook error: %v", err)
	}

	// Stop registered services (reverse order).
	services := f.registry.GetServices()
	for i := len(services) - 1; i >= 0; i-- {
		svc := services[i]
		logger.Info("[Plugin] stopping service %q", svc.Name)
		if err := svc.Stop(ctx); err != nil {
			logger.Warn("[Plugin] service %q Stop() error: %v", svc.Name, err)
		}
	}

	// Stop LifecyclePlugin plugins (reverse order).
	names := f.registry.PluginNames()
	for i := len(names) - 1; i >= 0; i-- {
		p, _ := f.registry.GetPlugin(names[i])
		if lp, ok := p.(LifecyclePlugin); ok {
			logger.Info("[Plugin] stopping lifecycle plugin %q", names[i])
			if err := lp.Stop(ctx); err != nil {
				logger.Warn("[Plugin] plugin %q Stop() error: %v", names[i], err)
			}
		}
	}

	return nil
}

// --- Accessors ---

// Registry returns the underlying plugin registry.
// Used by the server to query registered tools, hooks, CLI commands, etc.
func (f *Framework) Registry() *Registry {
	return f.registry
}

// Handle returns the framework Handle for external use.
func (f *Framework) Handle() Handle {
	return f.handle
}

// SetPromptPipeline attaches a PromptPipeline to the framework.
// Plugin-contributed sections/mutators are registered into this pipeline.
// Must be called before init() for plugins to contribute sections
func (f *Framework) SetPromptPipeline(pipeline *prompt.Pipeline) {
	f.promptPipeLine = pipeline
}

// PromptPipeline returns the attached PromptPipeline.
func (f *Framework) PromptPipeline() *prompt.Pipeline {
	return f.promptPipeLine
}
