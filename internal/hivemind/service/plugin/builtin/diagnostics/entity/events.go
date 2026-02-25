package entity

// DiagnosticEventType identifies the kind of diagnostic event.
type DiagnosticEventType string

const (
	// EventModelUsage is emitted when an LLM call completes (token counts, cost).
	EventModelUsage DiagnosticEventType = "model.usage"

	// EventWebhookReceived is emitted when a webhook HTTP request arrives.
	EventWebhookReceived DiagnosticEventType = "webhook.received"

	// EventWebhookProcessed is emitted when webhook processing completes.
	EventWebhookProcessed DiagnosticEventType = "webhook.processed"

	// EventWebhookError is emitted when webhook processing fails.
	EventWebhookError DiagnosticEventType = "webhook.error"

	// EventMessageQueued is emitted when a message enters the processing queue.
	EventMessageQueued DiagnosticEventType = "message.queued"

	// EventMessageProcessed is emitted when a message finishes processing.
	EventMessageProcessed DiagnosticEventType = "message.processed"

	// EventSessionStart is emitted when an agent session starts.
	EventSessionStart DiagnosticEventType = "session.start"

	// EventSessionEnd is emitted when an agent session ends.
	EventSessionEnd DiagnosticEventType = "session.end"

	// EventRunAttempt is emitted per LLM generation attempt.
	EventRunAttempt DiagnosticEventType = "run.attempt"
)

// DiagnosticEvent is a generic diagnostic event payload.
type DiagnosticEvent struct {
	// Type identifies the event kind.
	Type DiagnosticEventType `json:"type"`

	// Attrs holds event-specific key-value attributes.
	Attrs map[string]interface{} `json:"attrs,omitempty"`
}

// ModelUsageAttrs extracts model usage attributes from event attrs.
type ModelUsageAttrs struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMs   int64   `json:"duration_ms"`
}

// RunAttemptAttrs holds run attempt attributes.
type RunAttemptAttrs struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}
