package plugin

import (
	"context"

	"github.com/cloudwego/eino/components/model"
)

// RuntimeAPI is the bridge between plugins and core runtime modules.
// Plugins access core capabilities (LLM, Memory, etc.) through this interface.
type RuntimeAPI interface {
	// ModelManager returns the LLM model manager for building/retrieving chat models.
	// Return nil if the LLM module is not available.
	ModelManager() ModelManager
}

// ModelManager is a plugin-facing subset of the LLM ModelManager interface.
// It exposes only the capabilities that plugins need, decoupling the plugin
// package from the full llm/domain/service.ModelManager.
type ModelManager interface {
	// GetChatModel returns a chat model by provider ID and model ID.
	// Return nil if the model is not found.
	GetChatModel(ctx context.Context, provideID, modelID string) (model.BaseChatModel, error)

	// GetDefaultChatModel returns the default chat model for the current runtime.
	// Return nil if the default model is not set.
	GetDefaultChatModel(ctx context.Context) (model.BaseChatModel, error)
}

// runtimeAPIImpl creates a RuntimeAPI with the given dependencies.
// It implements the RuntimeAPI interface, exposing only the ModelManager.
type runtimeAPIImpl struct {
	modelManager ModelManager
}

var _ RuntimeAPI = (*runtimeAPIImpl)(nil)

// NewRuntimeAPI creates a RuntimeAPI with the given ModelManager.
// modelManager may be nil if the LLM module is not available.
func NewRuntimeAPI(modelManager ModelManager) RuntimeAPI {
	return &runtimeAPIImpl{modelManager: modelManager}
}

func (r runtimeAPIImpl) ModelManager() ModelManager {
	return r.modelManager
}

// PluginAPI is the registration interface given to plugins during Init().
// Through this API, plugins register their capabilities: Tool, CLI, Hook, Service.
//
// This corresponds to OpenClaw's OpenClawPluginApi with
// registerTool(), registerCli(), registerHook/on(), registerService().
type PluginAPI interface {
	// RegisterTool registers an Agent-callable tool.
	RegisterTool(tool ToolDefinition)

	// RegisterCLI registers a CLI subcommand registrar.
	RegisterCLI(registrar CLIRegistrar)

	// RegisterHook registers a lifecycle event hook.
	RegisterHook(event HookEvent, handler HookHandler)

	// RegisterService registers a background service with Start/Stop lifecycle.
	RegisterService(svc ServiceDefinition)
}

// pluginAPIImpl implements PluginAPI, collecting registrations into the Registry.
// This follows the K8s pattern where framework internals implement the
// public interfaces, keeping the implementation private.
type pluginAPIImpl struct {
	registry   *Registry
	pluginName string
}

var _ PluginAPI = (*pluginAPIImpl)(nil)

func newPluginAPI(registry *Registry, pluginName string) *pluginAPIImpl {
	return &pluginAPIImpl{
		registry:   registry,
		pluginName: pluginName,
	}
}

func (a *pluginAPIImpl) RegisterTool(tool ToolDefinition) {
	a.registry.addTool(a.pluginName, tool)
}

func (a *pluginAPIImpl) RegisterCLI(registrar CLIRegistrar) {
	a.registry.addCLI(a.pluginName, registrar)
}

func (a *pluginAPIImpl) RegisterHook(event HookEvent, handler HookHandler) {
	a.registry.addHook(a.pluginName, event, handler)
}

func (a *pluginAPIImpl) RegisterService(svc ServiceDefinition) {
	a.registry.addService(a.pluginName, svc)
}

// handleImpl implements Handle, providing plugins access to runtime resources.
type handleImpl struct {
	runtimeAPI RuntimeAPI
}

var _ Handle = (*handleImpl)(nil)

func newHandle(runtimeAPI RuntimeAPI) *handleImpl {
	return &handleImpl{runtimeAPI: runtimeAPI}
}

func (h *handleImpl) RuntimeAPI() RuntimeAPI {
	return h.runtimeAPI
}

// FireHooks fires all registered hooks for the given event.
// Hooks are called in registration order. If any hook returns an error,
// subsequent hooks are still called but the first error is returned.
func FireHooks(ctx context.Context, registry *Registry, event HookEvent, data interface{}) error {
	handlers := registry.GetHooks(event)
	var firstErr error
	for _, h := range handlers {
		if err := h(ctx, data); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
