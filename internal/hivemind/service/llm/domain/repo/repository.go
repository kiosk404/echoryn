package repo

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// ModelRepository defines the repository interface for model instances.
// Following DDD, this is a domain-layer port; the implementation is an infrastructure adapter.
type ModelRepository interface {
	// Save persists a model instance. If the ID already exist, it updates.
	Save(ctx context.Context, instance *entity.ModelInstance) error
	// FindByID retrieves a model instance by its ID.
	FindByID(ctx context.Context, id int64) (*entity.ModelInstance, error)
	// FindByRef retrieves a model instance by its reference.
	FindByRef(ctx context.Context, ref entity.ModelRef) (*entity.ModelInstance, error)
	// FindDefault retrieves the default model instance.
	FindDefault(ctx context.Context) (*entity.ModelInstance, error)
	// FindAllByProvider retrieves all model instances for a specific provider.
	FindAllByProvider(ctx context.Context, providerID string) ([]*entity.ModelInstance, error)
	// FindAllByType retrieves all model instances of a specific type.
	// FindAllByType retrieves all model instances of a specific type.
	FindAllByType(ctx context.Context, modelType entity.ModelType) ([]*entity.ModelInstance, error)
	// FindAll retrieves all model instances.
	FindAll(ctx context.Context) ([]*entity.ModelInstance, error)
	// Delete removes a model instance by its ID.
	Delete(ctx context.Context, id int64) error
	// SetDefault sets the default model instance by its ID.
	SetDefault(ctx context.Context, id int64) error
}

// ProviderRepository defines the repository interface for model providers.
type ProviderRepository interface {
	// Save persists a model provider. If the ID already exist, it updates.
	Save(ctx context.Context, provider *entity.ModelProvider) error
	// FindByID retrieves a model provider by its ID.
	FindByID(ctx context.Context, id string) (*entity.ModelProvider, error)
	// FindAll retrieves all model providers.
	FindAll(ctx context.Context) ([]*entity.ModelProvider, error)
	// Delete removes a model provider by its ID.
	Delete(ctx context.Context, id string) error
}
