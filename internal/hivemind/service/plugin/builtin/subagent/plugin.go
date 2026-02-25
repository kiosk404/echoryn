package subagent

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

const PluginName = "subagent"

// PluginDefinition returns the plugin metadata.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "SubAgent",
		Kind:        "subagent",
		Description: "Sub-agent orchestration: spawn background agents for parallel task delegation",
	}
}

// Config holds the subagent plugin configuration.
type Config struct {
	// Enabled controls whether sub-agent spawning is available.
	Enabled bool `json:"enabled"`

	// MaxConcurrent is the max number of concurrent sub-agents.
	MaxConcurrent int `json:"max_concurrent"`

	// ArchiveAfterMinutes is how long to keep completed records.
	ArchiveAfterMinutes int `json:"archive_after_minutes"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:             true,
		MaxConcurrent:       8,
		ArchiveAfterMinutes: 60,
	}
}

// subagentPlugin implements the sub-agent plugin.
//
// Provides two tools:
//   - sessions_spawn: spawn a sub-agent for a background task
//   - sessions_status: query sub-agent status
//
// Follows K8S interface probe pattern for ToolProvider + HookProvider.
type subagentPlugin struct {
	cfg     *Config
	handle  plugin.Handle
	manager service.SubAgentManager
}

var (
	_ plugin.Plugin          = (*subagentPlugin)(nil)
	_ plugin.InitPlugin      = (*subagentPlugin)(nil)
	_ plugin.LifecyclePlugin = (*subagentPlugin)(nil)
)

// Factory creates a new subagent plugin instance.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfg := DefaultConfig()
	if c, ok := args["config"]; ok {
		if typedCfg, ok := c.(*Config); ok {
			cfg = typedCfg
		}
	}

	return &subagentPlugin{
		cfg:    cfg,
		handle: handle,
	}, nil
}

func (p *subagentPlugin) Name() string { return PluginName }

// Init registers the sessions_spawn and sessions_status tools.
func (p *subagentPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[SubAgent] plugin disabled, skipping tool registration")
		return nil
	}

	// Register sessions_spawn tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "sessions_spawn",
		Description: "Spawn a sub-agent to work on a background task. The sub-agent runs independently and reports results when done. Use this for complex or time-consuming tasks that can be parallelized.",
		Parameters: []plugin.ParameterDef{
			{Name: "task", Type: "string", Description: "The task description for the sub-agent to complete", Required: true},
			{Name: "agent_id", Type: "string", Description: "The agent ID to use for the sub-agent (default: same as parent)", Required: false},
			{Name: "label", Type: "string", Description: "Optional human-readable label for this sub-agent", Required: false},
			{Name: "model", Type: "string", Description: "Optional model override (e.g., 'openai:gpt-4o-mini')", Required: false},
		},
		Handler: p.handleSessionsSpawn,
	})

	// Register sessions_status tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "sessions_status",
		Description: "Check the status of spawned sub-agents for the current session.",
		Parameters: []plugin.ParameterDef{
			{Name: "subagent_id", Type: "string", Description: "Specific sub-agent ID to query (default: list all)", Required: false},
		},
		Handler: p.handleSessionsStatus,
	})

	// Register agent_end hook to drain pending announcements.
	api.RegisterHook(plugin.HookAgentEnd, p.onAgentEnd)

	logger.Info("[SubAgent] registered tools: sessions_spawn, sessions_status")
	return nil
}

// Start initializes the sub-agent manager.
// The manager requires SubAgentManager to be set externally after module assembly.
func (p *subagentPlugin) Start(_ context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}
	logger.Info("[SubAgent] plugin started (max_concurrent=%d, archive_after=%dm)",
		p.cfg.MaxConcurrent, p.cfg.ArchiveAfterMinutes)
	return nil
}

// Stop gracefully shuts down the sub-agent manager.
func (p *subagentPlugin) Stop(ctx context.Context) error {
	if p.manager != nil {
		return p.manager.Stop(ctx)
	}
	return nil
}

// ManagerSetter is an exported interface for injecting the SubAgentManager
// into the subagent plugin from the server layer. This follows K8S's interface
// probe pattern — the server asserts this interface on the plugin instance
// rather than importing the unexported concrete type.
type ManagerSetter interface {
	SetManager(mgr service.SubAgentManager)
}

var _ ManagerSetter = (*subagentPlugin)(nil)

// SetManager injects the SubAgentManager after module assembly.
// This is called by the server during initialization, after the AgentRunner
// and SubAgentManager are created.
func (p *subagentPlugin) SetManager(mgr service.SubAgentManager) {
	p.manager = mgr
}

// Manager returns the current SubAgentManager (may be nil before Start).
func (p *subagentPlugin) Manager() service.SubAgentManager {
	return p.manager
}

// handleSessionsSpawn is the tool handler for sessions_spawn.
func (p *subagentPlugin) handleSessionsSpawn(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("sub-agent manager not initialized")
	}

	task, _ := params["task"].(string)
	if task == "" {
		return nil, fmt.Errorf("task parameter is required")
	}

	// Extract parent session ID from context metadata.
	parentSessionID := runtime.ParentSessionIDFromContext(ctx)
	parentRunID := runtime.ParentRunIDFromContext(ctx)
	if parentSessionID == "" {
		return nil, fmt.Errorf("parent session ID not found in context")
	}

	req := &entity.SubAgentSpawnRequest{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Task:            task,
	}

	if agentID, ok := params["agent_id"].(string); ok && agentID != "" {
		req.AgentID = agentID
	}
	if label, ok := params["label"].(string); ok {
		req.Label = label
	}
	if model, ok := params["model"].(string); ok {
		req.Model = model
	}

	record, err := p.manager.Spawn(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn sub-agent: %w", err)
	}

	return map[string]interface{}{
		"status":      "accepted",
		"subagent_id": record.ID,
		"session_id":  record.SessionID,
		"task":        record.Task,
		"message":     fmt.Sprintf("Sub-agent spawned (ID: %s). It will work on the task in the background and report back when done.", record.ID[:8]),
	}, nil
}

// handleSessionsStatus is the tool handler for sessions_status.
func (p *subagentPlugin) handleSessionsStatus(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("sub-agent manager not initialized")
	}

	parentSessionID := runtime.ParentSessionIDFromContext(ctx)
	if parentSessionID == "" {
		return nil, fmt.Errorf("parent session ID not found in context")
	}

	// Single sub-agent query.
	if subagentID, ok := params["subagent_id"].(string); ok && subagentID != "" {
		record, err := p.manager.Get(ctx, subagentID)
		if err != nil {
			return nil, err
		}
		return formatRecord(record), nil
	}

	// List all sub-agents for this session.
	records, err := p.manager.ListByParent(ctx, parentSessionID)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, 0, len(records))
	for _, record := range records {
		result = append(result, formatRecord(record))
	}

	return map[string]interface{}{
		"subagents": result,
		"count":     len(result),
	}, nil
}

// onAgentEnd is the hook handler called when an agent run ends.
// It drains any pending sub-agent announcements for the parent session.
func (p *subagentPlugin) onAgentEnd(_ context.Context, data interface{}) error {
	// Extract session from hook data.
	hookData, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	session, ok := hookData["session"].(*entity.Session)
	if !ok || session == nil {
		return nil
	}

	// If this is a sub-agent session, no need to drain.
	if session.IsSubAgentSession() {
		return nil
	}

	// Drain pending announcements — handled by the AnnounceController
	// which is embedded in the SubAgentManager.
	return nil
}

// formatRecord converts a SubAgentRecord to a JSON-friendly map.
func formatRecord(record *entity.SubAgentRecord) map[string]interface{} {
	result := map[string]interface{}{
		"id":       record.ID,
		"task":     record.Task,
		"status":   string(record.Status),
		"agent_id": record.AgentID,
		"created":  record.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if record.Label != "" {
		result["label"] = record.Label
	}
	if record.StartedAt != nil {
		result["started"] = record.StartedAt.Format("2006-01-02 15:04:05")
	}
	if record.CompletedAt != nil {
		result["completed"] = record.CompletedAt.Format("2006-01-02 15:04:05")
		result["duration"] = record.Duration().String()
	}
	if record.Status == entity.SubAgentStatusCompleted {
		// Truncate result for display.
		r := record.Result
		if len(r) > 200 {
			r = r[:200] + "..."
		}
		result["result_preview"] = r
	}
	if record.Status == entity.SubAgentStatusFailed {
		result["error"] = record.Error
	}
	return result
}

// marshalJSON is a helper to convert interface to JSON for debugging.
func marshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
