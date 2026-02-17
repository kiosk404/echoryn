package entity

import (
	"time"
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

// IsSubAgentSession returns true if this session was spawned by a sub-agent.
// Sub-agent sessions cannot spawn further sub-agents (max depth = 1).
// TODO(subagent): Use this check in SubAgentManager.Spawn() to enforce nesting limit.
func (s *Session) IsSubAgentSession() bool {
	return s.ParentSessionID != ""
}
