package memory_core

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentEntity "github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/manager"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "memory-core"

	// Kind groups this plugin under the "memory" slot.
	Kind = "memory"

	// minFlushMessages is the minimum number of messages in a session
	// before memory flush is triggered on agent_end.
	minFlushMessages = 4

	// memoryRecallInstruction is the system-level instruction injected
	// before agent start to guide the agent to use memory tools.
	memoryRecallInstruction = `## Memory System
You have access to a persistent memory system. Follow these guidelines:
- Before answering questions about past conversations, user preferences, or previously discussed topics, use the memory_search tool to recall relevant information.
- When you learn important facts, decisions, user preferences, or actionable information during a conversation, use the memory_write tool to save them for future reference.
- Use memory_delete to remove outdated or incorrect memories when appropriate.
- Memory files are organized as Markdown under the memory/ directory.`
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Memory Core",
		Kind:        Kind,
		Description: "Default memory system using SQLite + hybrid vector/keyword search",
	}
}

// Args holds the configuration for the memory-core plugin.
type Args struct {
	MemoryConfig *entity.MemoryConfig
}

// memoryCorePlugin is the runtime instance of the memory-core plugin.
type memoryCorePlugin struct {
	cfg                  *entity.MemoryConfig
	manager              *manager.Manager
	promptPipelineActive bool // set to true when PromptSections() is a called by the agent
}

// Factory is the PluginFactory for memory-core.
// It follows the K8s pattern: factory creates a plugin from args + handle.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	// Extract memory config from args.
	cfgRaw, ok := args["config"]
	if !ok {
		return nil, fmt.Errorf("memory-core: missing 'config' in plugin args")
	}
	memCfg, ok := cfgRaw.(*entity.MemoryConfig)
	if !ok {
		return nil, fmt.Errorf("memory-core: 'config' must be *entity.MemoryConfig, got %T", cfgRaw)
	}

	return &memoryCorePlugin{
		cfg: memCfg,
	}, nil
}

// Name implements plugin.Plugin.
func (p *memoryCorePlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
// Registers tools and hooks via the PluginAPI.
func (p *memoryCorePlugin) Init(api plugin.PluginAPI) error {
	// Register memory_search tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "memory_search",
		Description: "Search memory files using hybrid vector + keyword search. Returns relevant code/text snippets from indexed memory files.",
		Parameters: []plugin.ParameterDef{
			{Name: "query", Type: "string", Description: "The search query text", Required: true},
		},
		Handler: p.handleMemorySearch,
	})

	// Register memory_read tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "memory_read",
		Description: "Read specific lines from a memory file. Returns the file content within the specified line range.",
		Parameters: []plugin.ParameterDef{
			{Name: "path", Type: "string", Description: "Relative file path within workspace", Required: true},
			{Name: "from", Type: "number", Description: "Start line number (1-based, default: 1)", Required: false},
			{Name: "lines", Type: "number", Description: "Number of lines to read (default: all)", Required: false},
		},
		Handler: p.handleMemoryRead,
	})

	// Register memory_write tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "memory_write",
		Description: "Write or append content to a memory file. Use this to save important information, decisions, user preferences, and key facts for future reference. Files are Markdown format under the memory/ directory.",
		Parameters: []plugin.ParameterDef{
			{Name: "path", Type: "string", Description: "Relative file path within workspace (e.g., 'memory/2026-02-13.md'). Must be under the memory/ directory.", Required: true},
			{Name: "content", Type: "string", Description: "The Markdown content to write", Required: true},
			{Name: "append", Type: "boolean", Description: "If true, append to existing file instead of overwriting (default: true)", Required: false},
		},
		Handler: p.handleMemoryWrite,
	})

	// Register memory_delete tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "memory_delete",
		Description: "Delete a memory file and remove it from the search index. Use this to clean up outdated or incorrect memories.",
		Parameters: []plugin.ParameterDef{
			{Name: "path", Type: "string", Description: "Relative file path of the memory file to delete", Required: true},
		},
		Handler: p.handleMemoryDelete,
	})

	// Register lifecycle hooks.
	api.RegisterHook(plugin.HookBeforeAgentStart, p.onBeforeAgentStart)
	api.RegisterHook(plugin.HookAgentEnd, p.onAgentEnd)

	return nil
}

// Start implements plugin.LifecyclePlugin.
// Initializes the memory manager and performs initial sync.
func (p *memoryCorePlugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		logger.Info("[MemoryCore] memory system is disabled")
		return nil
	}

	logger.Info("[MemoryCore] starting memory-core plugin...")

	m, err := manager.Get(ctx, p.cfg)
	if err != nil {
		return fmt.Errorf("failed to create memory manager: %w", err)
	}
	p.manager = m

	// Initial sync.
	if err := m.Sync(ctx, manager.SyncOpts{Reason: "plugin-start"}); err != nil {
		logger.Warn("[MemoryCore] initial sync failed: %v", err)
		// Non-fatal.
	}

	status := m.Status()
	logger.Info("[MemoryCore] started (provider=%s, model=%s, files=%d, chunks=%d, fts=%v)",
		status.Provider, status.Model, status.FileCount, status.ChunkCount, status.FTSAvailable)

	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *memoryCorePlugin) Stop(ctx context.Context) error {
	if p.manager != nil {
		logger.Info("[MemoryCore] stopping memory-core plugin...")
		return p.manager.Close()
	}
	return nil
}

// --- Tool Handlers ---

func (p *memoryCorePlugin) handleMemorySearch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("memory system is not initialized")
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("parameter 'query' is required and must be a string")
	}

	results, err := p.manager.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("memory search failed: %w", err)
	}

	return results, nil
}

func (p *memoryCorePlugin) handleMemoryRead(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("memory system is not initialized")
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("parameter 'path' is required and must be a string")
	}

	from := 0
	if v, ok := params["from"]; ok {
		if f, ok := v.(float64); ok {
			from = int(f)
		}
	}

	lines := 0
	if v, ok := params["lines"]; ok {
		if l, ok := v.(float64); ok {
			lines = int(l)
		}
	}

	content, err := p.manager.ReadFile(path, from, lines)
	if err != nil {
		return nil, fmt.Errorf("memory read failed: %w", err)
	}

	return map[string]interface{}{
		"path":    path,
		"content": content,
	}, nil
}

func (p *memoryCorePlugin) handleMemoryWrite(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("memory system is not initialized")
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("parameter 'path' is required and must be a string")
	}

	content, ok := params["content"].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("parameter 'content' is required and must be a non-empty string")
	}

	// Default to append mode.
	appendMode := true
	if v, ok := params["append"]; ok {
		if b, ok := v.(bool); ok {
			appendMode = b
		}
	}

	if err := p.manager.WriteMemory(ctx, path, content, appendMode); err != nil {
		return nil, fmt.Errorf("memory write failed: %w", err)
	}

	return map[string]interface{}{
		"path":   path,
		"status": "written",
		"mode":   modeLabel(appendMode),
	}, nil
}

func (p *memoryCorePlugin) handleMemoryDelete(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("memory system is not initialized")
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("parameter 'path' is required and must be a string")
	}

	if err := p.manager.DeleteMemory(path); err != nil {
		return nil, fmt.Errorf("memory delete failed: %w", err)
	}

	return map[string]interface{}{
		"path":   path,
		"status": "deleted",
	}, nil
}

// --- Hook Handlers ---
// onBeforeAgentStart syncs memory before each agent session.
func (p *memoryCorePlugin) onBeforeAgentStart(ctx context.Context, data interface{}) error {
	if p.manager == nil {
		return nil
	}

	// Sync before agent starts to ensure latest memory is available.
	if err := p.manager.Sync(ctx, manager.SyncOpts{Reason: "before-agent-start"}); err != nil {
		logger.Warn("[MemoryCore] sync before agent start failed: %v", err)
	}

	// When PromptPipeline is active, MemorySection handles the instruction injection.
	// Skip the legacy hook-based injection to avoid duplication.
	// We detect this by checking if promptPipelineActive is set (set during Init
	// when the framework has a PromptPipeline).
	if p.promptPipelineActive {
		logger.Debug("[MemoryCore] PromptPipeline active, skipping legacy hook injection")
		return nil
	}

	// Legacy path: inject memory recall instruction as a system message.
	hookData, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}

	status := p.manager.Status()
	if status.ChunkCount > 0 {
		msg := agentEntity.NewSystemMessage(memoryRecallInstruction)
		var injected []*agentEntity.Message

		// Preserve existing injected messages from other hooks.
		if existing, ok := hookData["injected_messages"].([]*agentEntity.Message); ok {
			injected = existing
		}
		injected = append(injected, msg)
		hookData["injected_messages"] = injected

		logger.Debug("[MemoryCore] injected memory recall instruction (chunks=%d)", status.ChunkCount)
	}

	return nil
}

// onAgentEnd extracts key information from the conversation and persists it to memory.
func (p *memoryCorePlugin) onAgentEnd(ctx context.Context, data interface{}) error {
	if p.manager == nil {
		return nil
	}

	hookData, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}

	session, _ := hookData["session"].(*agentEntity.Session)
	if session == nil {
		return nil
	}

	// Only flush if the conversation is substantial enough.
	activeMessages := session.ActiveMessages()
	if len(activeMessages) < minFlushMessages {
		return nil
	}

	// Extract the latest turn's messages (last user + assistant pair).
	var lastUserMsg, lastAssistantMsg string
	for i := len(activeMessages) - 1; i >= 0; i-- {
		msg := activeMessages[i]
		if msg.Role == agentEntity.RoleAssistant && lastAssistantMsg == "" {
			lastAssistantMsg = msg.Content
		}
		if msg.Role == agentEntity.RoleUser && lastUserMsg == "" {
			lastUserMsg = msg.Content
		}
		if lastUserMsg != "" && lastAssistantMsg != "" {
			break
		}
	}

	if lastUserMsg == "" || lastAssistantMsg == "" {
		return nil
	}

	// Build a simple summary entry (no LLM call — lightweight approach).
	// Format: timestamped bullet points of user query + assistant key response.
	now := time.Now()
	datePath := fmt.Sprintf("memory/%s.md", now.Format("2006-01-02"))

	// Truncate long messages for the summary.
	userSnippet := truncate(lastUserMsg, 200)
	assistantSnippet := truncate(lastAssistantMsg, 400)

	entry := fmt.Sprintf("\n## %s\n\n- **User**: %s\n- **Assistant**: %s\n",
		now.Format("15:04:05"),
		userSnippet,
		assistantSnippet,
	)

	if err := p.manager.WriteMemory(ctx, datePath, entry, true); err != nil {
		logger.Warn("[MemoryCore] memory flush failed: %v", err)
		return nil // Non-fatal.
	}

	logger.Info("[MemoryCore] memory flush: appended conversation entry to %s", datePath)
	return nil
}

// --- PromptProvider Implementation ---

// PromptSections implements plugin.PromptProvider.
// Returns a MemorySection that injects memory recall instructions into the
// system prompt via the PromptPipeline, replacing the legacy hook-based injection.
//
// The MemorySection (Priority: 400) is placed after PersonaSection (300) and
// before RuntimeSection (900), giving the Agent memory-aware instructions
// as a first-class part of the system prompt.
func (p *memoryCorePlugin) PromptSections() []prompt.PromptSection {
	// Mark that the PromptPipeline is active — the framework called this method,
	// meaning MemorySection will be registered. The legacy hook injection can be skipped.
	p.promptPipelineActive = true
	return []prompt.PromptSection{
		&MemorySection{plugin: p},
	}
}

// MemorySection is a PromptSection that injects memory system instructions
// into the system prompt when the memory index has content.
//
// Priority 400: after Persona (300), before Runtime (900).
// This is the P1 replacement for the hook-based injection approach.
type MemorySection struct {
	plugin *memoryCorePlugin
}

func (s *MemorySection) Name() string  { return "memory" }
func (s *MemorySection) Priority() int { return 400 }

// Enabled returns true when the memory manager is initialized and has indexed content.
func (s *MemorySection) Enabled(_ context.Context, _ *prompt.PromptContext) bool {
	if s.plugin.manager == nil {
		return false
	}
	status := s.plugin.manager.Status()
	return status.ChunkCount > 0
}

// Render returns the memory recall instruction text.
func (s *MemorySection) Render(_ context.Context, _ *prompt.PromptContext) (string, error) {
	if s.plugin.manager == nil {
		return "", nil
	}

	status := s.plugin.manager.Status()
	if status.ChunkCount == 0 {
		return "", nil
	}

	// Enhanced instruction with index stats for Agent awareness.
	return fmt.Sprintf(`## Memory System

You have access to a persistent memory system with %d indexed files and %d content chunks.

Follow these guidelines:
- Before answering questions about past conversations, user preferences, or previously discussed topics, use the **memory_search** tool to recall relevant information.
- When you learn important facts, decisions, user preferences, or actionable information during a conversation, use the **memory_write** tool to save them for future reference.
- Use **memory_delete** to remove outdated or incorrect memories when appropriate.
- Memory files are organized as Markdown under the memory/ directory.`, status.FileCount, status.ChunkCount), nil
}

// --- Manager Access (for testing/diagnostics) ---

// Manager returns the underlying memory manager.
// This is exposed for diagnostics/status queries, not for general use.
func (p *memoryCorePlugin) Manager() *manager.Manager {
	return p.manager
}

// --- Helpers ---

func modeLabel(appendMode bool) string {
	if appendMode {
		return "append"
	}
	return "overwrite"
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*memoryCorePlugin)(nil)
	_ plugin.InitPlugin      = (*memoryCorePlugin)(nil)
	_ plugin.LifecyclePlugin = (*memoryCorePlugin)(nil)
)
