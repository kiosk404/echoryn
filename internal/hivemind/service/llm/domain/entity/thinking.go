package entity

import (
	"fmt"
	"strings"
)

// ThinkingLevel controls the LLM's reasoning depth / thinking time.
//
// Modeled after OpenClaw's ThinkLevel (6-tier) and adapted for Echoryn's Go architecture.
// Each provider maps this unified level to its own SDK parameters via ThinkingStrategy.
//
// Levels (ascending reasoning effort):
//
//	Off      → No extra reasoning; direct generation
//	Minimal  → Very light reasoning pass
//	Low      → Basic reasoning (default for "on" / reasoning models)
//	Medium   → Moderate reasoning
//	High     → Deep reasoning
//	XHigh    → Maximum reasoning (provider-dependent; may degrade to High)
type ThinkingLevel string

const (
	ThinkingLevelOff     ThinkingLevel = "off"
	ThinkingLevelMinimal ThinkingLevel = "minimal"
	ThinkingLevelLow     ThinkingLevel = "low"
	ThinkingLevelMedium  ThinkingLevel = "medium"
	ThinkingLevelHigh    ThinkingLevel = "high"
	ThinkingLevelXHigh   ThinkingLevel = "xhigh"
)

// allThinkingLevels is the ordered list from lowest to highest effort.
var allThinkingLevels = []ThinkingLevel{
	ThinkingLevelOff,
	ThinkingLevelMinimal,
	ThinkingLevelLow,
	ThinkingLevelMedium,
	ThinkingLevelHigh,
	ThinkingLevelXHigh,
}

// IsValid returns true if the level is a recognized ThinkingLevel.
func (l ThinkingLevel) IsValid() bool {
	switch l {
	case ThinkingLevelOff, ThinkingLevelMinimal, ThinkingLevelLow,
		ThinkingLevelMedium, ThinkingLevelHigh, ThinkingLevelXHigh:
		return true
	}
	return false
}

// IsEnabled returns true if thinking is active (anything above Off).
func (l ThinkingLevel) IsEnabled() bool {
	return l != "" && l != ThinkingLevelOff
}

// String returns the canonical string representation.
func (l ThinkingLevel) String() string {
	if l == "" {
		return "off"
	}
	return string(l)
}

// Ordinal returns the numeric ordinal (0=Off … 5=XHigh) for comparison.
func (l ThinkingLevel) Ordinal() int {
	switch l {
	case ThinkingLevelOff:
		return 0
	case ThinkingLevelMinimal:
		return 1
	case ThinkingLevelLow:
		return 2
	case ThinkingLevelMedium:
		return 3
	case ThinkingLevelHigh:
		return 4
	case ThinkingLevelXHigh:
		return 5
	default:
		return 0
	}
}

// ClampMax returns the level capped at maxLevel.
// Used for provider-level degradation (e.g., xhigh → high for unsupported providers).
func (l ThinkingLevel) ClampMax(maxLevel ThinkingLevel) ThinkingLevel {
	if l.Ordinal() > maxLevel.Ordinal() {
		return maxLevel
	}
	return l
}

// AllThinkingLevels returns all valid thinking levels in ascending order.
func AllThinkingLevels() []ThinkingLevel {
	return allThinkingLevels
}

// NormalizeThinkingLevel normalizes a raw string to a canonical ThinkingLevel.
//
// Supports aliases inspired by OpenClaw's normalizeThinkLevel:
//
//	"on"/"enable"/"enabled"  → Low
//	"min"/"minimal"/"think"  → Minimal
//	"thinkhard"              → Low
//	"thinkharder"/"harder"   → Medium
//	"ultra"/"max"/"highest"  → High
//	"xhigh"/"extrahigh"     → XHigh
//
// Returns ("", error) for unrecognized inputs.
func NormalizeThinkingLevel(raw string) (ThinkingLevel, error) {
	if raw == "" {
		return ThinkingLevelOff, nil
	}

	key := strings.TrimSpace(strings.ToLower(raw))
	// Collapse separators: "think-hard" → "thinkhard", "x_high" → "xhigh"
	collapsed := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(key)

	// xhigh / extrahigh
	if collapsed == "xhigh" || collapsed == "extrahigh" {
		return ThinkingLevelXHigh, nil
	}

	switch key {
	case "off", "disable", "disabled", "none":
		return ThinkingLevelOff, nil
	case "on", "enable", "enabled":
		return ThinkingLevelLow, nil
	case "min", "minimal", "think":
		return ThinkingLevelMinimal, nil
	}

	switch collapsed {
	case "low", "thinkhard":
		return ThinkingLevelLow, nil
	case "mid", "med", "medium", "thinkharder", "harder":
		return ThinkingLevelMedium, nil
	case "high", "ultra", "ultrathink", "thinkhardest", "highest", "max":
		return ThinkingLevelHigh, nil
	}

	// Try direct match for valid levels.
	candidate := ThinkingLevel(key)
	if candidate.IsValid() {
		return candidate, nil
	}

	return "", fmt.Errorf("unrecognized thinking level: %q", raw)
}

// ResolveThinkingDefault determines the default ThinkingLevel for a model.
//
// Priority chain (aligned with OpenClaw's resolveThinkingDefault):
//  1. Explicit agentDefault (if set and valid) → use it
//  2. Model is a reasoning model (Reasoning=true) → Low
//  3. Otherwise → Off
func ResolveThinkingDefault(agentDefault ThinkingLevel, isReasoningModel bool) ThinkingLevel {
	if agentDefault.IsValid() && agentDefault != "" {
		return agentDefault
	}
	if isReasoningModel {
		return ThinkingLevelLow
	}
	return ThinkingLevelOff
}
