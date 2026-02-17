package entity

import (
	"fmt"
	"net/http"
	"strings"
)

// FailoverReason classifies why a model request failed and triggered failover.
// Modeled after OpenClaw's FailoverReason type with K8S-style enum pattern.
type FailoverReason int32

const (
	// FailoverReason_Unknown is the default zero-value for unclassified errors.
	FailoverReason_Unknown FailoverReason = 0

	// FailoverReason_Auth indicates authentication failure (HTTP 401/403).
	FailoverReason_Auth FailoverReason = 1

	// FailoverReason_RateLimit indicates rate limiting (HTTP 429).
	FailoverReason_RateLimit FailoverReason = 2

	// FailoverReason_Billing indicates billing/quota issues (HTTP 402).
	FailoverReason_Billing FailoverReason = 3

	// FailoverReason_Timeout indicates request timeout (HTTP 408, context deadline exceeded, etc.).
	FailoverReason_Timeout FailoverReason = 4

	// FailoverReason_Format indicates request format errors (HTTP 400, invalid params, etc.).
	FailoverReason_Format FailoverReason = 5

	// FailoverReason_Unavailable indicates the model/provider is temporarily unavailable (HTTP 503).
	FailoverReason_Unavailable FailoverReason = 6

	// FailoverReason_ServerError indicates an internal server error from the provider (HTTP 500/502/504).
	FailoverReason_ServerError FailoverReason = 7
)

func (r FailoverReason) String() string {
	switch r {
	case FailoverReason_Unknown:
		return "unknown"
	case FailoverReason_Auth:
		return "auth"
	case FailoverReason_RateLimit:
		return "rate_limit"
	case FailoverReason_Billing:
		return "billing"
	case FailoverReason_Timeout:
		return "timeout"
	case FailoverReason_Format:
		return "format"
	case FailoverReason_Unavailable:
		return "unavailable"
	case FailoverReason_ServerError:
		return "server_error"
	default:
		return fmt.Sprintf("FailoverReason(%d)", r)
	}
}

// IsRetryable returns whether this failure reason suggests a retry with the same model
// might succeed (transient errors).
func (r FailoverReason) IsRetryable() bool {
	switch r {
	case FailoverReason_RateLimit, FailoverReason_Timeout,
		FailoverReason_Unavailable, FailoverReason_ServerError:
		return true
	default:
		return false
	}
}

// ShouldFailover returns whether this failure reason should trigger a switch
// to a fallback candidate model.
//
// Format errors (HTTP 400) are NOT failover-worthy because request format issues
// will persist regardless of which model/provider handles the request.
// Auth/Billing errors DO trigger failover because a different provider may have
// valid credentials or quota.
func (r FailoverReason) ShouldFailover() bool {
	switch r {
	case FailoverReason_Format:
		// Request format error: switching model won't help, the request itself is malformed.
		return false
	case FailoverReason_Auth, FailoverReason_Billing:
		// Non-transient but provider-specific: a different provider may succeed.
		return true
	case FailoverReason_RateLimit, FailoverReason_Timeout,
		FailoverReason_Unavailable, FailoverReason_ServerError:
		// Transient: failover is desirable for availability.
		return true
	default:
		// Unknown errors: attempt failover as a best-effort strategy.
		return true
	}
}

// HTTPStatusCode returns the canonical HTTP status code for this reason.
func (r FailoverReason) HTTPStatusCode() int {
	switch r {
	case FailoverReason_Auth:
		return http.StatusUnauthorized
	case FailoverReason_RateLimit:
		return http.StatusTooManyRequests
	case FailoverReason_Billing:
		return http.StatusPaymentRequired
	case FailoverReason_Timeout:
		return http.StatusRequestTimeout
	case FailoverReason_Format:
		return http.StatusBadRequest
	case FailoverReason_Unavailable:
		return http.StatusServiceUnavailable
	case FailoverReason_ServerError:
		return http.StatusInternalServerError
	default:
		return 0
	}
}

// FailoverError is a structured error type for model failover scenarios.
// It carries the classified reason, provider/model context, and optional HTTP status/code.
//
// Modeled after OpenClaw's FailoverError class, but using Go's error interface
// and K8S-style structured error patterns.
type FailoverError struct {
	// Reason classifies the failure.
	Reason FailoverReason `json:"reason"`

	// Provider is the provider that produced the error.
	Provider string `json:"provider,omitempty"`

	// Model is the model that produced the error.
	Model string `json:"model,omitempty"`

	// StatusCode is the HTTP status code from the provider response, if available.
	StatusCode int `json:"status_code,omitempty"`

	// Code is the provider-specific error code string, if available.
	Code string `json:"code,omitempty"`

	// Message is the human-readable error description.
	Message string `json:"message"`

	// Cause is the underlying error, if any.
	Cause error `json:"-"`
}

// Error implements the error interface.
func (e *FailoverError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[failover:%s]", e.Reason))

	if e.Provider != "" || e.Model != "" {
		sb.WriteString(fmt.Sprintf(" %s/%s:", e.Provider, e.Model))
	}

	sb.WriteString(" ")
	sb.WriteString(e.Message)

	if e.StatusCode != 0 {
		sb.WriteString(fmt.Sprintf(" (HTTP %d)", e.StatusCode))
	}
	if e.Code != "" {
		sb.WriteString(fmt.Sprintf(" [code=%s]", e.Code))
	}

	return sb.String()
}

// Unwrap implements Go's error unwrapping.
func (e *FailoverError) Unwrap() error {
	return e.Cause
}

// Is checks if this error matches the target type.
func (e *FailoverError) Is(target error) bool {
	t, ok := target.(*FailoverError)
	if !ok {
		return false
	}
	// If target only has Reason set, match by reason.
	if t.Provider == "" && t.Model == "" && t.Message == "" {
		return e.Reason == t.Reason
	}
	return false
}

// NewFailoverError creates a new FailoverError with the given parameters.
func NewFailoverError(reason FailoverReason, provider, model, message string) *FailoverError {
	return &FailoverError{
		Reason:     reason,
		Provider:   provider,
		Model:      model,
		StatusCode: reason.HTTPStatusCode(),
		Message:    message,
	}
}

// NewFailoverErrorFromCause wraps an existing error into a FailoverError by classifying it.
func NewFailoverErrorFromCause(err error, provider, model string) *FailoverError {
	if err == nil {
		return nil
	}

	// If already a FailoverError, enrich with context.
	if fe, ok := err.(*FailoverError); ok {
		if fe.Provider == "" {
			fe.Provider = provider
		}
		if fe.Model == "" {
			fe.Model = model
		}
		return fe
	}

	reason := ClassifyError(err)
	return &FailoverError{
		Reason:     reason,
		Provider:   provider,
		Model:      model,
		StatusCode: extractStatusCode(err),
		Code:       extractErrorCode(err),
		Message:    err.Error(),
		Cause:      err,
	}
}

// ClassifyError determines the FailoverReason from a raw error.
// It uses a layered approach: HTTP status → error code → message pattern matching.
// Modeled after OpenClaw's classifyFailoverReason + resolveFailoverReasonFromError.
func ClassifyError(err error) FailoverReason {
	if err == nil {
		return FailoverReason_Unknown
	}

	// Layer 1: Check if already classified.
	if fe, ok := err.(*FailoverError); ok {
		return fe.Reason
	}

	// Layer 2: HTTP status code.
	if status := extractStatusCode(err); status != 0 {
		if reason := classifyFromStatus(status); reason != FailoverReason_Unknown {
			return reason
		}
	}

	// Layer 3: Error code string.
	if code := extractErrorCode(err); code != "" {
		if reason := classifyFromCode(code); reason != FailoverReason_Unknown {
			return reason
		}
	}

	// Layer 4: Message pattern matching (last resort).
	return classifyFromMessage(err.Error())
}

// classifyFromStatus maps HTTP status codes to FailoverReason.
func classifyFromStatus(status int) FailoverReason {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return FailoverReason_Auth
	case status == http.StatusPaymentRequired:
		return FailoverReason_Billing
	case status == http.StatusTooManyRequests:
		return FailoverReason_RateLimit
	case status == http.StatusRequestTimeout:
		return FailoverReason_Timeout
	case status == http.StatusBadRequest:
		return FailoverReason_Format
	case status == http.StatusServiceUnavailable:
		return FailoverReason_Unavailable
	case status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusGatewayTimeout:
		return FailoverReason_ServerError
	default:
		return FailoverReason_Unknown
	}
}

// classifyFromCode maps error code strings to FailoverReason.
func classifyFromCode(code string) FailoverReason {
	upper := strings.ToUpper(code)
	switch {
	case upper == "ETIMEDOUT" || upper == "ESOCKETTIMEDOUT" ||
		upper == "ECONNRESET" || upper == "ECONNABORTED":
		return FailoverReason_Timeout
	case upper == "ECONNREFUSED":
		return FailoverReason_Unavailable
	default:
		return FailoverReason_Unknown
	}
}

// classifyFromMessage classifies errors by pattern matching on the error message.
// Modeled after OpenClaw's classifyFailoverReason in pi-embedded-helpers/errors.ts.
func classifyFromMessage(msg string) FailoverReason {
	lower := strings.ToLower(msg)

	// Timeout patterns.
	timeoutPatterns := []string{
		"timeout", "timed out", "deadline exceeded",
		"context deadline exceeded", "context canceled",
	}
	for _, p := range timeoutPatterns {
		if strings.Contains(lower, p) {
			return FailoverReason_Timeout
		}
	}

	// Rate limit patterns.
	rateLimitPatterns := []string{
		"rate limit", "rate_limit", "ratelimit",
		"too many requests", "quota exceeded",
		"throttl",
	}
	for _, p := range rateLimitPatterns {
		if strings.Contains(lower, p) {
			return FailoverReason_RateLimit
		}
	}

	// Auth patterns.
	authPatterns := []string{
		"unauthorized", "authentication", "invalid api key",
		"invalid_api_key", "forbidden", "access denied",
	}
	for _, p := range authPatterns {
		if strings.Contains(lower, p) {
			return FailoverReason_Auth
		}
	}

	// Billing patterns.
	billingPatterns := []string{
		"billing", "payment", "insufficient_quota",
		"insufficient funds", "credit",
	}
	for _, p := range billingPatterns {
		if strings.Contains(lower, p) {
			return FailoverReason_Billing
		}
	}

	// Unavailable patterns.
	unavailablePatterns := []string{
		"unavailable", "service overloaded", "overloaded",
		"connection refused",
	}
	for _, p := range unavailablePatterns {
		if strings.Contains(lower, p) {
			return FailoverReason_Unavailable
		}
	}

	// Server error patterns.
	serverPatterns := []string{
		"internal server error", "internal error", "bad gateway",
	}
	for _, p := range serverPatterns {
		if strings.Contains(lower, p) {
			return FailoverReason_ServerError
		}
	}

	return FailoverReason_Unknown
}

// statusCodeCarrier is an interface for errors that carry an HTTP status code.
type statusCodeCarrier interface {
	StatusCode() int
}

// statusCarrier is an interface for errors that carry a status field.
type statusCarrier interface {
	Status() int
}

// extractStatusCode attempts to extract an HTTP status code from an error.
func extractStatusCode(err error) int {
	if c, ok := err.(statusCodeCarrier); ok {
		return c.StatusCode()
	}
	if c, ok := err.(statusCarrier); ok {
		return c.Status()
	}
	return 0
}

// errorCodeCarrier is an interface for errors that carry a provider-specific error code.
type errorCodeCarrier interface {
	ErrorCode() string
}

// extractErrorCode attempts to extract an error code string from an error.
func extractErrorCode(err error) string {
	if c, ok := err.(errorCodeCarrier); ok {
		return c.ErrorCode()
	}
	return ""
}
