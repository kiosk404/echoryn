package entity

import (
	"fmt"
	"strings"
)

// FallbackConfig configures the model fallback behavior.
// Modeled after OpenClaw's model-fallback.ts parameters with K8S-style structured config.
type FallbackConfig struct {
	// Primary is the primary model to try first.
	Primary ModelRef `json:"primary"`

	// Fallbacks is the ordered list of fallback models to try if the primary fails.
	// If empty, only the primary model is tried.
	Fallbacks []ModelRef `json:"fallbacks,omitempty"`

	// MaxAttempts overrides the total number of attempts (primary + fallbacks).
	// 0 means try all candidates.
	MaxAttempts int `json:"max_attempts,omitempty"`

	// SkipOnCooldown skips models whose status is ModelStatus_Cooldown.
	SkipOnCooldown bool `json:"skip_on_cooldown,omitempty"`
}

// Candidates returns the ordered list of all candidate models (primary first, then fallbacks).
// It deduplicates by ModelRef.
func (c *FallbackConfig) Candidates() []ModelRef {
	seen := make(map[string]struct{})
	candidates := make([]ModelRef, 0, 1+len(c.Fallbacks))

	add := func(ref ModelRef) {
		key := ref.String()
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, ref)
	}

	add(c.Primary)
	for _, fb := range c.Fallbacks {
		add(fb)
	}

	return candidates
}

// EffectiveMaxAttempts returns the maximum number of attempts to make.
func (c *FallbackConfig) EffectiveMaxAttempts() int {
	total := 1 + len(c.Fallbacks)
	if c.MaxAttempts > 0 && c.MaxAttempts < total {
		return c.MaxAttempts
	}
	return total
}

// FallbackAttempt records the result of a single fallback attempt.
// Modeled after OpenClaw's FallbackAttempt type.
type FallbackAttempt struct {
	// Ref is the model that was attempted.
	Ref ModelRef `json:"ref"`

	// Error is the error message from the failed attempt.
	Error string `json:"error,omitempty"`

	// Reason is the classified failure reason (if applicable).
	Reason FailoverReason `json:"reason,omitempty"`

	// StatusCode is the HTTP status code from the provider (if available).
	StatusCode int `json:"status_code,omitempty"`

	// Skipped indicates the model was skipped without attempting (e.g., cooldown).
	Skipped bool `json:"skipped,omitempty"`

	// SkipReason explains why the model was skipped.
	SkipReason string `json:"skip_reason,omitempty"`
}

// FallbackResult holds the final outcome of a fallback execution.
type FallbackResult[T any] struct {
	// Value is the successful result (zero-value if all attempts failed).
	Value T

	// Ref is the model that produced the successful result.
	Ref ModelRef `json:"ref"`

	// Attempts records all attempts made (including successful one if any).
	Attempts []FallbackAttempt `json:"attempts"`

	// OK indicates whether any attempt succeeded.
	OK bool `json:"ok"`
}

// Summary returns a human-readable summary of all attempts.
func (r *FallbackResult[T]) Summary() string {
	if len(r.Attempts) == 0 {
		return "no attempts"
	}

	parts := make([]string, 0, len(r.Attempts))
	for _, a := range r.Attempts {
		if a.Skipped {
			parts = append(parts, fmt.Sprintf("%s: skipped (%s)", a.Ref, a.SkipReason))
		} else if a.Error != "" {
			reason := ""
			if a.Reason != FailoverReason_Unknown {
				reason = fmt.Sprintf(" (%s)", a.Reason)
			}
			parts = append(parts, fmt.Sprintf("%s: %s%s", a.Ref, a.Error, reason))
		}
	}

	return strings.Join(parts, " | ")
}

// AllFailedError returns a combined error if all attempts failed.
func (r *FallbackResult[T]) AllFailedError() error {
	if r.OK {
		return nil
	}
	return fmt.Errorf("all models failed (%d attempts): %s", len(r.Attempts), r.Summary())
}
