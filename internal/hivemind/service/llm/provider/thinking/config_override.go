package thinking

import "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"

// BudgetOverrides maps ThinkingLevel to budget_tokens, allowing configuration-driven
// customization of thinking budget values without code changes.
//
// Inspired by DeerFlow's config-driven when_thinking_enabled approach where
// thinking parameters are defined in config.yaml rather than hardcoded.
//
// Example JSON config:
//
//	{
//	  "thinking_budgets": {
//	    "anthropic": {"minimal": 2048, "low": 8192, "medium": 16384, "high": 32768, "xhigh": 65536},
//	    "gemini":    {"minimal": 2048, "low": 8192, "medium": 16384, "high": 32768}
//	  }
//	}
type BudgetOverrides map[entity.ThinkingLevel]int

// OverrideConfig holds per-provider thinking parameter overrides loaded from
// the Hivemind configuration file. When set, these values take precedence
// over the hardcoded defaults in each Strategy implementation.
type OverrideConfig struct {
	// Budgets maps provider name → ThinkingLevel → budget_tokens.
	// Used by ClaudeStrategy and GeminiStrategy to override their default budget tables.
	Budgets map[string]BudgetOverrides
}

// globalOverrides holds the currently active override config.
// Set via ApplyOverrides during initialization; nil means use hardcoded defaults.
var globalOverrides *OverrideConfig

// ApplyOverrides installs configuration-driven overrides for thinking strategies.
// Call this during Hivemind initialization after loading the server config.
// Pass nil to reset to hardcoded defaults.
func ApplyOverrides(cfg *OverrideConfig) {
	globalOverrides = cfg
}

// GetBudgetOverride returns the configured budget for a provider+level combination.
// Returns (budget, true) if an override exists, or (0, false) to use the hardcoded default.
func GetBudgetOverride(providerName string, level entity.ThinkingLevel) (int, bool) {
	if globalOverrides == nil || globalOverrides.Budgets == nil {
		return 0, false
	}
	providerBudgets, ok := globalOverrides.Budgets[providerName]
	if !ok {
		return 0, false
	}
	budget, ok := providerBudgets[level]
	return budget, ok
}
