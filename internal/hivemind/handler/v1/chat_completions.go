package v1

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/internal/pkg/core"
	"github.com/kiosk404/echoryn/pkg/errorx"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// ChatCompletionsHandler handles POST /v1/chat/completions (OpenAI-compatible).
//
// Modeled after OpenClaw's openai-http.ts:
//   - Resolves agent from model field (e.g., "eidolon/agent-id")
//   - Resolves session from X-Session-Key header or user field
//   - Maps messages to RunRequest
//   - Supports both stream=true (SSE) and stream=false (JSON)
type ChatCompletionsHandler struct {
	svc            service.AgentService
	llmManager     llmService.ModelManager
	defaultAgentID string
	defaultModel   string
}

// NewChatCompletionsHandler creates a new ChatCompletionsHandler.
func NewChatCompletionsHandler(svc service.AgentService, llmManager llmService.ModelManager, defaultAgentID, defaultModel string) *ChatCompletionsHandler {
	if defaultAgentID == "" {
		defaultAgentID = "main"
	}
	if defaultModel == "" {
		defaultModel = "eidolon"
	}
	return &ChatCompletionsHandler{
		svc:            svc,
		llmManager:     llmManager,
		defaultAgentID: defaultAgentID,
		defaultModel:   defaultModel,
	}
}

// Handle is the main entry point for POST /v1/chat/completions.
func (h *ChatCompletionsHandler) Handle(c *gin.Context) {
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrBind, "bind chat completion request"), nil)
		return
	}

	if len(req.Messages) == 0 {
		core.WriteResponse(c, errorx.WithCode(ErrMessagesEmpty, "messages array is required and must not be empty"), nil)
		return
	}

	// Resolve agent ID from model field (OpenClaw: resolveAgentIdForRequest).
	agentID := h.resolveAgentID(c, req.Model)

	// Resolve session key (OpenClaw: resolveSessionKey).
	sessionID := h.resolveSessionID(c, req.User, agentID)

	// Extract the last user message as input; merge system messages as extra prompt.
	userInput, extraSystem := extractUserInput(req.Messages)
	if userInput == "" {
		core.WriteResponse(c, errorx.WithCode(ErrNoUserMessage, "no user message found in messages array"), nil)
		return
	}

	// Ensure agent exists; auto-create a default one if it doesn't.
	if err := h.ensureAgent(c, agentID, extraSystem); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrEnsureAgent, "ensure agent %q", agentID), nil)
		return
	}

	// Build RunRequest.
	runReq := &runtime.RunRequest{
		AgentID:   agentID,
		SessionID: sessionID,
		Input:     userInput,
	}

	// Execute the agent run.
	sr, err := h.svc.Run(c.Request.Context(), runReq)
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrAgentRun, "run agent %q", agentID), nil)
		return
	}

	completionID := "chatcmpl-" + uuid.New().String()[:8]
	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	if req.Stream {
		h.handleStream(c, sr, completionID, model)
	} else {
		h.handleNonStream(c, sr, completionID, model)
	}
}

// handleStream sends SSE chunks following the OpenAI streaming format.
//
// OpenClaw equivalent: the streaming branch in openai-http.ts that subscribes
// to onAgentEvent and emits SSE data chunks with "chat.completion.chunk" objects.
func (h *ChatCompletionsHandler) handleStream(
	c *gin.Context,
	sr *schema.StreamReader[*entity.AgentEvent],
	completionID, model string,
) {
	// Set SSE headers.
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	w := c.Writer
	created := time.Now().Unix()

	// Send initial role chunk (OpenClaw: send role chunk before deltas).
	h.writeSSEChunk(w, completionID, model, created, &ChatMessageDelta{Role: "assistant"}, nil, nil)
	w.Flush()

	var toolCallIndex int
	var lastUsage *ChatCompletionUsage

	for {
		// Check client disconnect.
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}

		event, err := sr.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Warn("[ChatCompletions] stream recv error (code=%d): %v", ErrStreamRecv, err)
			break
		}

		switch event.Type {
		case entity.EventTextDelta:
			h.writeSSEChunk(w, completionID, model, created, &ChatMessageDelta{
				Content: event.Delta,
			}, nil, nil)
			w.Flush()

		case entity.EventToolCallStart:
			if event.ToolCall != nil {
				delta := &ChatMessageDelta{
					ToolCalls: []ToolCallChunk{{
						Index: toolCallIndex,
						ID:    event.ToolCall.ID,
						Type:  "function",
						Function: ToolCallFunction{
							Name:      event.ToolCall.Name,
							Arguments: event.ToolCall.Arguments,
						},
					}},
				}
				h.writeSSEChunk(w, completionID, model, created, delta, nil, nil)
				w.Flush()
				toolCallIndex++
			}

		case entity.EventDone:
			if event.Usage != nil {
				lastUsage = &ChatCompletionUsage{
					PromptTokens:     event.Usage.PromptTokens,
					CompletionTokens: event.Usage.CompletionTokens,
					TotalTokens:      event.Usage.TotalTokens,
				}
			}

		case entity.EventError:
			// Send error as a text delta so the client sees it.
			h.writeSSEChunk(w, completionID, model, created, &ChatMessageDelta{
				Content: "\n[Error: " + event.Error + "]",
			}, nil, nil)
			w.Flush()
		}
	}

	// Send final chunk with finish_reason="stop".
	finishReason := "stop"
	h.writeSSEChunk(w, completionID, model, created, &ChatMessageDelta{}, &finishReason, lastUsage)
	w.Flush()

	// Send [DONE] sentinel (OpenAI SSE convention).
	fmt.Fprintf(w, "data: [DONE]\n\n")
	w.Flush()
}

// handleNonStream collects all events and returns a single JSON response.
//
// OpenClaw equivalent: the non-streaming branch that waits for agentCommand
// to complete, then builds a single chat.completion response.
func (h *ChatCompletionsHandler) handleNonStream(
	c *gin.Context,
	sr *schema.StreamReader[*entity.AgentEvent],
	completionID, model string,
) {
	var content strings.Builder
	var toolCalls []ToolCallChunk
	var usage *ChatCompletionUsage
	var lastErr string
	toolCallIndex := 0

	for {
		event, err := sr.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Warn("[ChatCompletions] non-stream recv error (code=%d): %v", ErrStreamRecv, err)
			break
		}

		switch event.Type {
		case entity.EventTextDelta:
			content.WriteString(event.Delta)

		case entity.EventToolCallStart:
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, ToolCallChunk{
					Index: toolCallIndex,
					ID:    event.ToolCall.ID,
					Type:  "function",
					Function: ToolCallFunction{
						Name:      event.ToolCall.Name,
						Arguments: event.ToolCall.Arguments,
					},
				})
				toolCallIndex++
			}

		case entity.EventDone:
			if event.Usage != nil {
				usage = &ChatCompletionUsage{
					PromptTokens:     event.Usage.PromptTokens,
					CompletionTokens: event.Usage.CompletionTokens,
					TotalTokens:      event.Usage.TotalTokens,
				}
			}

		case entity.EventError:
			lastErr = event.Error
		}
	}

	if lastErr != "" && content.Len() == 0 {
		core.WriteResponse(c, errorx.WithCode(ErrNonStreamResult, "%s", lastErr), nil)
		return
	}

	msg := &ChatMessage{
		Role:    "assistant",
		Content: content.String(),
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	core.WriteResponse(c, nil, ChatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: usage,
	})
}

// writeSSEChunk writes a single SSE data chunk in OpenAI chat.completion.chunk format.
func (h *ChatCompletionsHandler) writeSSEChunk(
	w gin.ResponseWriter,
	id, model string,
	created int64,
	delta *ChatMessageDelta,
	finishReason *string,
	usage *ChatCompletionUsage,
) {
	chunk := ChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []ChatCompletionChunkChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: finishReason,
		}},
		Usage: usage,
	}

	data, err := json.Marshal(chunk)
	if err != nil {
		logger.Warn("[ChatCompletions] marshal chunk error: %v", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// resolveAgentID extracts agent ID from the model field or X-Agent-Id header.
//
// Parsing rules (aligned with OpenClaw http-utils.ts):
//   - "eidolon/<agent-id>" → agent-id
//   - "agent:<agent-id>" → agent-id
//   - X-Agent-Id header takes priority
//   - Otherwise: default agent
func (h *ChatCompletionsHandler) resolveAgentID(c *gin.Context, model string) string {
	// Priority 1: X-Agent-Id header.
	if agentID := c.GetHeader("X-Agent-Id"); agentID != "" {
		return agentID
	}

	// Priority 2: Parse from model field.
	if model != "" {
		// "eidolon/<agent-id>"
		if strings.HasPrefix(model, "eidolon/") {
			return strings.TrimPrefix(model, "eidolon/")
		}
		// "agent:<agent-id>"
		if strings.HasPrefix(model, "agent:") {
			return strings.TrimPrefix(model, "agent:")
		}
	}

	return h.defaultAgentID
}

// resolveSessionID determines the session ID from header or user field.
//
// OpenClaw equivalent: resolveSessionKey in http-utils.ts.
func (h *ChatCompletionsHandler) resolveSessionID(c *gin.Context, user, agentID string) string {
	// Priority 1: X-Session-Key header.
	if sessionKey := c.GetHeader("X-Session-Key"); sessionKey != "" {
		return sessionKey
	}

	// Priority 2: Derive from user field.
	if user != "" {
		return fmt.Sprintf("%s:user:%s", agentID, user)
	}

	// Empty = auto-create new session.
	return ""
}

// extractUserInput extracts the last user message content and any system prompts.
// Returns (userInput, extraSystemPrompt).
func extractUserInput(messages []ChatMessage) (string, string) {
	var userInput string
	var systemParts []string

	for _, msg := range messages {
		switch msg.Role {
		case "system", "developer":
			systemParts = append(systemParts, msg.Content)
		case "user":
			userInput = msg.Content
		}
	}

	extraSystem := strings.Join(systemParts, "\n")
	return userInput, extraSystem
}

// ensureAgent checks if the agent exists; if not, auto-creates a default one
// with the system's default model bound as FallbackConfig.Primary.
// This allows /v1/chat/completions to work even without pre-creating agents,
// matching OpenClaw's behavior where a "main" agent always exists.
func (h *ChatCompletionsHandler) ensureAgent(c *gin.Context, agentID, extraSystem string) error {
	_, err := h.svc.GetAgent(c.Request.Context(), agentID)
	if err == nil {
		return nil
	}

	// Auto-create agent with the given system prompt.
	agent := &entity.Agent{
		ID:           agentID,
		Name:         agentID,
		SystemPrompt: extraSystem,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Bind the system default model so FallbackConfig is not empty.
	// If no default model is available (e.g., no API key configured), refuse to create
	// the agent — an agent without a valid model ref will always fail at chat time.
	if h.llmManager == nil {
		return fmt.Errorf("LLM manager not initialized, cannot auto-create agent %q", agentID)
	}

	defaultModel, err := h.llmManager.GetDefaultModel(c.Request.Context())
	if err != nil {
		return fmt.Errorf("cannot auto-create agent %q: no default model available (check your models config and API keys): %w", agentID, err)
	}

	ref := llmEntity.ModelRef{
		ProviderID: defaultModel.ProviderID,
		ModelID:    defaultModel.ModelID,
	}
	agent.ModelRef = ref
	agent.Fallback = llmEntity.FallbackConfig{
		Primary: ref,
	}
	logger.Info("[ChatCompletions] auto-creating agent %q with default model %s", agentID, ref)

	return h.svc.CreateAgent(c.Request.Context(), agent)
}
