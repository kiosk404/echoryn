package handler

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/internal/golem/service/node"
	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// TaskExecutor implements node.TaskHandler.
// It executes tasks dispatched from Hivemind via the heartbeat stream.
type TaskExecutor struct {
	nodeService *node.Service

	mu        sync.Mutex
	cancelFns map[string]context.CancelFunc // taskID → cancel function
}

var _ node.TaskHandler = (*TaskExecutor)(nil)

// NewTaskExecutor creates a new TaskExecutor.
func NewTaskExecutor(nodeService *node.Service) *TaskExecutor {
	return &TaskExecutor{
		nodeService: nodeService,
		cancelFns:   make(map[string]context.CancelFunc),
	}
}

// HandleTask executes a dispatched task asynchronously and reports the result.
func (h *TaskExecutor) HandleTask(ctx context.Context, task *pb.Task) {
	// Check if node is draining or cordoned.
	status := h.nodeService.Status()
	if status == pb.NodeStatus_NODE_STATUS_DRAINING || status == pb.NodeStatus_NODE_STATUS_CORDONED {
		logger.Warn("[Golem] rejecting task %s: node is %s", task.Id, status.String())
		h.reportResult(ctx, task.Id, false, nil, fmt.Sprintf("node is %s", status.String()))
		return
	}

	// Execute asynchronously.
	go h.executeTask(ctx, task)
}

// CancelTask cancels a running task.
func (h *TaskExecutor) CancelTask(taskID string, reason string) {
	logger.Info("[Golem] cancelling task: id=%s reason=%s", taskID, reason)

	h.mu.Lock()
	cancel, ok := h.cancelFns[taskID]
	h.mu.Unlock()

	if ok {
		cancel()
		logger.Info("[Golem] task %s cancelled", taskID)
	} else {
		logger.Warn("[Golem] task %s not found for cancellation", taskID)
	}
}

// executeTask runs a task synchronously and reports the result back to Hivemind.
func (h *TaskExecutor) executeTask(ctx context.Context, task *pb.Task) {
	h.nodeService.IncrActiveTasks()
	defer h.nodeService.DecrActiveTasks()

	// Create a cancellable context for this task.
	taskCtx, cancel := context.WithCancel(ctx)
	h.mu.Lock()
	h.cancelFns[task.Id] = cancel
	h.mu.Unlock()

	defer func() {
		cancel()
		h.mu.Lock()
		delete(h.cancelFns, task.Id)
		h.mu.Unlock()
	}()

	logger.Info("[Golem] executing task: id=%s skill=%s", task.Id, task.SkillName)

	var output string
	var execErr error

	switch task.SkillName {
	case "shell":
		output, execErr = h.executeShell(taskCtx, task.Payload)
	case "fileops":
		output, execErr = h.executeFileOps(taskCtx, task.Payload)
	default:
		execErr = fmt.Errorf("unknown skill: %s", task.SkillName)
	}

	if execErr != nil {
		logger.Warn("[Golem] task %s execution failed: %v", task.Id, execErr)
		h.reportResult(ctx, task.Id, false, []byte(output), execErr.Error())
		return
	}

	logger.Info("[Golem] task %s completed successfully (output_len=%d)", task.Id, len(output))
	h.reportResult(ctx, task.Id, true, []byte(output), "")
}

// reportResult sends the task result back to Hivemind via ReportTaskResult RPC.
func (h *TaskExecutor) reportResult(ctx context.Context, taskID string, success bool, output []byte, errMsg string) {
	result := &pb.TaskResult{
		TaskId:  taskID,
		Success: success,
		Output:  output,
		Error:   errMsg,
	}

	if err := h.nodeService.ReportTaskResult(ctx, result); err != nil {
		logger.Warn("[Golem] failed to report result for task %s: %v", taskID, err)
	}
}

// shellPayload is the expected JSON payload for the "shell" skill.
type shellPayload struct {
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir,omitempty"`
}

// executeShell executes a shell command and returns the combined output.
func (h *TaskExecutor) executeShell(ctx context.Context, payload []byte) (string, error) {
	var p shellPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("invalid shell payload: %w", err)
	}

	if p.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	// Apply a default timeout if context doesn't already have one.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	logger.Info("[Golem/shell] executing: %s", p.Command)

	cmd := exec.CommandContext(ctx, "sh", "-c", p.Command)
	if p.WorkingDir != "" {
		cmd.Dir = p.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Build result.
	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("[stderr] ")
		result.WriteString(stderr.String())
	}

	// Truncate if too large (gRPC message size limit).
	output := result.String()
	const maxOutputSize = 64 * 1024 // 64KB
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize] + "\n... (output truncated)"
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return output, fmt.Errorf("command exited with code %d: %s", exitErr.ExitCode(), output)
		}
		return output, fmt.Errorf("command execution failed: %w", err)
	}

	return output, nil
}

// fileOpsPayload is the expected JSON payload for the "fileops" skill.
type fileOpsPayload struct {
	Operation string `json:"operation"` // "read", "write", "list", "delete"
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"` // for "write" operation
}

// executeFileOps handles file operations.
func (h *TaskExecutor) executeFileOps(ctx context.Context, payload []byte) (string, error) {
	var p fileOpsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("invalid fileops payload: %w", err)
	}

	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	switch p.Operation {
	case "read":
		return h.executeShell(ctx, mustMarshal(shellPayload{Command: fmt.Sprintf("cat %q", p.Path)}))
	case "write":
		return h.executeShell(ctx, mustMarshal(shellPayload{Command: fmt.Sprintf("cat > %q << 'ECHORYN_EOF'\n%s\nECHORYN_EOF", p.Path, p.Content)}))
	case "list":
		return h.executeShell(ctx, mustMarshal(shellPayload{Command: fmt.Sprintf("ls -la %q", p.Path)}))
	case "delete":
		return h.executeShell(ctx, mustMarshal(shellPayload{Command: fmt.Sprintf("rm -f %q", p.Path)}))
	default:
		return "", fmt.Errorf("unknown file operation: %s (supported: read, write, list, delete)", p.Operation)
	}
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
