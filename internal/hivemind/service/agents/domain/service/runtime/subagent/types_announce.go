package subagent

// AnnounceDeliveryMode describes how the announcement was delivered.
//
// v2: Only two delivery modes remain:
//   - Steer: inject into running agent turn (real-time, lowest latency)
//   - Queue (pending): store for runner to consume during session persistence
//
// The "direct" mode is eliminated. Previously, direct delivery wrote to the
// session independently (via SessionWriteLock) and triggered a new agent turn,
// causing the dual-lock lost-update problem. Now, all pending announcements
// are consumed by the runner within its own session write path.
type AnnounceDeliveryMode string

const (
	// AnnounceDeliverySteer injects the event into the currently running agent turn.
	AnnounceDeliverySteer AnnounceDeliveryMode = "steer"

	// AnnounceDeliveryQueue enqueues the event for delivery by the runner when
	// the current run completes (or triggers a new run if parent is idle).
	AnnounceDeliveryQueue AnnounceDeliveryMode = "queue"
)

// steerChannelBufferSize is the buffer size for steer channels.
// Matches DefaultMaxChildrenPerAgent so all children can announce without blocking.
const steerChannelBufferSize = 8

// SteerChannel enables injecting messages into a running agent turn.
// The Ch field is a buffered channel that the runner polls between LLM calls
// to pick up sub-agent announcements in real time (lowest latency path).
//
// Created by SessionController.MarkRunActive() and consumed by
// SteerAwareChatModel in the AgentFlow.
type SteerChannel struct {
	// Ch is a buffered channel for injecting messages into the active run.
	// Capacity 8 matches the default max children per agent.
	Ch chan string
}
