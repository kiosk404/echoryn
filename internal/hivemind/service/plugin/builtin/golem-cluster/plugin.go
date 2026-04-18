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
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/scheduler"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/pkg/protocol"
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
	scheduler  scheduler.Scheduler
}

// Factory is the PluginFactory for golem-cluster.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfg := DefaultConfig()
	if c, ok := args["config"].(*Config); ok && c != nil {
		cfg = c
	}

	var reg registry.Registry
	var disp dispatcher.Dispatcher
	var sched scheduler.Scheduler
	if r, ok := args["registry"].(registry.Registry); ok {
		reg = r
	}
	if d, ok := args["dispatcher"].(dispatcher.Dispatcher); ok {
		disp = d
	}
	if s, ok := args["scheduler"].(scheduler.Scheduler); ok {
		sched = s
	}

	if !cfg.Enabled {
		logger.Info("[golem-cluster] plugin disabled via config")
	}

	return &golemClusterPlugin{
		cfg:        cfg,
		registry:   reg,
		dispatcher: disp,
		scheduler:  sched,
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
		Category: "cluster",
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

	api.RegisterTool(plugin.ToolDefinition{
		Name:        "cluster_execute_skill",
		Description: "Execute a skill on the most suitable Golem node, automatically selected by the AI scheduler. Unlike cluster_dispatch_task which requires you to specify a node, this tool uses LLM-enhanced scheduling to find the best node for the task based on skills, capabilities, load, and semantic fit.",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "skill_name",
				Type:        "string",
				Description: "The skill to execute (e.g., 'shell', 'fileops', 'data-analysis'). The scheduler will find nodes that have this skill installed.",
				Required:    true,
			},
			{
				Name:        "payload",
				Type:        "object",
				Description: "JSON parameters for the skill. For 'shell' skill: {\"command\": \"<shell command>\"}. For 'fileops' skill: {\"operation\": \"read|write|list\", \"path\": \"<file path>\", \"content\": \"<optional content for write>\"}.",
				Required:    true,
			},
			{
				Name:        "description",
				Type:        "string",
				Description: "A human-readable description of what this task does. Used by the LLM scheduler to semantically match the best node. Example: 'Analyze user behavior data from the last 3 days'.",
				Required:    false,
			},
			{
				Name:        "timeout",
				Type:        "string",
				Description: "Optional execution timeout (e.g., '30s', '5m'). Defaults to '30s'.",
				Required:    false,
			},
			{
				Name:        "preferred_tags",
				Type:        "object",
				Description: "Optional tag preferences for node selection (e.g., {\"env\": \"prod\", \"region\": \"us-west\"}).",
				Required:    false,
			},
		},
		Handler: p.handleExecuteSkill,
		Category: "cluster",
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
		return map[string]interface{}{
			"success": false,
			"error":   "Golem cluster infrastructure (Registry/Dispatcher) is not available. No Golem nodes are connected. Please complete the task using other available tools instead.",
		}, nil
	}

	// Pre-flight check: verify that at least one Golem node is registered.
	if nodes, err := p.registry.ListNodes(nil); err != nil || len(nodes) == 0 {
		logger.Warn("[golem-cluster] cluster_dispatch_task: no Golem nodes available, returning graceful error")
		return map[string]interface{}{
			"success": false,
			"error":   "No Golem worker nodes are currently connected to the cluster. Cannot dispatch tasks. Please complete the task using other available tools (e.g., direct LLM generation, llm_task) instead of dispatching to Golem nodes.",
		}, nil
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
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Target node %q not found in the cluster. Use cluster_list_nodes to see available nodes.", nodeIDOrName),
		}, nil
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
			return map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Node %q does not have capability or skill %q (capabilities: %v, skills: %v). Use cluster_list_nodes to check available skills on each node.", nodeIDOrName, skillName, caps, skillNames),
			}, nil
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
		logger.Warn("[golem-cluster] cluster_dispatch_task dispatch failed: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to dispatch task to node %q: %v. The node may be offline or unreachable. Please try a different approach.", nodeIDOrName, err),
		}, nil
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

func (p *golemClusterPlugin) handleExecuteSkill(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.scheduler == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Scheduler not available — cluster_execute_skill requires the scheduler to be initialized. No Golem nodes are connected to this Hivemind instance. Please complete the task using other available tools (e.g., direct LLM generation) instead of relying on Golem cluster execution.",
		}, nil
	}
	if p.registry == nil || p.dispatcher == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Golem cluster infrastructure (Registry/Dispatcher) is not available. No Golem nodes are connected. Please complete the task using other available tools instead.",
		}, nil
	}

	// Pre-flight check: verify that at least one Golem node is registered
	// before doing any heavy work. This provides a fast, clear failure message
	// to the LLM so it can choose an alternative approach.
	if nodes, err := p.registry.ListNodes(nil); err != nil || len(nodes) == 0 {
		logger.Warn("[golem-cluster] cluster_execute_skill: no Golem nodes available, returning graceful error")
		return map[string]interface{}{
			"success": false,
			"error":   "No Golem worker nodes are currently connected to the cluster. Cannot execute remote skills. Please complete the task using other available tools (e.g., direct LLM generation, llm_task) instead of dispatching to Golem nodes.",
		}, nil
	}

	// Parse required parameters.
	skillName, _ := params["skill_name"].(string)
	if skillName == "" {
		return nil, fmt.Errorf("skill_name parameter is required")
	}

	// Parse payload.
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

	// Parse optional parameters.
	description, _ := params["description"].(string)
	timeoutStr, _ := params["timeout"].(string)
	if timeoutStr == "" {
		timeoutStr = "30s"
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 30 * time.Second
	}

	// Parse preferred tags.
	var preferredTags map[string]string
	if tags, ok := params["preferred_tags"].(map[string]interface{}); ok {
		preferredTags = make(map[string]string, len(tags))
		for k, v := range tags {
			if s, ok := v.(string); ok {
				preferredTags[k] = s
			}
		}
	}

	// Build the protocol task.
	task := &protocol.Task{
		ID:        uuid.New().String(),
		Name:      fmt.Sprintf("execute-%s", skillName),
		SkillName: skillName,
		Payload:   payloadBytes,
		Status:    protocol.TaskStatusPending,
		Priority:  protocol.TaskPriorityNormal,
		Timeout:   timeout,
	}

	// Build the scheduling request with LLMMode.
	reqBuilder := scheduler.NewScheduleRequest(task).
		WithLLMMode().
		WithRequiredSkills(skillName)

	if len(preferredTags) > 0 {
		reqBuilder.WithPreferredTags(preferredTags)
	}
	if description != "" {
		reqBuilder.WithHints(&scheduler.ScheduleHints{
			Description: description,
		})
	}

	schedReq := reqBuilder.Build()

	logger.Info("[golem-cluster] executing skill %q via LLM-enhanced scheduler (task_id=%s)", skillName, task.ID)

	// Schedule the task — the scheduler will:
	// 1. Filter candidates by hard constraints
	// 2. LLM pre-filter the eligible nodes
	// 3. AISelector score the remaining candidates
	// 4. Dispatch to the best node
	decision, err := p.scheduler.Schedule(ctx, schedReq)
	if err != nil {
		// Return scheduling failures as a graceful result so the LLM can adapt
		// instead of crashing the entire agent flow.
		logger.Warn("[golem-cluster] cluster_execute_skill scheduling failed: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Golem scheduling failed: %v. No suitable Golem node could be found or dispatched to. Please complete the task using other available tools instead.", err),
		}, nil
	}

	// Build result.
	result := map[string]interface{}{
		"task_id":       task.ID,
		"selected_node": decision.SelectedNodeID,
		"schedule_mode": string(decision.Mode),
		"reason":        decision.Reason,
		"candidates":    decision.CandidateCount,
		"eligible":      decision.EligibleCount,
		"latency_ms":    decision.Latency.Milliseconds(),
		"skill":         skillName,
		"success":       true,
	}

	// Include scoring breakdown if available.
	if len(decision.Scores) > 0 {
		scores := make([]map[string]interface{}, 0, len(decision.Scores))
		for _, s := range decision.Scores {
			if !s.Eligible {
				continue
			}
			scores = append(scores, map[string]interface{}{
				"node_id":    s.NodeID,
				"total":      fmt.Sprintf("%.3f", s.TotalScore),
				"capability": fmt.Sprintf("%.2f", s.CapabilityScore),
				"skill":      fmt.Sprintf("%.2f", s.SkillScore),
				"resource":   fmt.Sprintf("%.2f", s.ResourceScore),
				"load":       fmt.Sprintf("%.2f", s.LoadScore),
				"tag":        fmt.Sprintf("%.2f", s.TagScore),
				"affinity":   fmt.Sprintf("%.2f", s.AffinityScore),
			})
		}
		result["scoring_breakdown"] = scores
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
	// Always populate ClusterInfo — even when no nodes are registered.
	// This allows ClusterAwarenessSection to render a "no nodes available" notice
	// so the LLM knows not to attempt Golem-based tool calls.
	v := version.Get()
	clusterInfo := &prompt.ClusterInfo{
		HivemindID: "hivemind-0",
		Version:    v.GitVersion,
	}

	if s.plugin.registry == nil {
		logger.Warn("[golem-cluster] clusterInfoInjector.Render: registry is nil, injecting empty ClusterInfo")
		pc.ClusterInfo = clusterInfo
		return "", nil
	}

	nodes, err := s.plugin.registry.ListNodes(nil)
	if err != nil {
		logger.Warn("[golem-cluster] failed to list nodes for prompt injection: %v", err)
		pc.ClusterInfo = clusterInfo
		return "", nil
	}

	logger.Info("[golem-cluster] clusterInfoInjector.Render: found %d nodes in registry", len(nodes))

	if len(nodes) == 0 {
		logger.Info("[golem-cluster] clusterInfoInjector.Render: no nodes registered, injecting empty ClusterInfo")
		pc.ClusterInfo = clusterInfo
		return "", nil
	}

	// Populate ClusterInfo on the PromptContext — this is read by
	// IdentitySection and ClusterAwarenessSection.
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
