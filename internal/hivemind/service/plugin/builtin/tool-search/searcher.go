package toolsearch

import (
	"sort"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
)

// SearchResult holds a matched tool with its relevance score.
type SearchResult struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	SearchHint  string  `json:"search_hint,omitempty"`
	Score       float64 `json:"score"`
}

// Searcher provides tool discovery by keyword matching.
type Searcher struct {
	registry *plugin.Registry
}

// NewSearcher creates a new Searcher backed by the given plugin registry.
func NewSearcher(registry *plugin.Registry) *Searcher {
	return &Searcher{registry: registry}
}

// SelectByNames returns full ToolDefinitions for the given exact names.
func (s *Searcher) SelectByNames(names []string) []plugin.ToolDefinition {
	allTools := s.registry.GetTools()
	var results []plugin.ToolDefinition
	for _, name := range names {
		name = strings.TrimSpace(name)
		if def, ok := allTools[name]; ok {
			results = append(results, def)
		}
	}
	return results
}

// Search performs keyword-based fuzzy matching across tool names,
// descriptions, and search hints. Returns results sorted by relevance.
func (s *Searcher) Search(query string, maxResults int) []SearchResult {
	if maxResults <= 0 {
		maxResults = 10
	}

	allTools := s.registry.GetTools()
	query = strings.ToLower(strings.TrimSpace(query))
	keywords := strings.Fields(query)

	var results []SearchResult
	for _, def := range allTools {
		score := s.score(def, keywords)
		if score > 0 {
			results = append(results, SearchResult{
				Name:        def.Name,
				Description: def.Description,
				SearchHint:  def.SearchHint,
				Score:       score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results
}

// score calculates relevance score for a tool against the given keywords.
func (s *Searcher) score(def plugin.ToolDefinition, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	nameLower := strings.ToLower(def.Name)
	descLower := strings.ToLower(def.Description)
	hintLower := strings.ToLower(def.SearchHint)
	catLower := strings.ToLower(def.Category)

	total := 0.0
	for _, kw := range keywords {
		kwScore := 0.0
		if nameLower == kw {
			kwScore = 10.0
		} else if strings.Contains(nameLower, kw) {
			kwScore = 5.0
		}
		if strings.Contains(hintLower, kw) {
			kwScore += 3.0
		}
		if catLower == kw {
			kwScore += 2.0
		}
		if strings.Contains(descLower, kw) {
			kwScore += 1.0
		}
		total += kwScore
	}

	return total / float64(len(keywords))
}
