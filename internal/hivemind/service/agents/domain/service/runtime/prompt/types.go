package prompt

import (
	"context"
	"time"
)

// AgentLike is a minimal interface that PromptContext needs from the Agent entity.
// This breaks the import cycle: prompt package does not import entity package.
// The actual entity.Agent is assigned via the concrete field in PromptContext.
//
// We use a concrete struct pointer below (from the entity package) —
// but to avoid import cycles, we use an interface-free approach:
// PromptContext holds fields that mirror what sections need, and the
// caller (ContextBuilder/Runner) populates them from the real Agent entity.

// AgentPromptInfo carries the agent fields that PromptSections need.
// This is populated by the caller from entity.Agent to avoid import cycles.
type AgentPromptInfo struct {
	// ID is the agent ID.
	ID string

	// Name is the display name.
	Name string

	// SystemPrompt is the raw user-defined system prompt text.
	SystemPrompt string

	// Persona carries identity and prompt config.
	Persona *AgentPersonaInfo
}

// AgentPersonaInfo mirrors entity.AgentPersona without importing entity.
type AgentPersonaInfo struct {
	Identity     *AgentIdentityInfo
	PromptMode   string
	WorkspaceDir string
}

// AgentIdentityInfo mirrors entity.AgentIdentity without importing entity.
type AgentIdentityInfo struct {
	Name     string
	Emoji    string
	Creature string
	Vibe     string
	Theme    string
}

// PromptMode controls the section assembly granularity, analogous to
// OpenClaw's "full" / "minimal" / "none" prompt modes.
//
//   - Full: all sections included (main Agent)
//   - Minimal: only core sections (sub-Agent, lightweight tasks)
//   - None: identity line only (extreme minimalism)
type PromptMode string

const (
	PromptModeFull    PromptMode = "full"
	PromptModeMinimal PromptMode = "minimal"
	PromptModeNone    PromptMode = "none"
)

// PromptSection is the fundamental interface for prompt contribution.
//
// Each Section is responsible for rendering one logical segment of the
// system prompt. Sections are assembled in Priority order by the Pipeline.
//
// Analogous to K8s AdmissionHandler: each handler processes an AdmissionReview
// and may modify the object. Here, each section appends text to the prompt.
type PromptSection interface {
	// Name returns the unique identifier of this section (for dedup/debug/logging).
	Name() string

	// Priority determines assembly order (lower = earlier in prompt).
	// Builtin sections use 100-999; plugin sections should use 1000+.
	Priority() int

	// Enabled returns whether this section should appear in the final prompt.
	// Sections can dynamically decide based on PromptContext (e.g., PromptMode, Agent config).
	// Analogous to K8s webhook matchPolicy.
	Enabled(ctx context.Context, pc *PromptContext) bool

	// Render produces the Markdown text for this section.
	// Returns empty string to skip (no error).
	// A non-nil error is logged but does NOT abort the pipeline (failurePolicy: Ignore).
	Render(ctx context.Context, pc *PromptContext) (string, error)
}

// PromptMutator allows plugins to transform the fully assembled prompt text.
// This is analogous to K8s MutatingWebhook — applied after all sections
// have been rendered, before token budget validation.
type PromptMutator interface {
	// Name returns the mutator identifier.
	Name() string

	// Priority determines execution order among mutators (lower = first).
	Priority() int

	// Mutate receives the assembled prompt and returns the transformed version.
	Mutate(ctx context.Context, pc *PromptContext, assembled string) (string, error)
}

// ToolSummary is a lightweight description of an available tool,
// used by ToolingSection to enumerate capabilities in the system prompt.
type ToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"` // "plugin" or "mcp"
}

// ClusterInfo carries Hivemind-Golem cluster topology information.
// This enables the Agent to be aware of its distributed environment,
// analogous to K8s Downward API (pods knowing their node/namespace).
//
// TODO(cluster): Populate from Scheduler/GolemRegistry when available.
type ClusterInfo struct {
	// HivemindID is the identifier of the current Hivemind instance.
	HivemindID string `json:"hivemind_id"`

	// Version is the Eidolon software version.
	Version string `json:"version"`

	// Golems lists the connected Golem worker nodes.
	// Empty when running in standalone mode.
	Golems []GolemInfo `json:"golems,omitempty"`
}

// GolemInfo describes a connected Golem worker node.
type GolemInfo struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Status string            `json:"status"` // "Ready" / "NotReady"
	Skills []string          `json:"skills"` // e.g., ["browser", "code_edit", "terminal"]
	Labels map[string]string `json:"labels,omitempty"`
}

// PromptContext is the data envelope passed to every PromptSection.Render().
//
// It carries all information that sections may need — agent config, session state,
// cluster topology, runtime metadata, available tools, etc.
//
// Analogous to K8s AdmissionReview: a single object carrying the full request context.
type PromptContext struct {
	// Agent carries agent fields needed by sections.
	Agent *AgentPromptInfo

	// SessionID is the current session ID (empty for new sessions).
	SessionID string

	// Mode controls section filtering granularity.
	Mode PromptMode

	// --- Distributed awareness ---

	// ClusterInfo carries Hivemind-Golem topology.
	// Nil means standalone mode (no Golem nodes connected).
	ClusterInfo *ClusterInfo

	// NodeID is the current Hivemind instance identifier.
	NodeID string

	// --- Runtime metadata ---

	// Timezone is the server timezone name (e.g., "Asia/Shanghai").
	Timezone string

	// Now is the current server time (set once per prompt assembly).
	Now time.Time

	// ModelName is the primary model being used for this run.
	ModelName string

	// --- Module contributions ---

	// Tools lists all available tools (plugin + MCP) with short descriptions.
	Tools []ToolSummary

	// --- Extensibility ---

	// Extra holds additional key-value data that custom sections may need.
	// Analogous to K8s annotations — unstructured extensibility.
	Extra map[string]interface{}
}
