package repo

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
)

// AgentRepository defines the persistence interface for Agent entities.
type AgentRepository interface {
	// Create stores a new agent.
	Create(ctx context.Context, agent *entity.Agent) error
	// Get retrieves an agent by ID.
	Get(ctx context.Context, id string) (*entity.Agent, error)
	// Update updates an existing agent.
	Update(ctx context.Context, agent *entity.Agent) error
	// Delete removes an agent by ID.
	Delete(ctx context.Context, id string) error
	// List returns all agents.
	List(ctx context.Context) ([]*entity.Agent, error)
}
