package service

import (
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// CompatManager manages model compatibility rules and applies normalization
// patches to model configurations before they are sent to provider SDKs.
//
// This is the Go equivalent of OpenClaw's normalizeModelCompat, but implemented
// as a centralized rule engine following K8S admission controller patterns.
//
// Rules are collected from two sources:
// 1. Provider plugins that implement spi.CompatPlugin (automatic)
// 2. Explicitly registered rules (manual)
type CompatManager struct {
	mu    sync.RWMutex
	rules []entity.ModelCompatRule

	// cache stores the resolved compat config per model ref to avoid re-evaluation.
	cache sync.Map // key: ModelRef.String(), value: *entity.ModelCompatConfig
}

// NewCompatManager creates a new CompatManager and collects rules from the registry.
func NewCompatManager(registry *provider.Registry) *CompatManager {
	cm := &CompatManager{
		rules: make([]entity.ModelCompatRule, 0),
	}

	// Collect compat rules from all plugins that implement CompatPlugin.
	registry.Range(func(name string, factory spi.PluginFactory) bool {
		plugin := factory()
		if cp, ok := plugin.(spi.CompatPlugin); ok {
			providerRules := cp.CompatRules()
			cm.rules = append(cm.rules, providerRules...)
			logger.Info("[Compat] loaded %d rules from provider %q", len(providerRules), name)
		}
		return true
	})

	// Register built-in cross-provider compat rules.
	cm.rules = append(cm.rules, builtinCompatRules()...)

	logger.Info("[Compat] total compat rules: %d", len(cm.rules))
	return cm
}

// RegisterRule adds a custom compat rule.
func (cm *CompatManager) RegisterRule(rule entity.ModelCompatRule) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.rules = append(cm.rules, rule)
	// Invalidate cache.
	cm.cache = sync.Map{}
}

// ResolveCompat returns the resolved compatibility configuration for a given model.
// Results are cached per ModelRef.
func (cm *CompatManager) ResolveCompat(instance *entity.ModelInstance, provider *entity.ModelProvider) *entity.ModelCompatConfig {
	cacheKey := entity.ModelRef{
		ProviderID: instance.ProviderID,
		ModelID:    instance.ModelID,
	}.String()

	if cached, ok := cm.cache.Load(cacheKey); ok {
		return cached.(*entity.ModelCompatConfig)
	}

	resolved := cm.evaluate(instance, provider)
	cm.cache.Store(cacheKey, resolved)
	return resolved
}

// InvalidateCache clears the compat cache (call after rule changes).
func (cm *CompatManager) InvalidateCache() {
	cm.cache = sync.Map{}
}

// evaluate applies all matching rules in order to build the final compat config.
func (cm *CompatManager) evaluate(instance *entity.ModelInstance, prov *entity.ModelProvider) *entity.ModelCompatConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := &entity.ModelCompatConfig{}

	for _, rule := range cm.rules {
		if !rule.Matcher.Matches(instance, prov) {
			continue
		}
		mergeCompat(result, &rule.Patches)
	}

	return result
}

// mergeCompat applies patches from src into dst (non-nil fields override).
func mergeCompat(dst, src *entity.ModelCompatConfig) {
	if src.SupportsDeveloperRole != nil {
		dst.SupportsDeveloperRole = src.SupportsDeveloperRole
	}
	if src.SupportsSystemRole != nil {
		dst.SupportsSystemRole = src.SupportsSystemRole
	}
	if src.SupportsFunctionCall != nil {
		dst.SupportsFunctionCall = src.SupportsFunctionCall
	}
	if src.SupportsStreaming != nil {
		dst.SupportsStreaming = src.SupportsStreaming
	}
	if src.SupportsVision != nil {
		dst.SupportsVision = src.SupportsVision
	}
	if src.MaxTokensFieldName != "" {
		dst.MaxTokensFieldName = src.MaxTokensFieldName
	}
	if src.RequiresMaxTokens {
		dst.RequiresMaxTokens = true
	}
	if src.StopSequenceSupported != nil {
		dst.StopSequenceSupported = src.StopSequenceSupported
	}
	if src.TemperatureRange != nil {
		dst.TemperatureRange = src.TemperatureRange
	}
}

// builtinCompatRules returns cross-provider compat rules that are always applied.
// Modeled after OpenClaw's normalizeModelCompat for known API quirks.
func builtinCompatRules() []entity.ModelCompatRule {
	boolFalse := false
	boolTrue := true

	return []entity.ModelCompatRule{
		{
			Name:        "o1-no-system-role",
			Description: "OpenAI O1 models do not support the system role; use developer role instead.",
			Matcher: entity.ModelCompatMatcher{
				ModelIDs: []string{"o1", "o1-mini", "o1-preview"},
			},
			Patches: entity.ModelCompatConfig{
				SupportsSystemRole:    &boolFalse,
				SupportsDeveloperRole: &boolTrue,
			},
		},
		{
			Name:        "o3-use-developer-role",
			Description: "OpenAI O3 models prefer developer role over system role.",
			Matcher: entity.ModelCompatMatcher{
				ModelIDs: []string{"o3-mini", "o3"},
			},
			Patches: entity.ModelCompatConfig{
				SupportsDeveloperRole: &boolTrue,
			},
		},
		{
			Name:        "anthropic-no-developer-role",
			Description: "Anthropic Claude models do not support the developer role.",
			Matcher: entity.ModelCompatMatcher{
				ModelClasses: []entity.ModelClass{entity.ModelClass_Claude},
			},
			Patches: entity.ModelCompatConfig{
				SupportsDeveloperRole: &boolFalse,
				SupportsSystemRole:    &boolTrue,
			},
		},
		{
			Name:        "anthropic-requires-max-tokens",
			Description: "Anthropic requires max_tokens to be explicitly set.",
			Matcher: entity.ModelCompatMatcher{
				APITypes: []entity.ModelAPI{entity.ModelAPI_AnthropicMessages},
			},
			Patches: entity.ModelCompatConfig{
				RequiresMaxTokens: true,
			},
		},
		{
			Name:        "gemini-temperature-range",
			Description: "Gemini models accept temperature in 0.0-2.0 range.",
			Matcher: entity.ModelCompatMatcher{
				ModelClasses: []entity.ModelClass{entity.ModelClass_Gemini},
			},
			Patches: entity.ModelCompatConfig{
				TemperatureRange: &entity.FloatRange{Min: 0.0, Max: 2.0},
			},
		},
		{
			Name:        "ollama-no-developer-role",
			Description: "Ollama models do not support the developer role.",
			Matcher: entity.ModelCompatMatcher{
				ModelClasses: []entity.ModelClass{entity.ModelClass_Ollama},
			},
			Patches: entity.ModelCompatConfig{
				SupportsDeveloperRole: &boolFalse,
			},
		},
	}
}
