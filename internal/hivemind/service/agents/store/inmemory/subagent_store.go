package inmemory

import (
	"context"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
)

// SubAgentStore is an in-memory implementation of service.SubAgentRegistry.
type SubAgentStore struct {
	mu      sync.RWMutex
	records map[string]*entity.SubAgentRecord
}

func NewSubAgentStore() *SubAgentStore {
	return &SubAgentStore{
		records: make(map[string]*entity.SubAgentRecord),
	}
}

func (s *SubAgentStore) Save(_ context.Context, record *entity.SubAgentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.ID] = record
	return nil
}

func (s *SubAgentStore) Get(_ context.Context, id string) (*entity.SubAgentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	if !ok {
		return nil, errno.ErrSubAgentNotFound
	}
	return r, nil
}

func (s *SubAgentStore) ListByParent(_ context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*entity.SubAgentRecord
	for _, r := range s.records {
		if r.ParentSessionID == parentSessionID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *SubAgentStore) ListNonTerminal(_ context.Context) ([]*entity.SubAgentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*entity.SubAgentRecord
	for _, r := range s.records {
		if !r.Status.IsTerminal() {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *SubAgentStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}
