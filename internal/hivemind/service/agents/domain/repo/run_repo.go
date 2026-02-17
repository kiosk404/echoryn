package repo

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
)

// RunRepository defines the persistence interface for Run entities.
type RunRepository interface {
	// Create stores a new run.
	Create(ctx context.Context, run *entity.Run) error
	// Get retrieves a run by ID.
	Get(ctx context.Context, id string) (*entity.Run, error)
	// Update updates an existing run.
	Update(ctx context.Context, run *entity.Run) error
	// ListBySession returns all runs for a given session.
	ListBySession(ctx context.Context, sessionID string) ([]*entity.Run, error)
}
