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
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta,omitempty"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type chatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// HivemindClient is the HTTP client for hivemind /v1/chat/completions.
type HivemindClient struct {
	BaseURL    string
	SessionKey string
	Model      string
	HTTPClient *http.Client
}

// NewHivemindClient creates a new client.
func NewHivemindClient(baseURL, sessionKey, model string, httpClient *http.Client) *HivemindClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	return &HivemindClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		SessionKey: sessionKey,
		Model:      model,
		HTTPClient: httpClient,
	}
}

// StreamCallback is called for each text delta during streaming.
type StreamCallback func(delta string)

// ChatStream sends messages and streams the response, calling cb for each delta.
// Returns the full assistant reply when done.
func (c *HivemindClient) ChatStream(ctx context.Context, messages []ChatMessage, cb StreamCallback) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   true,
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer for large chunks
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

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

		for _, choice := range chunk.Choices {
			if choice.Delta != nil && choice.Delta.Content != "" {
				fullContent.WriteString(choice.Delta.Content)
				if cb != nil {
					cb(choice.Delta.Content)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fullContent.String(), fmt.Errorf("read stream: %w", err)
	}

	return fullContent.String(), nil
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
