// Package protocol provides the Go domain types used by the scheduler and
// other internal services to represent Golem nodes, tasks, and their lifecycle
// These types mirror the protobuf definitions in pkg/proto/golem/ but are pure
// Go structs - free of protobuf dependencies - so internal packages can use them
// without pulling in the generated code.
// Conversion helpers between proto - domain types are provided in covert.go
package protocol

import (
	"time"

	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ==========================================================================
// Proto → Domain conversions
// ==========================================================================

// NodeInfoFromProto converts a protobuf NodeInfo to the domain type.
func NodeInfoFromProto(p *pb.NodeInfo) NodeInfo {
	if p == nil {
		return NodeInfo{}
	}
	ni := NodeInfo{
		ID:      p.Id,
		Name:    p.Name,
		Address: p.Address,
		Status:  nodeStatusFromProto(p.Status),
		Labels:  p.Labels,
		Version: p.Version,
	}
	if p.SystemInfo != nil {
		ni.SystemInfo = SystemInfo{
			CPUCores:   int(p.SystemInfo.CpuCores),
			MemoryMB:   int(p.SystemInfo.MemoryMb),
			DiskFreeMB: int(p.SystemInfo.DiskFreeMb),
			OS:         p.SystemInfo.Os,
			Arch:       p.SystemInfo.Arch,
			Hostname:   p.SystemInfo.Hostname,
		}
	}
	for _, c := range p.Capabilities {
		ni.Capabilities = append(ni.Capabilities, Capability{
			Name:        c.Name,
			Version:     c.Version,
			Description: c.Description,
		})
	}
	if p.RegisteredAt != nil {
		ni.RegisteredAt = p.RegisteredAt.AsTime()
	}
	if p.LastSeenAt != nil {
		ni.LastSeenAt = p.LastSeenAt.AsTime()
	}
	return ni
}

// NodeLoadInfoFromProto converts a protobuf NodeLoadInfo to the domain type.
func NodeLoadInfoFromProto(p *pb.NodeLoadInfo) NodeLoadInfo {
	if p == nil {
		return NodeLoadInfo{}
	}
	return NodeLoadInfo{
		CPUPercent:    p.CpuPercent,
		MemoryPercent: p.MemoryPercent,
		DiskFreeMB:    p.DiskFreeMb,
		ActiveTasks:   int(p.ActiveTasks),
		QueuedTasks:   int(p.QueuedTasks),
	}
}

// TaskFromProto converts a protobuf Task to the domain type.
func TaskFromProto(p *pb.Task) *Task {
	if p == nil {
		return nil
	}
	t := &Task{
		ID:             p.Id,
		Name:           p.Name,
		SkillName:      p.SkillName,
		Payload:        p.Payload,
		Status:         taskStatusFromProto(p.Status),
		Priority:       TaskPriority(p.Priority),
		AssignedNodeID: p.AssignedNodeId,
		SessionID:      p.SessionId,
		AgentID:        p.AgentId,
		Result:         p.Result,
		Error:          p.Error,
		Metadata:       p.Metadata,
	}
	if p.Timeout != nil {
		t.Timeout = p.Timeout.AsDuration()
	}
	t.CreatedAt = protoTSToTime(p.CreatedAt)
	if p.StartedAt != nil {
		st := p.StartedAt.AsTime()
		t.StartedAt = &st
	}
	if p.CompletedAt != nil {
		ct := p.CompletedAt.AsTime()
		t.CompletedAt = &ct
	}
	return t
}

// TaskResultFromProto converts a protobuf TaskResult to the domain type.
func TaskResultFromProto(p *pb.TaskResult) *TaskResult {
	if p == nil {
		return nil
	}
	return &TaskResult{
		TaskID:  p.TaskId,
		Success: p.Success,
		Output:  p.Output,
		Error:   p.Error,
	}
}

// TaskProgressFromProto converts a protobuf TaskProgress to the domain type.
func TaskProgressFromProto(p *pb.TaskProgress) *TaskProgress {
	if p == nil {
		return nil
	}
	return &TaskProgress{
		TaskID:          p.TaskId,
		ProgressPercent: p.ProgressPercent,
		StatusMessage:   p.StatusMessage,
		PartialResult:   p.PartialResult,
	}
}

// ==========================================================================
// Domain → Proto conversions
// ==========================================================================

// NodeInfoToProto converts a domain NodeInfo to the protobuf type.
func NodeInfoToProto(n *NodeInfo) *pb.NodeInfo {
	if n == nil {
		return nil
	}
	p := &pb.NodeInfo{
		Id:      n.ID,
		Name:    n.Name,
		Address: n.Address,
		Status:  nodeStatusToProto(n.Status),
		SystemInfo: &pb.SystemInfo{
			CpuCores:   int32(n.SystemInfo.CPUCores),
			MemoryMb:   int64(n.SystemInfo.MemoryMB),
			DiskFreeMb: int64(n.SystemInfo.DiskFreeMB),
			Os:         n.SystemInfo.OS,
			Arch:       n.SystemInfo.Arch,
			Hostname:   n.SystemInfo.Hostname,
		},
		Labels:       n.Labels,
		Version:      n.Version,
		RegisteredAt: timestamppb.New(n.RegisteredAt),
		LastSeenAt:   timestamppb.New(n.LastSeenAt),
	}
	for _, c := range n.Capabilities {
		p.Capabilities = append(p.Capabilities, &pb.Capability{
			Name:        c.Name,
			Version:     c.Version,
			Description: c.Description,
		})
	}
	return p
}

// NodeLoadInfoToProto converts a domain NodeLoadInfo to the protobuf type.
func NodeLoadInfoToProto(n *NodeLoadInfo) *pb.NodeLoadInfo {
	if n == nil {
		return nil
	}
	return &pb.NodeLoadInfo{
		CpuPercent:    n.CPUPercent,
		MemoryPercent: n.MemoryPercent,
		DiskFreeMb:    n.DiskFreeMB,
		ActiveTasks:   int32(n.ActiveTasks),
		QueuedTasks:   int32(n.QueuedTasks),
	}
}

// TaskToProto converts a domain Task to the protobuf type.
func TaskToProto(t *Task) *pb.Task {
	if t == nil {
		return nil
	}
	p := &pb.Task{
		Id:             t.ID,
		Name:           t.Name,
		SkillName:      t.SkillName,
		Payload:        t.Payload,
		Status:         taskStatusToProto(t.Status),
		Priority:       pb.TaskPriority(t.Priority),
		AssignedNodeId: t.AssignedNodeID,
		SessionId:      t.SessionID,
		AgentId:        t.AgentID,
		Result:         t.Result,
		Error:          t.Error,
		Metadata:       t.Metadata,
		CreatedAt:      timestamppb.New(t.CreatedAt),
	}
	if t.Timeout > 0 {
		p.Timeout = durationpb.New(t.Timeout)
	}
	if t.StartedAt != nil {
		p.StartedAt = timestamppb.New(*t.StartedAt)
	}
	if t.CompletedAt != nil {
		p.CompletedAt = timestamppb.New(*t.CompletedAt)
	}
	return p
}

// ==========================================================================
// Internal enum converters
// ==========================================================================

func nodeStatusFromProto(s pb.NodeStatus) string {
	switch s {
	case pb.NodeStatus_NODE_STATUS_ONLINE:
		return "online"
	case pb.NodeStatus_NODE_STATUS_OFFLINE:
		return "offline"
	case pb.NodeStatus_NODE_STATUS_DRAINING:
		return "draining"
	case pb.NodeStatus_NODE_STATUS_CORDONED:
		return "cordoned"
	default:
		return "unknown"
	}
}

func nodeStatusToProto(s string) pb.NodeStatus {
	switch s {
	case "online":
		return pb.NodeStatus_NODE_STATUS_ONLINE
	case "offline":
		return pb.NodeStatus_NODE_STATUS_OFFLINE
	case "draining":
		return pb.NodeStatus_NODE_STATUS_DRAINING
	case "cordoned":
		return pb.NodeStatus_NODE_STATUS_CORDONED
	default:
		return pb.NodeStatus_NODE_STATUS_UNSPECIFIED
	}
}

func taskStatusFromProto(s pb.TaskStatus) TaskStatusValue {
	switch s {
	case pb.TaskStatus_TASK_STATUS_PENDING:
		return TaskStatusPending
	case pb.TaskStatus_TASK_STATUS_ASSIGNED:
		return TaskStatusAssigned
	case pb.TaskStatus_TASK_STATUS_RUNNING:
		return TaskStatusRunning
	case pb.TaskStatus_TASK_STATUS_COMPLETED:
		return TaskStatusCompleted
	case pb.TaskStatus_TASK_STATUS_FAILED:
		return TaskStatusFailed
	case pb.TaskStatus_TASK_STATUS_CANCELLED:
		return TaskStatusCancelled
	case pb.TaskStatus_TASK_STATUS_TIMED_OUT:
		return TaskStatusTimedOut
	default:
		return TaskStatusPending
	}
}

func taskStatusToProto(s TaskStatusValue) pb.TaskStatus {
	switch s {
	case TaskStatusPending:
		return pb.TaskStatus_TASK_STATUS_PENDING
	case TaskStatusAssigned:
		return pb.TaskStatus_TASK_STATUS_ASSIGNED
	case TaskStatusRunning:
		return pb.TaskStatus_TASK_STATUS_RUNNING
	case TaskStatusCompleted:
		return pb.TaskStatus_TASK_STATUS_COMPLETED
	case TaskStatusFailed:
		return pb.TaskStatus_TASK_STATUS_FAILED
	case TaskStatusCancelled:
		return pb.TaskStatus_TASK_STATUS_CANCELLED
	case TaskStatusTimedOut:
		return pb.TaskStatus_TASK_STATUS_TIMED_OUT
	default:
		return pb.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func protoTSToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}
