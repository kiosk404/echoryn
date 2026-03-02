package thinking

import (
	einoOpenAI "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// OpenAIStrategy maps ThinkingLevel to OpenAI's reasoning_effort parameter.
//
// OpenAI supports three levels: low, medium, high.
// ThinkingLevel mapping:
//
//	Off/Minimal → no option (reasoning disabled)
//	Low         → ReasoningEffortLevelLow
//	Medium      → ReasoningEffortLevelMedium
//	High/XHigh  → ReasoningEffortLevelHigh
type OpenAIStrategy struct{}

func (s *OpenAIStrategy) Name() string                   { return "openai" }
func (s *OpenAIStrategy) MaxLevel() entity.ThinkingLevel { return entity.ThinkingLevelHigh }

func (s *OpenAIStrategy) Apply(level entity.ThinkingLevel) []model.Option {
	effort, ok := OpenAIReasoningEffort(level)
	if !ok {
		return nil
	}
	return []model.Option{einoOpenAI.WithReasoningEffort(effort)}
}

// OpenAIReasoningEffort maps a ThinkingLevel to the corresponding OpenAI ReasoningEffortLevel.
// Returns (effort, true) on success, or ("", false) if the level has no mapping.
func OpenAIReasoningEffort(level entity.ThinkingLevel) (einoOpenAI.ReasoningEffortLevel, bool) {
	switch level {
	case entity.ThinkingLevelLow:
		return einoOpenAI.ReasoningEffortLevelLow, true
	case entity.ThinkingLevelMedium:
		return einoOpenAI.ReasoningEffortLevelMedium, true
	case entity.ThinkingLevelHigh, entity.ThinkingLevelXHigh:
		return einoOpenAI.ReasoningEffortLevelHigh, true
	default:
		return "", false
	}
}
