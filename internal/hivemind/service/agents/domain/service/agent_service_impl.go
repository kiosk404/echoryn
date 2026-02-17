package service

import (
	"context"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
)

// agentServiceImpl implements the AgentService interface.
type agentServiceImpl struct {
	agentRepo   repo.AgentRepository
	sessionRepo repo.SessionRepository
	runRepo     repo.RunRepository
	runner      *runtime.AgentRunner
}

func NewAgentService(agentRepo repo.AgentRepository,
	sessionRepo repo.SessionRepository,
	runRepo repo.RunRepository, runner *runtime.AgentRunner) AgentService {
	return &agentServiceImpl{
		agentRepo:   agentRepo,
		sessionRepo: sessionRepo,
		runRepo:     runRepo,
		runner:      runner,
	}
}

func (a agentServiceImpl) CreateAgent(ctx context.Context, agent *entity.Agent) error {
	return a.agentRepo.Create(ctx, agent)
}

func (a agentServiceImpl) GetAgent(ctx context.Context, id string) (*entity.Agent, error) {
	return a.agentRepo.Get(ctx, id)
}

func (a agentServiceImpl) ListAgents(ctx context.Context) ([]*entity.Agent, error) {
	return a.agentRepo.List(ctx)
}

func (a agentServiceImpl) UpdateAgent(ctx context.Context, agent *entity.Agent) error {
	return a.agentRepo.Update(ctx, agent)
}

func (a agentServiceImpl) DeleteAgent(ctx context.Context, id string) error {
	return a.agentRepo.Delete(ctx, id)
}

func (a agentServiceImpl) GetSession(ctx context.Context, id string) (*entity.Session, error) {
	return a.sessionRepo.Get(ctx, id)
}

func (a agentServiceImpl) ListSessionsByAgent(ctx context.Context, agentID string) ([]*entity.Session, error) {
	return a.sessionRepo.ListByAgent(ctx, agentID)
}

func (a agentServiceImpl) DeleteSession(ctx context.Context, id string) error {
	return a.sessionRepo.Delete(ctx, id)
}

func (a agentServiceImpl) Run(ctx context.Context, req *runtime.RunRequest) (*schema.StreamReader[*entity.AgentEvent], error) {
	return a.runner.Run(ctx, req)
}

func (a agentServiceImpl) GetRun(ctx context.Context, id string) (*entity.Run, error) {
	return a.runRepo.Get(ctx, id)
}

func (a agentServiceImpl) ListRunsBySession(ctx context.Context, sessionID string) ([]*entity.Run, error) {
	return a.runRepo.ListBySession(ctx, sessionID)
}
