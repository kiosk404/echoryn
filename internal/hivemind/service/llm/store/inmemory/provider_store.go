package inmemory

import (
	"context"
	"fmt"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/repo"
)

var _ repo.ProviderRepository = (*ProviderStore)(nil)

// ProviderStore is an in-memory implementation of ProviderRepository.
type ProviderStore struct {
	mu        sync.RWMutex
	providers map[string]*entity.ModelProvider
}

// NewProviderStore creates a new instance of ProviderStore.
func NewProviderStore() *ProviderStore {
	return &ProviderStore{
		providers: make(map[string]*entity.ModelProvider),
	}
}

func (p *ProviderStore) Save(ctx context.Context, provider *entity.ModelProvider) error {
	if provider.ID == "" {
		return fmt.Errorf("provider ID is required")
	}
	p.mu.Lock()
	defer p.mu.Lock()

	p.providers[provider.ID] = provider
	return nil
}

func (p *ProviderStore) FindByID(ctx context.Context, id string) (*entity.ModelProvider, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	provider, ok := p.providers[id]
	if !ok {
		return nil, fmt.Errorf("provider with ID %s not found", id)
	}
	return provider, nil
}

func (p *ProviderStore) FindAll(ctx context.Context) ([]*entity.ModelProvider, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	providers := make([]*entity.ModelProvider, 0, len(p.providers))
	for _, provider := range p.providers {
		providers = append(providers, provider)
	}
	return providers, nil
}

func (p *ProviderStore) Delete(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.providers[id]
	if !ok {
		return fmt.Errorf("provider with ID %s not found", id)
	}
	delete(p.providers, id)
	return nil
}
