package chat

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// ChatMessage is a single message in the OpenAI Chat Completions format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the request body for /v1/chat/completions.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatResponse is the non-streaming response.
type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      *ChatMessage `json:"message,omitempty"`
		FinishReason string       `json:"finish_reason"`
	} `json:"choices"`
	Error *chatError `json:"error,omitempty"`
}

// chatChunk is a single SSE streaming chunk.
type chatChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta *struct {
			Role      string          `json:"role,omitempty"`
			Content   string          `json:"content,omitempty"`
			ToolCalls []toolCallChunk `json:"tool_calls,omitempty"`
		} `json:"delta,omitempty"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	// Usage is populated only on the final chunk (finish_reason != null).
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// toolCallChunk matches the OpenAI tool_call delta in streaming mode.
type toolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type chatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// ChatStreamResult holds the result of a streaming chat interaction.
type ChatStreamResult struct {
	Content          string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

// HivemindClient is the HTTP client for hivemind /v1/chat/completions.
type HivemindClient struct {
	BaseURL    string
	SessionKey string
	Model      string
	HTTPClient *http.Client

	// lastRunID stores the run ID from the most recent request response header.
	lastRunID string
}

// NewHivemindClient creates a new client.
func NewHivemindClient(baseURL, sessionKey, model string, httpClient *http.Client) *HivemindClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 1200 * time.Second}
	}

	return &HivemindClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		SessionKey: sessionKey,
		Model:      model,
		HTTPClient: httpClient,
	}
}

// LastRunID returns the run ID of the most recent chat request.
func (c *HivemindClient) LastRunID() string { return c.lastRunID }

// StreamCallback is called for each text delta during streaming.
type StreamCallback func(delta string)

// ToolCallCallback is called when a tool call is detected in the stream.
// name is the tool function name, arguments is the JSON arguments string.
type ToolCallCallback func(name string)

// ChatStream sends messages and streams the response, calling cb for each delta.
// toolCb is called when a tool call is detected (may be nil).
// Returns a ChatStreamResult with content and token usage when done.
func (c *HivemindClient) ChatStream(ctx context.Context, messages []ChatMessage, cb StreamCallback, toolCb ToolCallCallback) (*ChatStreamResult, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.SessionKey != "" {
		req.Header.Set("X-Session-Key", c.SessionKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Extract Run ID from response header for later abort capability.
	if rid := resp.Header.Get("X-Run-ID"); rid != "" {
		c.lastRunID = rid
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer for large chunks
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	result := &ChatStreamResult{}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Extract usage from final chunk (when present).
		if chunk.Usage != nil {
			result.PromptTokens = chunk.Usage.PromptTokens
			result.CompletionTokens = chunk.Usage.CompletionTokens
			result.TotalTokens = chunk.Usage.TotalTokens
		}

		for _, choice := range chunk.Choices {
			if choice.Delta != nil {
				if choice.Delta.Content != "" {
					fullContent.WriteString(choice.Delta.Content)
					if cb != nil {
						cb(choice.Delta.Content)
					}
				}
				// Notify tool call events so the TUI can show progress.
				if toolCb != nil {
					for _, tc := range choice.Delta.ToolCalls {
						if tc.Function.Name != "" {
							toolCb(tc.Function.Name)
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		result.Content = fullContent.String()
		return result, fmt.Errorf("read stream: %w", err)
	}

	result.Content = fullContent.String()
	return result, nil
}

// Chat sends messages and returns the full response (non-streaming).
func (c *HivemindClient) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.SessionKey != "" {
		req.Header.Set("X-Session-Key", c.SessionKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("server error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message == nil {
		return "", fmt.Errorf("empty response from server")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// Abort sends a DELETE request to cancel the currently running agent execution.
func (c *HivemindClient) Abort(ctx context.Context) error {
	runID := c.lastRunID
	if runID == "" {
		return fmt.Errorf("no active run to abort")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.BaseURL+"/v1/runs/"+runID, nil)
	if err != nil {
		return fmt.Errorf("create abort request: %w", err)
	}
	if c.SessionKey != "" {
		req.Header.Set("X-Session-Key", c.SessionKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("abort request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("abort failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
