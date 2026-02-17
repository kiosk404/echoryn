package inmemory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/repo"
)

// Compile-time interface check
var _ repo.ModelRepository = (*ModelStore)(nil)

// ModelStore is an in-memory implementation of ModelRepository.
type ModelStore struct {
	mu        sync.RWMutex
	models    map[int64]*entity.ModelInstance
	refIndex  map[string]int64 // "provider/modelID" -> instance ID
	defaultID int64
	nextID    atomic.Int64
}

// NewModelStore creates a new in-memory model store.
func NewModelStore() *ModelStore {
	return &ModelStore{
		models:    make(map[int64]*entity.ModelInstance),
		refIndex:  make(map[string]int64),
		defaultID: 0,
		nextID:    atomic.Int64{},
	}
}

func refKey(providerID, modelID string) string {
	return providerID + "/" + modelID
}

func (m *ModelStore) Save(ctx context.Context, instance *entity.ModelInstance) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if instance.ID == 0 {
		instance.ID = m.nextID.Add(1) - 1
	}

	m.models[instance.ID] = instance
	m.refIndex[refKey(instance.ProviderID, instance.ModelID)] = instance.ID

	if instance.IsDefault {
		m.defaultID = instance.ID
	}
	return nil
}

func (m *ModelStore) FindByID(ctx context.Context, id int64) (*entity.ModelInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	instance, ok := m.models[id]
	if !ok {
		return nil, fmt.Errorf("model instance with ID %d not found", id)
	}
	return instance, nil
}

func (m *ModelStore) FindByRef(ctx context.Context, ref entity.ModelRef) (*entity.ModelInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.refIndex[refKey(ref.ProviderID, ref.ModelID)]
	if !ok {
		return nil, fmt.Errorf("model instance with ref %s not found", ref.String())
	}
	instance, ok := m.models[id]
	if !ok {
		return nil, fmt.Errorf("model instance with ID %d not found", id)
	}
	return instance, nil
}

func (m *ModelStore) FindDefault(ctx context.Context) (*entity.ModelInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultID == 0 {
		return nil, fmt.Errorf("default model instance not set")
	}
	instance, ok := m.models[m.defaultID]
	if !ok {
		return nil, fmt.Errorf("default model instance with ID %d not found", m.defaultID)
	}
	return instance, nil
}

func (m *ModelStore) FindAllByProvider(ctx context.Context, providerID string) ([]*entity.ModelInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*entity.ModelInstance
	for _, instance := range m.models {
		if instance.ProviderID == providerID {
			result = append(result, instance)
		}
	}
	return result, nil
}

func (m *ModelStore) FindAllByType(ctx context.Context, modelType entity.ModelType) ([]*entity.ModelInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*entity.ModelInstance
	for _, instance := range m.models {
		if instance.Type == modelType {
			result = append(result, instance)
		}
	}
	return result, nil
}

func (m *ModelStore) FindAll(ctx context.Context) ([]*entity.ModelInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*entity.ModelInstance = make([]*entity.ModelInstance, 0, len(m.models))
	for _, instance := range m.models {
		result = append(result, instance)
	}
	return result, nil
}

func (m *ModelStore) Delete(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	instance, ok := m.models[id]
	if !ok {
		return fmt.Errorf("model instance with ID %d not found", id)
	}
	delete(m.models, id)
	delete(m.refIndex, refKey(instance.ProviderID, instance.ModelID))
	if instance.IsDefault {
		m.defaultID = 0
	}
	return nil
}

func (m *ModelStore) SetDefault(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	instance, ok := m.models[id]
	if !ok {
		return fmt.Errorf("model instance with ID %d not found", id)
	}
	instance.IsDefault = true
	m.defaultID = id
	return nil
}
