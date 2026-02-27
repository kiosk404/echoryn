package protocol

import (
	"time"
)

// ==========================================================================
// Node Types
// ==========================================================================

// NodeStatus represents the operational state of a Golem node.
type NodeStatus string

const (
	NodeStatusOnline   NodeStatus = "online"
	NodeStatusOffline  NodeStatus = "offline"
	NodeStatusDraining NodeStatus = "draining"
	NodeStatusCordoned NodeStatus = "cordoned"
)

// Capability describes a single capability advertised by a Golem node.
type Capability struct {
	Name        string
	Version     string
	Description string
}

// SystemInfo contains static hardware / OS information reported once at registration.
type SystemInfo struct {
	CPUCores   int
	MemoryMB   int
	DiskFreeMB int
	OS         string
	Arch       string
	Hostname   string
}

// NodeInfo is the static registration data for a Golem node.
type NodeInfo struct {
	ID           string
	Name         string
	Address      string
	Status       string // "online", "offline", "draining", "cordoned"
	SystemInfo   SystemInfo
	Capabilities []Capability
	Labels       map[string]string
	Version      string
	RegisteredAt time.Time
	LastSeenAt   time.Time
}

// NodeLoadInfo is the dynamic load snapshot sent with each heartbeat.
type NodeLoadInfo struct {
	CPUPercent    float64
	MemoryPercent float64
	DiskFreeMB    int64
	ActiveTasks   int
	QueuedTasks   int
}

// ==========================================================================
// Task Types
// ==========================================================================

// TaskStatus represents the lifecycle state of a task.
type TaskStatusValue string

const (
	TaskStatusPending   TaskStatusValue = "pending"
	TaskStatusAssigned  TaskStatusValue = "assigned"
	TaskStatusRunning   TaskStatusValue = "running"
	TaskStatusCompleted TaskStatusValue = "completed"
	TaskStatusFailed    TaskStatusValue = "failed"
	TaskStatusCancelled TaskStatusValue = "cancelled"
	TaskStatusTimedOut  TaskStatusValue = "timed_out"
)

// TaskPriority controls scheduling order (higher value = higher priority).
type TaskPriority int

const (
	TaskPriorityLow      TaskPriority = 1
	TaskPriorityNormal   TaskPriority = 5
	TaskPriorityHigh     TaskPriority = 8
	TaskPriorityCritical TaskPriority = 10
)

// Task is the central work unit dispatched to Golem nodes.
type Task struct {
	ID             string
	Name           string
	SkillName      string
	Payload        []byte // JSON-encoded skill parameters
	Status         TaskStatusValue
	Priority       TaskPriority
	AssignedNodeID string
	SessionID      string
	AgentID        string
	Result         []byte // JSON-encoded result
	Error          string
	Timeout        time.Duration
	CreatedAt      time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	Metadata       map[string]string
}

// TaskProgress carries incremental progress from a running task.
type TaskProgress struct {
	TaskID          string
	ProgressPercent float64
	StatusMessage   string
	PartialResult   []byte
}

// TaskResult carries the final outcome of a completed task.
type TaskResult struct {
	TaskID  string
	Success bool
	Output  []byte
	Error   string
}
