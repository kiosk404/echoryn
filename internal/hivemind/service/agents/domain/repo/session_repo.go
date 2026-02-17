package repo

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
)

// SessionRepository defines the persistence interface for Session entities.
type SessionRepository interface {
	// Create stores a new session.
	Create(ctx context.Context, session *entity.Session) error
	// Get retrieves a session by ID.
	Get(ctx context.Context, id string) (*entity.Session, error)
	// Update updates an existing session.
	Update(ctx context.Context, session *entity.Session) error
	// Delete removes a session by ID.
	Delete(ctx context.Context, id string) error
	// ListByAgent returns all sessions for a given agent.
	ListByAgent(ctx context.Context, agentID string) ([]*entity.Session, error)
}
