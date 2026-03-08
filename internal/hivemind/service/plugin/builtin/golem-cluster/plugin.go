// Package golem_cluster provides the golem-cluster plugin for Hivemind.
//
// This plugin bridges the Golem subsystem to the Agent runtime, enabling:
//   - cluster_list_nodes tool: allows the Agent to query connected Golem nodes
//   - ClusterAwareness prompt injection: injects Golem topology into the system prompt
//
// Without this plugin, the Agent has no awareness of connected Golem nodes
// and cannot answer questions about "entities" or "golems" in the cluster.
package golem_cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/dispatcher"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"github.com/kiosk404/echoryn/pkg/version"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "golem-cluster"

	// Kind marks this as a "general" plugin (no slot exclusion).
	Kind = "general"
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Golem Cluster",
		Kind:        Kind,
		Description: "Bridges NodeManager to Agent runtime, providing cluster awareness and node query tools",
	}
}

// Config holds the configuration for the golem-cluster plugin.
type Config struct {
	Enabled bool `json:"enabled"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
	}
}

// golemClusterPlugin is the runtime instance of the golem-cluster plugin.
type golemClusterPlugin struct {
	cfg        *Config
	registry   registry.Registry
	dispatcher dispatcher.Dispatcher
}

// Factory is the PluginFactory for golem-cluster.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfg := DefaultConfig()
	if c, ok := args["config"].(*Config); ok && c != nil {
		cfg = c
	}

	var reg registry.Registry
	var disp dispatcher.Dispatcher
	if r, ok := args["registry"].(registry.Registry); ok {
		reg = r
	}
	if d, ok := args["dispatcher"].(dispatcher.Dispatcher); ok {
		disp = d
	}

	if !cfg.Enabled {
		logger.Info("[golem-cluster] plugin disabled via config")
	}

	return &golemClusterPlugin{
		cfg:        cfg,
		registry:   reg,
		dispatcher: disp,
	}, nil
}

func (p *golemClusterPlugin) Name() string { return PluginName }

// --- InitPlugin interface ---

// Init registers tools via the PluginAPI (same pattern as all other built-in plugins).
// Handlers hold a reference to p and read p.registry / p.dispatcher at invocation time,
// so there is no dependency on injection order.
func (p *golemClusterPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		return nil
	}

	api.RegisterTool(plugin.ToolDefinition{
		Name:        "cluster_list_nodes",
		Description: "List all connected Golem worker nodes in the Echoryn cluster, including their status, capabilities, and load information.",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "status_filter",
				Type:        "string",
				Description: "Optional status filter: 'online', 'offline', 'cordoned', 'draining'. Empty means all nodes.",
				Required:    false,
			},
		},
		Handler: p.handleListNodes,
	})

	api.RegisterTool(plugin.ToolDefinition{
		Name:        "cluster_get_node",
		Description: "Get detailed information about a specific Golem worker node by its ID or name.",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "node_id",
				Type:        "string",
				Description: "The ID or name of the Golem node to query.",
				Required:    true,
			},
		},
		Handler: p.handleGetNode,
	})

	api.RegisterTool(plugin.ToolDefinition{
		Name:        "cluster_dispatch_task",
		Description: "Dispatch a skill/task to a specific Golem worker node for remote execution. Use this to execute shell commands, file operations, or any other skill on a connected Golem node. The node must have the required capability (e.g., 'shell' for shell commands).",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "node_id",
				Type:        "string",
				Description: "The ID or name of the target Golem node. Use cluster_list_nodes to find available nodes.",
				Required:    true,
			},
			{
				Name:        "skill_name",
				Type:        "string",
				Description: "The skill to execute on the Golem node. Available skills depend on the node's capabilities (e.g., 'shell' for executing shell commands, 'fileops' for file operations).",
				Required:    true,
			},
			{
				Name:        "payload",
				Type:        "object",
				Description: "JSON parameters for the skill. For 'shell' skill: {\"command\": \"<shell command>\"}. For 'fileops' skill: {\"operation\": \"read|write|list\", \"path\": \"<file path>\", \"content\": \"<optional content for write>\"}.",
				Required:    true,
			},
			{
				Name:        "timeout",
				Type:        "string",
				Description: "Optional execution timeout (e.g., '30s', '5m'). Defaults to '30s'.",
				Required:    false,
			},
		},
		Handler: p.handleDispatchTask,
	})

	return nil
}

// --- PromptProvider interface ---

// PromptSections returns a ClusterInfoSection that dynamically injects
// Golem topology data into the PromptContext before section rendering.
//
// NOTE: We intentionally do NOT guard on p.registry == nil here because
// PromptSections() is called during Framework.Init() → probeAndRegister(),
// which runs BEFORE injectGolemDeps() sets the registry. The injector
// holds a reference to the plugin and reads registry lazily at Render time.
func (p *golemClusterPlugin) PromptSections() []prompt.PromptSection {
	if !p.cfg.Enabled {
		return nil
	}
	return []prompt.PromptSection{
		&clusterInfoInjector{plugin: p},
	}
}

// --- Tool Handlers ---

// nodeInfo is the JSON output for a single node in tool results.
type nodeInfo struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Status          string          `json:"status"`
	Address         string          `json:"address"`
	Capabilities    []string        `json:"capabilities"`
	InstalledSkills []nodeSkillInfo `json:"installed_skills,omitempty"`
	Version         string          `json:"version"`
	Cordoned        bool            `json:"cordoned"`
	RunningTasks    int32           `json:"running_tasks"`
	CPUPercent      float64         `json:"cpu_percent,omitempty"`
	MemPercent      float64         `json:"memory_percent,omitempty"`
}

// nodeSkillInfo describes a skill installed on a Golem node (for tool output).
type nodeSkillInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities,omitempty"`
}

func (p *golemClusterPlugin) handleListNodes(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.registry == nil {
		return map[string]interface{}{
			"nodes": []interface{}{},
			"total": 0,
			"note":  "Registry not available",
		}, nil
	}

	nodes, err := p.registry.ListNodes(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Apply optional status filter.
	if filterStr, ok := params["status_filter"].(string); ok && filterStr != "" {
		filterStr = strings.ToLower(filterStr)
		var filtered []*registry.NodeState
		for _, n := range nodes {
			if strings.ToLower(n.Status.Phase.String()) == "node_status_"+filterStr ||
				strings.EqualFold(nodeStatusLabel(n), filterStr) {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}

	result := make([]nodeInfo, 0, len(nodes))
	for _, n := range nodes {
		info := nodeInfo{
			ID:              n.Spec.NodeID,
			Name:            n.Spec.NodeName,
			Status:          nodeStatusLabel(n),
			Address:         n.Spec.GRPCAddress,
			Capabilities:    capNames(n),
			InstalledSkills: installedSkillInfos(n),
			Version:         n.Spec.Version,
			Cordoned:        n.Spec.Cordoned,
			RunningTasks:    n.Status.RunningTasks,
		}
		if n.Status.Load != nil {
			info.CPUPercent = n.Status.Load.CpuPercent
			info.MemPercent = n.Status.Load.MemoryPercent
		}
		result = append(result, info)
	}

	return map[string]interface{}{
		"nodes": result,
		"total": len(result),
	}, nil
}

func (p *golemClusterPlugin) handleGetNode(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.registry == nil {
		return nil, fmt.Errorf("Registry not available")
	}

	nodeIDOrName, _ := params["node_id"].(string)
	if nodeIDOrName == "" {
		return nil, fmt.Errorf("node_id parameter is required")
	}

	// Resolve by ID or name, then fetch full state.
	resolvedID, err := p.resolveNodeID(nodeIDOrName)
	if err != nil {
		return nil, err
	}
	n, err := p.registry.GetNode(resolvedID)
	if err != nil {
		return nil, fmt.Errorf("node %q not found", nodeIDOrName)
	}

	info := nodeInfo{
		ID:              n.Spec.NodeID,
		Name:            n.Spec.NodeName,
		Status:          nodeStatusLabel(n),
		Address:         n.Spec.GRPCAddress,
		Capabilities:    capNames(n),
		InstalledSkills: installedSkillInfos(n),
		Version:         n.Spec.Version,
		Cordoned:        n.Spec.Cordoned,
		RunningTasks:    n.Status.RunningTasks,
	}
	if n.Status.Load != nil {
		info.CPUPercent = n.Status.Load.CpuPercent
		info.MemPercent = n.Status.Load.MemoryPercent
	}

	return info, nil
}

func (p *golemClusterPlugin) handleDispatchTask(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.registry == nil || p.dispatcher == nil {
		return nil, fmt.Errorf("Registry or Dispatcher not available")
	}

	// Parse required parameters.
	nodeIDOrName, _ := params["node_id"].(string)
	if nodeIDOrName == "" {
		return nil, fmt.Errorf("node_id parameter is required")
	}
	skillName, _ := params["skill_name"].(string)
	if skillName == "" {
		return nil, fmt.Errorf("skill_name parameter is required")
	}

	// Parse payload (can be map or string).
	var payloadBytes []byte
	switch v := params["payload"].(type) {
	case map[string]interface{}:
		var err error
		payloadBytes, err = json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
	case string:
		payloadBytes = []byte(v)
	default:
		if v != nil {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal payload: %w", err)
			}
			payloadBytes = b
		}
	}

	// Parse timeout.
	timeoutStr, _ := params["timeout"].(string)
	if timeoutStr == "" {
		timeoutStr = "30s"
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 30 * time.Second
	}

	// Resolve node ID (try direct lookup, then by name).
	nodeID, err := p.resolveNodeID(nodeIDOrName)
	if err != nil {
		return nil, err
	}

	// Check that the node has the required capability or installed skill.
	node, _ := p.registry.GetNode(nodeID)
	if node != nil {
		hasMatch := false
		// Check capabilities.
		for _, c := range node.Spec.Capabilities {
			if strings.EqualFold(c.Name, skillName) {
				hasMatch = true
				break
			}
		}
		// Check installed skills (by name).
		if !hasMatch {
			for _, sk := range node.Spec.InstalledSkills {
				if strings.EqualFold(sk.Name, skillName) {
					hasMatch = true
					break
				}
			}
		}
		if !hasMatch {
			caps := capNames(node)
			skillNames := make([]string, len(node.Spec.InstalledSkills))
			for i, sk := range node.Spec.InstalledSkills {
				skillNames[i] = sk.Name
			}
			return nil, fmt.Errorf("node %q does not have capability or skill %q (capabilities: %v, skills: %v)", nodeIDOrName, skillName, caps, skillNames)
		}
	}

	// Build the gRPC Task.
	task := &pb.Task{
		Id:        uuid.New().String(),
		SkillName: skillName,
		Payload:   payloadBytes,
		Status:    pb.TaskStatus_TASK_STATUS_PENDING,
		Priority:  pb.TaskPriority_TASK_PRIORITY_NORMAL,
		Timeout:   durationpb.New(timeout),
	}

	// Apply timeout to the dispatch context.
	dispatchCtx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()

	logger.Info("[golem-cluster] dispatching task %s (skill=%s) to node %s", task.Id, skillName, nodeID)

	resp, err := p.dispatcher.Dispatch(dispatchCtx, nodeID, task)
	if err != nil {
		return nil, fmt.Errorf("failed to dispatch task to node %q: %w", nodeIDOrName, err)
	}

	if !resp.Accepted {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("task rejected by node: %s", resp.RejectReason),
		}, nil
	}

	// Build the result from the task execution outcome.
	result := map[string]interface{}{
		"task_id": task.Id,
		"node_id": nodeID,
		"skill":   skillName,
	}

	if resp.TaskResult != nil {
		result["success"] = resp.TaskResult.Success
		if resp.TaskResult.Output != nil {
			result["output"] = string(resp.TaskResult.Output)
		}
		if resp.TaskResult.Error != "" {
			result["error"] = resp.TaskResult.Error
		}
	} else {
		result["success"] = resp.Accepted
		result["output"] = "(no output)"
	}

	return result, nil
}

// resolveNodeID resolves a node ID or name to the actual node ID.
func (p *golemClusterPlugin) resolveNodeID(nodeIDOrName string) (string, error) {
	// Try direct ID lookup.
	if _, err := p.registry.GetNode(nodeIDOrName); err == nil {
		return nodeIDOrName, nil
	}

	// Try searching by name.
	allNodes, err := p.registry.ListNodes(nil)
	if err != nil {
		return "", fmt.Errorf("node %q not found", nodeIDOrName)
	}
	for _, node := range allNodes {
		if strings.EqualFold(node.Spec.NodeName, nodeIDOrName) {
			return node.Spec.NodeID, nil
		}
	}
	return "", fmt.Errorf("node %q not found", nodeIDOrName)
}

// --- ClusterInfoInjector PromptSection ---
//
// This section does NOT render its own text — the builtin ClusterAwarenessSection
// (Priority 150) handles the actual rendering. Instead, this injector populates
// the PromptContext.ClusterInfo field so that both IdentitySection and
// ClusterAwarenessSection can access Golem topology data.
//
// Priority 99 ensures it runs BEFORE IdentitySection (100) and
// ClusterAwarenessSection (150).

type clusterInfoInjector struct {
	plugin *golemClusterPlugin // lazy reference — registry is set after Init()
}

func (s *clusterInfoInjector) Name() string  { return "cluster_info_injector" }
func (s *clusterInfoInjector) Priority() int { return 99 }

func (s *clusterInfoInjector) Enabled(_ context.Context, _ *prompt.PromptContext) bool {
	return s.plugin.registry != nil
}

func (s *clusterInfoInjector) Render(_ context.Context, pc *prompt.PromptContext) (string, error) {
	if s.plugin.registry == nil {
		logger.Warn("[golem-cluster] clusterInfoInjector.Render: registry is nil!")
		return "", nil
	}

	nodes, err := s.plugin.registry.ListNodes(nil)
	if err != nil {
		logger.Warn("[golem-cluster] failed to list nodes for prompt injection: %v", err)
		return "", nil
	}

	logger.Info("[golem-cluster] clusterInfoInjector.Render: found %d nodes in registry", len(nodes))

	if len(nodes) == 0 {
		logger.Info("[golem-cluster] clusterInfoInjector.Render: no nodes registered, returning empty")
		return "", nil
	}

	// Populate ClusterInfo on the PromptContext — this is read by
	// IdentitySection and ClusterAwarenessSection.
	v := version.Get()
	clusterInfo := &prompt.ClusterInfo{
		HivemindID: "hivemind-0",
		Version:    v.GitVersion,
	}
	for _, n := range nodes {
		logger.Info("[golem-cluster] Render: node %s has %d capabilities, %d InstalledSkills",
			n.Spec.NodeName, len(n.Spec.Capabilities), len(n.Spec.InstalledSkills))
		golemInfo := prompt.GolemInfo{
			ID:     n.Spec.NodeID,
			Name:   n.Spec.NodeName,
			Status: nodeStatusLabel(n),
			Skills: capNames(n),
		}
		// Populate installed skills from the node's registered InstalledSkills.
		for _, sk := range n.Spec.InstalledSkills {
			golemInfo.InstalledSkills = append(golemInfo.InstalledSkills, prompt.GolemSkillInfo{
				Name:         sk.Name,
				Description:  sk.Description,
				Capabilities: sk.Capabilities,
			})
		}
		logger.Info("[golem-cluster] Render: golemInfo for %s: Skills=%v, InstalledSkills=%d",
			n.Spec.NodeName, golemInfo.Skills, len(golemInfo.InstalledSkills))
		clusterInfo.Golems = append(clusterInfo.Golems, golemInfo)
	}
	pc.ClusterInfo = clusterInfo

	// Return empty string — ClusterAwarenessSection handles the actual rendering.
	return "", nil
}

// --- Helpers ---

func nodeStatusLabel(n *registry.NodeState) string {
	switch {
	case n.Spec.Cordoned:
		return "Cordoned"
	default:
		// Convert proto enum to human-readable label.
		s := n.Status.Phase.String()
		// "NODE_STATUS_ONLINE" → "Online"
		s = strings.TrimPrefix(s, "NODE_STATUS_")
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
		}
		return "Unknown"
	}
}

func capNames(n *registry.NodeState) []string {
	if len(n.Spec.Capabilities) == 0 {
		return nil
	}
	names := make([]string, 0, len(n.Spec.Capabilities))
	for _, c := range n.Spec.Capabilities {
		names = append(names, c.Name)
	}
	return names
}

func installedSkillInfos(n *registry.NodeState) []nodeSkillInfo {
	if len(n.Spec.InstalledSkills) == 0 {
		return nil
	}
	skills := make([]nodeSkillInfo, len(n.Spec.InstalledSkills))
	for i, sk := range n.Spec.InstalledSkills {
		skills[i] = nodeSkillInfo{
			Name:         sk.Name,
			Description:  sk.Description,
			Capabilities: sk.Capabilities,
		}
	}
	return skills
}
