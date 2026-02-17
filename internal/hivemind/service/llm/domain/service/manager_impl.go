package service

import (
	"context"
	"fmt"
	"sync"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/internal/pkg/options"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Compile-time interface check.
var _ ModelManager = (*modelManagerImpl)(nil)

// modelManagerImpl is the concrete implementation of ModelManager.
// It uses a provider.Registry (K8S scheduler style) to discover and instantiate
// built-in providers instead of hard-coding them.
type modelManagerImpl struct {
	opts         *options.ModelOptions
	modelRepo    repo.ModelRepository
	providerRepo repo.ProviderRepository
	registry     *provider.Registry
	compatMgr    *CompatManager

	// chatModelCache is a lazily-populated cache of Eino BaseChatModel instances.
	// Key: ModelRef.String() ("provider/model"), Value: einoModel.BaseChatModel.
	chatModelCache sync.Map

	// pluginCache caches provider plugin instances to avoid repeated factory calls.
	// Key: providerID (string), Value: spi.ProviderPlugin.
	pluginCache sync.Map
}

// NewModelManager creates a new ModelManager with the given dependencies.
// The registry parameter follows the K8S pattern where the caller provides a
// pre-populated Registry (typically from provider.NewInTreeRegistry()).
func NewModelManager(
	opts *options.ModelOptions,
	modelRepo repo.ModelRepository,
	providerRepo repo.ProviderRepository,
	registry *provider.Registry,
) ModelManager {
	return &modelManagerImpl{
		opts:         opts,
		modelRepo:    modelRepo,
		providerRepo: providerRepo,
		registry:     registry,
		compatMgr:    NewCompatManager(registry),
	}
}

// --- Provider Management ---

func (m *modelManagerImpl) RegisterProvider(ctx context.Context, p *entity.ModelProvider) error {
	if p.ID == "" {
		return fmt.Errorf("provider ID is required")
	}
	logger.Info("[LLM] registering provider: %s (class=%s, baseURL=%s)", p.ID, p.ModelClass, p.BaseURL)
	return m.providerRepo.Save(ctx, p)
}

func (m *modelManagerImpl) GetProvider(ctx context.Context, providerID string) (*entity.ModelProvider, error) {
	return m.providerRepo.FindByID(ctx, providerID)
}

func (m *modelManagerImpl) ListProviders(ctx context.Context) ([]*entity.ModelProvider, error) {
	return m.providerRepo.FindAll(ctx)
}

// --- Model Management ---

func (m *modelManagerImpl) RegisterModel(ctx context.Context, instance *entity.ModelInstance) (int64, error) {
	if instance.ModelID == "" {
		return 0, fmt.Errorf("model ID is required")
	}
	if instance.ProviderID == "" {
		return 0, fmt.Errorf("provider ID is required")
	}

	// Verify provider exists.
	if _, err := m.providerRepo.FindByID(ctx, instance.ProviderID); err != nil {
		return 0, fmt.Errorf("provider %q not registered: %w", instance.ProviderID, err)
	}

	if instance.Status == 0 {
		instance.Status = entity.ModelStatus_Ready
	}

	if err := m.modelRepo.Save(ctx, instance); err != nil {
		return 0, err
	}

	logger.Info("[LLM] registered model: %s/%s (id=%d, type=%s)",
		instance.ProviderID, instance.ModelID, instance.ID, instance.Type)
	return instance.ID, nil
}

func (m *modelManagerImpl) GetModelByID(ctx context.Context, id int64) (*entity.ModelInstance, error) {
	return m.modelRepo.FindByID(ctx, id)
}

func (m *modelManagerImpl) GetModelByRef(ctx context.Context, ref entity.ModelRef) (*entity.ModelInstance, error) {
	return m.modelRepo.FindByRef(ctx, ref)
}

func (m *modelManagerImpl) GetDefaultModel(ctx context.Context) (*entity.ModelInstance, error) {
	return m.modelRepo.FindDefault(ctx)
}

func (m *modelManagerImpl) SetDefaultModel(ctx context.Context, id int64) error {
	return m.modelRepo.SetDefault(ctx, id)
}

func (m *modelManagerImpl) ListModelsByProvider(ctx context.Context, providerID string) ([]*entity.ModelInstance, error) {
	return m.modelRepo.FindAllByProvider(ctx, providerID)
}

func (m *modelManagerImpl) ListModelsByType(ctx context.Context, modelType entity.ModelType) ([]*entity.ModelInstance, error) {
	return m.modelRepo.FindAllByType(ctx, modelType)
}

func (m *modelManagerImpl) ListAllModels(ctx context.Context) ([]*entity.ModelInstance, error) {
	return m.modelRepo.FindAll(ctx)
}

// --- ChatModel (Eino) ---

// GetChatModel returns a cached or newly-created Eino ChatModel for the given ModelRef.
// The creation is lazy: ChatModel instances are only created on first access (with nil params), then cached.
// For custom LLM params, use BuildChatModel instead (which always creates a fresh instance).
func (m *modelManagerImpl) GetChatModel(ctx context.Context, ref entity.ModelRef) (einoModel.BaseChatModel, error) {
	cacheKey := ref.String()

	// Fast path: check cache.
	if cached, ok := m.chatModelCache.Load(cacheKey); ok {
		return cached.(einoModel.BaseChatModel), nil
	}

	// Slow path: build with nil params (provider defaults) and cache.
	cm, err := m.BuildChatModel(ctx, ref, nil)
	if err != nil {
		return nil, err
	}

	// Cache for future use (LoadOrStore handles race conditions).
	actual, _ := m.chatModelCache.LoadOrStore(cacheKey, cm)
	return actual.(einoModel.BaseChatModel), nil
}

// BuildChatModel builds a fresh Eino ChatModel with the given LLM params.
// Unlike GetChatModel, this always creates a new instance (not cached),
// because different params produce different model configurations.
// params may be nil, in which case provider defaults are used.
func (m *modelManagerImpl) BuildChatModel(ctx context.Context, ref entity.ModelRef, params *entity.LLMParams) (einoModel.BaseChatModel, error) {
	instance, err := m.modelRepo.FindByRef(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("model %s not found: %w", ref, err)
	}

	prov, err := m.providerRepo.FindByID(ctx, ref.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("provider %q not found: %w", ref.ProviderID, err)
	}

	// Get or create the plugin instance (cached to avoid repeated factory calls).
	chatPlugin, err := m.getChatPlugin(ref.ProviderID)
	if err != nil {
		return nil, err
	}

	cm, err := chatPlugin.BuildChatModel(ctx, instance, prov, params)
	if err != nil {
		return nil, fmt.Errorf("build chat model for %s: %w", ref, err)
	}

	return cm, nil
}

// getChatPlugin returns a cached ChatModelPlugin for the given provider.
// Plugin instances are cached in pluginCache to avoid repeated factory calls,
// which matters for out-of-tree plugins that may have non-trivial initialization.
func (m *modelManagerImpl) getChatPlugin(providerID string) (spi.ChatModelPlugin, error) {
	// Fast path: check cache.
	if cached, ok := m.pluginCache.Load(providerID); ok {
		chatPlugin, ok := cached.(spi.ChatModelPlugin)
		if !ok {
			return nil, fmt.Errorf("provider %q does not implement ChatModelPlugin", providerID)
		}
		return chatPlugin, nil
	}

	// Slow path: create from factory and cache.
	factory, err := m.registry.Get(providerID)
	if err != nil {
		return nil, fmt.Errorf("provider plugin %q not found in registry: %w", providerID, err)
	}

	plugin := factory()
	m.pluginCache.LoadOrStore(providerID, plugin)

	chatPlugin, ok := plugin.(spi.ChatModelPlugin)
	if !ok {
		return nil, fmt.Errorf("provider %q does not implement ChatModelPlugin", providerID)
	}
	return chatPlugin, nil
}

// GetDefaultChatModel returns the Eino ChatModel for the system default model.
func (m *modelManagerImpl) GetDefaultChatModel(ctx context.Context) (einoModel.BaseChatModel, error) {
	defaultInstance, err := m.modelRepo.FindDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("no default model configured: %w", err)
	}

	ref := entity.ModelRef{
		ProviderID: defaultInstance.ProviderID,
		ModelID:    defaultInstance.ModelID,
	}
	return m.GetChatModel(ctx, ref)
}

// --- Compatibility (model-compat) ---

// ResolveCompat returns the resolved compatibility configuration for a model.
func (m *modelManagerImpl) ResolveCompat(ctx context.Context, ref entity.ModelRef) (*entity.ModelCompatConfig, error) {
	instance, err := m.modelRepo.FindByRef(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("model %s not found: %w", ref, err)
	}

	prov, err := m.providerRepo.FindByID(ctx, ref.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("provider %q not found: %w", ref.ProviderID, err)
	}

	return m.compatMgr.ResolveCompat(instance, prov), nil
}

// --- Model Status ---

// SetModelStatus updates the runtime status of a model.
func (m *modelManagerImpl) SetModelStatus(ctx context.Context, ref entity.ModelRef, status entity.ModelStatus) error {
	instance, err := m.modelRepo.FindByRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("model %s not found: %w", ref, err)
	}

	instance.Status = status
	if err := m.modelRepo.Save(ctx, instance); err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	// Invalidate ChatModel cache when status changes.
	m.chatModelCache.Delete(ref.String())

	logger.Info("[LLM] model %s status updated to %s", ref, status)
	return nil
}

// --- Lifecycle ---

// Initialize loads all providers and models from configuration + registry.
//
// The flow mirrors K8S scheduler initialization:
// 1. Walk the Registry to discover all in-tree providers
// 2. For each registered provider, check if env var (API key) is available
// 3. If mode is "merge", register discovered providers (unless overridden by user config)
// 4. Apply user-configured providers (may override in-tree defaults)
// 5. Set the default model
func (m *modelManagerImpl) Initialize(ctx context.Context) error {
	if m.opts == nil {
		logger.Info("[LLM] no model options provided, skipping initialization")
		return nil
	}

	logger.Info("[LLM] initializing model manager (mode=%s, user_providers=%d, registry_plugins=%d)",
		m.opts.Mode, len(m.opts.Providers), m.registry.Len())

	// Phase 1: Register built-in providers from Registry (if mode is "merge").
	if m.opts.Mode != "replace" {
		if err := m.registerFromRegistry(ctx); err != nil {
			return fmt.Errorf("register from registry: %w", err)
		}
	}

	// Phase 2: Register user-configured providers.
	for providerID, providerCfg := range m.opts.Providers {
		if err := m.registerProviderFromConfig(ctx, providerID, providerCfg); err != nil {
			logger.Warn("[LLM] failed to register user provider %q: %v", providerID, err)
			continue
		}
	}

	// Phase 3: Set default model if configured.
	if m.opts.DefaultProvider != "" && m.opts.DefaultModel != "" {
		ref := entity.ModelRef{ProviderID: m.opts.DefaultProvider, ModelID: m.opts.DefaultModel}
		if inst, err := m.modelRepo.FindByRef(ctx, ref); err == nil {
			if err := m.modelRepo.SetDefault(ctx, inst.ID); err != nil {
				logger.Warn("[LLM] failed to set default model %s: %v", ref, err)
			} else {
				logger.Info("[LLM] default model set to: %s", ref)
			}
		} else {
			logger.Warn("[LLM] configured default model %s not found", ref)
		}
	}

	// Log summary.
	allModels, _ := m.modelRepo.FindAll(ctx)
	allProviders, _ := m.providerRepo.FindAll(ctx)
	logger.Info("[LLM] initialization complete: %d providers, %d models", len(allProviders), len(allModels))

	// Phase 4: Refresh compat manager rules (plugins may have added new rules during init).
	m.compatMgr = NewCompatManager(m.registry)

	return nil
}

// registerFromRegistry walks the provider.Registry, instantiates each plugin,
// and auto-discovers providers whose API keys are available in the environment.
func (m *modelManagerImpl) registerFromRegistry(ctx context.Context) error {
	m.registry.Range(func(name string, factory spi.PluginFactory) bool {
		// Skip if user has explicitly configured this provider.
		if _, exists := m.opts.Providers[name]; exists {
			logger.Info("[LLM] skipping registry plugin %q (overridden by user config)", name)
			return true
		}

		// Instantiate the plugin (like K8S calling PluginFactory).
		plugin := factory()

		// Get the default config (includes env var reference for API key).
		defaultCfg := plugin.DefaultConfig()

		// Only register if API key is available in environment.
		apiKey := helper.ResolveEnvValue(defaultCfg.APIKey)
		if apiKey == "" {
			return true
		}

		// Resolve the API key value in config.
		defaultCfg.APIKey = apiKey

		logger.Info("[LLM] auto-discovered provider from registry: %s", name)

		// Build provider entity via the plugin.
		providerEntity, err := plugin.BuildProvider(defaultCfg)
		if err != nil {
			logger.Warn("[LLM] failed to build provider %q: %v", name, err)
			return true
		}

		if err := m.RegisterProvider(ctx, providerEntity); err != nil {
			logger.Warn("[LLM] failed to register provider %q: %v", name, err)
			return true
		}

		// Build and register all models via the plugin.
		models, err := plugin.BuildModels(providerEntity, defaultCfg)
		if err != nil {
			logger.Warn("[LLM] failed to build models for %q: %v", name, err)
			return true
		}

		for _, model := range models {
			if _, err := m.RegisterModel(ctx, model); err != nil {
				logger.Warn("[LLM] failed to register model %s/%s: %v", name, model.ModelID, err)
			}
		}

		return true
	})

	return nil
}

// registerProviderFromConfig handles user-provided ProviderConfig.
// It checks whether a matching plugin exists in the Registry for enhanced behavior,
// otherwise falls back to generic construction via helper.BasePlugin.
func (m *modelManagerImpl) registerProviderFromConfig(ctx context.Context, providerID string, cfg *options.ProviderConfig) error {
	cfg.APIKey = helper.ResolveEnvValue(cfg.APIKey)

	// Try to find a matching plugin in the registry for provider-specific behavior.
	var plugin spi.ProviderPlugin

	factory, err := m.registry.Get(providerID)
	if err == nil {
		plugin = factory()
	} else {
		// No registered plugin — use a generic BasePlugin.
		plugin = &helper.BasePlugin{PluginName: providerID}
	}

	// Build provider entity.
	providerEntity, buildErr := plugin.BuildProvider(cfg)
	if buildErr != nil {
		return fmt.Errorf("build provider %q: %w", providerID, buildErr)
	}

	if err := m.RegisterProvider(ctx, providerEntity); err != nil {
		return err
	}

	// Build and register all models.
	models, buildErr := plugin.BuildModels(providerEntity, cfg)
	if buildErr != nil {
		return fmt.Errorf("build models for %q: %w", providerID, buildErr)
	}

	for _, model := range models {
		if _, err := m.RegisterModel(ctx, model); err != nil {
			logger.Warn("[LLM] failed to register model %s/%s: %v", providerID, model.ModelID, err)
		}
	}

	return nil
}
