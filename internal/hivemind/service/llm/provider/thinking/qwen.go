package thinking

import (
	einoQwen "github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// QwenStrategy maps ThinkingLevel to Qwen's EnableThinking option.
//
// Qwen only supports binary thinking (on/off).
// Any level above Off enables thinking mode.
type QwenStrategy struct{}

func (s *QwenStrategy) Name() string                   { return "qwen" }
func (s *QwenStrategy) MaxLevel() entity.ThinkingLevel { return entity.ThinkingLevelHigh }

func (s *QwenStrategy) Apply(level entity.ThinkingLevel) []model.Option {
	if level.IsEnabled() {
		return []model.Option{einoQwen.WithEnableThinking(true)}
	}
	return nil
}
