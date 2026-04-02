package integration

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/subagent"
	"github.com/kiosk404/echoryn/internal/hivemind/service/messagebus"
	"github.com/kiosk404/echoryn/internal/hivemind/service/team"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
)

// TeamSubsystemDeps holds all the wired components for the Team subsystem.
type TeamSubsystemDeps struct {
	Orchestrator    team.TeamOrchestrator
	TemplateService team.TeamTemplateService
	MessageBus      messagebus.MessageBus
	AppService      team.TeamApplicationService
	WorkerIndex     *WorkerIndex
	ExecutionPort   team.ExecutionPort
	EventBridge     *team.EventBridge
	PolicyService   *team.DefaultPolicyService
}

// AssembleTeamSubsystem is the single entry point that replaces the old
// injectTeamDependencies() in initializers.go.
//
// It creates all Team BC, Collaboration BC, and cross-BC adapters,
// then wires them together. After this call, the entire Team subsystem is ready.
//
// Call sequence:
//  1. Create TeamRegistry + TemplateService + load templates
//  2. Create MessageBus
//  3. Create Orchestrator (with PolicyService)
//  4. Wire MessageBus resolver
//  5. Wire cross-BC adapters (ExecutionPort + EventBridge)
//  6. Create TeamApplicationService Facade
func AssembleTeamSubsystem(
	ctx context.Context,
	subAgentManager subagent.Manager,
	sessionRepo repo.SessionRepository,
	defaultAgentID string,
) (*TeamSubsystemDeps, error) {
	// 1. Team BC: registry + template service + load templates
	teamRegistry := team.NewInMemoryTeamRegistry()
	templateSvc := team.NewTemplateService(teamRegistry)

	templateDirs := paths.ResolveTemplatesDirs()
	loader := team.NewTemplateLoader(teamRegistry, templateDirs...)
	count, err := loader.LoadAll(ctx)
	if err != nil {
		logger.Warn("[Integration] template loading errors: %v", err)
	}
	if count > 0 {
		logger.Info("[Integration] loaded %d team templates from %v", count, templateDirs)
	}

	// 2. Collaboration BC: message bus
	bus := messagebus.NewMessageBus(nil)

	// 3. Team BC: orchestrator with policy
	policyService := team.NewDefaultPolicyService()
	orchestrator := team.NewOrchestrator(teamRegistry, templateSvc, subAgentManager, sessionRepo, bus, defaultAgentID)

	// 4. Wire message bus resolver
	if settable, ok := bus.(messagebus.ResolverSetter); ok {
		settable.SetTeamMemberResolver(orchestrator.(messagebus.TeamMemberResolver))
	}

	// 5. Cross-BC: ExecutionPort adapter
	workerIndex := NewWorkerIndex()
	execPort := NewTeamExecutionAdapter(subAgentManager, workerIndex)
	orchestrator.SetExecutionPort(execPort)
	logger.Info("[Integration] wired ExecutionPort: team.Orchestrator → subagent.Manager (via adapter)")

	// 6. Cross-BC: EventBridge adapter
	eventBridge := team.NewEventBridge(orchestrator, teamRegistry)
	subAgentManager.SetLifecycleHook(eventBridge)
	if setter, ok := orchestrator.(interface {
		SetEventBridge(bridge *team.EventBridge)
	}); ok {
		setter.SetEventBridge(eventBridge)
	}
	logger.Info("[Integration] wired EventBridge: subagent.LifecycleHook → team.NotifyMemberCompleted")

	// 7. Application Service Facade
	appService := team.NewTeamApplicationService(orchestrator, templateSvc)

	logger.Info("[Integration] Team subsystem fully assembled")

	return &TeamSubsystemDeps{
		Orchestrator:    orchestrator,
		TemplateService: templateSvc,
		MessageBus:      bus,
		AppService:      appService,
		WorkerIndex:     workerIndex,
		ExecutionPort:   execPort,
		EventBridge:     eventBridge,
		PolicyService:   policyService,
	}, nil
}

// SetupTeamTranscript creates and registers the TranscriptPlugin onto the message bus.
// This is separated from AssembleTeamSubsystem because it depends on plugin config.
func SetupTeamTranscript(bus messagebus.MessageBus, enabled bool, outputDir string) {
	if !enabled {
		logger.Info("[Integration] team transcript disabled")
		return
	}
	transcript := messagebus.NewTranscriptPlugin(messagebus.TranscriptConfig{
		Enabled:   true,
		OutputDir: outputDir,
		Format:    "markdown",
	})
	if hookable, ok := bus.(messagebus.HookRegistrar); ok {
		hookable.RegisterHook(messagebus.HookMessageSent, transcript.OnMessageSent)
		hookable.RegisterHook(messagebus.HookMessageBroadcast, transcript.OnMessageBroadcast)
		logger.Info("[Integration] team transcript enabled (output_dir=%s)", outputDir)
	}
}
