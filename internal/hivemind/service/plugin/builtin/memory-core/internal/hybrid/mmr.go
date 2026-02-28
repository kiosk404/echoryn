package hybrid

import (
	"math"
	"regexp"
	"strings"
)

// MMRConfig configures Maximal Marginal Relevance re-ranking.
// MMR balances relevance with diversity by iteratively selecting results
// that maximize: λ * relevance - (1-λ) * max_similarity_to_selected
//
// Reference: Carbonell & Goldstein, "The Use of MMR, Diversity-Based Reranking" (1998)
type MMRConfig struct {
	// Enabled controls whether MMR re-ranking is applied. Default: false (opt-in).
	Enabled bool `json:"enabled"`

	// Lambda controls the trade-off between relevance and diversity.
	// 0 = max diversity, 1 = max relevance. Default: 0.7.
	Lambda float64 `json:"lambda"`
}

// DefaultMMRConfig returns the default MMR configuration (disabled, λ=0.7).
func DefaultMMRConfig() MMRConfig {
	return MMRConfig{
		Enabled: false,
		Lambda:  0.7,
	}
}

// mmrItem is an internal representation for MMR processing.
type mmrItem struct {
	index   int
	id      string
	score   float64
	content string
}

var mmrTokenPattern = regexp.MustCompile(`[a-z0-9_]+`)

// tokenize extracts lowercase alphanumeric tokens from text for Jaccard similarity.
func tokenize(text string) map[string]struct{} {
	tokens := mmrTokenPattern.FindAllString(strings.ToLower(text), -1)
	set := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		set[t] = struct{}{}
	}
	return set
}

// jaccardSimilarity computes the Jaccard similarity between two token sets.
// Returns a value in [0, 1] where 1 means identical sets.
func jaccardSimilarity(setA, setB map[string]struct{}) float64 {
	if len(setA) == 0 && len(setB) == 0 {
		return 1
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}

	// Iterate over the smaller set for efficiency.
	smaller, larger := setA, setB
	if len(setA) > len(setB) {
		smaller, larger = setB, setA
	}

	intersectionSize := 0
	for token := range smaller {
		if _, ok := larger[token]; ok {
			intersectionSize++
		}
	}

	unionSize := len(setA) + len(setB) - intersectionSize
	if unionSize == 0 {
		return 0
	}
	return float64(intersectionSize) / float64(unionSize)
}

// maxSimilarityToSelected computes the maximum Jaccard similarity between
// an item and all already-selected items.
func maxSimilarityToSelected(itemTokens map[string]struct{}, selected []map[string]struct{}) float64 {
	maxSim := 0.0
	for _, selTokens := range selected {
		sim := jaccardSimilarity(itemTokens, selTokens)
		if sim > maxSim {
			maxSim = sim
		}
	}
	return maxSim
}

// ApplyMMR re-ranks search results using Maximal Marginal Relevance.
//
// The algorithm iteratively selects items that balance relevance with diversity:
//  1. Normalize scores to [0,1]
//  2. For each slot, select the candidate that maximizes MMR = λ*relevance - (1-λ)*max_similarity
//  3. Use original score as tiebreaker
//
// Matches OpenClaw's applyMMRToHybridResults + mmrRerank.
func ApplyMMR(results []HybridResult, cfg MMRConfig) []HybridResult {
	if !cfg.Enabled || len(results) <= 1 {
		return results
	}

	lambda := math.Max(0, math.Min(1, cfg.Lambda))

	// If lambda is 1, no diversity penalty — just return as-is (already sorted by score).
	if lambda == 1 {
		return results
	}

	n := len(results)

	// Pre-tokenize all items for efficiency.
	tokenCache := make([]map[string]struct{}, n)
	for i, r := range results {
		tokenCache[i] = tokenize(r.Snippet)
	}

	// Normalize scores to [0, 1].
	maxScore, minScore := results[0].Score, results[0].Score
	for _, r := range results[1:] {
		if r.Score > maxScore {
			maxScore = r.Score
		}
		if r.Score < minScore {
			minScore = r.Score
		}
	}
	scoreRange := maxScore - minScore

	normalizeScore := func(score float64) float64 {
		if scoreRange == 0 {
			return 1 // All scores equal.
		}
		return (score - minScore) / scoreRange
	}

	// Track which items are still available.
	remaining := make(map[int]struct{}, n)
	for i := 0; i < n; i++ {
		remaining[i] = struct{}{}
	}

	selected := make([]HybridResult, 0, n)
	selectedTokens := make([]map[string]struct{}, 0, n)

	for len(remaining) > 0 {
		bestIdx := -1
		bestMMR := math.Inf(-1)
		bestOrigScore := math.Inf(-1)

		for idx := range remaining {
			normalizedRelevance := normalizeScore(results[idx].Score)
			maxSim := maxSimilarityToSelected(tokenCache[idx], selectedTokens)
			mmrScore := lambda*normalizedRelevance - (1-lambda)*maxSim

			// Use original score as tiebreaker (higher is better).
			if mmrScore > bestMMR || (mmrScore == bestMMR && results[idx].Score > bestOrigScore) {
				bestMMR = mmrScore
				bestIdx = idx
				bestOrigScore = results[idx].Score
			}
		}

		if bestIdx < 0 {
			break
		}

		selected = append(selected, results[bestIdx])
		selectedTokens = append(selectedTokens, tokenCache[bestIdx])
		delete(remaining, bestIdx)
	}

	return selected
}
