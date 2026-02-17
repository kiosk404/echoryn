package service

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

type ModelManager interface {
	// --- Provider Management ---

	// RegisterProvider registers a new LLM provider
	RegisterProvider(ctx context.Context, provider *entity.ModelProvider) error
	// GetProvider retrieves the LLM provider by its ID.
	GetProvider(ctx context.Context, providerID string) (*entity.ModelProvider, error)
	// ListProviders lists all registered LLM providers.
	ListProviders(ctx context.Context) ([]*entity.ModelProvider, error)

	// --- Model Management ---

	// RegisterModel registers a new LLM model.
	RegisterModel(ctx context.Context, instance *entity.ModelInstance) (int64, error)
	// GetModelByID retrieves the LLM model by its ID.
	GetModelByID(ctx context.Context, id int64) (*entity.ModelInstance, error)
	// GetModelByRef retrieves a model by provider+model reference.
	GetModelByRef(ctx context.Context, ref entity.ModelRef) (*entity.ModelInstance, error)
	// GetDefaultModel retrieves the default LLM model.
	GetDefaultModel(ctx context.Context) (*entity.ModelInstance, error)
	// SetDefaultModel sets the default LLM model.
	SetDefaultModel(ctx context.Context, id int64) error
	// ListModelsByProvider lists all LLM models of the given provider.
	ListModelsByProvider(ctx context.Context, providerID string) ([]*entity.ModelInstance, error)
	// ListModelsByType lists all LLM models of the given type.
	ListModelsByType(ctx context.Context, modelType entity.ModelType) ([]*entity.ModelInstance, error)
	// ListAllModels lists all registered LLM models.
	ListAllModels(ctx context.Context) ([]*entity.ModelInstance, error)

	// --- ChatModel (Eino) ---

	// GetChatModel returns a cached Eino BaseChatModel for the given model reference.
	// Instance are lazily created and cached.
	// For custom LLM params, use BuildChatModel instead.
	// Callers needing tool-calling should assert ToolCallingChatModel on the result
	GetChatModel(ctx context.Context, ref entity.ModelRef) (model.BaseChatModel, error)

	// BuildChatModel creates a new Eino BaseChatModel instance with the given params.
	// Unlike GetChatModel, this always create a new instance. (not cached).
	// because different params produce different models.
	BuildChatModel(ctx context.Context, ref entity.ModelRef, params *entity.LLMParams) (model.BaseChatModel, error)

	// GetDefaultChatModel returns the Eino BaseChatModel for the default model.
	GetDefaultChatModel(ctx context.Context) (model.BaseChatModel, error)

	// --- Model Status ---

	// ResolveCompat resolves the compatibility rules for the given model reference.
	// This normalize API differences across providers into a common format.
	ResolveCompat(ctx context.Context, ref entity.ModelRef) (*entity.ModelCompatConfig, error)

	// --- Model Status ---

	// SetModelStatus sets the status of the given model reference. (Ready/Disabled/Error/Cooldown)
	SetModelStatus(ctx context.Context, ref entity.ModelRef, status entity.ModelStatus) error

	// -- Lifecycle Management ---

	// Initialize initializes the model manager.
	// This should be called once after all providers are registered.
	Initialize(ctx context.Context) error
}
