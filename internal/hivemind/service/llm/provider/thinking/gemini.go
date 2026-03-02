package thinking

import (
	einoGemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"google.golang.org/genai"
)

// GeminiStrategy maps ThinkingLevel to Gemini's ThinkingConfig.
//
// Gemini uses IncludeThoughts (bool) + optional ThinkingBudget (token count).
// ThinkingLevel mapping:
//
//	Off         → IncludeThoughts: false
//	Minimal     → IncludeThoughts: true, ThinkingBudget: 1024
//	Low         → IncludeThoughts: true, ThinkingBudget: 4096
//	Medium      → IncludeThoughts: true, ThinkingBudget: 10240
//	High        → IncludeThoughts: true, ThinkingBudget: 32768
//	XHigh       → IncludeThoughts: true (no budget limit)
type GeminiStrategy struct{}

func (s *GeminiStrategy) Name() string                   { return "gemini" }
func (s *GeminiStrategy) MaxLevel() entity.ThinkingLevel { return entity.ThinkingLevelXHigh }

// GeminiBudgetTokens maps ThinkingLevel to optional ThinkingBudget.
// XHigh uses nil budget (unlimited).
var GeminiBudgetTokens = map[entity.ThinkingLevel]*int32{
	entity.ThinkingLevelMinimal: Int32Ptr(1024),
	entity.ThinkingLevelLow:     Int32Ptr(4096),
	entity.ThinkingLevelMedium:  Int32Ptr(10240),
	entity.ThinkingLevelHigh:    Int32Ptr(32768),
	entity.ThinkingLevelXHigh:   nil, // unlimited
}

func (s *GeminiStrategy) Apply(level entity.ThinkingLevel) []model.Option {
	budget, ok := GeminiBudgetTokens[level]
	if !ok {
		return nil
	}
	cfg := &genai.ThinkingConfig{
		IncludeThoughts: true,
		ThinkingBudget:  budget,
	}
	return []model.Option{einoGemini.WithThinkingConfig(cfg)}
}

func Int32Ptr(v int32) *int32 { return &v }
