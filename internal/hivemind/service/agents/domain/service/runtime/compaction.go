package runtime

import (
	"context"
	"fmt"
	"strings"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Compactor performs context compaction by summarizing old conversation history
// using an LLM, then replacing the summarized messages with the summary.
//
// This is the Echoryn equivalent of OpenClaw's compaction.ts:
//
//   - summarizeInStages(): split messages into N chunks, summarize each, merge
//   - summarizeWithFallback(): progressive fallback (full → exclude-large → simple)
//   - Apply result to Session (CompactionSummary + FirstKeptIndex)
//
// Compaction is triggered when:
//  1. Context overflow error during LLM execution (reactive)
//  2. Post-turn threshold check: tokens > compactionThreshold * windowSize (proactive)
type Compactor struct {
	estimator           *TokenEstimator
	compactionThreshold float64
	keepRecentTurns     int
}

// CompactorConfig holds configuration for the compactor.
type CompactorConfig struct {
	// CompactionThreshold: when (estimated tokens / window) > this, trigger compaction.
	// Default: 0.8.
	CompactionThreshold float64

	// KeepRecentTurns: number of recent user→assistant turn pairs to preserve.
	// Default: 3 (keep last 3 exchanges verbatim, summarize the rest).
	KeepRecentTurns int
}

// DefaultCompactorConfig returns sensible defaults.
func DefaultCompactorConfig() CompactorConfig {
	return CompactorConfig{
		CompactionThreshold: 0.8,
		KeepRecentTurns:     3,
	}
}

// NewCompactor creates a new Compactor.
func NewCompactor(estimator *TokenEstimator, cfg CompactorConfig) *Compactor {
	if cfg.CompactionThreshold <= 0 {
		cfg.CompactionThreshold = 0.8
	}
	if cfg.KeepRecentTurns <= 0 {
		cfg.KeepRecentTurns = 3
	}
	return &Compactor{
		estimator:           estimator,
		compactionThreshold: cfg.CompactionThreshold,
		keepRecentTurns:     cfg.KeepRecentTurns,
	}
}

// ShouldCompact checks if post-turn proactive compaction is needed.
func (c *Compactor) ShouldCompact(session *entity.Session, windowInfo ContextWindowInfo) bool {
	if len(session.ActiveMessages()) == 0 {
		return false
	}
	activeSchemaMessages := ToSchemaMessages(session.ActiveMessages())
	estimated := c.estimator.EstimateMessages(activeSchemaMessages)
	ratio := float64(estimated) / float64(windowInfo.UsableTokens)
	return ratio > c.compactionThreshold
}

// Compact performs compaction on the session using the provided ChatModel.
//
// Flow:
//  1. Identify messages to summarize (all active messages except last N turns)
//  2. Split into chunks by token budget
//  3. Summarize each chunk with the LLM
//  4. Merge partial summaries into a final summary
//  5. Apply to session (CompactionSummary + FirstKeptIndex)
//
// Returns the summary text and error.
func (c *Compactor) Compact(
	ctx context.Context,
	session *entity.Session,
	chatModel einoModel.BaseChatModel,
	windowInfo ContextWindowInfo,
) (string, error) {
	activeMessages := session.ActiveMessages()
	if len(activeMessages) == 0 {
		return "", fmt.Errorf("no messages to compact")
	}

	// Find the split point: keep last N user→assistant turn pairs.
	splitIdx := c.findCompactionSplitPoint(activeMessages)
	if splitIdx <= 0 {
		return "", fmt.Errorf("not enough messages to compact (only %d, need at least 1 before keep boundary)", len(activeMessages))
	}

	messagesToSummarize := activeMessages[:splitIdx]
	logger.Info("[Compactor] compacting %d messages (keeping last %d), session=%s, compactionCount=%d",
		len(messagesToSummarize), len(activeMessages)-splitIdx, session.ID, session.CompactionCount+1)

	// Build the existing summary prefix (if any previous compaction).
	existingSummary := session.CompactionSummary

	// Summarize.
	summary, err := c.summarize(ctx, chatModel, messagesToSummarize, existingSummary, windowInfo)
	if err != nil {
		return "", fmt.Errorf("compaction summarization failed: %w", err)
	}

	// Apply to session.
	absoluteKeptFrom := session.FirstKeptIndex + splitIdx
	session.ApplyCompaction(summary, absoluteKeptFrom)

	logger.Info("[Compactor] compaction completed: session=%s, summary_len=%d, first_kept=%d, compaction_count=%d",
		session.ID, len(summary), absoluteKeptFrom, session.CompactionCount)

	return summary, nil
}

// findCompactionSplitPoint returns the index in activeMessages where we stop summarizing.
// Everything before this index gets summarized; everything from this index onward is kept.
func (c *Compactor) findCompactionSplitPoint(messages []*entity.Message) int {
	// Count user→assistant turn pairs from the end.
	turnsFound := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == entity.RoleUser {
			turnsFound++
			if turnsFound >= c.keepRecentTurns {
				return i
			}
		}
	}
	// Not enough turns to keep; summarize everything except the last message.
	if len(messages) > 1 {
		return len(messages) - 1
	}
	return 0
}

// summarize performs the multi-stage summarization.
//
// Strategy (following OpenClaw's summarizeInStages):
//  1. If messages fit in a single chunk → single-pass summary
//  2. Otherwise → split into chunks → summarize each → merge
func (c *Compactor) summarize(
	ctx context.Context,
	chatModel einoModel.BaseChatModel,
	messages []*entity.Message,
	existingSummary string,
	windowInfo ContextWindowInfo,
) (string, error) {
	schemaMessages := ToSchemaMessages(messages)
	totalTokens := c.estimator.EstimateMessages(schemaMessages)

	// Target: summary should fit in ~20% of the usable window.
	summaryBudget := windowInfo.UsableTokens / 5
	if summaryBudget < 1000 {
		summaryBudget = 1000
	}

	// Determine chunk size: each chunk should be small enough to summarize.
	// Use 40% of usable window per chunk (following OpenClaw's computeAdaptiveChunkRatio).
	chunkBudget := int(float64(windowInfo.UsableTokens) * 0.4)
	if chunkBudget < 2000 {
		chunkBudget = 2000
	}

	if totalTokens <= chunkBudget {
		// Single-pass: all messages fit in one chunk.
		return c.summarizeChunk(ctx, chatModel, schemaMessages, existingSummary, summaryBudget)
	}

	// Multi-stage: split → summarize each chunk → merge.
	chunks := c.splitIntoChunks(schemaMessages, chunkBudget)
	logger.Debug("[Compactor] multi-stage: %d chunks from %d messages (%d est. tokens)",
		len(chunks), len(messages), totalTokens)

	var partialSummaries []string
	for i, chunk := range chunks {
		prefix := existingSummary
		if i > 0 && len(partialSummaries) > 0 {
			prefix = strings.Join(partialSummaries, "\n\n")
		}

		partial, err := c.summarizeChunk(ctx, chatModel, chunk, prefix, summaryBudget/len(chunks))
		if err != nil {
			// Fallback: if any chunk fails, try a simple description.
			logger.Warn("[Compactor] chunk %d/%d summarization failed: %v, using simple fallback",
				i+1, len(chunks), err)
			partial = fmt.Sprintf("[Summary of %d messages in conversation chunk %d/%d could not be generated]",
				len(chunk), i+1, len(chunks))
		}
		partialSummaries = append(partialSummaries, partial)
	}

	// If only one chunk succeeded, return it directly.
	if len(partialSummaries) == 1 {
		return partialSummaries[0], nil
	}

	// Merge partial summaries into a final summary.
	return c.mergeSummaries(ctx, chatModel, partialSummaries, summaryBudget)
}

// summarizeChunk summarizes a single chunk of messages using the LLM.
func (c *Compactor) summarizeChunk(
	ctx context.Context,
	chatModel einoModel.BaseChatModel,
	messages []*schema.Message,
	existingSummary string,
	maxTokens int,
) (string, error) {
	var promptBuilder strings.Builder

	promptBuilder.WriteString("You are a conversation summarizer. Summarize the following conversation messages concisely, preserving:\n")
	promptBuilder.WriteString("- Key decisions and conclusions\n")
	promptBuilder.WriteString("- Important facts and data points\n")
	promptBuilder.WriteString("- Tool call results that are still relevant\n")
	promptBuilder.WriteString("- User preferences and requirements expressed\n\n")
	promptBuilder.WriteString(fmt.Sprintf("Keep the summary under %d tokens. Write in the same language as the conversation.\n\n", maxTokens))

	if existingSummary != "" {
		promptBuilder.WriteString("Previous conversation summary:\n")
		promptBuilder.WriteString(existingSummary)
		promptBuilder.WriteString("\n\n---\n\nNew messages to summarize:\n\n")
	} else {
		promptBuilder.WriteString("Messages to summarize:\n\n")
	}

	for _, msg := range messages {
		role := string(msg.Role)
		content := msg.Content
		// Truncate very long messages in the prompt.
		if len([]rune(content)) > 2000 {
			runes := []rune(content)
			content = string(runes[:1000]) + "\n...[truncated]...\n" + string(runes[len(runes)-500:])
		}
		promptBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))

		for _, tc := range msg.ToolCalls {
			promptBuilder.WriteString(fmt.Sprintf("  → Tool call: %s(%s)\n", tc.Function.Name, truncateStr(tc.Function.Arguments, 200)))
		}
	}

	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "You are a precise conversation summarizer. Output only the summary, no preamble."},
		{Role: schema.User, Content: promptBuilder.String()},
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Content), nil
}

// mergeSummaries merges multiple partial summaries into a final summary.
func (c *Compactor) mergeSummaries(
	ctx context.Context,
	chatModel einoModel.BaseChatModel,
	partials []string,
	maxTokens int,
) (string, error) {
	var promptBuilder strings.Builder
	promptBuilder.WriteString("Merge the following partial conversation summaries into a single cohesive summary.\n")
	promptBuilder.WriteString(fmt.Sprintf("Keep the final summary under %d tokens. Preserve all key information.\n\n", maxTokens))

	for i, partial := range partials {
		promptBuilder.WriteString(fmt.Sprintf("--- Part %d/%d ---\n%s\n\n", i+1, len(partials), partial))
	}

	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "You are a precise conversation summarizer. Output only the merged summary, no preamble."},
		{Role: schema.User, Content: promptBuilder.String()},
	})
	if err != nil {
		// Fallback: just concatenate partials.
		logger.Warn("[Compactor] merge failed: %v, concatenating partials", err)
		return strings.Join(partials, "\n\n---\n\n"), nil
	}

	return strings.TrimSpace(resp.Content), nil
}

// splitIntoChunks splits messages into chunks based on estimated token budget.
func (c *Compactor) splitIntoChunks(messages []*schema.Message, chunkBudget int) [][]*schema.Message {
	var chunks [][]*schema.Message
	var current []*schema.Message
	currentTokens := 0

	for _, msg := range messages {
		msgTokens := c.estimator.EstimateMessage(msg)
		if currentTokens+msgTokens > chunkBudget && len(current) > 0 {
			chunks = append(chunks, current)
			current = nil
			currentTokens = 0
		}
		current = append(current, msg)
		currentTokens += msgTokens
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// truncateStr truncates a string to maxLen runes with an ellipsis.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
