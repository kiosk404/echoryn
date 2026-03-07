package trace

// LLM semantic attribute keys aligned with OpenTelemetry GenAI Semantic Conventions.
//
// Reference: https://opentelemetry.io/docs/specs/semconv/gen-ai/
//
// These constants define the standard attribute keys for LLM tracing.
// All gen_ai.* attributes follow the OTEL specification to ensure
// compatibility with OTEL-based backends (Jaeger, Grafana Tempo, etc.).
const (
	// --- gen_ai.system: Identifies the LLM provider ---

	// AttrGenAISystem identifies the GenAI provider vendor (e.g., "openai", "anthropic").
	AttrGenAISystem = "gen_ai.system"

	// --- gen_ai.request.*: Request-level attributes ---

	// AttrGenAIRequestModel is the model name requested by the caller (e.g., "gpt-4o").
	AttrGenAIRequestModel = "gen_ai.request.model"

	// AttrGenAIRequestMaxTokens is the maximum number of output tokens requested.
	AttrGenAIRequestMaxTokens = "gen_ai.request.max_tokens"

	// AttrGenAIRequestTemperature is the sampling temperature.
	AttrGenAIRequestTemperature = "gen_ai.request.temperature"

	// AttrGenAIRequestTopP is the nucleus sampling parameter.
	AttrGenAIRequestTopP = "gen_ai.request.top_p"

	// AttrGenAIRequestStopSequences is the stop sequences list.
	AttrGenAIRequestStopSequences = "gen_ai.request.stop_sequences"

	// AttrGenAIRequestFrequencyPenalty is the frequency penalty.
	AttrGenAIRequestFrequencyPenalty = "gen_ai.request.frequency_penalty"

	// AttrGenAIRequestPresencePenalty is the presence penalty.
	AttrGenAIRequestPresencePenalty = "gen_ai.request.presence_penalty"

	// --- gen_ai.response.*: Response-level attributes ---

	// AttrGenAIResponseModel is the model name actually used for generation.
	AttrGenAIResponseModel = "gen_ai.response.model"

	// AttrGenAIResponseID is the provider's response/completion ID.
	AttrGenAIResponseID = "gen_ai.response.id"

	// AttrGenAIResponseFinishReasons is the list of finish reasons.
	AttrGenAIResponseFinishReasons = "gen_ai.response.finish_reasons"

	// --- gen_ai.usage.*: Token usage attributes ---

	// AttrGenAIUsageInputTokens is the number of input/prompt tokens.
	AttrGenAIUsageInputTokens = "gen_ai.usage.input_tokens"

	// AttrGenAIUsageOutputTokens is the number of output/completion tokens.
	AttrGenAIUsageOutputTokens = "gen_ai.usage.output_tokens"

	// --- gen_ai.token.*: Cost tracking ---

	// AttrGenAITokenCostUSD is the estimated cost in USD for this operation.
	// This is an Echoryn extension (not in OTEL spec).
	AttrGenAITokenCostUSD = "gen_ai.token.cost_usd"

	// --- gen_ai.tool.*: Tool/function call attributes ---

	// AttrGenAIToolName is the name of the tool being called.
	AttrGenAIToolName = "gen_ai.tool.name"

	// AttrGenAIToolCallID is the provider-assigned tool call identifier.
	AttrGenAIToolCallID = "gen_ai.tool.call_id"

	// --- Echoryn-specific agent attributes ---

	// AttrAgentID is the Echoryn agent identifier.
	AttrAgentID = "echoryn.agent.id"

	// AttrAgentName is the Echoryn agent human-readable name.
	AttrAgentName = "echoryn.agent.name"

	// AttrSessionID is the session identifier.
	AttrSessionID = "echoryn.session.id"

	// AttrRunID is the run identifier.
	AttrRunID = "echoryn.run.id"

	// AttrRunStatus is the final run status (completed, failed, cancelled).
	AttrRunStatus = "echoryn.run.status"

	// AttrRunInput is the user input text (may be truncated for privacy).
	AttrRunInput = "echoryn.run.input"

	// AttrRunOutput is the assistant output text (may be truncated for privacy).
	AttrRunOutput = "echoryn.run.output"

	// AttrTurnNumber is the turn number within the run (1-indexed).
	AttrTurnNumber = "echoryn.turn.number"

	// AttrTurnRetryReason is the reason for retrying a turn.
	AttrTurnRetryReason = "echoryn.turn.retry_reason"

	// AttrCompactionAttempt is the compaction attempt number.
	AttrCompactionAttempt = "echoryn.compaction.attempt"

	// AttrCompactionTokensBefore is the estimated token count before compaction.
	AttrCompactionTokensBefore = "echoryn.compaction.tokens_before"

	// AttrCompactionTokensAfter is the estimated token count after compaction.
	AttrCompactionTokensAfter = "echoryn.compaction.tokens_after"

	// AttrModelRef is the fully-qualified model reference (provider/model).
	AttrModelRef = "echoryn.model_ref"

	// AttrSubAgentID is the sub-agent identifier.
	AttrSubAgentID = "echoryn.subagent.id"

	// AttrContextWindowSize is the model's context window size in tokens.
	AttrContextWindowSize = "echoryn.context_window.size"

	// AttrContextTokensUsed is the estimated tokens used in the context.
	AttrContextTokensUsed = "echoryn.context.tokens_used"

	// AttrToolLoopDetected indicates whether a tool loop was detected.
	AttrToolLoopDetected = "echoryn.tool_loop.detected"

	// --- gen_ai.content.*: Event names for prompt/completion content ---

	// EventGenAIContentPrompt is the event name for recording prompt content.
	EventGenAIContentPrompt = "gen_ai.content.prompt"

	// EventGenAIContentCompletion is the event name for recording completion content.
	EventGenAIContentCompletion = "gen_ai.content.completion"

	// EventGenAIToolInput is the event name for recording tool input.
	EventGenAIToolInput = "gen_ai.tool.input"

	// EventGenAIToolOutput is the event name for recording tool output.
	EventGenAIToolOutput = "gen_ai.tool.output"
)
