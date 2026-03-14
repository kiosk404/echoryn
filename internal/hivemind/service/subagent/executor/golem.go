package executor

import (
	"context"
	"fmt"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/scheduler"
	"github.com/kiosk404/echoryn/internal/hivemind/service/subagent/observer"
	"github.com/kiosk404/echoryn/internal/pkg/protocol"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// GolemExecutor dispatches SubAgent tasks to remote Golem nodes.
// It implements the N:1 mapping: multiple SubAgents can share a single Golem node.
//
// Core flow:
//  1. Build Golem ScheduleRequest from SubAgent ExecuteRequest
//  2. Apply node affinity policy (maps to ScheduleHints.Affinity)
//  3. Submit to Golem Scheduler
//  4. Track task→node mapping in NodeTaskIndex
//  5. Monitor task progress asynchronously
//
// Fault tolerance:
//   - Scheduling phase failure → direct fallback to LocalExecutor (no state migration needed)
//   - Execution phase failure → mark SubAgent as Failed, let TeamOrchestrator decide next steps
type GolemExecutor struct {
	scheduler     scheduler.Scheduler
	nodeTaskIndex *NodeTaskIndex
	localFallback Executor // for scheduling-phase fallback
	emitter       *observer.Emitter
}

// NewGolemExecutor creates a GolemExecutor with the given Golem scheduler.
func NewGolemExecutor(sched scheduler.Scheduler, localFallback Executor) *GolemExecutor {
	return &GolemExecutor{
		scheduler:     sched,
		nodeTaskIndex: NewNodeTaskIndex(),
		localFallback: localFallback,
		emitter:       observer.NewEmitter(nil), // no-op by default
	}
}

// NewGolemExecutorWithObserver creates a GolemExecutor with an attached Observer.
func NewGolemExecutorWithObserver(sched scheduler.Scheduler, localFallback Executor, obs observer.Observer) *GolemExecutor {
	return &GolemExecutor{
		scheduler:     sched,
		nodeTaskIndex: NewNodeTaskIndex(),
		localFallback: localFallback,
		emitter:       observer.NewEmitter(obs),
	}
}

func (e *GolemExecutor) Name() string { return "golem" }

// Execute dispatches the SubAgent task to a Golem node.
// If scheduling fails (no available nodes), it falls back to local execution.
func (e *GolemExecutor) Execute(ctx context.Context, req *ExecuteRequest) error {
	// Build ScheduleRequest from ExecuteRequest.
	schedReq := e.buildScheduleRequest(req)

	logger.Info("[GolemExecutor] scheduling SubAgent: session=%s, agent=%s, team=%s",
		req.SessionID, req.AgentID, req.TeamID)

	// Submit to scheduler.
	decision, err := e.scheduler.Schedule(ctx, schedReq)
	if err != nil {
		// Scheduling phase failure → fallback to local.
		logger.Warn("[GolemExecutor] scheduling failed: %v, falling back to local executor", err)
		e.emitter.Fallback("", req.SessionID, string(ExecutionStrategyGolem), err.Error())
		if e.localFallback != nil {
			return e.localFallback.Execute(ctx, req)
		}
		return fmt.Errorf("golem scheduling failed and no local fallback: %w", err)
	}

	// Record the assignment in the node-task index.
	if decision != nil && decision.SelectedNodeID != "" {
		e.nodeTaskIndex.Add(decision.SelectedNodeID, req.SessionID, req.TeamID)
		e.emitter.Routed("", req.SessionID, observer.LocationGolem, string(ExecutionStrategyGolem), decision.SelectedNodeID, req.TeamID)
		logger.Info("[GolemExecutor] SubAgent assigned to node: session=%s, node=%s",
			req.SessionID, decision.SelectedNodeID)
	}

	return nil
}

// buildScheduleRequest creates a Golem ScheduleRequest from a SubAgent ExecuteRequest.
func (e *GolemExecutor) buildScheduleRequest(req *ExecuteRequest) *scheduler.ScheduleRequest {
	task := &protocol.Task{
		ID:        fmt.Sprintf("subagent-%s", req.SessionID),
		Name:      fmt.Sprintf("SubAgent: %s", truncate(req.Input, 100)),
		SkillName: "subagent",
		SessionID: req.SessionID,
		AgentID:   req.AgentID,
		Metadata: map[string]string{
			"team_id": req.TeamID,
		},
	}

	schedReq := &scheduler.ScheduleRequest{
		Task: task,
		Mode: scheduler.AIMode,
		Hints: &scheduler.ScheduleHints{
			Description: fmt.Sprintf("SubAgent task: %s", truncate(req.Input, 200)),
		},
	}

	// Apply node affinity based on team membership.
	if req.TeamID != "" {
		e.applyAffinityPolicy(schedReq, req)
	}

	// Apply node selector as preferred tags.
	if len(req.NodeSelector) > 0 {
		schedReq.PreferredTags = req.NodeSelector
	}

	return schedReq
}

// applyAffinityPolicy translates NodeAffinityPolicy to ScheduleHints.
func (e *GolemExecutor) applyAffinityPolicy(schedReq *scheduler.ScheduleRequest, req *ExecuteRequest) {
	switch req.AffinityPolicy {
	case NodeAffinityPreferColocate, NodeAffinityRequireColocate:
		// Find existing team member nodes and set affinity.
		nodeIDs := e.nodeTaskIndex.GetNodesByTeam(req.TeamID)
		if len(nodeIDs) > 0 {
			// Prefer the first node that has team members.
			schedReq.Hints.Affinity = nodeIDs[0]
		}

	case NodeAffinityPreferSpread, NodeAffinityRequireSpread:
		// Set anti-affinity for nodes that already have team members.
		nodeIDs := e.nodeTaskIndex.GetNodesByTeam(req.TeamID)
		schedReq.Hints.AntiAffinity = nodeIDs
	}

	// Add team context for scheduler AI reasoning.
	if schedReq.Hints.CustomContext == nil {
		schedReq.Hints.CustomContext = make(map[string]string)
	}
	schedReq.Hints.CustomContext["team_id"] = req.TeamID
	schedReq.Hints.CustomContext["affinity_policy"] = string(req.AffinityPolicy)
}

// --- Node Task Index ---

// NodeTaskIndex maintains a reverse index of task→node mappings.
// It supports:
//   - Querying all active SubAgents on a given node
//   - Node affinity scheduling (co-locate team members)
//   - Node-level resource budget control
type NodeTaskIndex struct {
	mu sync.RWMutex
	// taskToNode maps sessionID → nodeID
	taskToNode map[string]string
	// nodeToTasks maps nodeID → set of sessionIDs
	nodeToTasks map[string]map[string]bool
	// taskToTeam maps sessionID → teamID
	taskToTeam map[string]string
}

// NewNodeTaskIndex creates an empty index.
func NewNodeTaskIndex() *NodeTaskIndex {
	return &NodeTaskIndex{
		taskToNode:  make(map[string]string),
		nodeToTasks: make(map[string]map[string]bool),
		taskToTeam:  make(map[string]string),
	}
}

// Add records a task→node mapping.
func (idx *NodeTaskIndex) Add(nodeID, sessionID, teamID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.taskToNode[sessionID] = nodeID
	if idx.nodeToTasks[nodeID] == nil {
		idx.nodeToTasks[nodeID] = make(map[string]bool)
	}
	idx.nodeToTasks[nodeID][sessionID] = true
	if teamID != "" {
		idx.taskToTeam[sessionID] = teamID
	}
}

// Remove cleans up a task mapping.
func (idx *NodeTaskIndex) Remove(sessionID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	nodeID, ok := idx.taskToNode[sessionID]
	if !ok {
		return
	}

	delete(idx.taskToNode, sessionID)
	delete(idx.taskToTeam, sessionID)
	if tasks, ok := idx.nodeToTasks[nodeID]; ok {
		delete(tasks, sessionID)
		if len(tasks) == 0 {
			delete(idx.nodeToTasks, nodeID)
		}
	}
}

// GetNodesByTeam returns all node IDs that have members of the given team.
func (idx *NodeTaskIndex) GetNodesByTeam(teamID string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	nodeSet := make(map[string]bool)
	for sessionID, tid := range idx.taskToTeam {
		if tid == teamID {
			if nodeID, ok := idx.taskToNode[sessionID]; ok {
				nodeSet[nodeID] = true
			}
		}
	}

	nodes := make([]string, 0, len(nodeSet))
	for nodeID := range nodeSet {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

// GetTasksOnNode returns all session IDs for tasks running on a given node.
func (idx *NodeTaskIndex) GetTasksOnNode(nodeID string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	tasks, ok := idx.nodeToTasks[nodeID]
	if !ok {
		return nil
	}

	result := make([]string, 0, len(tasks))
	for sessionID := range tasks {
		result = append(result, sessionID)
	}
	return result
}

// --- Helpers ---

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
