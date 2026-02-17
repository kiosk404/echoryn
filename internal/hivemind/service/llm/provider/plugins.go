package provider

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/anthropic"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/deepseek"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/gemini"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/glm"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/kimi"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/ollama"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/openai"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/qwen"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
)

func NewInTreeRegistry() *Registry {
	r := NewRegistry()

	r.MustRegister(anthropic.Name, func() spi.ProviderPlugin { return anthropic.New() })
	r.MustRegister(openai.Name, func() spi.ProviderPlugin { return openai.New() })
	r.MustRegister(gemini.Name, func() spi.ProviderPlugin { return gemini.New() })
	r.MustRegister(deepseek.Name, func() spi.ProviderPlugin { return deepseek.New() })
	r.MustRegister(glm.Name, func() spi.ProviderPlugin { return glm.New() })
	r.MustRegister(kimi.Name, func() spi.ProviderPlugin { return kimi.New() })
	r.MustRegister(qwen.Name, func() spi.ProviderPlugin { return qwen.New() })
	r.MustRegister(ollama.Name, func() spi.ProviderPlugin { return ollama.New() })
	return r
}
