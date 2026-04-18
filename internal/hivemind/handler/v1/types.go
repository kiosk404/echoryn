package v1

import (
	"time"
)

// --- OpenAI Chat Completions API Types ---
// Modeled after OpenClaw's openai-http.ts request/response schemas.

// ChatCompletionRequest is the OpenAI-compatible request body for /v1/chat/completions.
type ChatCompletionRequest struct {
	// Model can be "echoryn", "echoryn/<agent-id>", or "agent:<agent-id>".
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

// --- Team API ---

// CreateTeamRequest is the request body for POST /v1/teams.
type CreateTeamRequest struct {
	// TemplateID creates a team from a template (mutually exclusive with Name).
	TemplateID string `json:"template_id,omitempty"`

	// Name is used for ad-hoc team creation (when TemplateID is empty).
	Name string `json:"name,omitempty"`

	// TaskDescription describes the task for the team.
	TaskDescription string `json:"task_description" binding:"required"`

	// Strategy is the coordination strategy (parallel, pipeline, debate, leader_directed).
	Strategy string `json:"strategy,omitempty"`
}

// SendTeamMessageRequest is the request body for POST /v1/teams/:id/messages.
type SendTeamMessageRequest struct {
	// Recipient is the member label or ID (for point-to-point messages).
	Recipient string `json:"recipient,omitempty"`

	// Content is the message text.
	Content string `json:"content" binding:"required"`

	// Broadcast sends the message to all team members.
	Broadcast bool `json:"broadcast,omitempty"`
}

// TeamTemplateResponse is the response for team template endpoints.
type TeamTemplateResponse struct {
	ID              string                       `json:"id"`
	Name            string                       `json:"name"`
	Description     string                       `json:"description,omitempty"`
	DefaultStrategy string                       `json:"default_strategy"`
	Members         []TeamTemplateMemberResponse `json:"members"`
}

// TeamTemplateMemberResponse is a member spec in a template response.
type TeamTemplateMemberResponse struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Label    string `json:"label"`
	IsLeader bool   `json:"is_leader,omitempty"`
}

// TeamResponse is the response for team endpoints.
type TeamResponse struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Strategy string               `json:"strategy"`
	Status   string               `json:"status"`
	Members  []TeamMemberResponse `json:"members"`
}

// TeamMemberResponse is a team member in the response.
type TeamMemberResponse struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id,omitempty"`
	Label     string `json:"label"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	IsLeader  bool   `json:"is_leader,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	Progress  string `json:"progress,omitempty"`
}

// --- Common ---

const timeFormat = time.RFC3339

// FormatTime formats a time value for API responses.
func FormatTime(t time.Time) string {
	return t.Format(timeFormat)
}

// ─────────────────────────────────────────────────────────────────────────────
// Info endpoint types (GET /v1/info, /v1/tools, /v1/nodes, /v1/skills)
// ─────────────────────────────────────────────────────────────────────────────

// SystemInfoResponse is the response for GET /v1/info.
type SystemInfoResponse struct {
	DefaultModel string `json:"default_model"`
}

// ToolGroupResponse represents a group of tools by category.
type ToolGroupResponse struct {
	Category string   `json:"category"`
	Tools    []string `json:"tools"`
}

// ToolsListResponse is the response for GET /v1/tools.
type ToolsListResponse struct {
	Groups []ToolGroupResponse `json:"groups"`
	Total  int                 `json:"total"`
}

// NodeInfoResponse represents a single Golem node in the info response.
type NodeInfoResponse struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Status string            `json:"status"`
	Labels map[string]string `json:"labels,omitempty"`
}

// NodesListResponse is the response for GET /v1/nodes.
type NodesListResponse struct {
	Nodes []NodeInfoResponse `json:"nodes"`
	Total int                `json:"total"`
}

// SkillGroupResponse represents a group of skills by source.
type SkillGroupResponse struct {
	Source string   `json:"source"`
	Skills []string `json:"skills"`
}

// SkillsListResponse is the response for GET /v1/skills.
type SkillsListResponse struct {
	Groups []SkillGroupResponse `json:"groups"`
	Total  int                  `json:"total"`
}
