package embedding

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// openAIProvider implements Provider using the OpenAI embeddings API.
type openAIProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// OpenAIOptions configures the OpenAI embedding provider.
type OpenAIOptions struct {
	APIKey  string
	BaseURL string
	Model   string
}

// NewOpenAIProvider creates an OpenAI-compatible embedding provider.
func NewOpenAIProvider(opts OpenAIOptions) Provider {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := opts.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &openAIProvider{
		apiKey:  opts.APIKey,
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (p *openAIProvider) ID() string    { return "openai" }
func (p *openAIProvider) Model() string { return p.model }

func (p *openAIProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return results[0], nil
}

func (p *openAIProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := openAIEmbeddingRequest{
		Input: texts,
		Model: p.model,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for _, item := range result.Data {
		if item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}
	return embeddings, nil
}

type openAIEmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIEmbeddingResponse struct {
	Data []openAIEmbeddingData `json:"data"`
}

type openAIEmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}
