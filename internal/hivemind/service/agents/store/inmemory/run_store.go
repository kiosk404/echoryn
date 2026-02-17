package inmemory

import (
	"context"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
)

type RunStore struct {
	mu   sync.RWMutex
	runs map[string]*entity.Run
}

func NewRunStore() *RunStore {
	return &RunStore{
		runs: make(map[string]*entity.Run),
	}
}

func (s *RunStore) Create(_ context.Context, run *entity.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	return nil
}

func (s *RunStore) Get(_ context.Context, id string) (*entity.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
	if !ok {
		return nil, errno.ErrRunNotFound
	}
	return run, nil
}

func (s *RunStore) Update(_ context.Context, run *entity.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	return nil
}

func (s *RunStore) ListBySession(_ context.Context, sessionID string) ([]*entity.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runs := make([]*entity.Run, 0, len(s.runs))
	for _, run := range s.runs {
		if run.SessionID == sessionID {
			runs = append(runs, run)
		}
	}
	return runs, nil
}
