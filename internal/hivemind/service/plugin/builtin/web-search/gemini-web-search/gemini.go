package gemini_websearch

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

// SearchResult holds the response from a web search.
type SearchResult struct {
	// Content is the AI-synthesized answer text.
	Content string `json:"content"`

	// Citations are the grounding source references.
	Citations []Citation `json:"citations,omitempty"`

	// SearchQueries are the actual queries the model executed.
	SearchQueries []string `json:"search_queries,omitempty"`
}

// Citation represents a single web source reference.
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// geminiSearcher performs web searches via Gemini's Google Search grounding.
type geminiSearcher struct {
	apiKey  string
	model   string
	timeout time.Duration
}

// newGeminiSearcher creates a new Gemini searcher from config.
// It resolves the API key from config first, then env variable.
func newGeminiSearcher(cfg *Config) (*geminiSearcher, error) {
	apiKey := cfg.Gemini.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("gemini API key not configured: set gemini.api_key in config or GEMINI_API_KEY env var")
	}

	model := cfg.Gemini.Model
	if model == "" {
		model = defaultGeminiModel
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(defaultTimeoutSeconds) * time.Second
	}

	return &geminiSearcher{
		apiKey:  apiKey,
		model:   model,
		timeout: timeout,
	}, nil
}

// Search performs a web search query using Gemini with Google Search grounding.
func (g *geminiSearcher) Search(ctx context.Context, query string) (*SearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	// Create Gemini client.
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  g.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", sanitizeError(err, g.apiKey))
	}

	// Build the request with Google Search grounding enabled.
	resp, err := client.Models.GenerateContent(ctx, g.model,
		[]*genai.Content{
			genai.NewContentFromText(query, genai.RoleUser),
		},
		&genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{GoogleSearch: &genai.GoogleSearch{}},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("Gemini API error: %w", sanitizeError(err, g.apiKey))
	}

	return parseGeminiResponse(resp), nil
}

// parseGeminiResponse extracts content and citations from Gemini's response.
func parseGeminiResponse(resp *genai.GenerateContentResponse) *SearchResult {
	result := &SearchResult{}

	if resp == nil || len(resp.Candidates) == 0 {
		result.Content = "No response from Gemini"
		return result
	}

	candidate := resp.Candidates[0]

	// Extract text content from parts.
	if candidate.Content != nil {
		var parts []string
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
		result.Content = strings.Join(parts, "\n")
	}

	// Extract grounding metadata (citations).
	if candidate.GroundingMetadata != nil {
		gm := candidate.GroundingMetadata

		// Extract grounding chunks as citations.
		for _, chunk := range gm.GroundingChunks {
			if chunk.Web != nil && chunk.Web.URI != "" {
				result.Citations = append(result.Citations, Citation{
					URL:   chunk.Web.URI,
					Title: chunk.Web.Title,
				})
			}
		}

		// Extract search queries.
		result.SearchQueries = gm.WebSearchQueries
	}

	if result.Content == "" {
		result.Content = "No content in Gemini response"
	}

	return result
}

// sanitizeError removes API key from error messages to prevent leakage.
func sanitizeError(err error, apiKey string) error {
	if err == nil || apiKey == "" {
		return err
	}
	msg := err.Error()
	sanitized := strings.ReplaceAll(msg, apiKey, "***")
	if msg == sanitized {
		return err
	}
	return fmt.Errorf("%s", sanitized)
}
