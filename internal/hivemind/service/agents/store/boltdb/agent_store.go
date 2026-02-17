package boltdb

import (
	"context"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// AgentStore implements the AgentRepository interface using BoltDB.
type AgentStore struct {
	db *bolt.DB
}

// NewAgentStore creates a new BoltDB-backed AgentStore.
func NewAgentStore(db *DB) *AgentStore {
	return &AgentStore{db: db.Bolt()}
}

// Create adds a new agent to the store.
func (s *AgentStore) Create(_ context.Context, agent *entity.Agent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAgentStore)
		data, err := json.Marshal(agent)
		if err != nil {
			return fmt.Errorf("failed to marshal agent: %w", err)
		}
		return b.Put([]byte(agent.ID), data)
	})
}

// Get retrieves an agent by its ID.
func (s *AgentStore) Get(_ context.Context, id string) (*entity.Agent, error) {
	var agent entity.Agent
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAgentStore)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("agent %q not found", id)
		}
		return json.Unmarshal(data, &agent)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent: %w", err)
	}
	return &agent, nil
}

// Update modifies an existing agent in the store.
func (s *AgentStore) Update(_ context.Context, agent *entity.Agent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAgentStore)
		// Check if agent exists
		if b.Get([]byte(agent.ID)) == nil {
			return fmt.Errorf("agent %q not found", agent.ID)
		}

		data, err := json.Marshal(agent)
		if err != nil {
			return fmt.Errorf("failed to marshal agent: %w", err)
		}
		return b.Put([]byte(agent.ID), data)
	})
}

// Delete removes an agent from the store.
func (s *AgentStore) Delete(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAgentStore)
		return b.Delete([]byte(id))
	})
}

// List returns all agents in the store.
func (s *AgentStore) List(_ context.Context) ([]*entity.Agent, error) {
	var agents []*entity.Agent
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAgentStore)
		return b.ForEach(func(k, v []byte) error {
			var agent entity.Agent
			if err := json.Unmarshal(v, &agent); err != nil {
				return fmt.Errorf("failed to unmarshal agent: %w", err)
			}
			agents = append(agents, &agent)
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	return agents, nil
}
