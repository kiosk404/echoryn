package runtime

import (
	"github.com/cloudwego/eino/schema"
)

// TokenEstimator estimates token counts for messages.
//
// Since the project has no local tokenizer (token, etc.), we use a
// character-based heuristic: ~4 characters per token for English,
// ~2 characters per token for CJK, blended to ~3.5 chars/token on average.
//
// This follows OpenClaw's approach of approximate token counting for
// context window management — the exact count comes from the LLM API response.
//
// The ratio is configurable to allow tuning per model family.
type TokenEstimator struct {
	charsPerToken float64
}

const (
	// DefaultCharsPerToken is a reasonable default for mixed-language content.
	// English ~4, CJK ~2, blended ~3.5. We use 3.5 for safety.
	DefaultCharsPerToken = 3.5

	// PerMessageOverhead accounts for message framing overhead
	// (role tokens, delimiters, etc.) per message.
	PerMessageOverhead = 4
)

// NewTokenEstimator creates a new estimator with the given chars-per-token ratio.
// If ratio <= 0, DefaultCharsPerToken is used.
func NewTokenEstimator(charsPerToken float64) *TokenEstimator {
	if charsPerToken <= 0 {
		charsPerToken = DefaultCharsPerToken
	}
	return &TokenEstimator{charsPerToken: charsPerToken}
}

// EstimateString estimates tokens for a raw string.
func (te *TokenEstimator) EstimateString(s string) int {
	if len(s) == 0 {
		return 0
	}
	// Use rune count for Unicode awareness.
	runeCount := 0
	for range s {
		runeCount++
	}
	return int(float64(runeCount)/te.charsPerToken) + 1
}

// EstimateMessage estimates tokens for a single Eino schema.Message.
func (te *TokenEstimator) EstimateMessage(msg *schema.Message) int {
	if msg == nil {
		return 0
	}
	tokens := PerMessageOverhead
	tokens += te.EstimateString(msg.Content)
	tokens += te.EstimateString(msg.Name)

	for _, tc := range msg.ToolCalls {
		tokens += te.EstimateString(tc.Function.Name)
		tokens += te.EstimateString(tc.Function.Arguments)
		tokens += 4 // tool call framing
	}
	return tokens
}

// EstimateMessages estimates total tokens for a slice of messages.
func (te *TokenEstimator) EstimateMessages(msgs []*schema.Message) int {
	total := 0
	for _, msg := range msgs {
		total += te.EstimateMessage(msg)
	}
	return total
}
