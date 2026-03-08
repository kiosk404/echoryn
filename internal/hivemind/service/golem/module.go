// Package golem provides a unified Golem cluster management subsystem.
// It contains three main components:
//   - registry: Node registration and state management
//   - dispatcher: Stream-based task dispatch to Golem nodes via heartbeat streams
//   - scheduler: Task scheduling with AI-driven node selection
//   - tokenmanager: Bootstrap Token management for node joining
//
// The typical initialization flow is:
//
//		registry := registry.Config{...}.Complete().New()
//	 tokenManager := tokenmanager.Config{...}.Complete().New()
//		dispatcher := dispatcher.NewStreamDispatcher(streamManager)
//		scheduler := scheduler.DefaultSchedulerConfig().Complete(profileProvider, dispatcher).New()
package golem

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/dispatcher"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/scheduler"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/tokenmanager"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
)

// Config holds the complete configuration for the Golem subsystem.
type Config struct {
	Registry     registry.Config
	Scheduler    scheduler.SchedulerConfig
	TokenManager tokenmanager.Config
}

// CompletedConfig is a sealed configuration ready for use.
type CompletedConfig struct {
	registryConfig     *registry.CompletedConfig
	schedulerConfig    *scheduler.SchedulerConfig
	tokenManagerConfig *tokenmanager.CompletedConfig
}

// Complete validates and fills in default values.
func (c *Config) Complete() *CompletedConfig {
	return &CompletedConfig{
		registryConfig:     c.Registry.Complete(),
		schedulerConfig:    &c.Scheduler,
		tokenManagerConfig: c.TokenManager.Complete(),
	}
}

// Module is the assembled Golem subsystem.
type Module struct {
	Registry     registry.Registry
	Dispatcher   dispatcher.Dispatcher
	Scheduler    scheduler.Scheduler
	TokenManager tokenmanager.TokenManager

	profileProvider *scheduler.RegistryProfileProvider

	// streamDispatcher is kept for binding the StreamManager after handler creation.
	streamDispatcher *dispatcher.StreamDispatcher

	// schedulerConfig is retained for rebuilding the scheduler with LLM support.
	schedulerConfig *scheduler.SchedulerConfig
}

// New creates a fully initialized Golem subsystem.
// Note: The StreamManager must be bound after creation via BindStreamManager().
// LLM scheduling support can be enabled post-creation via BindLLMManager().
func (c *CompletedConfig) New() (*Module, error) {
	// 1. Create registry.
	reg, err := c.registryConfig.New()
	if err != nil {
		return nil, fmt.Errorf("golem: failed to create registry: %w", err)
	}

	// 2. Create token manager.
	tm, err := c.tokenManagerConfig.New()
	if err != nil {
		return nil, fmt.Errorf("golem: failed to create token manager: %w", err)
	}

	// 3. Create profile provider (adapts registry to scheduler's interface).
	profileProvider := scheduler.NewRegistryProfileProvider(reg)

	// 4. Create stream dispatcher (StreamManager will be bound later).
	disp := dispatcher.NewStreamDispatcher(nil) // StreamManager set via BindStreamManager()

	// 5. Create scheduler. (LLMManager will be bound later via BindLLMManager)
	schedConfig, err := c.schedulerConfig.Complete(profileProvider, disp)
	if err != nil {
		return nil, fmt.Errorf("golem: failed to complete scheduler config: %w", err)
	}
	sched := schedConfig.New()

	return &Module{
		Registry:         reg,
		Dispatcher:       disp,
		Scheduler:        sched,
		TokenManager:     tm,
		profileProvider:  profileProvider,
		streamDispatcher: disp,
		schedulerConfig:  c.schedulerConfig,
	}, nil
}

// BindStreamManager binds the StreamManager to the dispatcher.
// This must be called after the NodeServiceHandler is created,
// since it implements the StreamManager interface.
func (m *Module) BindStreamManager(mgr dispatcher.StreamManager) {
	m.streamDispatcher.SetStreamManager(mgr)
}

// BindLLMManager enables LLMMode scheduling by rebuilding the scheduler with
// LLM support. This must be called after the LLM module is initialized.
// It follows the same lazy-binding pattern as BindStreamManager.
func (m *Module) BindLLMManager(llmMgr llmService.ModelManager) error {
	if llmMgr == nil {
		return nil
	}
	if m.schedulerConfig == nil {
		return fmt.Errorf("golem: schedulerConfig not available for LLM binding")
	}

	// Rebuild the scheduler with LLM support.
	schedConfig, err := m.schedulerConfig.Complete(m.profileProvider, m.Dispatcher, llmMgr)
	if err != nil {
		return fmt.Errorf("golem: failed to rebuild scheduler with LLM support: %w", err)
	}
	m.Scheduler = schedConfig.New()
	return nil
}

// Start starts all components of the Golem subsystem.
func (m *Module) Start(ctx context.Context) error {
	if err := m.TokenManager.Start(ctx); err != nil {
		return fmt.Errorf("golem: failed to start token manager: %w", err)
	}
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
	if err := m.TokenManager.Stop(ctx); err != nil {
		errs = append(errs, fmt.Errorf("tokenmanager: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("golem: stop errors: %v", errs)
	}
	return nil
}
