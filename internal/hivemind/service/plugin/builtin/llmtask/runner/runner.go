package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/llmtask/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// LLMCaller abstracts the actual LLM invocation.
// This interface allows the runner to be tested independently and
// decouples the plugin from the specific LLM implementation.
type LLMCaller interface {
	// Call sends a prompt to the LLM and returns the raw text response.
	// The provider and model parameters identify which model to use.
	// If provider/model are empty, the implementation should use defaults.
	Call(ctx context.Context, opts CallOptions) (*CallResult, error)
}

// CallOptions holds the parameters for an LLM call.
type CallOptions struct {
	Provider    string
	Model       string
	SystemMsg   string
	UserMsg     string
	Temperature *float64
	MaxTokens   int
	JSONMode    bool
}

// CallResult is the raw result from an LLM call.
type CallResult struct {
	Text         string
	InputTokens  int64
	OutputTokens int64
}

// Runner executes llm-task requests.
type Runner struct {
	caller LLMCaller
	cfg    *entity.LLMTaskConfig
}

// New creates a new Runner with the given LLM caller and config.
func New(caller LLMCaller, cfg *entity.LLMTaskConfig) *Runner {
	return &Runner{
		caller: caller,
		cfg:    cfg,
	}
}

// codeBlockRegex strips markdown code fences from LLM output.
var codeBlockRegex = regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*\n?(.*?)\\s*```\\s*$")

// stripCodeFences removes markdown code fences from text.
func stripCodeFences(text string) string {
	text = strings.TrimSpace(text)
	if matches := codeBlockRegex.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return text
}

// Run executes an LLM task request and returns the parsed result.
func (r *Runner) Run(ctx context.Context, req *entity.LLMTaskRequest) (*entity.LLMTaskResult, error) {
	// Resolve provider/model.
	provider := req.Provider
	if provider == "" {
		provider = r.cfg.DefaultProvider
	}
	model := req.Model
	if model == "" {
		model = r.cfg.DefaultModel
	}

	// Check allowed models.
	if len(r.cfg.AllowedModels) > 0 {
		modelKey := provider + "/" + model
		allowed := false
		for _, am := range r.cfg.AllowedModels {
			if am == modelKey {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("model %q is not in the allowed models list", modelKey)
		}
	}

	// Resolve maxTokens and timeout.
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = r.cfg.MaxTokens
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = r.cfg.TimeoutMs
	}

	// Build messages.
	//
	// System message: JSON format constraints + optional schema.
	// User message: prompt (task instruction) + optional input data.
	//
	// Previous bug: prompt was only in system message, and when input was nil,
	// user message was empty. DeepSeek and other LLMs require at least one
	// user-role message, returning 400 "No query found in messages".
	//
	// Fix: prompt always goes into user message so the actual task instruction
	// is never lost. System message only contains the JSON output format rules.
	systemMsg := buildSystemPrompt(req.Schema)
	userMsg := buildUserMessage(req.Prompt, req.Input)

	// Apply timeout.
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	start := time.Now()

	// Invoke the LLM.
	callResult, err := r.caller.Call(callCtx, CallOptions{
		Provider:    provider,
		Model:       model,
		SystemMsg:   systemMsg,
		UserMsg:     userMsg,
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()

	// Strip code fences and parse JSON.
	rawText := callResult.Text
	cleaned := stripCodeFences(rawText)

	var parsed interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("LLM returned invalid JSON: %w\nraw output:\n%s", err, rawText)
	}

	// Validate against schema if provided.
	if len(req.Schema) > 0 {
		if err := validateJSONSchema(parsed, req.Schema); err != nil {
			return nil, fmt.Errorf("LLM output does not match schema: %w", err)
		}
	}

	result := &entity.LLMTaskResult{
		JSON:         parsed,
		Raw:          rawText,
		Provider:     provider,
		Model:        model,
		InputTokens:  callResult.InputTokens,
		OutputTokens: callResult.OutputTokens,
		DurationMs:   durationMs,
	}

	logger.Info("[LLMTask] completed (provider=%s, model=%s, duration=%dms, in=%d, out=%d)",
		provider, model, durationMs, callResult.InputTokens, callResult.OutputTokens)

	return result, nil
}

// buildSystemPrompt constructs the system message with JSON format constraints only.
// The actual task instruction (prompt) is placed in the user message to ensure
// it is never lost when input is nil.
func buildSystemPrompt(schema json.RawMessage) string {
	var sb strings.Builder
	sb.WriteString("You are a structured data generator. You MUST respond with valid JSON only.\n")
	sb.WriteString("Do NOT include any explanation, markdown formatting, or code fences.\n")
	sb.WriteString("Respond with ONLY the JSON object/array.\n")

	if len(schema) > 0 {
		sb.WriteString("\nThe output MUST conform to this JSON Schema:\n")
		sb.Write(schema)
	}

	return sb.String()
}

// buildUserMessage constructs the user message combining the task prompt and
// optional input data. This ensures the prompt (task instruction) is always
// present in the user-role message, fixing the "No query found in messages"
// error from LLMs that require at least one user message.
func buildUserMessage(prompt string, input interface{}) string {
	var sb strings.Builder
	sb.WriteString(prompt)

	if input != nil {
		inputJSON, err := json.Marshal(input)
		if err == nil {
			sb.WriteString("\n\nInput data:\n")
			sb.Write(inputJSON)
		}
	}

	return sb.String()
}

// validateJSONSchema performs basic JSON schema validation.
// This is a simplified validation — it checks the top-level type.
// For production use, a full JSON Schema validator (e.g., github.com/santhosh-tekuri/jsonschema)
// should be integrated.
func validateJSONSchema(data interface{}, schema json.RawMessage) error {
	var schemaDef map[string]interface{}
	if err := json.Unmarshal(schema, &schemaDef); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	expectedType, _ := schemaDef["type"].(string)
	if expectedType == "" {
		return nil // No type constraint.
	}

	switch expectedType {
	case "object":
		if _, ok := data.(map[string]interface{}); !ok {
			return fmt.Errorf("expected object, got %T", data)
		}
	case "array":
		if _, ok := data.([]interface{}); !ok {
			return fmt.Errorf("expected array, got %T", data)
		}
	case "string":
		if _, ok := data.(string); !ok {
			return fmt.Errorf("expected string, got %T", data)
		}
	case "number":
		if _, ok := data.(float64); !ok {
			return fmt.Errorf("expected number, got %T", data)
		}
	case "boolean":
		if _, ok := data.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", data)
		}
	}

	return nil
}
