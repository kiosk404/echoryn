// Package llmtask implements the "llm-task" built-in plugin.
//
// This plugin provides a generic JSON-only LLM task tool that can be
// invoked by agents for structured sub-tasks (summarization, classification,
// data extraction, etc.). It corresponds to OpenClaw's "llm-task" plugin.
//
// Registered capabilities:
//   - Tool: "llm_task" — run a structured JSON-only LLM task
//
// The tool accepts a prompt, optional input payload, optional JSON Schema,
// and optional provider/model overrides. It returns parsed JSON output.
//
// The plugin obtains the LLM ChatModel via RuntimeAPI.ModelManager(),
// bridging to the core LLM module without direct package dependency.
package llmtask

import (
	"context"
	"encoding/json"
	"fmt"

	einomodel "github.com/cloudwego/eino/components/model"
	einoschema "github.com/cloudwego/eino/schema"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/llmtask/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/llmtask/runner"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "llm-task"
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "LLM Task",
		Kind:        "", // No slot constraint.
		Description: "Generic JSON-only LLM tool for structured tasks callable from agents and workflows",
	}
}

// llmTaskPlugin is the runtime instance.
type llmTaskPlugin struct {
	cfg    *entity.LLMTaskConfig
	runner *runner.Runner
	handle plugin.Handle
}

// Factory is the PluginFactory for llm-task.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return &llmTaskPlugin{
			cfg:    entity.DefaultLLMTaskConfig(),
			handle: handle,
		}, nil
	}
	cfg, ok := cfgRaw.(*entity.LLMTaskConfig)
	if !ok {
		return nil, fmt.Errorf("llm-task: 'config' must be *entity.LLMTaskConfig, got %T", cfgRaw)
	}
	return &llmTaskPlugin{
		cfg:    cfg,
		handle: handle,
	}, nil
}

// Name implements plugin.Plugin.
func (p *llmTaskPlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
func (p *llmTaskPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[LLMTask] plugin is disabled, skipping tool registration")
		return nil
	}

	// Register the llm_task tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "llm_task",
		Description: "Run a structured JSON-only LLM task. Accepts a prompt and optional input/schema, returns parsed JSON. Useful for summarization, classification, data extraction, and other structured sub-tasks.",
		Parameters: []plugin.ParameterDef{
			{Name: "prompt", Type: "string", Description: "The instruction prompt for the LLM (what task to perform)", Required: true},
			{Name: "input", Type: "object", Description: "Optional structured input data to pass to the LLM", Required: false},
			{Name: "schema", Type: "object", Description: "Optional JSON Schema to validate the LLM output against", Required: false},
			{Name: "provider", Type: "string", Description: "Override the LLM provider (e.g., 'deepseek', 'openai')", Required: false},
			{Name: "model", Type: "string", Description: "Override the model ID (e.g., 'deepseek-chat', 'gpt-4')", Required: false},
			{Name: "temperature", Type: "number", Description: "Temperature for generation (0.0 to 2.0)", Required: false},
			{Name: "max_tokens", Type: "number", Description: "Maximum output tokens", Required: false},
			{Name: "timeout_ms", Type: "number", Description: "Timeout in milliseconds", Required: false},
		},
		Handler: p.handleLLMTask,
	})

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *llmTaskPlugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}

	// Wire the real LLM caller via RuntimeAPI.ModelManager().
	mm := p.handle.RuntimeAPI().ModelManager()
	if mm == nil {
		logger.Warn("[LLMTask] RuntimeAPI.ModelManager() is nil, llm_task tool will not function")
		p.runner = runner.New(&placeholderCaller{}, p.cfg)
	} else {
		p.runner = runner.New(&modelManagerCaller{mm: mm}, p.cfg)
	}

	logger.Info("[LLMTask] started (default_provider=%s, default_model=%s, max_tokens=%d)",
		p.cfg.DefaultProvider, p.cfg.DefaultModel, p.cfg.MaxTokens)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *llmTaskPlugin) Stop(ctx context.Context) error {
	logger.Info("[LLMTask] stopped")
	return nil
}

// --- Tool Handler ---

func (p *llmTaskPlugin) handleLLMTask(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.runner == nil {
		return nil, fmt.Errorf("llm-task plugin is not initialized")
	}

	// Parse the request from tool parameters.
	req, err := parseRequest(params)
	if err != nil {
		return nil, fmt.Errorf("invalid llm_task parameters: %w", err)
	}

	// Execute the task.
	result, err := p.runner.Run(ctx, req)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// parseRequest converts raw tool parameters to an LLMTaskRequest.
func parseRequest(params map[string]interface{}) (*entity.LLMTaskRequest, error) {
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return nil, fmt.Errorf("parameter 'prompt' is required and must be a non-empty string")
	}

	req := &entity.LLMTaskRequest{
		Prompt: prompt,
	}

	// Optional: input.
	if v, ok := params["input"]; ok {
		req.Input = v
	}

	// Optional: schema.
	if v, ok := params["schema"]; ok && v != nil {
		schemaBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("invalid 'schema' parameter: %w", err)
		}
		req.Schema = schemaBytes
	}

	// Optional: provider.
	if v, ok := params["provider"].(string); ok {
		req.Provider = v
	}

	// Optional: model.
	if v, ok := params["model"].(string); ok {
		req.Model = v
	}

	// Optional: temperature.
	if v, ok := params["temperature"].(float64); ok {
		req.Temperature = &v
	}

	// Optional: max_tokens.
	if v, ok := params["max_tokens"].(float64); ok {
		req.MaxTokens = int(v)
	}

	// Optional: timeout_ms.
	if v, ok := params["timeout_ms"].(float64); ok {
		req.TimeoutMs = int64(v)
	}

	return req, nil
}

// --- LLM Callers ---

// modelManagerCaller implements runner.LLMCaller using the plugin.ModelManager
// obtained from RuntimeAPI. This is the production caller.
type modelManagerCaller struct {
	mm plugin.ModelManager
}

func (c *modelManagerCaller) Call(ctx context.Context, opts runner.CallOptions) (*runner.CallResult, error) {
	var chatModel einomodel.BaseChatModel
	var err error

	if opts.Provider != "" && opts.Model != "" {
		// Explicit provider/model override from the tool parameters.
		chatModel, err = c.mm.GetChatModel(ctx, opts.Provider, opts.Model)
	} else if ref, ok := llmEntity.ActiveModelRefFromContext(ctx); ok && ref.ProviderID != "" && ref.ModelID != "" {
		// No explicit override — inherit the model the parent agent is currently
		// using (injected by RunWithFallback). This ensures llm_task uses the same
		// model that already succeeded for the parent agent, instead of the system
		// default which may be misconfigured or unavailable.
		logger.Info("[LLMTask] using active agent model from context: %s/%s", ref.ProviderID, ref.ModelID)
		chatModel, err = c.mm.GetChatModel(ctx, ref.ProviderID, ref.ModelID)
	} else {
		// Final fallback: system default model.
		chatModel, err = c.mm.GetDefaultChatModel(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get chat model: %w", err)
	}

	// Build messages.
	messages := make([]*einoschema.Message, 0, 2)
	if opts.SystemMsg != "" {
		messages = append(messages, einoschema.SystemMessage(opts.SystemMsg))
	}
	if opts.UserMsg != "" {
		messages = append(messages, einoschema.UserMessage(opts.UserMsg))
	}

	// Invoke the model.
	resp, err := chatModel.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM generate failed: %w", err)
	}

	result := &runner.CallResult{
		Text: resp.Content,
	}
	if resp.ResponseMeta != nil && resp.ResponseMeta.Usage != nil {
		result.InputTokens = int64(resp.ResponseMeta.Usage.PromptTokens)
		result.OutputTokens = int64(resp.ResponseMeta.Usage.CompletionTokens)
	}
	return result, nil
}

// placeholderCaller is a fallback stub LLMCaller that returns an error
// when RuntimeAPI.ModelManager() is not available.
type placeholderCaller struct{}

func (c *placeholderCaller) Call(ctx context.Context, opts runner.CallOptions) (*runner.CallResult, error) {
	return nil, fmt.Errorf("LLM caller not configured: RuntimeAPI.ModelManager() is nil")
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*llmTaskPlugin)(nil)
	_ plugin.InitPlugin      = (*llmTaskPlugin)(nil)
	_ plugin.LifecyclePlugin = (*llmTaskPlugin)(nil)
)
