package entity

import (
	"time"

	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// Agent represents a configured AI agent with its persona, model binding, and tool access.
//
// Modeled after:
// - OpenClaw: agent definition with system prompt, model selection, tool bindings
// - K8S: spec + status separation pattern
type Agent struct {
	// ID is the unique identifier for this agent.
	ID string `json:"id"`

	// Name is the human-readable display name.
	Name string `json:"name"`

	// Description is a brief description of this agent's purpose.
	Description string `json:"description,omitempty"`

	// SystemPrompt is the system instruction / persona for this agent.
	// This defines the agent's behavior, personality, and constraints.
	//
	// When PromptPipeline is active, this is injected as the PersonaSection (priority 300).
	// When PromptPipeline is nil (backward compat), this is used directly as the full system prompt.
	SystemPrompt string `json:"system_prompt"`

	// Persona defines the agent's identity and prompt assembly configuration.
	// This is the Eidolon equivalent of OpenClaw's IdentityConfig + workspace files.
	//
	// When nil, the Pipeline uses default identity and the raw SystemPrompt.
	// When set, the Pipeline renders identity from Persona.Identity and
	// uses SystemPrompt as the user-defined persona text (PersonaSection).
	Persona *AgentPersona `json:"persona,omitempty"`

	// ModelRef is the primary LLM model binding for this agent.
	ModelRef llmEntity.ModelRef `json:"model_ref"`

	// Fallback defines the model fallback configuration.
	// When the primary model fails, candidates are tried in order.
	Fallback llmEntity.FallbackConfig `json:"fallback"`

	// Tools is the list of tool names this agent can use.
	// References tool IDs registered in the plugin.Registry.
	Tools []string `json:"tools,omitempty"`

	// MCPServers is the list of MCP server names this agent can use.
	// References server names defined in mcp.json.
	// If empty, all connected MCP servers' tools are available.
	MCPServers []string `json:"mcp_servers,omitempty"`

	// MaxTurns is the maximum number of tool-call turns per run.
	// Prevents infinite tool loops. 0 means use module default.
	MaxTurns int `json:"max_turns,omitempty"`

	// Temperature controls the LLM sampling temperature.
	// nil means use model default.
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens is the maximum output tokens for LLM generation.
	// nil means use model default.
	MaxTokens *int `json:"max_tokens,omitempty"`

	// CreatedAt is when this agent was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this agent was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentPersona defines the agent's identity and prompt assembly configuration.
//
// This is the Eidolon equivalent of OpenClaw's IdentityConfig + workspace file system.
// Follows K8s CRD Spec pattern — declarative definition of desired state.
//
//   - Identity: core identity attributes (name, emoji, vibe, etc.)
//   - PromptMode: controls section granularity (full/minimal/none)
//   - WorkspaceDir: future P1 — directory for SOUL.md / AGENTS.md etc.
//   - ExtraSections: user-defined additional prompt sections (sidecar injection pattern)
type AgentPersona struct {
	// Identity defines the agent's name, emoji, and personality traits.
	// Analogous to OpenClaw's IdentityConfig.
	Identity *AgentIdentity `json:"identity,omitempty"`

	// PromptMode controls section assembly granularity.
	// "full" (default), "minimal" (sub-agent), "none" (identity line only).
	PromptMode string `json:"prompt_mode,omitempty"`

	// WorkspaceDir is the directory for persona files (SOUL.md, IDENTITY.md, AGENTS.md, etc.).
	// When set, the PromptPipeline's WorkspaceLoader reads .md files from this directory
	// and registers them as dynamic PromptSections (P1 feature).
	WorkspaceDir string `json:"workspace_dir,omitempty"`

	// ExtraSections holds user-defined additional prompt sections.
	// Key is the section heading, value is the Markdown content.
	// Analogous to K8s sidecar injection — extra content added without modifying core pipeline.
	ExtraSections map[string]string `json:"extra_sections,omitempty"`
}

// AgentIdentity defines the agent's core identity attributes.
// Analogous to OpenClaw's IdentityConfig (name, emoji, theme, creature, vibe).
type AgentIdentity struct {
	// Name is the agent's display name (e.g., "Aria").
	Name string `json:"name,omitempty"`

	// Emoji is the agent's representative emoji (e.g., "🦊").
	Emoji string `json:"emoji,omitempty"`

	// Creature is the agent's creature type (e.g., "familiar").
	Creature string `json:"creature,omitempty"`

	// Vibe describes the agent's communication style (e.g., "sharp and warm").
	Vibe string `json:"vibe,omitempty"`

	// Theme is the agent's personality theme (e.g., "warm-chaos").
	Theme string `json:"theme,omitempty"`
}

// EffectivePromptMode returns the agent's prompt mode, defaulting to "full".
func (a *Agent) EffectivePromptMode() string {
	if a.Persona != nil && a.Persona.PromptMode != "" {
		return a.Persona.PromptMode
	}
	return "full"
}

// LLMParams converts agent configuration to LLM parameters.
func (a *Agent) LLMParams() *llmEntity.LLMParams {
	params := &llmEntity.LLMParams{}
	if a.Temperature != nil {
		t := float32(*a.Temperature)
		params.Temperature = &t
	}
	if a.MaxTokens != nil {
		params.MaxTokens = *a.MaxTokens
	}
	return params
}

// EffectiveMaxTurns returns the max turns, falling back to defaultMax if not set.
func (a *Agent) EffectiveMaxTurns(defaultMax int) int {
	if a.MaxTurns > 0 {
		return a.MaxTurns
	}
	return defaultMax
}
