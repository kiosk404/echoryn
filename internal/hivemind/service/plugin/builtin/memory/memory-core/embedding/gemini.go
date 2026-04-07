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

// geminiProvider implements Provider using the Google Gemini embeddings API.
type geminiProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// GeminiOptions configures the Gemini embedding provider.
type GeminiOptions struct {
	APIKey  string
	BaseURL string
	Model   string
}

// NewGeminiProvider creates a Gemini-compatible embedding provider.
func NewGeminiProvider(opts GeminiOptions) Provider {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	model := opts.Model
	if model == "" {
		model = "text-embedding-004"
	}
	return &geminiProvider{
		apiKey:  opts.APIKey,
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (p *geminiProvider) ID() string    { return "gemini" }
func (p *geminiProvider) Model() string { return p.model }

func (p *geminiProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return results[0], nil
}

func (p *geminiProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Gemini batchEmbedContents supports up to 100 requests per batch.
	const maxBatchSize = 100
	if len(texts) <= maxBatchSize {
		return p.embedBatchSingle(ctx, texts)
	}

	// Split into batches.
	var allResults [][]float32
	for i := 0; i < len(texts); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := p.embedBatchSingle(ctx, texts[i:end])
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}
		allResults = append(allResults, batch...)
	}
	return allResults, nil
}

func (p *geminiProvider) embedBatchSingle(ctx context.Context, texts []string) ([][]float32, error) {
	// Build batchEmbedContents request.
	requests := make([]geminiEmbedRequest, len(texts))
	modelPath := fmt.Sprintf("models/%s", p.model)
	for i, text := range texts {
		requests[i] = geminiEmbedRequest{
			Model:   modelPath,
			Content: geminiContent{Parts: []geminiPart{{Text: text}}},
		}
	}

	reqBody := geminiBatchEmbedRequest{
		Requests: requests,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/%s:batchEmbedContents?key=%s", p.baseURL, modelPath, p.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result geminiBatchEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	embeddings := make([][]float32, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		embeddings[i] = emb.Values
	}
	return embeddings, nil
}

// --- Gemini API types ---

type geminiBatchEmbedRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiEmbedRequest struct {
	Model   string        `json:"model"`
	Content geminiContent `json:"content"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiBatchEmbedResponse struct {
	Embeddings []geminiEmbedding `json:"embeddings"`
}

type geminiEmbedding struct {
	Values []float32 `json:"values"`
}
