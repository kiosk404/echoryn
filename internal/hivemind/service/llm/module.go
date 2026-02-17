package llm

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/store/inmemory"
	"github.com/kiosk404/echoryn/internal/pkg/options"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Config holds the configuration for the LLM module.
type Config struct {
	ModelOptions *options.ModelOptions

	// OutOfTreeRegistry allows registering additional provider plugins
	// beyond the built-in ones. Similar to K8S scheduler's WithPlugin() mechanism.
	// If nil, only in-tree providers are available.
	OutOfTreeRegistry *provider.Registry
}

// CompletedConfig is the validated and completed configuration.
type CompletedConfig struct {
	*Config
}

// Complete validates and fills defaults.
func (c *Config) Complete() CompletedConfig {
	if c.ModelOptions == nil {
		c.ModelOptions = options.NewModelOptions()
	}
	return CompletedConfig{c}
}

// Module is the top-level LLM module, holding all domain services.
//
// It exposes:
// - Manager: core CRUD + ChatModel building
// - Prober: model availability probing (model-scan)
// - Fallback: model fallback execution (model-fallback)
// - Registry: provider plugin registry
type Module struct {
	Manager  service.ModelManager
	Prober   *service.ModelProber
	Fallback *service.FallbackExecutor
	Registry *provider.Registry
}

// New creates and initializes the LLM module from a completed config.
// This follows the K8S-style: Config → Complete() → New() pattern.
//
// Initialization flow:
// 1. Build the in-tree provider Registry
// 2. Merge out-of-tree providers (if any)
// 3. Create in-memory stores (repository layer)
// 4. Create ModelManager with Registry injection
// 5. Initialize: Registry-based provider discovery + user config + compat rules
// 6. Create auxiliary services: Prober, FallbackExecutor
func (c CompletedConfig) New(ctx context.Context) (*Module, error) {
	logger.Info("[LLM] creating LLM module...")

	// Build provider registry (K8S-style: in-tree + out-of-tree merge).
	registry := provider.NewInTreeRegistry()
	if c.OutOfTreeRegistry != nil {
		if err := registry.Merge(c.OutOfTreeRegistry); err != nil {
			return nil, fmt.Errorf("failed to merge out-of-tree providers: %w", err)
		}
	}
	logger.Info("[LLM] provider registry initialized with %d plugins", registry.Len())

	// Infrastructure layer: in-memory repositories.
	modelStore := inmemory.NewModelStore()
	providerStore := inmemory.NewProviderStore()

	// Domain service layer: model manager with registry injection.
	manager := service.NewModelManager(c.ModelOptions, modelStore, providerStore, registry)

	// Initialize: load providers and models from registry + config + env.
	if err := manager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize LLM module: %w", err)
	}

	// Auxiliary domain services.
	prober := service.NewModelProber(modelStore, providerStore, registry, manager)
	fallback := service.NewFallbackExecutor(modelStore, manager)

	return &Module{
		Manager:  manager,
		Prober:   prober,
		Fallback: fallback,
		Registry: registry,
	}, nil
}

// --- ChatModel convenience methods ---

// ChatModel returns a cached Eino BaseChatModel for the given provider/model reference.
func (m *Module) ChatModel(ctx context.Context, ref entity.ModelRef) (model.BaseChatModel, error) {
	return m.Manager.GetChatModel(ctx, ref)
}

// BuildChatModel builds a fresh Eino BaseChatModel with the given LLM params.
// Unlike ChatModel, this always creates a new instance (not cached).
// params may be nil to use provider defaults.
func (m *Module) BuildChatModel(ctx context.Context, ref entity.ModelRef, params *entity.LLMParams) (model.BaseChatModel, error) {
	return m.Manager.BuildChatModel(ctx, ref, params)
}

// DefaultChatModel returns the Eino BaseChatModel for the system default model.
func (m *Module) DefaultChatModel(ctx context.Context) (model.BaseChatModel, error) {
	return m.Manager.GetDefaultChatModel(ctx)
}

// -- Fallback convenience methods --

// ChatModelWithFallback returns a cached Eino BaseChatModel with fallback logic.
// It attempts to use the primary model specified in config, falling back to
// other candidates if the primary is unavailable or in cooldown.
// params may be nil to use provider defaults.
func (m *Module) ChatModelWithFallback(ctx context.Context, config entity.FallbackConfig, params *entity.LLMParams) (model.BaseChatModel, entity.ModelRef, error) {
	return m.Fallback.GetChatModelWithFallback(ctx, config, params)
}

// -- Probe convenience methods --

// ProbeModel checks if the specified model is available and responsive.
// It returns true if the model is available, false otherwise.
func (m *Module) ProbeModel(ctx context.Context, ref entity.ModelProbeSpec) (*entity.ModelScanResult, error) {
	return m.Prober.ProbeModel(ctx, ref)
}

// ScanModels probes multiple models in parallel and returns their availability status.
func (m *Module) ScanModels(ctx context.Context, specs []entity.ModelProbeSpec, onProcess func(completed, total int)) ([]*entity.ModelScanResult, error) {
	return m.Prober.ScanModels(ctx, specs, onProcess)
}

// -- Compat convenience methods --

func (m *Module) ResolveCompat(ctx context.Context, provider entity.ModelRef) (*entity.ModelCompatConfig, error) {
	return m.Manager.ResolveCompat(ctx, provider)
}

// -- Status management methods --

// UpdateModelStatus updates the status of a model in the repository.
func (m *Module) UpdateModelStatus(ctx context.Context, ref entity.ModelRef, status entity.ModelStatus) error {
	return m.Manager.SetModelStatus(ctx, ref, status)
}
