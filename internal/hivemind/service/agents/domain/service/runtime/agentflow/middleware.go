package agentflow

import (
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ---------------------------------------------------------------------------
// StreamMiddleware — response-side streaming chunk processing pipeline
// ---------------------------------------------------------------------------
//
// Aligned with OpenClaw's response-side stream wrappers that intercept and
// transform chunks as they flow from the LLM to the client:
//   - wrapStreamTrimToolCallNames (attempt.ts) → TrimToolCallNamesMiddleware
//   - payloadLogger (anthropic-payload-log.ts) → ChunkLoggerMiddleware
//
// Unlike OpenClaw which wraps the entire stream's asyncIterator, Echoryn
// integrates middleware into the ReplayChunkCallback's consumption loop.
// This is more idiomatic for the Eino callback model.

// StreamMiddleware processes a single streaming schema.Message chunk in-flight.
// Implementations should be lightweight and non-blocking.
type StreamMiddleware interface {
	// Name returns a human-readable identifier for logging/debugging.
	Name() string
	// ProcessChunk transforms a message chunk. It may return the same pointer
	// if no changes are needed (reference-equality optimization).
	ProcessChunk(chunk *schema.Message) *schema.Message
}

// StreamMiddlewareChain applies a sequence of StreamMiddleware to each chunk.
type StreamMiddlewareChain struct {
	middlewares []StreamMiddleware
}

// NewStreamMiddlewareChain creates an empty chain.
func NewStreamMiddlewareChain() *StreamMiddlewareChain {
	return &StreamMiddlewareChain{}
}

// Register appends a middleware to the chain.
func (c *StreamMiddlewareChain) Register(m StreamMiddleware) *StreamMiddlewareChain {
	c.middlewares = append(c.middlewares, m)
	return c
}

// Apply runs all registered middlewares on a chunk in order.
// Returns the (potentially transformed) chunk.
func (c *StreamMiddlewareChain) Apply(chunk *schema.Message) *schema.Message {
	if c == nil || len(c.middlewares) == 0 || chunk == nil {
		return chunk
	}
	for _, m := range c.middlewares {
		chunk = m.ProcessChunk(chunk)
		if chunk == nil {
			return nil // Middleware filtered out the chunk entirely.
		}
	}
	return chunk
}

// NewDefaultStreamMiddlewareChain creates a chain with all built-in middlewares
// registered in the recommended order:
//
//	TrimToolCallNamesMiddleware → ChunkLoggerMiddleware
func NewDefaultStreamMiddlewareChain() *StreamMiddlewareChain {
	return NewStreamMiddlewareChain().
		Register(&TrimToolCallNamesMiddleware{}).
		Register(&ChunkLoggerMiddleware{Enabled: false})
}

// ---------------------------------------------------------------------------
// TrimToolCallNamesMiddleware
// ---------------------------------------------------------------------------
//
// Aligned with OpenClaw's wrapStreamTrimToolCallNames (attempt.ts):
// Trims whitespace from tool call function names in streaming chunks.
//
// This is the response-side companion to ToolCallNameTrimSanitizer (request-side).
// The sanitizer cleans historical messages, while this middleware cleans live
// streaming output from the model in real-time.
//
// Applied unconditionally (same as OpenClaw — attempt.ts line 847).
type TrimToolCallNamesMiddleware struct{}

func (m *TrimToolCallNamesMiddleware) Name() string { return "trim_tool_call_names" }

func (m *TrimToolCallNamesMiddleware) ProcessChunk(chunk *schema.Message) *schema.Message {
	if len(chunk.ToolCalls) == 0 {
		return chunk
	}

	modified := false
	for _, tc := range chunk.ToolCalls {
		if tc.Function.Name != strings.TrimSpace(tc.Function.Name) {
			modified = true
			break
		}
	}
	if !modified {
		return chunk // Reference-equality: no allocation.
	}

	cleaned := *chunk
	cleaned.ToolCalls = make([]schema.ToolCall, len(chunk.ToolCalls))
	for i, tc := range chunk.ToolCalls {
		cleaned.ToolCalls[i] = tc
		cleaned.ToolCalls[i].Function.Name = strings.TrimSpace(tc.Function.Name)
	}
	return &cleaned
}

// ChunkLoggerMiddleware Aligned with OpenClaw's anthropicPayloadLogger / cacheTrace (debug tools).
// Logs streaming chunk metadata for debugging/diagnostics.
//
// Unlike OpenClaw's JSONL file writers, Echoryn uses the structured logger
// subsystem. This middleware is disabled by default and can be enabled via
// configuration for troubleshooting streaming issues.
type ChunkLoggerMiddleware struct {
	// Enabled controls whether logging is active. When false, ProcessChunk
	// is a no-op pass-through (zero overhead).
	Enabled bool
}

func (m *ChunkLoggerMiddleware) Name() string { return "chunk_logger" }

func (m *ChunkLoggerMiddleware) ProcessChunk(chunk *schema.Message) *schema.Message {
	if !m.Enabled || chunk == nil {
		return chunk
	}

	// Log a concise summary of the chunk for debugging.
	var parts []string
	if chunk.Content != "" {
		parts = append(parts, "text:"+truncate(chunk.Content, 50))
	}
	if chunk.ReasoningContent != "" {
		parts = append(parts, "reasoning:"+truncate(chunk.ReasoningContent, 50))
	}
	if len(chunk.ToolCalls) > 0 {
		for _, tc := range chunk.ToolCalls {
			parts = append(parts, "tool:"+tc.Function.Name)
		}
	}

	if len(parts) > 0 {
		logger.Debug("[StreamMiddleware/ChunkLogger] chunk: %s", strings.Join(parts, ", "))
	}

	return chunk // Pass-through, no mutation.
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
