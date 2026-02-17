package entity

// ModelCompatConfig describes compatibility flags for a specific model or provider.
// Different providers have subtle API differences; this struct captures those quirks
// so the system can normalize behavior before sending requests.
//
// Modeled after OpenClaw's ModelCompatConfig (supportsDeveloperRole, supportsStore,
// supportsReasoningEffort, maxTokensField).
type ModelCompatConfig struct {
	// SupportsDeveloperRole indicates whether the model supports the "developer" role
	// in the message list (some providers only support "system").
	SupportsDeveloperRole *bool `json:"supports_developer_role,omitempty"`

	// SupportsSystemRole indicates whether the model supports the "system" role.
	// Some models (e.g., O1) require using "developer" instead.
	SupportsSystemRole *bool `json:"supports_system_role,omitempty"`

	// SupportsFunctionCall indicates whether the model reliably supports tool/function calling.
	// Used to skip tool-calling for models that claim support but don't work well.
	SupportsFunctionCall *bool `json:"supports_function_call,omitempty"`

	// SupportsStreaming indicates whether the model supports streaming responses.
	SupportsStreaming *bool `json:"supports_streaming,omitempty"`

	// SupportsVision indicates whether the model supports image/vision input.
	SupportsVision *bool `json:"supports_vision,omitempty"`

	// MaxTokensFieldName overrides the field name used for max output tokens.
	// Some providers use "max_tokens", others use "max_completion_tokens".
	MaxTokensFieldName string `json:"max_tokens_field_name,omitempty"`

	// RequiresMaxTokens indicates this model requires max_tokens to be explicitly set.
	RequiresMaxTokens bool `json:"requires_max_tokens,omitempty"`

	// StopSequenceSupported indicates whether the model supports custom stop sequences.
	StopSequenceSupported *bool `json:"stop_sequence_supported,omitempty"`

	// TemperatureRange constrains the temperature range for this model (some models
	// only accept 0.0-1.0 instead of the default 0.0-2.0).
	TemperatureRange *FloatRange `json:"temperature_range,omitempty"`
}

// FloatRange represents a min/max float range.
type FloatRange struct {
	Min float32 `json:"min"`
	Max float32 `json:"max"`
}

// GetBoolOrDefault returns the value of a *bool pointer, or the default if nil.
func GetBoolOrDefault(ptr *bool, defaultVal bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

// ModelCompatRule is a rule that applies compatibility patches to a model's configuration.
// Each rule has a matcher (which models it applies to) and a set of patches.
//
// This follows the K8S admission webhook pattern: rules intercept and mutate
// model configuration before it reaches the provider SDK.
type ModelCompatRule struct {
	// Name is a human-readable name for this rule.
	Name string `json:"name"`

	// Description explains what this rule does.
	Description string `json:"description,omitempty"`

	// Matcher determines which models this rule applies to.
	Matcher ModelCompatMatcher `json:"matcher"`

	// Patches are the compatibility fixes to apply.
	Patches ModelCompatConfig `json:"patches"`
}

// ModelCompatMatcher determines whether a compat rule should apply to a given model.
type ModelCompatMatcher struct {
	// ProviderIDs matches models from specific providers (empty = match all).
	ProviderIDs []string `json:"provider_ids,omitempty"`

	// ModelIDs matches specific model IDs (empty = match all).
	ModelIDs []string `json:"model_ids,omitempty"`

	// ModelClasses matches specific model classes (empty = match all).
	ModelClasses []ModelClass `json:"model_classes,omitempty"`

	// APITypes matches specific API types (empty = match all).
	APITypes []ModelAPI `json:"api_types,omitempty"`

	// BaseURLContains matches providers whose BaseURL contains this substring.
	BaseURLContains string `json:"base_url_contains,omitempty"`
}

// Matches checks whether this matcher matches the given model instance and provider.
func (m *ModelCompatMatcher) Matches(instance *ModelInstance, provider *ModelProvider) bool {
	if len(m.ProviderIDs) > 0 && !containsString(m.ProviderIDs, instance.ProviderID) {
		return false
	}
	if len(m.ModelIDs) > 0 && !containsString(m.ModelIDs, instance.ModelID) {
		return false
	}
	if len(m.ModelClasses) > 0 && !containsModelClass(m.ModelClasses, provider.ModelClass) {
		return false
	}
	if len(m.APITypes) > 0 && !containsAPI(m.APITypes, provider.API) {
		return false
	}
	if m.BaseURLContains != "" {
		found := false
		if provider.BaseURL != "" {
			for i := 0; i <= len(provider.BaseURL)-len(m.BaseURLContains); i++ {
				if provider.BaseURL[i:i+len(m.BaseURLContains)] == m.BaseURLContains {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func containsModelClass(slice []ModelClass, val ModelClass) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func containsAPI(slice []ModelAPI, val ModelAPI) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
