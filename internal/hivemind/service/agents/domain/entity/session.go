package entity

import (
	"fmt"
	"strconv"
	"time"

	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// Session represents a persistent conversation context between a user and an agent.
//
// Modeled after:
// - OpenClaw: SessionManager with message history and metadata + compaction tracking
type Session struct {
	// ID is the unique session identifier.
	ID string `json:"id"`

	// AgentID is the agent this session is bound to.
	AgentID string `json:"agent_id"`

	// ParentSessionID is set when this session was spawned by a sub-agent.
	// Empty for top-level sessions. Used to enforce max nesting depth (1).
	// TODO(subagent): Populate this field when SubAgentManager.Spawn() creates the session.
	ParentSessionID string `json:"parent_session_id,omitempty"`

	// Messages is the ordered history of all messages in this session.
	Messages []*Message `json:"messages"`

	// Usage tracks cumulative token usage across all runs.
	Usage *TokenUsage `json:"usage,omitempty"`

	// Metadata holds arbitrary key-value pairs for extensibility.
	Metadata map[string]string `json:"metadata,omitempty"`

	// --- Compaction state (OpenClaw equivalent: compactionCount + summary) ---

	// CompactionSummary holds the LLM-generated summary of compacted messages.
	// When present, this replaces all messages before FirstKeptIndex.
	CompactionSummary string `json:"compaction_summary,omitempty"`

	// CompactionCount tracks how many times this session has been compacted.
	CompactionCount int `json:"compaction_count,omitempty"`

	// MemoryFlushCompactionCount records the CompactionCount value at which
	// the last memory flush was performed. This prevents multiple flushes
	// within the same compaction cycle (aligned with OpenClaw's memoryFlushCompactionCount).
	// A nil-equivalent (0 with CompactionCount > 0) means no flush in this cycle.
	MemoryFlushCompactionCount int `json:"memory_flush_compaction_count,omitempty"`

	// MemoryFlushAt is the timestamp of the last memory flush.
	MemoryFlushAt *time.Time `json:"memory_flush_at,omitempty"`

	// ThinkingLevel is the session-level thinking level override.
	// Persisted across turns so that a user's `/think medium` directive
	// remains effective for subsequent messages in the same session.
	// Empty means no session-level override (use agent/model default).
	//
	// Priority: per-message directive > session > agent > model default.
	// Aligned with OpenClaw's sessionEntry.thinkingLevel.
	ThinkingLevel llmEntity.ThinkingLevel `json:"thinking_level,omitempty"`

	// FirstKeptIndex is the index in Messages from which history is kept verbatim.
	// Messages[0:FirstKeptIndex] have been summarized into CompactionSummary.
	FirstKeptIndex int `json:"first_kept_index,omitempty"`

	// CreatedAt is when this session was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this session was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// AppendMessage appends a message to the session history.
func (s *Session) AppendMessage(msg *Message) {
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// AppendMessages appends multiple messages to the session history.
func (s *Session) AppendMessages(msgs []*Message) {
	s.Messages = append(s.Messages, msgs...)
	s.UpdatedAt = time.Now()
}

// AddUsage accumulates token usage.
func (s *Session) AddUsage(usage *TokenUsage) {
	if usage == nil {
		return
	}
	if s.Usage == nil {
		s.Usage = &TokenUsage{}
	}
	s.Usage.PromptTokens += usage.PromptTokens
	s.Usage.CompletionTokens += usage.CompletionTokens
	s.Usage.TotalTokens += usage.TotalTokens
}

// ActiveMessages returns the messages that are still active (not compacted).
// If compaction has occurred, only messages from FirstKeptIndex onward are returned.
func (s *Session) ActiveMessages() []*Message {
	if s.FirstKeptIndex >= len(s.Messages) {
		return nil
	}
	return s.Messages[s.FirstKeptIndex:]
}

// ApplyCompaction records a compaction result.
// summary is the LLM-generated summary of the compacted messages.
// keptFrom is the index from which messages are kept verbatim.
func (s *Session) ApplyCompaction(summary string, keptFrom int) {
	s.CompactionSummary = summary
	s.FirstKeptIndex = keptFrom
	s.CompactionCount++
	s.UpdatedAt = time.Now()
}

// HasCompaction returns true if this session has been compacted at least once.
func (s *Session) HasCompaction() bool {
	return s.CompactionSummary != ""
}

// ShouldMemoryFlush returns true if a memory flush should be executed.
// Conditions (aligned with OpenClaw's shouldRunMemoryFlush):
//  1. Session must have active messages
//  2. The flush hasn't already been done in this compaction cycle
//     (memoryFlushCompactionCount != compactionCount only when a flush was recorded)
func (s *Session) ShouldMemoryFlush() bool {
	if len(s.ActiveMessages()) < 4 {
		return false
	}
	// If MemoryFlushCompactionCount matches CompactionCount and MemoryFlushAt is set,
	// a flush was already performed in this compaction cycle — skip.
	if s.MemoryFlushAt != nil && s.MemoryFlushCompactionCount == s.CompactionCount {
		return false
	}
	return true
}

// RecordMemoryFlush records that a memory flush was performed.
func (s *Session) RecordMemoryFlush() {
	now := time.Now()
	s.MemoryFlushAt = &now
	s.MemoryFlushCompactionCount = s.CompactionCount
	s.UpdatedAt = now
}

// IsSubAgentSession returns true if this session was spawned by a sub-agent.
func (s *Session) IsSubAgentSession() bool {
	return s.ParentSessionID != ""
}

// SpawnDepth returns the current nesting depth of this session.
// Top-level sessions have depth 0, direct sub-agents have depth 1, etc.
// Aligned with OpenClaw's getSubagentDepthFromSessionStore.
func (s *Session) SpawnDepth() int {
	depth, _ := s.MetadataInt("spawn_depth")
	return depth
}

// SetSpawnDepth sets the spawn depth in session metadata.
func (s *Session) SetSpawnDepth(depth int) {
	if s.Metadata == nil {
		s.Metadata = make(map[string]string)
	}
	s.Metadata["spawn_depth"] = fmt.Sprintf("%d", depth)
}

// MetadataInt reads an integer from session metadata.
func (s *Session) MetadataInt(key string) (int, bool) {
	if s.Metadata == nil {
		return 0, false
	}
	v, ok := s.Metadata[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
