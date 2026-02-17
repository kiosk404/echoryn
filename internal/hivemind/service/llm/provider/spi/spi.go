package spi

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/pkg/options"
)

// ProviderPlugin is the interface for provider plugins.
type ProviderPlugin interface {
	// Name returns the name of the provider plugin.
	Name() string
	// DefaultConfig returns the default configuration for the provider plugin.
	DefaultConfig() *options.ProviderConfig
	// BuildProvider builds a model provider instance.
	BuildProvider(cfg *options.ProviderConfig) (*entity.ModelProvider, error)
	// BuildModels builds model instances for the given provider.
	BuildModels(provider *entity.ModelProvider, cfg *options.ProviderConfig) ([]*entity.ModelInstance, error)
}

// ChatModelPlugin extends ProviderPlugin with the ability to build Eino BaseChatModel
// instances for actual LLM inference ( Generate, StreamGenerate, etc.)
type ChatModelPlugin interface {
	ProviderPlugin
	// BuildChatModel builds a BaseChatModel instance for the given model instance and provider
	// params may be nil, in which case provider defaults are used.
	// The returned BaseChatModel supports Generate and Stream.
	// Most providers also implement BaseChatModel, but some may require additional
	// configuration to be passed in params.
	BuildChatModel(ctx context.Context, instance *entity.ModelInstance, provider *entity.ModelProvider, params *entity.LLMParams) (model.BaseChatModel, error)
}

// CompatPlugin extends ProviderPlugin with the ability to provide compatibility rules
// for model instances. This allows each provider to declare API quirks and behavioral
// differences from other providers.
type CompatPlugin interface {
	ProviderPlugin
	// CompatRules returns the compatibility rules for the provider plugin.
	CompatRules() []entity.ModelCompatRule
}

type ProbePlugin interface {
	ProviderPlugin
	// Probe performs a lightweight health check on the model instance.
	// It should send a minimal request to verify the model is responding.
	// The timeout context should be respected.
	Probe(ctx context.Context, instance *entity.ModelInstance, plugin *entity.ModelProvider) (*entity.ProbeResult, error)
}

// PluginFactory is a function that creates a ProviderPlugin instance.
type PluginFactory func() ProviderPlugin
