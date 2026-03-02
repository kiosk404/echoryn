package thinking

import (
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// NoopStrategy is the default strategy for providers that don't support
// runtime thinking options (e.g., Deepseek, Ollama, Kimi, GLM)
//
// Deepseek models like deepseek-reasoner have reasoning built-in
// and don't expose a runtime knob, Ollama's thinking is config-level only.
type NoopStrategy struct {
	ProviderName string
}

func (s *NoopStrategy) Name() string { return s.ProviderName }
func (s *NoopStrategy) MaxLevel() entity.ThinkingLevel {
	return entity.ThinkingLevelOff
}
func (s *NoopStrategy) Apply(_ entity.ThinkingLevel) []model.Option {
	return nil
}
