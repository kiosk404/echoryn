package inmemory

import (
	"context"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
)

// AgentStore is an in-memory implementation of repo.AgentRepository.
type AgentStore struct {
	mu     sync.RWMutex
	agents map[string]*entity.Agent
}

// NewAgentStore creates a new AgentStore instance.
func NewAgentStore() *AgentStore {
	return &AgentStore{
		agents: make(map[string]*entity.Agent),
	}
}

// Create creates a new agent.
func (s *AgentStore) Create(_ context.Context, agent *entity.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agent.ID] = agent
	return nil
}

// Get returns an agent by ID.
func (s *AgentStore) Get(_ context.Context, id string) (*entity.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.agents[id]
	if !ok {
		return nil, errno.ErrAgentNotFound
	}
	return agent, nil
}

// Update updates an agent.
func (s *AgentStore) Update(_ context.Context, agent *entity.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[agent.ID]; !ok {
		return errno.ErrAgentNotFound
	}
	s.agents[agent.ID] = agent
	return nil
}

// Delete deletes an agent by ID.
func (s *AgentStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[id]; !ok {
		return errno.ErrAgentNotFound
	}
	delete(s.agents, id)
	return nil
}

// List returns all agents.
func (s *AgentStore) List(_ context.Context) ([]*entity.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agents := make([]*entity.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, agent)
	}
	return agents, nil
}
