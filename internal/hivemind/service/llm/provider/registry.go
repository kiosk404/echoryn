package provider

import (
	"fmt"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
)

// Registry is a thread-safe registry for LLM providers.
type Registry struct {
	mu       sync.RWMutex
	registry map[string]spi.PluginFactory
}

// NewRegistry creates a new instance of the Registry.
func NewRegistry() *Registry {
	return &Registry{
		registry: make(map[string]spi.PluginFactory),
	}
}

// Register adds a provider plugin factory to the registry.
// Returns an error if a plugin with the same name is already registered
func (r *Registry) Register(name string, factory spi.PluginFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.registry[name]; ok {
		return fmt.Errorf("provider %s is already registered", name)
	}

	r.registry[name] = factory
	return nil
}

// MustRegister adds a provider plugin factory to the registry.
// Panics if a plugin with the same name is already registered
func (r *Registry) MustRegister(name string, factory spi.PluginFactory) {
	if err := r.Register(name, factory); err != nil {
		panic(err)
	}
}

// Unregister removes a provider plugin factory from the registry.
// Returns an error if the plugin is not registered
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.registry[name]; !ok {
		return fmt.Errorf("provider %s is not registered", name)
	}
	delete(r.registry, name)
	return nil
}

// Get returns the plugin factory for the given name.
// Returns an error if the plugin is not registered
func (r *Registry) Get(name string) (spi.PluginFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.registry[name]
	if !ok {
		return nil, fmt.Errorf("provider %s is not registered", name)
	}
	return factory, nil
}

// List returns all registered provider names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.registry))
	for name := range r.registry {
		names = append(names, name)
	}
	return names
}

// Merge combines another registry into this one.
// Returns an error if any of the plugins in the other registry are already registered
func (r *Registry) Merge(other *Registry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, factory := range other.registry {
		if err := r.Register(name, factory); err != nil {
			return err
		}
	}
	return nil
}

// Len returns the number of registered providers.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.registry)
}

// Range iterates over all registered providers and calls the given function.
// If the function returns false, the iteration is stopped.
func (r *Registry) Range(fn func(name string, factory spi.PluginFactory) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, factory := range r.registry {
		if !fn(name, factory) {
			break
		}
	}
}
