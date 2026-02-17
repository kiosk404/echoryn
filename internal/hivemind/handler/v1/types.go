package v1

import (
	"time"
)

// --- OpenAI Chat Completions API Types ---
// Modeled after OpenClaw's openai-http.ts request/response schemas.

// ChatCompletionRequest is the OpenAI-compatible request body for /v1/chat/completions.
type ChatCompletionRequest struct {
	// Model can be "eidolon", "eidolon/<agent-id>", or "agent:<agent-id>".
	Model string `json:"model"`

	// Messages is the conversation history.
	Messages []ChatMessage `json:"messages" binding:"required"`

	// Stream controls whether the response is streamed via SSE.
	Stream bool `json:"stream,omitempty"`

	// User is used for session key isolation (optional).
	User string `json:"user,omitempty"`

	// Temperature controls sampling (optional, overrides agent default).
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens limits the output tokens (optional, overrides agent default).
	MaxTokens *int `json:"max_tokens,omitempty"`
}

// ChatMessage is a single message in the OpenAI Chat Completions format.
type ChatMessage struct {
	Role       string          `json:"role" binding:"required"`
	Content    string          `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCalls  []ToolCallChunk `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// ToolCallChunk represents a tool call in OpenAI format.
type ToolCallChunk struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction represents the function part of a tool call.
type ToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// --- Non-streaming response ---

// ChatCompletionResponse is the OpenAI-compatible non-streaming response.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *ChatCompletionUsage   `json:"usage,omitempty"`
}

// ChatCompletionChoice is a single choice in the response.
type ChatCompletionChoice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	FinishReason string       `json:"finish_reason"`
}

// ChatCompletionUsage reports token usage.
type ChatCompletionUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// --- Streaming response (SSE chunks) ---

// ChatCompletionChunk is a single SSE chunk for streaming responses.
type ChatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []ChatCompletionChunkChoice `json:"choices"`
	Usage   *ChatCompletionUsage        `json:"usage,omitempty"`
}

// ChatCompletionChunkChoice is a single choice in a streaming chunk.
type ChatCompletionChunkChoice struct {
	Index        int               `json:"index"`
	Delta        *ChatMessageDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

// ChatMessageDelta is the delta payload in streaming mode.
type ChatMessageDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []ToolCallChunk `json:"tool_calls,omitempty"`
}

// --- Models API ---

// ModelObject is a single model in the OpenAI /v1/models response.
type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// ModelListResponse is the response for GET /v1/models.
type ModelListResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// --- Agent API ---

// CreateAgentRequest is the request body for POST /v1/agents.
type CreateAgentRequest struct {
	ID           string           `json:"id" binding:"required"`
	Name         string           `json:"name" binding:"required"`
	Description  string           `json:"description,omitempty"`
	SystemPrompt string           `json:"system_prompt"`
	ModelRef     *ModelRefRequest `json:"model_ref,omitempty"`
	Tools        []string         `json:"tools,omitempty"`
	MaxTurns     int              `json:"max_turns,omitempty"`
	Temperature  *float64         `json:"temperature,omitempty"`
	MaxTokens    *int             `json:"max_tokens,omitempty"`
}

// ModelRefRequest is a model reference in the API request.
type ModelRefRequest struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

// AgentResponse is the response for agent endpoints.
type AgentResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools,omitempty"`
	MaxTurns     int      `json:"max_turns,omitempty"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

// SessionResponse is the response for session endpoints.
type SessionResponse struct {
	ID           string `json:"id"`
	AgentID      string `json:"agent_id"`
	MessageCount int    `json:"message_count"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// --- Common ---

const timeFormat = time.RFC3339

// FormatTime formats a time value for API responses.
func FormatTime(t time.Time) string {
	return t.Format(timeFormat)
}
