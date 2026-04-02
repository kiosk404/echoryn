// Package team provides the team collaboration module for multi-agent coordination.
// It enables SubAgents to form teams with peer-to-peer communication capabilities.
//
// Key entities:
//   - TeamTemplate: reusable team structure definition (like a K8s Deployment spec)
//   - Team: instantiated team with actual members (like a K8s ReplicaSet)
//   - TeamMember: individual member bound to a SubAgent session
//
// Architecture:
//
//	TeamTemplate → InstantiateTeam() → Team{Members}
//	                                    ├── TeamMember (SubAgent-A)
//	                                    ├── TeamMember (SubAgent-B)
//	                                    └── TeamMember (SubAgent-C)
package team

import (
	"fmt"
	"time"
)

// --- Worker Reference (Value Object) ---

// WorkerRef is a logical reference to an executing worker (SubAgent).
// It is opaque to the Execution BC — Team BC never accesses SubAgentRecordID or SessionID directly.
// The mapping from WorkerRef to execution internals is maintained exclusively in the integration layer.
//
// This is the "boundary object" that separates Team BC from Execution BC concerns.
type WorkerRef struct {
	// ID is the logical identifier (format: "worker-<uuid>").
	// Only the integration layer knows the mapping to SubAgentRecordID.
	ID string `json:"id"`
}

// String returns a string representation of the WorkerRef.
func (wr WorkerRef) String() string {
	return wr.ID
}

// --- Coordination Strategy ---

// CoordinationStrategy defines how team members collaborate.
type CoordinationStrategy string

const (
	// CoordinationParallel: all members execute in parallel, results aggregated when all complete.
	CoordinationParallel CoordinationStrategy = "parallel"

	// CoordinationPipeline: members execute sequentially, each member's output feeds the next.
	CoordinationPipeline CoordinationStrategy = "pipeline"

	// CoordinationDebate: members provide different perspectives on the same problem, leader synthesizes.
	CoordinationDebate CoordinationStrategy = "debate"

	// CoordinationLeaderDirected: leader drives the workflow, freely assigns tasks and coordinates members.
	CoordinationLeaderDirected CoordinationStrategy = "leader_directed"
)

// IsValid returns true if the strategy is a known value.
func (s CoordinationStrategy) IsValid() bool {
	switch s {
	case CoordinationParallel, CoordinationPipeline, CoordinationDebate, CoordinationLeaderDirected:
		return true
	}
	return false
}

// --- Team Status ---

// TeamStatus represents the lifecycle state of a team.
//
// State machine:
//
//	Creating → Active ↔ Working → Completing → Dissolved
type TeamStatus string

const (
	TeamStatusCreating   TeamStatus = "creating"
	TeamStatusActive     TeamStatus = "active"
	TeamStatusWorking    TeamStatus = "working"
	TeamStatusCompleting TeamStatus = "completing"
	TeamStatusDissolved  TeamStatus = "dissolved"
)

// IsTerminal returns true if the team has reached a terminal state.
func (s TeamStatus) IsTerminal() bool {
	return s == TeamStatusDissolved
}

// --- Team Member Status ---

// TeamMemberStatus represents the state of an individual team member.
type TeamMemberStatus string

const (
	TeamMemberStatusIdle      TeamMemberStatus = "idle"
	TeamMemberStatusRunning   TeamMemberStatus = "running"
	TeamMemberStatusCompleted TeamMemberStatus = "completed"
	TeamMemberStatusFailed    TeamMemberStatus = "failed"
)

// IsTerminal returns true if the member has reached a terminal state.
func (s TeamMemberStatus) IsTerminal() bool {
	return s == TeamMemberStatusCompleted || s == TeamMemberStatusFailed
}

// --- Team Template ---

// TeamTemplate defines a reusable team structure.
// Templates are "define once, instantiate many times" — analogous to a K8s Deployment spec.
type TeamTemplate struct {
	// ID is the unique identifier for this template (e.g., "software-dev-team-v1").
	ID string `json:"id" yaml:"id"`

	// Name is the human-readable template name.
	Name string `json:"name" yaml:"name"`

	// Version is the template version for compatibility tracking.
	Version string `json:"version" yaml:"version"`

	// Description provides context about the template's purpose.
	Description string `json:"description" yaml:"description"`

	// MemberSpecs defines the roles within this team structure.
	MemberSpecs []*MemberSpec `json:"member_specs" yaml:"member_specs"`

	// DefaultStrategy is the default coordination strategy for teams created from this template.
	DefaultStrategy CoordinationStrategy `json:"default_strategy" yaml:"default_strategy"`

	// LeaderSpecIndex identifies which MemberSpec is the leader (index into MemberSpecs).
	LeaderSpecIndex int `json:"leader_spec_index" yaml:"leader_spec_index"`

	// SystemPrompt is the team-level system prompt injected into all members.
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`

	// Tags are optional labels for filtering/searching templates.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// CreatedAt is when this template was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when this template was last modified.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// Validate checks the template for consistency.
func (t *TeamTemplate) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("template ID is required")
	}
	if t.Name == "" {
		return fmt.Errorf("template name is required")
	}
	if len(t.MemberSpecs) == 0 {
		return fmt.Errorf("template must have at least one member spec")
	}
	if t.LeaderSpecIndex < 0 || t.LeaderSpecIndex >= len(t.MemberSpecs) {
		return fmt.Errorf("leader_spec_index %d is out of range [0, %d)", t.LeaderSpecIndex, len(t.MemberSpecs))
	}
	if !t.DefaultStrategy.IsValid() {
		return fmt.Errorf("invalid default strategy: %s", t.DefaultStrategy)
	}

	// Validate each member spec.
	ids := make(map[string]bool)
	hasLeader := false
	for i, spec := range t.MemberSpecs {
		if err := spec.Validate(); err != nil {
			return fmt.Errorf("member_specs[%d]: %w", i, err)
		}
		if ids[spec.ID] {
			return fmt.Errorf("duplicate member spec ID: %s", spec.ID)
		}
		ids[spec.ID] = true
		if spec.IsLeader {
			hasLeader = true
		}
	}

	if !hasLeader {
		return fmt.Errorf("template must have at least one leader spec (is_leader=true)")
	}

	return nil
}

// MemberSpec defines a role within a team template.
// This is a specification (not an instance) — it describes what kind of member
// should be created when the template is instantiated.
type MemberSpec struct {
	// ID is the unique identifier within the template (e.g., "lead", "frontend", "reviewer").
	ID string `json:"id" yaml:"id"`

	// Role is the functional role (e.g., "lead", "frontend", "backend", "reviewer").
	Role string `json:"role" yaml:"role"`

	// DisplayName is the human-readable name shown in UI/logs.
	DisplayName string `json:"display_name" yaml:"display_name"`

	// Description explains what this role does.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// RecommendedModel suggests which LLM model to use (e.g., "opus" for Lead, "sonnet" for Teammate).
	RecommendedModel string `json:"recommended_model,omitempty" yaml:"recommended_model,omitempty"`

	// IsLeader indicates whether this role has leader privileges.
	IsLeader bool `json:"is_leader" yaml:"is_leader"`

	// CanCommunicate indicates whether this role can send/receive peer messages.
	CanCommunicate bool `json:"can_communicate" yaml:"can_communicate"`

	// SystemPrompt is the role-specific system prompt.
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`

	// DefaultTask is the default task description for this role.
	DefaultTask string `json:"default_task,omitempty" yaml:"default_task,omitempty"`

	// Skills lists the capabilities this role should have.
	Skills []string `json:"skills,omitempty" yaml:"skills,omitempty"`

	// MinCount is the minimum number of instances for this role (default: 1).
	MinCount int `json:"min_count,omitempty" yaml:"min_count,omitempty"`

	// MaxCount is the maximum number of instances for this role (0 means no limit).
	MaxCount int `json:"max_count,omitempty" yaml:"max_count,omitempty"`
}

// Validate checks the member spec for consistency.
func (s *MemberSpec) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("member spec ID is required")
	}
	if s.Role == "" {
		return fmt.Errorf("member spec role is required")
	}
	if s.DisplayName == "" {
		return fmt.Errorf("member spec display_name is required")
	}
	if s.MinCount < 0 {
		return fmt.Errorf("min_count must be >= 0")
	}
	if s.MaxCount < 0 {
		return fmt.Errorf("max_count must be >= 0")
	}
	if s.MaxCount > 0 && s.MinCount > s.MaxCount {
		return fmt.Errorf("min_count (%d) > max_count (%d)", s.MinCount, s.MaxCount)
	}
	return nil
}

// EffectiveMinCount returns the effective minimum count (defaults to 1).
func (s *MemberSpec) EffectiveMinCount() int {
	if s.MinCount <= 0 {
		return 1
	}
	return s.MinCount
}

// --- Team Instance ---

// Team is an instantiated team with actual SubAgent members.
// Created from a TeamTemplate via TeamOrchestrator.InstantiateTeam().
type Team struct {
	// ID is the unique identifier for this team instance.
	ID string `json:"id"`

	// Name is the human-readable team name.
	Name string `json:"name"`

	// TemplateID references the template used to create this team (empty for ad-hoc teams).
	TemplateID string `json:"template_id,omitempty"`

	// TemplateVersion is the template version at instantiation time.
	TemplateVersion string `json:"template_version,omitempty"`

	// ParentSessionID is the session that created this team.
	ParentSessionID string `json:"parent_session_id"`

	// ParentRunID is the run that triggered team creation.
	ParentRunID string `json:"parent_run_id,omitempty"`

	// TaskDescription is the overall task assigned to the team.
	TaskDescription string `json:"task_description"`

	// Strategy is the coordination strategy for this team.
	Strategy CoordinationStrategy `json:"strategy"`

	// Status is the current lifecycle state of the team.
	Status TeamStatus `json:"status"`

	// Members are the actual SubAgent instances in this team.
	Members []*TeamMember `json:"members"`

	// LeaderID is the member ID of the team leader.
	LeaderID string `json:"leader_id,omitempty"`

	// CreatedAt is when this team was instantiated.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this team was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// CompletedAt is when this team reached a terminal state.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Result holds the aggregated team output.
	Result *TeamResult `json:"result,omitempty"`

	// Metadata is arbitrary key-value metadata for extensibility.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GetMember returns the team member with the given ID, or nil if not found.
func (t *Team) GetMember(memberID string) *TeamMember {
	for _, m := range t.Members {
		if m.ID == memberID {
			return m
		}
	}
	return nil
}

// GetMemberBySessionID returns the team member with the given session ID, or nil if not found.
func (t *Team) GetMemberBySessionID(sessionID string) *TeamMember {
	for _, m := range t.Members {
		if m.SessionID == sessionID {
			return m
		}
	}
	return nil
}

// GetLeader returns the team leader member, or nil if not set.
func (t *Team) GetLeader() *TeamMember {
	if t.LeaderID == "" {
		return nil
	}
	return t.GetMember(t.LeaderID)
}

// ActiveMembers returns all non-terminal members.
func (t *Team) ActiveMembers() []*TeamMember {
	var active []*TeamMember
	for _, m := range t.Members {
		if !m.Status.IsTerminal() {
			active = append(active, m)
		}
	}
	return active
}

// AllMembersTerminal returns true if all members have reached a terminal state.
func (t *Team) AllMembersTerminal() bool {
	for _, m := range t.Members {
		if !m.Status.IsTerminal() {
			return false
		}
	}
	return len(t.Members) > 0
}

// MarkCompleting transitions the team to completing state.
func (t *Team) MarkCompleting() {
	t.Status = TeamStatusCompleting
	t.UpdatedAt = time.Now()
}

// MarkDissolved transitions the team to dissolved state.
func (t *Team) MarkDissolved(result *TeamResult) {
	now := time.Now()
	t.Status = TeamStatusDissolved
	t.CompletedAt = &now
	t.Result = result
	t.UpdatedAt = now
}

// --- Team Member ---

// TeamMember represents a SubAgent instance within a team.
type TeamMember struct {
	// ID is the unique identifier for this member within the team.
	ID string `json:"id"`

	// SpecID references the MemberSpec this member was created from.
	SpecID string `json:"spec_id,omitempty"`

	// WorkerRef is the logical reference to the executing worker.
	// This is the v2 replacement for SubAgentRecordID — Team BC
	// uses this opaque reference instead of Execution BC internals.
	WorkerRef WorkerRef `json:"worker_ref,omitempty"`

	// SubAgentRecordID links to the SubAgentRecord in the SubAgent subsystem.
	// Deprecated: kept for backward compatibility during migration.
	// Use WorkerRef instead. Will be removed after full migration.

	// SubAgentRecordID links to the SubAgentRecord in the SubAgent subsystem.
	SubAgentRecordID string `json:"subagent_record_id,omitempty"`

	// SessionID is the SubAgent's independent session ID.
	SessionID string `json:"session_id"`

	// AgentID is the Agent ID used by this member.
	AgentID string `json:"agent_id"`

	// Role is the functional role (e.g., "lead", "frontend").
	Role string `json:"role"`

	// Label is the display name for this member.
	Label string `json:"label"`

	// Task is the specific task assigned to this member.
	Task string `json:"task,omitempty"`

	// Status is the current state of this member.
	Status TeamMemberStatus `json:"status"`

	// JoinedAt is when this member joined the team.
	JoinedAt time.Time `json:"joined_at"`

	// CompletedAt is when this member finished their task.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// NodeID is the Golem node ID if this member is executing remotely.
	NodeID string `json:"node_id,omitempty"`

	// Progress is a human-readable progress description.
	Progress string `json:"progress,omitempty"`
}

// MarkRunning transitions the member to running state.
func (m *TeamMember) MarkRunning() {
	m.Status = TeamMemberStatusRunning
}

// MarkCompleted transitions the member to completed state.
func (m *TeamMember) MarkCompleted() {
	now := time.Now()
	m.Status = TeamMemberStatusCompleted
	m.CompletedAt = &now
}

// MarkFailed transitions the member to failed state.
func (m *TeamMember) MarkFailed() {
	now := time.Now()
	m.Status = TeamMemberStatusFailed
	m.CompletedAt = &now
}

// --- Team Result ---

// TeamResult holds the aggregated output of a completed team.
type TeamResult struct {
	// Summary is a high-level summary of the team's work.
	Summary string `json:"summary,omitempty"`

	// MemberResults maps member ID to their individual output.
	MemberResults map[string]string `json:"member_results,omitempty"`

	// Success indicates whether the team completed successfully.
	Success bool `json:"success"`

	// Error holds error details if the team failed.
	Error string `json:"error,omitempty"`
}

// --- Template Filter ---

// TemplateFilter defines criteria for listing/searching templates.
type TemplateFilter struct {
	// Tags filters templates by tag (OR match).
	Tags []string `json:"tags,omitempty"`

	// NamePrefix filters templates by name prefix.
	NamePrefix string `json:"name_prefix,omitempty"`
}

// --- Max Team Size ---

// DefaultMaxTeamMembers is the default maximum number of members in a team.
// Aligned with DefaultMaxConcurrent in the subagent package.
const DefaultMaxTeamMembers = 8
