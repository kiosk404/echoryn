package thinking

import (
	einoClaude "github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// ClaudeStrategy maps ThinkingLevel to Claude's extended thinking API.
//
// Claude uses a budget_tokens approach: the higher the level, the larger the budget.
// ThinkingLevel mapping:
//
//	Off         → Thinking disabled
//	Minimal     → BudgetTokens = 1024
//	Low         → BudgetTokens = 4096
//	Medium      → BudgetTokens = 10240
//	High        → BudgetTokens = 32768
//	XHigh       → BudgetTokens = 65536
type ClaudeStrategy struct{}

func (s *ClaudeStrategy) Name() string                   { return "claude" }
func (s *ClaudeStrategy) MaxLevel() entity.ThinkingLevel { return entity.ThinkingLevelXHigh }

// ClaudeBudgetTokens maps ThinkingLevel to Claude budget_tokens.
var ClaudeBudgetTokens = map[entity.ThinkingLevel]int{
	entity.ThinkingLevelMinimal: 1024,
	entity.ThinkingLevelLow:     4096,
	entity.ThinkingLevelMedium:  10240,
	entity.ThinkingLevelHigh:    32768,
	entity.ThinkingLevelXHigh:   65536,
}

func (s *ClaudeStrategy) Apply(level entity.ThinkingLevel) []model.Option {
	budget, ok := ClaudeBudgetTokens[level]
	if !ok {
		return nil
	}
	return []model.Option{
		einoClaude.WithThinking(&einoClaude.Thinking{
			Enable:       true,
			BudgetTokens: budget,
		}),
	}
}
