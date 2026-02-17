package service

import (
	"context"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
)

// AgentService is the application-level service interface for agent management and execution.
//
// It provides:
// - Agent CRUD and management
// - Agent Session management
// - Run execution (delegates to AgentRunner)
type AgentService interface {
	// --- Agent CRUD ---

	CreateAgent(ctx context.Context, agent *entity.Agent) error
	GetAgent(ctx context.Context, id string) (*entity.Agent, error)
	ListAgents(ctx context.Context) ([]*entity.Agent, error)
	UpdateAgent(ctx context.Context, agent *entity.Agent) error
	DeleteAgent(ctx context.Context, id string) error

	// --- Session Management ---

	GetSession(ctx context.Context, id string) (*entity.Session, error)
	ListSessionsByAgent(ctx context.Context, agentID string) ([]*entity.Session, error)
	DeleteSession(ctx context.Context, id string) error

	// --- Run Execution ---

	// Run starts an agent execution and returns a streaming event reader.
	// Events are consumed via sr.Recv() until io.EOF is received.
	Run(ctx context.Context, req *runtime.RunRequest) (*schema.StreamReader[*entity.AgentEvent], error)

	// GetRun retrieves the details of a previously started run.
	GetRun(ctx context.Context, id string) (*entity.Run, error)

	// ListRunsBySession returns all runs for a session.
	ListRunsBySession(ctx context.Context, sessionID string) ([]*entity.Run, error)
}
