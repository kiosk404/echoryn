package agentflow

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ---------------------------------------------------------------------------
// MessageSanitizer — request-side message preprocessing pipeline
// ---------------------------------------------------------------------------
//
// Aligned with OpenClaw's request-side stream wrappers that transform the
// message array BEFORE sending it to the LLM:
//   - dropThinkingBlocks (thinking.ts)  → ThinkingBlockSanitizer
//   - sanitizeToolCallIds (tool-call-id.ts) → ToolCallIDSanitizer
//   - wrapStreamTrimToolCallNames (attempt.ts) request-side → ToolCallNameTrimSanitizer

// MessageSanitizer cleans or transforms a slice of messages before they are
// sent to the LLM. Implementations MUST NOT mutate the input slice; they
// return a new slice if changes were made, or the original slice if nothing
// was changed (reference-equality optimization, aligned with OpenClaw).
type MessageSanitizer interface {
	// Name returns a human-readable identifier for logging.
	Name() string
	// Sanitize processes messages and returns a (potentially new) slice.
	Sanitize(messages []*schema.Message) []*schema.Message
}

// SanitizerPipeline chains multiple MessageSanitizer instances.
// It applies each sanitizer in registration order.
type SanitizerPipeline struct {
	sanitizers []MessageSanitizer
}

// NewSanitizerPipeline creates an empty pipeline.
func NewSanitizerPipeline() *SanitizerPipeline {
	return &SanitizerPipeline{}
}

// Register appends a sanitizer to the pipeline.
func (p *SanitizerPipeline) Register(s MessageSanitizer) *SanitizerPipeline {
	p.sanitizers = append(p.sanitizers, s)
	return p
}

// Apply runs all registered sanitizers in order.
func (p *SanitizerPipeline) Apply(messages []*schema.Message) []*schema.Message {
	if p == nil || len(p.sanitizers) == 0 {
		return messages
	}
	for _, s := range p.sanitizers {
		messages = s.Sanitize(messages)
	}
	return messages
}

// NewDefaultSanitizerPipeline creates a pipeline with all built-in sanitizers
// registered in the recommended order (aligned with OpenClaw's wrapper ordering):
//
//	ThinkingBlockSanitizer → ToolCallIDSanitizer → ToolCallNameTrimSanitizer
func NewDefaultSanitizerPipeline() *SanitizerPipeline {
	return NewSanitizerPipeline().
		Register(&ThinkingBlockSanitizer{}).
		Register(&ToolCallIDSanitizer{}).
		Register(&ToolCallNameTrimSanitizer{})
}

// ThinkingBlockSanitizer
//
// Aligned with OpenClaw's dropThinkingBlocks (thinking.ts):
// Strips ReasoningContent from assistant messages in the conversation history.
//
// Some providers (e.g., GitHub Copilot's Claude endpoint) reject subsequent
// requests that contain "thinking" blocks (with thinkingSignature). This
// sanitizer removes them from the history before sending to the LLM.
//
// Unlike OpenClaw which operates on multi-content blocks with type:"thinking",
// Eino uses a flat ReasoningContent field, so we simply clear it.
//
// Multi-turn thinking preservation (inspired by DeerFlow):
//
//   - DeepSeek requires reasoning_content on ALL assistant messages in multi-turn.
//     Set KeepReasoningContent=true for DeepSeek provider.
//   - Claude/Gemini/most providers: strip thinking blocks (default behavior).
//   - Gemini via OpenAI gateway: thought_signature in Extra is preserved by default
//     (Extra is not touched by this sanitizer).
type ThinkingBlockSanitizer struct {
	// KeepReasoningContent when true, preserves ReasoningContent on assistant
	// messages instead of stripping them. Set this for providers like DeepSeek
	// that require reasoning_content on all assistant messages in multi-turn
	// conversations. Default false strips ReasoningContent (safe for most providers).
	KeepReasoningContent bool
}

func (s *ThinkingBlockSanitizer) Name() string { return "thinking_block_sanitizer" }

func (s *ThinkingBlockSanitizer) Sanitize(messages []*schema.Message) []*schema.Message {
	if s.KeepReasoningContent {
		return messages
	}

	modified := false
	for _, msg := range messages {
		if msg.Role == schema.Assistant && msg.ReasoningContent != "" {
			modified = true
			break
		}
	}
	if !modified {
		return messages // Reference-equality: no allocation when nothing changes.
	}

	result := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		if msg.Role == schema.Assistant && msg.ReasoningContent != "" {
			// Shallow copy the message, clear ReasoningContent.
			cleaned := *msg
			cleaned.ReasoningContent = ""
			result[i] = &cleaned
			logger.Debug("[Sanitizer/ThinkingBlock] stripped ReasoningContent from assistant message %d", i)
		} else {
			result[i] = msg
		}
	}
	return result
}

// ToolCallIDSanitizer
//
// Aligned with OpenClaw's sanitizeToolCallIds (tool-call-id.ts):
// Normalizes tool call IDs to ensure they meet provider format requirements.
//
// Different providers have different ID format requirements:
//   - Mistral: [a-zA-Z0-9]{9} (strict 9-char alphanumeric)
//   - Most others: alphanumeric + some punctuation
//
// This sanitizer ensures all tool call IDs and their corresponding tool result
// references use a consistent, safe format. It also guarantees uniqueness.
type ToolCallIDSanitizer struct {
	// Strict9 enables Mistral-compatible mode: IDs are truncated/hashed to
	// exactly 9 alphanumeric characters. Default false uses "strict" mode
	// (strips non-alphanumeric but preserves length).
	Strict9 bool
}

func (s *ToolCallIDSanitizer) Name() string { return "tool_call_id_sanitizer" }

func (s *ToolCallIDSanitizer) Sanitize(messages []*schema.Message) []*schema.Message {
	// Build a mapping from original ID → sanitized ID.
	idMap := make(map[string]string)
	usedIDs := make(map[string]struct{})
	modified := false

	// First pass: collect all tool call IDs and build the mapping.
	for _, msg := range messages {
		if msg.Role == schema.Assistant {
			for _, tc := range msg.ToolCalls {
				if tc.ID == "" {
					continue
				}
				if _, exists := idMap[tc.ID]; exists {
					continue
				}
				sanitized := s.sanitizeID(tc.ID)
				sanitized = s.makeUnique(sanitized, usedIDs)
				if sanitized != tc.ID {
					modified = true
				}
				idMap[tc.ID] = sanitized
				usedIDs[sanitized] = struct{}{}
			}
		}
	}

	if !modified {
		return messages // Reference-equality optimization.
	}

	// Second pass: rewrite IDs in both assistant tool calls and tool result messages.
	result := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case schema.Assistant:
			if len(msg.ToolCalls) == 0 {
				result[i] = msg
				continue
			}
			cleaned := *msg
			cleaned.ToolCalls = make([]schema.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				cleaned.ToolCalls[j] = tc
				if newID, ok := idMap[tc.ID]; ok {
					cleaned.ToolCalls[j].ID = newID
				}
			}
			result[i] = &cleaned

		case schema.Tool:
			if newID, ok := idMap[msg.ToolCallID]; ok && newID != msg.ToolCallID {
				cleaned := *msg
				cleaned.ToolCallID = newID
				result[i] = &cleaned
			} else {
				result[i] = msg
			}

		default:
			result[i] = msg
		}
	}

	logger.Debug("[Sanitizer/ToolCallID] rewritten %d tool call IDs", len(idMap))
	return result
}

// sanitizeID strips non-alphanumeric characters and optionally truncates to 9 chars.
func (s *ToolCallIDSanitizer) sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	sanitized := b.String()
	if s.Strict9 && len(sanitized) > 9 {
		// Hash to ensure uniqueness within 9 chars (aligned with OpenClaw's makeUniqueToolId).
		h := sha256.Sum256([]byte(id))
		sanitized = fmt.Sprintf("%x", h[:5])[:9]
	}
	if sanitized == "" {
		// Fallback: generate a hash-based ID from the original.
		h := sha256.Sum256([]byte(id))
		sanitized = fmt.Sprintf("tc%x", h[:4])
	}
	return sanitized
}

// makeUnique appends a suffix if the ID already exists in the used set.
func (s *ToolCallIDSanitizer) makeUnique(id string, used map[string]struct{}) string {
	if _, exists := used[id]; !exists {
		return id
	}
	for i := 1; i < 100; i++ {
		candidate := fmt.Sprintf("%s%d", id, i)
		if s.Strict9 && len(candidate) > 9 {
			candidate = candidate[:9]
		}
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
	// Extreme fallback: hash.
	h := sha256.Sum256([]byte(id + "unique"))
	return fmt.Sprintf("%x", h[:5])
}

// ---------------------------------------------------------------------------
// ToolCallNameTrimSanitizer
// ---------------------------------------------------------------------------
//
// Aligned with OpenClaw's wrapStreamTrimToolCallNames (attempt.ts):
// Trims whitespace from tool call names in assistant messages.
//
// Some models (notably certain fine-tuned variants) emit tool call names with
// leading/trailing spaces like " read_file ". Since tool dispatch uses exact
// string matching, this causes lookup failures.
//
// This is the request-side companion to TrimToolCallNamesMiddleware (response-side).
// It cleans historical messages, while the middleware cleans live streaming chunks.
type ToolCallNameTrimSanitizer struct{}

func (s *ToolCallNameTrimSanitizer) Name() string { return "tool_call_name_trim_sanitizer" }

func (s *ToolCallNameTrimSanitizer) Sanitize(messages []*schema.Message) []*schema.Message {
	modified := false
	for _, msg := range messages {
		if msg.Role != schema.Assistant {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name != strings.TrimSpace(tc.Function.Name) {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}
	if !modified {
		return messages // Reference-equality optimization.
	}

	result := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		if msg.Role != schema.Assistant || len(msg.ToolCalls) == 0 {
			result[i] = msg
			continue
		}
		cleaned := *msg
		cleaned.ToolCalls = make([]schema.ToolCall, len(msg.ToolCalls))
		for j, tc := range msg.ToolCalls {
			cleaned.ToolCalls[j] = tc
			cleaned.ToolCalls[j].Function.Name = strings.TrimSpace(tc.Function.Name)
		}
		result[i] = &cleaned
	}

	logger.Debug("[Sanitizer/ToolCallNameTrim] trimmed tool call names in messages")
	return result
}
