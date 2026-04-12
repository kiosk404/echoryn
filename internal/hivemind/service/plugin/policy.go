package plugin

// PolicyContext carries the context for tool policy evaluation.
type PolicyContext struct {
	// AgentID is the agent executing the turn.
	AgentID string
	// ProviderID is the LLM provider (e.g., "openai", "deepseek", "ollama").
	ProviderID string
	// ChannelType is the IM channel type (e.g., "feishu", "telegram", "api").
	ChannelType string
	// IsSubAgent indicates whether this is a sub-agent run.
	IsSubAgent bool
	// Profile is the requested tool profile (e.g., "coding", "minimal", "full").
	Profile string
}

// PolicyLayer is a single layer in the tool policy pipeline.
// Each layer filters the tool list and returns the surviving tools.
// Deny always wins — if any layer removes a tool, it stays removed.
type PolicyLayer interface {
	// Name returns a human-readable identifier for this layer.
	Name() string
	// Filter returns the subset of tools that pass this layer's policy.
	Filter(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition
}

// ToolPolicyPipeline applies a chain of PolicyLayers to filter tools.
// Aligned with OpenClaw's 7-layer tool-policy-pipeline.ts.
type ToolPolicyPipeline struct {
	layers []PolicyLayer
}

// NewToolPolicyPipeline creates an empty pipeline.
func NewToolPolicyPipeline() *ToolPolicyPipeline {
	return &ToolPolicyPipeline{}
}

// AddLayer appends a PolicyLayer to the pipeline.
func (p *ToolPolicyPipeline) AddLayer(layer PolicyLayer) {
	p.layers = append(p.layers, layer)
}

// Apply runs all layers sequentially, returning the filtered tool list.
// Short-circuits on empty result.
func (p *ToolPolicyPipeline) Apply(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition {
	for _, layer := range p.layers {
		tools = layer.Filter(tools, ctx)
		if len(tools) == 0 {
			return tools
		}
	}
	return tools
}

// AllowDeny holds allow/deny lists for a specific context.
// Deny always wins over Allow.
type AllowDeny struct {
	Allow []string
	Deny  []string
}

// applyAllowDeny applies allow/deny rules to a tool list.
// Supports group:xxx references (expanded via ExpandGroups).
func applyAllowDeny(tools []ToolDefinition, ad AllowDeny) []ToolDefinition {
	denyNames := ExpandGroups(ad.Deny)
	denySet := make(map[string]struct{}, len(denyNames))
	for _, name := range denyNames {
		denySet[name] = struct{}{}
	}

	if len(ad.Allow) > 0 {
		allowNames := ExpandGroups(ad.Allow)
		allowSet := make(map[string]struct{}, len(allowNames))
		for _, name := range allowNames {
			allowSet[name] = struct{}{}
		}
		var result []ToolDefinition
		for _, t := range tools {
			if _, denied := denySet[t.Name]; denied {
				continue
			}
			if _, allowed := allowSet[t.Name]; allowed {
				result = append(result, t)
			}
		}
		return result
	}

	// No allow list — only apply deny.
	var result []ToolDefinition
	for _, t := range tools {
		if _, denied := denySet[t.Name]; !denied {
			result = append(result, t)
		}
	}
	return result
}

// --- Layer 1: ProfilePolicy ---

// ProfilePolicy filters tools to a named preset (minimal/coding/full/golem/team).
type ProfilePolicy struct {
	// Profiles maps profile name → allowed tool names.
	// Empty slice means no tools; nil/missing key means allow all.
	Profiles map[string][]string
}

func (p *ProfilePolicy) Name() string { return "profile" }

func (p *ProfilePolicy) Filter(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition {
	if ctx.Profile == "" || ctx.Profile == "full" {
		return tools
	}
	if p.Profiles == nil {
		return tools
	}
	allowed, ok := p.Profiles[ctx.Profile]
	if !ok {
		return tools
	}
	allowSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowSet[name] = struct{}{}
	}
	var result []ToolDefinition
	for _, t := range tools {
		if _, ok := allowSet[t.Name]; ok {
			result = append(result, t)
		}
	}
	return result
}

// --- Layer 2: ProviderPolicy ---

// ProviderPolicy filters tools based on the LLM provider.
// Some providers (e.g., Ollama) may not support certain tool schemas.
type ProviderPolicy struct {
	// ByProvider maps provider ID → allow/deny rules.
	ByProvider map[string]AllowDeny
}

func (p *ProviderPolicy) Name() string { return "provider" }

func (p *ProviderPolicy) Filter(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition {
	if ctx.ProviderID == "" || p.ByProvider == nil {
		return tools
	}
	ad, ok := p.ByProvider[ctx.ProviderID]
	if !ok {
		return tools
	}
	return applyAllowDeny(tools, ad)
}

// --- Layer 3: GlobalPolicy ---

// GlobalPolicy applies global allow/deny rules to all agents.
type GlobalPolicy struct {
	AllowDeny
}

func (p *GlobalPolicy) Name() string { return "global" }

func (p *GlobalPolicy) Filter(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition {
	return applyAllowDeny(tools, p.AllowDeny)
}

// --- Layer 4: AgentPolicy ---

// AgentPolicy applies per-agent allow/deny rules.
type AgentPolicy struct {
	// ByAgent maps agent ID → allow/deny rules.
	ByAgent map[string]AllowDeny
}

func (p *AgentPolicy) Name() string { return "agent" }

func (p *AgentPolicy) Filter(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition {
	if ctx.AgentID == "" || p.ByAgent == nil {
		return tools
	}
	ad, ok := p.ByAgent[ctx.AgentID]
	if !ok {
		return tools
	}
	return applyAllowDeny(tools, ad)
}

// --- Layer 5: ChannelPolicy ---

// ChannelPolicy applies per-IM-channel allow/deny rules.
// Different channels (Feishu/Telegram/API) can have different tool permissions.
type ChannelPolicy struct {
	// ByChannel maps channel type → allow/deny rules.
	ByChannel map[string]AllowDeny
}

func (p *ChannelPolicy) Name() string { return "channel" }

func (p *ChannelPolicy) Filter(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition {
	if ctx.ChannelType == "" || p.ByChannel == nil {
		return tools
	}
	ad, ok := p.ByChannel[ctx.ChannelType]
	if !ok {
		return tools
	}
	return applyAllowDeny(tools, ad)
}

// --- Layer 6: SubAgentPolicy ---

// SubAgentPolicy applies hardcoded deny rules for sub-agent runs.
// Sub-agents must not access orchestration tools to prevent recursive spawning
// and privilege escalation.
type SubAgentPolicy struct {
	// DenyList is the hardcoded list of tools denied for sub-agents.
	DenyList []string
}

func (p *SubAgentPolicy) Name() string { return "subagent" }

func (p *SubAgentPolicy) Filter(tools []ToolDefinition, ctx PolicyContext) []ToolDefinition {
	if !ctx.IsSubAgent || len(p.DenyList) == 0 {
		return tools
	}
	denySet := make(map[string]struct{}, len(p.DenyList))
	for _, name := range p.DenyList {
		denySet[name] = struct{}{}
	}
	var result []ToolDefinition
	for _, t := range tools {
		if _, denied := denySet[t.Name]; !denied {
			result = append(result, t)
		}
	}
	return result
}

// NewDefaultPipeline creates a ToolPolicyPipeline with all 6 layers configured
// from the given options.
func NewDefaultPolicyPipeline(globalAD AllowDeny, byProvider map[string]AllowDeny) *ToolPolicyPipeline {
	p := NewToolPolicyPipeline()
	p.AddLayer(&ProfilePolicy{Profiles: DefaultProfiles})
	p.AddLayer(&ProviderPolicy{ByProvider: byProvider})
	p.AddLayer(&GlobalPolicy{AllowDeny: globalAD})
	p.AddLayer(&AgentPolicy{})
	p.AddLayer(&ChannelPolicy{})
	p.AddLayer(&SubAgentPolicy{
		DenyList: []string{"sessions_spawn", "sessions_status", "team_create", "team_dissolve"},
	})
	return p
}
