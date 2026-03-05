// Package golem provides a unified Golem cluster management subsystem.
// It contains three main components:
//   - registry: Node registration and state management
//   - dispatcher: Stream-based task dispatch to Golem nodes via heartbeat streams
//   - scheduler: Task scheduling with AI-driven node selection
//
// The typical initialization flow is:
//
//	registry := registry.Config{...}.Complete().New()
//	dispatcher := dispatcher.NewStreamDispatcher(streamManager)
//	scheduler := scheduler.DefaultSchedulerConfig().Complete(profileProvider, dispatcher).New()
package golem

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/dispatcher"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/scheduler"
)

// Config holds the complete configuration for the Golem subsystem.
type Config struct {
	Registry  registry.Config
	Scheduler scheduler.SchedulerConfig
}

// CompletedConfig is a sealed configuration ready for use.
type CompletedConfig struct {
	registryConfig  *registry.CompletedConfig
	schedulerConfig *scheduler.SchedulerConfig
}

// Complete validates and fills in default values.
func (c *Config) Complete() *CompletedConfig {
	return &CompletedConfig{
		registryConfig:  c.Registry.Complete(),
		schedulerConfig: &c.Scheduler,
	}
}

// Module is the assembled Golem subsystem.
type Module struct {
	Registry   registry.Registry
	Dispatcher dispatcher.Dispatcher
	Scheduler  scheduler.Scheduler

	profileProvider *scheduler.RegistryProfileProvider

	// streamDispatcher is kept for binding the StreamManager after handler creation.
	streamDispatcher *dispatcher.StreamDispatcher
}

// New creates a fully initialized Golem subsystem.
// Note: The StreamManager must be bound after creation via BindStreamManager().
func (c *CompletedConfig) New() (*Module, error) {
	// 1. Create registry.
	reg, err := c.registryConfig.New()
	if err != nil {
		return nil, fmt.Errorf("golem: failed to create registry: %w", err)
	}

	// 2. Create profile provider (adapts registry to scheduler's interface).
	profileProvider := scheduler.NewRegistryProfileProvider(reg)

	// 3. Create stream dispatcher (StreamManager will be bound later).
	disp := dispatcher.NewStreamDispatcher(nil) // StreamManager set via BindStreamManager()

	// 4. Create scheduler.
	schedConfig, err := c.schedulerConfig.Complete(profileProvider, disp)
	if err != nil {
		return nil, fmt.Errorf("golem: failed to complete scheduler config: %w", err)
	}
	sched := schedConfig.New()

	return &Module{
		Registry:         reg,
		Dispatcher:       disp,
		Scheduler:        sched,
		profileProvider:  profileProvider,
		streamDispatcher: disp,
	}, nil
}

// BindStreamManager binds the StreamManager to the dispatcher.
// This must be called after the NodeServiceHandler is created,
// since it implements the StreamManager interface.
func (m *Module) BindStreamManager(mgr dispatcher.StreamManager) {
	m.streamDispatcher.SetStreamManager(mgr)
}

// Start starts all components of the Golem subsystem.
func (m *Module) Start(ctx context.Context) error {
	if err := m.Registry.Start(ctx); err != nil {
		return fmt.Errorf("golem: failed to start registry: %w", err)
	}
	if err := m.Dispatcher.Start(ctx); err != nil {
		return fmt.Errorf("golem: failed to start dispatcher: %w", err)
	}
	if err := m.Scheduler.Start(ctx); err != nil {
		return fmt.Errorf("golem: failed to start scheduler: %w", err)
	}
	return nil
}

// Stop gracefully stops all components.
func (m *Module) Stop(ctx context.Context) error {
	var errs []error

	if err := m.Scheduler.Stop(ctx); err != nil {
		errs = append(errs, fmt.Errorf("scheduler: %w", err))
	}
	if err := m.Dispatcher.Stop(ctx); err != nil {
		errs = append(errs, fmt.Errorf("dispatcher: %w", err))
	}
	if err := m.Registry.Stop(ctx); err != nil {
		errs = append(errs, fmt.Errorf("registry: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("golem: stop errors: %v", errs)
	}
	return nil
}
