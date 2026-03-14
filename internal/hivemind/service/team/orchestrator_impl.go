package team

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/subagent"
	"github.com/kiosk404/echoryn/internal/hivemind/service/messagebus"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// orchestratorImpl is the default implementation of TeamOrchestrator.
// It coordinates team lifecycle, spawns SubAgents, and manages member state.
//
// Design: K8s Deployment controller pattern
//   - InstantiateTeam(): like applying a Deployment spec
//   - Members: like Pods managed by a ReplicaSet
//   - WaitForAll(): like waiting for rollout complete
type orchestratorImpl struct {
	registry        TeamRegistry
	templateService TeamTemplateService
	subAgentManager subagent.Manager
	sessionRepo     repo.SessionRepository // for ensuring parent session exists
	messageBus      messagebus.MessageBus
	maxTeamMembers  int
	defaultAgentID  string       // default agent for external client sessions
	eventBridge     *EventBridge // optional: for auto-registering members

	mu      sync.RWMutex
	waitChs map[string]chan struct{} // teamID → completion signal
}

// NewOrchestrator creates a new TeamOrchestrator.
func NewOrchestrator(
	registry TeamRegistry,
	templateService TeamTemplateService,
	subAgentManager subagent.Manager,
	sessionRepo repo.SessionRepository,
	messageBus messagebus.MessageBus,
	defaultAgentID string,
) TeamOrchestrator {
	if defaultAgentID == "" {
		defaultAgentID = "main"
	}
	return &orchestratorImpl{
		registry:        registry,
		templateService: templateService,
		subAgentManager: subAgentManager,
		sessionRepo:     sessionRepo,
		messageBus:      messageBus,
		maxTeamMembers:  DefaultMaxTeamMembers,
		defaultAgentID:  defaultAgentID,
		waitChs:         make(map[string]chan struct{}),
	}
}

var _ TeamOrchestrator = (*orchestratorImpl)(nil)

// InstantiateTeam creates a team from a template.
//
// Flow:
//  1. Fetch TeamTemplate
//  2. Create Team instance
//  3. For each MemberSpec: apply overrides → Spawn SubAgent → create TeamMember
//  4. Set Leader
//  5. Register mailboxes
//  6. Persist to TeamRegistry
func (o *orchestratorImpl) InstantiateTeam(ctx context.Context, req *InstantiateTeamRequest) (*Team, error) {
	// 0. Ensure parent session exists (create if not).
	if err := o.ensureParentSession(ctx, req.ParentSessionID); err != nil {
		return nil, fmt.Errorf("failed to ensure parent session: %w", err)
	}

	// 1. Fetch template.
	template, err := o.templateService.GetTemplate(ctx, req.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	if template == nil {
		return nil, fmt.Errorf("template %s not found", req.TemplateID)
	}

	// 2. Create Team instance.
	now := time.Now()
	team := &Team{
		ID:              generateTeamID(),
		Name:            template.Name,
		TemplateID:      template.ID,
		TemplateVersion: template.Version,
		ParentSessionID: req.ParentSessionID,
		ParentRunID:     req.ParentRunID,
		TaskDescription: req.TaskDescription,
		Strategy:        template.DefaultStrategy,
		Status:          TeamStatusCreating,
		Members:         make([]*TeamMember, 0),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Apply strategy override.
	if req.StrategyOverride != "" && req.StrategyOverride.IsValid() {
		team.Strategy = req.StrategyOverride
	}

	// 3. Spawn members from template specs.
	for _, spec := range template.MemberSpecs {
		// Check if disabled by override.
		if override, ok := req.MemberOverrides[spec.ID]; ok && override.Disabled {
			logger.Info("[TeamOrchestrator] skipping disabled member spec: %s", spec.ID)
			continue
		}

		// Determine the number of instances to create.
		instanceCount := spec.EffectiveMinCount()

		for i := 0; i < instanceCount; i++ {
			member, err := o.spawnMember(ctx, team, spec, req.MemberOverrides[spec.ID], i)
			if err != nil {
				// If spawning fails, log and continue with other members.
				logger.Warn("[TeamOrchestrator] failed to spawn member %s[%d]: %v", spec.ID, i, err)
				continue
			}
			team.Members = append(team.Members, member)

			// Set leader.
			if spec.IsLeader && team.LeaderID == "" {
				team.LeaderID = member.ID
			}
		}
	}

	if len(team.Members) == 0 {
		return nil, fmt.Errorf("failed to spawn any team members")
	}

	// 4. Register mailboxes for all members.
	for _, member := range team.Members {
		o.messageBus.RegisterMailbox(member.SessionID)
	}

	// 5. Transition to active.
	team.Status = TeamStatusActive
	team.UpdatedAt = time.Now()

	// 6. Persist team.
	if err := o.registry.Save(ctx, team); err != nil {
		return nil, fmt.Errorf("failed to save team: %w", err)
	}

	// 7. Create wait channel.
	o.mu.Lock()
	o.waitChs[team.ID] = make(chan struct{})
	o.mu.Unlock()

	logger.Info("[TeamOrchestrator] instantiated team: id=%s, name=%s, template=%s, members=%d, strategy=%s",
		team.ID, team.Name, template.ID, len(team.Members), team.Strategy)

	return team, nil
}

// CreateTeam creates an ad-hoc team without a template.
func (o *orchestratorImpl) CreateTeam(ctx context.Context, req *CreateTeamRequest) (*Team, error) {
	if !req.Strategy.IsValid() {
		return nil, fmt.Errorf("invalid coordination strategy: %s", req.Strategy)
	}

	now := time.Now()
	team := &Team{
		ID:              generateTeamID(),
		Name:            req.Name,
		ParentSessionID: req.ParentSessionID,
		ParentRunID:     req.ParentRunID,
		TaskDescription: req.TaskDescription,
		Strategy:        req.Strategy,
		Status:          TeamStatusActive,
		Members:         make([]*TeamMember, 0),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := o.registry.Save(ctx, team); err != nil {
		return nil, fmt.Errorf("failed to save team: %w", err)
	}

	o.mu.Lock()
	o.waitChs[team.ID] = make(chan struct{})
	o.mu.Unlock()

	logger.Info("[TeamOrchestrator] created ad-hoc team: id=%s, name=%s, strategy=%s",
		team.ID, team.Name, team.Strategy)

	return team, nil
}

// DissolveTeam dissolves a team and cleans up resources.
func (o *orchestratorImpl) DissolveTeam(ctx context.Context, teamID string) error {
	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}
	if team == nil {
		return fmt.Errorf("team %s not found", teamID)
	}

	// Cancel all active members.
	for _, member := range team.Members {
		if !member.Status.IsTerminal() {
			if member.SubAgentRecordID != "" {
				if err := o.subAgentManager.Cancel(ctx, member.SubAgentRecordID); err != nil {
					logger.Warn("[TeamOrchestrator] failed to cancel member %s: %v", member.ID, err)
				}
			}
			member.MarkFailed()
		}
		// Unregister mailbox.
		o.messageBus.UnregisterMailbox(member.SessionID)
	}

	// Mark dissolved.
	team.MarkDissolved(&TeamResult{
		Summary: "Team dissolved",
		Success: false,
	})

	// Unregister EventBridge entries for this team.
	if o.eventBridge != nil {
		o.eventBridge.UnregisterTeam(teamID)
	}

	if err := o.registry.Save(ctx, team); err != nil {
		return fmt.Errorf("failed to save dissolved team: %w", err)
	}

	// Signal waiters.
	o.mu.Lock()
	if ch, ok := o.waitChs[teamID]; ok {
		close(ch)
		delete(o.waitChs, teamID)
	}
	o.mu.Unlock()

	logger.Info("[TeamOrchestrator] dissolved team: id=%s", teamID)
	return nil
}

func (o *orchestratorImpl) GetTeam(ctx context.Context, teamID string) (*Team, error) {
	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}
	return team, nil
}

// AddMember adds a new member to an existing team.
func (o *orchestratorImpl) AddMember(ctx context.Context, teamID string, req *AddMemberRequest) (*TeamMember, error) {
	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}
	if team == nil {
		return nil, fmt.Errorf("team %s not found", teamID)
	}
	if team.Status.IsTerminal() {
		return nil, fmt.Errorf("team %s is dissolved", teamID)
	}

	// Check member limit.
	if len(team.Members) >= o.maxTeamMembers {
		return nil, fmt.Errorf("team %s has reached max member limit (%d)", teamID, o.maxTeamMembers)
	}

	// Spawn the SubAgent.
	spawnReq := &entity.SubAgentSpawnRequest{
		ParentSessionID: team.ParentSessionID,
		ParentRunID:     team.ParentRunID,
		Task:            req.Task,
		Label:           req.Label,
	}
	if req.AgentID != "" {
		spawnReq.AgentID = req.AgentID
	}
	if req.Model != "" {
		spawnReq.Model = req.Model
	}

	record, err := o.subAgentManager.Spawn(ctx, spawnReq)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn SubAgent: %w", err)
	}

	member := &TeamMember{
		ID:               generateMemberID(),
		SubAgentRecordID: record.ID,
		SessionID:        record.SessionID,
		AgentID:          record.AgentID,
		Role:             req.Role,
		Label:            req.Label,
		Task:             req.Task,
		Status:           TeamMemberStatusRunning,
		JoinedAt:         time.Now(),
	}

	// Register mailbox.
	o.messageBus.RegisterMailbox(member.SessionID)

	// Register in EventBridge for lifecycle tracking.
	if o.eventBridge != nil {
		o.eventBridge.RegisterMember(teamID, member.ID, member.SessionID)
	}

	// Add to registry.
	if err := o.registry.AddMember(ctx, teamID, member); err != nil {
		return nil, fmt.Errorf("failed to add member to registry: %w", err)
	}

	// Update leader if requested.
	if req.IsLeader {
		// Re-fetch team to avoid overwriting the member we just added.
		freshTeam, err := o.registry.Get(ctx, teamID)
		if err == nil && freshTeam != nil {
			freshTeam.LeaderID = member.ID
			if err := o.registry.Save(ctx, freshTeam); err != nil {
				logger.Warn("[TeamOrchestrator] failed to update leader: %v", err)
			}
		}
	}

	logger.Info("[TeamOrchestrator] added member: team=%s, member=%s, role=%s", teamID, member.ID, member.Role)
	return member, nil
}

// RemoveMember removes a member from a team.
func (o *orchestratorImpl) RemoveMember(ctx context.Context, teamID string, memberID string) error {
	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}
	if team == nil {
		return fmt.Errorf("team %s not found", teamID)
	}

	member := team.GetMember(memberID)
	if member == nil {
		return fmt.Errorf("member %s not found in team %s", memberID, teamID)
	}

	// Cancel SubAgent if still running.
	if !member.Status.IsTerminal() && member.SubAgentRecordID != "" {
		if err := o.subAgentManager.Cancel(ctx, member.SubAgentRecordID); err != nil {
			logger.Warn("[TeamOrchestrator] failed to cancel member SubAgent: %v", err)
		}
	}

	// Unregister mailbox.
	o.messageBus.UnregisterMailbox(member.SessionID)

	// Remove from registry.
	if err := o.registry.RemoveMember(ctx, teamID, memberID); err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	logger.Info("[TeamOrchestrator] removed member: team=%s, member=%s", teamID, memberID)
	return nil
}

// ScaleMembers scales the number of instances for a specific member spec.
func (o *orchestratorImpl) ScaleMembers(ctx context.Context, teamID string, specID string, count int) error {
	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}
	if team == nil {
		return fmt.Errorf("team %s not found", teamID)
	}

	// Count current instances of this spec.
	current := 0
	for _, m := range team.Members {
		if m.SpecID == specID && !m.Status.IsTerminal() {
			current++
		}
	}

	if count > current {
		// Scale up: need to spawn more members.
		// Requires template to know the spec details.
		if team.TemplateID == "" {
			return fmt.Errorf("scale requires a template-based team")
		}
		template, err := o.templateService.GetTemplate(ctx, team.TemplateID)
		if err != nil {
			return fmt.Errorf("failed to get template: %w", err)
		}
		if template == nil {
			return fmt.Errorf("template %s not found", team.TemplateID)
		}

		var spec *MemberSpec
		for _, s := range template.MemberSpecs {
			if s.ID == specID {
				spec = s
				break
			}
		}
		if spec == nil {
			return fmt.Errorf("member spec %s not found in template", specID)
		}

		for i := current; i < count; i++ {
			member, err := o.spawnMember(ctx, team, spec, nil, i)
			if err != nil {
				logger.Warn("[TeamOrchestrator] scale up failed for spec %s[%d]: %v", specID, i, err)
				continue
			}
			if err := o.registry.AddMember(ctx, teamID, member); err != nil {
				logger.Warn("[TeamOrchestrator] failed to persist scaled member: %v", err)
			}
			o.messageBus.RegisterMailbox(member.SessionID)
		}
	} else if count < current {
		// Scale down: remove excess members.
		toRemove := current - count
		for _, m := range team.Members {
			if toRemove <= 0 {
				break
			}
			if m.SpecID == specID && !m.Status.IsTerminal() {
				if err := o.RemoveMember(ctx, teamID, m.ID); err != nil {
					logger.Warn("[TeamOrchestrator] scale down failed for member %s: %v", m.ID, err)
				} else {
					toRemove--
				}
			}
		}
	}

	logger.Info("[TeamOrchestrator] scaled spec %s: %d → %d in team %s", specID, current, count, teamID)
	return nil
}

// WaitForAll waits for all team members to complete.
func (o *orchestratorImpl) WaitForAll(ctx context.Context, teamID string) (*TeamResult, error) {
	o.mu.RLock()
	ch, ok := o.waitChs[teamID]
	o.mu.RUnlock()

	if !ok {
		// Team might already be dissolved.
		team, err := o.registry.Get(ctx, teamID)
		if err != nil {
			return nil, err
		}
		if team != nil && team.Result != nil {
			return team.Result, nil
		}
		return nil, fmt.Errorf("team %s not found or no wait channel", teamID)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ch:
		team, err := o.registry.Get(ctx, teamID)
		if err != nil {
			return nil, err
		}
		if team != nil && team.Result != nil {
			return team.Result, nil
		}
		return &TeamResult{Summary: "Team completed", Success: true}, nil
	}
}

// SetCoordinationStrategy updates the team's strategy.
func (o *orchestratorImpl) SetCoordinationStrategy(ctx context.Context, teamID string, strategy CoordinationStrategy) error {
	if !strategy.IsValid() {
		return fmt.Errorf("invalid coordination strategy: %s", strategy)
	}

	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}
	if team == nil {
		return fmt.Errorf("team %s not found", teamID)
	}

	team.Strategy = strategy
	team.UpdatedAt = time.Now()

	if err := o.registry.Save(ctx, team); err != nil {
		return fmt.Errorf("failed to save team: %w", err)
	}

	logger.Info("[TeamOrchestrator] updated strategy: team=%s, strategy=%s", teamID, strategy)
	return nil
}

// GetTeamMemberSessionIDs implements messagebus.TeamMemberResolver.
func (o *orchestratorImpl) GetTeamMemberSessionIDs(ctx context.Context, teamID string) ([]string, error) {
	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}
	if team == nil {
		return nil, fmt.Errorf("team %s not found", teamID)
	}

	sessionIDs := make([]string, 0, len(team.Members))
	for _, m := range team.Members {
		sessionIDs = append(sessionIDs, m.SessionID)
	}
	return sessionIDs, nil
}

// NotifyMemberCompleted is called when a team member's SubAgent completes or fails.
// It updates the member's status and checks if all members have reached terminal states.
// If all members are done, it signals the WaitForAll channel.
func (o *orchestratorImpl) NotifyMemberCompleted(ctx context.Context, teamID string, memberID string, status TeamMemberStatus, output string) error {
	team, err := o.registry.Get(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}
	if team == nil {
		return fmt.Errorf("team %s not found", teamID)
	}

	// Update the member's status.
	if err := o.registry.UpdateMemberStatus(ctx, teamID, memberID, status); err != nil {
		return fmt.Errorf("failed to update member status: %w", err)
	}

	logger.Info("[TeamOrchestrator] member completed: team=%s, member=%s, status=%s", teamID, memberID, status)

	// Reload team to get fresh state.
	team, err = o.registry.Get(ctx, teamID)
	if err != nil {
		return err
	}

	// Check if all members have reached terminal states.
	if team.AllMembersTerminal() {
		o.completeTeam(ctx, team)
	}

	return nil
}

// FindTeamBySessionID looks up which team a given session belongs to.
// Returns the team and the member, or nil if the session is not part of any team.
func (o *orchestratorImpl) FindTeamBySessionID(ctx context.Context, sessionID string) (*Team, *TeamMember, error) {
	// This is a simple scan — for production, consider maintaining a sessionID → teamID index.
	// For now, iterate teams from the registry.
	// Since InMemoryTeamRegistry doesn't have a "list all" method, we rely on the parent session
	// being the same. This is a known limitation of the current implementation.
	//
	// A practical optimization would be to maintain a reverse index in the orchestrator.
	return nil, nil, nil
}

// completeTeam marks a team as dissolved and signals waiters.
func (o *orchestratorImpl) completeTeam(ctx context.Context, team *Team) {
	// Aggregate results from all members.
	memberResults := make(map[string]string)
	allSuccess := true
	for _, m := range team.Members {
		if m.Status == TeamMemberStatusFailed {
			allSuccess = false
		}
		if m.Progress != "" {
			memberResults[m.ID] = m.Progress
		}
	}

	team.MarkDissolved(&TeamResult{
		Summary:       "All team members completed",
		Success:       allSuccess,
		MemberResults: memberResults,
	})

	if err := o.registry.Save(ctx, team); err != nil {
		logger.Warn("[TeamOrchestrator] failed to save completed team: %v", err)
	}

	// Signal waiters.
	o.mu.Lock()
	if ch, ok := o.waitChs[team.ID]; ok {
		close(ch)
		delete(o.waitChs, team.ID)
	}
	o.mu.Unlock()

	// Unregister mailboxes.
	for _, m := range team.Members {
		o.messageBus.UnregisterMailbox(m.SessionID)
	}

	// Unregister EventBridge entries.
	if o.eventBridge != nil {
		o.eventBridge.UnregisterTeam(team.ID)
	}

	logger.Info("[TeamOrchestrator] team completed: id=%s, success=%v, members=%d", team.ID, allSuccess, len(team.Members))
}

// SetEventBridge sets the EventBridge for auto-registering spawned members.
// Called by server.go after both the orchestrator and EventBridge are created.
func (o *orchestratorImpl) SetEventBridge(bridge *EventBridge) {
	o.eventBridge = bridge
}

// --- Internal helpers ---

// spawnMember creates a SubAgent and returns a TeamMember.
func (o *orchestratorImpl) spawnMember(
	ctx context.Context,
	team *Team,
	spec *MemberSpec,
	override *MemberOverride,
	instanceIndex int,
) (*TeamMember, error) {
	// Build spawn request.
	task := spec.DefaultTask
	agentID := ""
	model := spec.RecommendedModel
	label := spec.DisplayName

	// Apply overrides.
	if override != nil {
		if override.Task != "" {
			task = override.Task
		}
		if override.AgentID != "" {
			agentID = override.AgentID
		}
		if override.Model != "" {
			model = override.Model
		}
	}

	// Include team context in the task.
	if team.TaskDescription != "" && task == "" {
		task = team.TaskDescription
	}

	if instanceIndex > 0 {
		label = fmt.Sprintf("%s-%d", label, instanceIndex)
	}

	spawnReq := &entity.SubAgentSpawnRequest{
		ParentSessionID: team.ParentSessionID,
		ParentRunID:     team.ParentRunID,
		Task:            task,
		AgentID:         agentID,
		Label:           label,
		Model:           model,
	}

	record, err := o.subAgentManager.Spawn(ctx, spawnReq)
	if err != nil {
		return nil, fmt.Errorf("spawn failed: %w", err)
	}

	member := &TeamMember{
		ID:               generateMemberID(),
		SpecID:           spec.ID,
		SubAgentRecordID: record.ID,
		SessionID:        record.SessionID,
		AgentID:          record.AgentID,
		Role:             spec.Role,
		Label:            label,
		Task:             task,
		Status:           TeamMemberStatusRunning,
		JoinedAt:         time.Now(),
	}

	// Register in EventBridge for lifecycle tracking.
	if o.eventBridge != nil {
		o.eventBridge.RegisterMember(team.ID, member.ID, member.SessionID)
	}

	return member, nil
}

// --- ID generation ---

var (
	teamCounter   uint64
	memberCounter uint64
	idMu          sync.Mutex
)

func generateTeamID() string {
	idMu.Lock()
	defer idMu.Unlock()
	teamCounter++
	return fmt.Sprintf("team-%d-%d", time.Now().UnixNano(), teamCounter)
}

func generateMemberID() string {
	idMu.Lock()
	defer idMu.Unlock()
	memberCounter++
	return fmt.Sprintf("member-%d-%d", time.Now().UnixNano(), memberCounter)
}

// ensureParentSession creates the parent session if it doesn't exist.
// This handles the case where external clients (like echoctl) generate
// their own session IDs that haven't been registered server-side.
func (o *orchestratorImpl) ensureParentSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil // No parent session needed.
	}

	// Check if session already exists.
	_, err := o.sessionRepo.Get(ctx, sessionID)
	if err == nil {
		return nil // Session exists, nothing to do.
	}

	// Session doesn't exist, create it with the default agent.
	session := &entity.Session{
		ID:        sessionID,
		AgentID:   o.defaultAgentID,
		Messages:  make([]*entity.Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := o.sessionRepo.Create(ctx, session); err != nil {
		return fmt.Errorf("failed to create parent session %s: %w", sessionID, err)
	}

	logger.Info("[TeamOrchestrator] created parent session for external client: %s", sessionID)
	return nil
}
