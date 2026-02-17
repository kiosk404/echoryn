package inmemory

import (
	"context"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
)

// SessionStore is an in-memory implementation of the SessionStore interface.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*entity.Session
}

// NewSessionStore creates a new instance of the SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*entity.Session),
	}
}

func (s *SessionStore) Create(_ context.Context, session *entity.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *SessionStore) Get(_ context.Context, id string) (*entity.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, errno.ErrSessionNotFound
	}
	return session, nil
}

func (s *SessionStore) Update(_ context.Context, session *entity.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Check if the session exists
	if _, ok := s.sessions[session.ID]; !ok {
		return errno.ErrSessionNotFound
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *SessionStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Check if the session exists
	if _, ok := s.sessions[id]; !ok {
		return errno.ErrSessionNotFound
	}
	delete(s.sessions, id)
	return nil
}

func (s *SessionStore) ListByAgent(_ context.Context, agentID string) ([]*entity.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sessions := make([]*entity.Session, 0)
	for _, session := range s.sessions {
		if session.AgentID == agentID {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}
