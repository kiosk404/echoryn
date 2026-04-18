package memory_core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	agentEntity "github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory/memory-core/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory/memory-core/manager"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "memory-core"

	// Kind groups this plugin under the "memory" slot.
	Kind = "memory"

	// minFlushMessages is the minimum number of messages in a session
	// before memory flush is triggered.
	minFlushMessages = 4

	// memoryFlushPrompt is the user-facing prompt sent to the LLM during
	// pre-compaction memory flush (aligned with OpenClaw's DEFAULT_MEMORY_FLUSH_PROMPT).
	memoryFlushPrompt = "Pre-compaction memory flush. " +
		"Store durable memories now (use memory/YYYY-MM-DD.md; create memory/ if needed). " +
		"IMPORTANT: If the file already exists, APPEND new content only and do not overwrite existing entries. " +
		"Focus on: user preferences, key decisions, important facts, technical choices, and action items. " +
		"If nothing worth storing, reply with exactly: NOTHING_TO_STORE"

	// memoryFlushSystemPrompt is the system prompt for the memory flush LLM call.
	memoryFlushSystemPrompt = "Pre-compaction memory flush turn. " +
		"The session is near auto-compaction; capture durable memories to disk. " +
		"You have access to memory_write tool to write memories. " +
		"Write concise, structured Markdown entries. Usually reply NOTHING_TO_STORE."

	// nothingToStoreToken is the sentinel reply indicating no memories need to be stored.
	nothingToStoreToken = "NOTHING_TO_STORE"

	// memoryRecallInstruction is the system-level instruction injected
	// before agent start to guide the agent to use memory tools.
	memoryRecallInstruction = `## Memory System
You have access to a persistent memory system. Follow these guidelines:
- Before answering questions about past conversations, user preferences, or previously discussed topics, use the memory_search tool to recall relevant information.
- When you learn important facts, decisions, user preferences, or actionable information during a conversation, use the memory_write tool to save them for future reference.
- Use memory_delete to remove outdated or incorrect memories when appropriate.
- Memory files are organized as Markdown under the memory/ directory.`

	// sessionMemorySystemPrompt is the system prompt for post-turn memory extraction.
	// Differentiated from pre-compaction flush: focuses on incremental summary
	// rather than comprehensive dump before compression.
	sessionMemorySystemPrompt = "You are a session memory extractor. Your job is to identify and summarize " +
		"the most important information from recent conversation turns that should be remembered for future sessions. " +
		"Focus on: user preferences, decisions made, action items, key facts learned, and technical choices. " +
		"Be concise — output only what is worth persisting. If nothing significant was discussed, reply exactly: NOTHING_TO_STORE"

	// sessionMemoryUserPrompt is the template for the user-facing extraction prompt.
	sessionMemoryUserPrompt = "Recent conversation turns to analyze for durable memories:\n\n%s\n" +
		"Extract key information worth remembering. Reply NOTHING_TO_STORE if nothing significant."
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

	// Session memory tracking fields (post-turn extraction).
	handle    plugin.Handle
	trackerMu sync.RWMutex
	trackers  map[string]*SessionMemoryTracker // sessionID → tracker
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
		cfg:    memCfg,
		handle: handle,
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
		Name: "memory_search",
		Description: "Mandatory recall step: semantically search MEMORY.md + memory/*.md before " +
			"answering questions about prior work, decisions, dates, people, preferences, or todos; " +
			"returns top snippets with path + lines.",
		Parameters: []plugin.ParameterDef{
			{Name: "query", Type: "string", Description: "The search query text", Required: true},
		},
		Handler: p.handleMemorySearch,
		Category: "memory",
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
		Category: "memory",
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
		Category: "memory",	
	})

	// Register memory_delete tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "memory_delete",
		Description: "Delete a memory file and remove it from the search index. Use this to clean up outdated or incorrect memories.",
		Parameters: []plugin.ParameterDef{
			{Name: "path", Type: "string", Description: "Relative file path of the memory file to delete", Required: true},
		},
		Handler: p.handleMemoryDelete,
		Category: "memory",
	})

	// Register lifecycle hooks.
	api.RegisterHook(plugin.HookBeforeAgentStart, p.onBeforeAgentStart)
	api.RegisterHook(plugin.HookBeforeCompaction, p.onBeforeCompaction)

	// Register post-turn hook for automatic session memory extraction.
	api.RegisterAfterTurnHook(p.onAfterTurn)

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

// --- Session Memory Post-Turn Methods ---

// getOrCreateTracker returns the tracker for a session, creating one if needed.
// Thread-safe via double-checked locking.
func (p *memoryCorePlugin) getOrCreateTracker(sessionID string) *SessionMemoryTracker {
	p.trackerMu.RLock()
	if t, ok := p.trackers[sessionID]; ok {
		p.trackerMu.RUnlock()
		return t
	}
	p.trackerMu.RUnlock()

	p.trackerMu.Lock()
	defer p.trackerMu.Unlock()

	// Double-check after acquiring write lock.
	if t, ok := p.trackers[sessionID]; ok {
		return t
	}

	t := NewSessionMemoryTracker()
	if p.trackers == nil {
		p.trackers = make(map[string]*SessionMemoryTracker)
	}
	p.trackers[sessionID] = t
	return t
}

// onAfterTurn implements the PostSamplingHook for automatic session memory extraction.
// Registered via api.RegisterAfterTurnHook during Init().
//
// Trigger conditions (dual-threshold, OR logic):
//   - Turns since last extraction >= TurnThreshold (default 10)
//   - OR cumulative tokens since last extraction >= TokenThreshold (default 8192)
//
// Plus guards: MinMessages check, MinInterval protection.
func (p *memoryCorePlugin) onAfterTurn(ctx context.Context, data plugin.AfterTurnData) error {
	smCfg := p.cfg.SessionMemory
	if !smCfg.Enabled || p.manager == nil {
		return nil
	}

	// 1. Get or create session tracker.
	tracker := p.getOrCreateTracker(data.SessionID)

	// 2. Record this turn.
	tracker.RecordTurn(data.TokensUsed)

	// 3. Check dual threshold + interval guard.
	if !tracker.ShouldExtract(smCfg) {
		return nil
	}

	// 4. Guard: need enough messages to make extraction meaningful.
	if len(data.Messages) < smCfg.MinMessages {
		logger.Debug("[MemoryCore] session memory skipped: insufficient messages (%d < %d)",
			len(data.Messages), smCfg.MinMessages)
		return nil
	}

	logger.Info("[MemoryCore] session memory extraction triggered: session=%s, turns=%d, tokens=%d",
		data.SessionID, tracker.TurnCount(), tracker.TokenCount())

	// 5. Execute LLM-driven extraction.
	if err := p.llmExtractMemories(ctx, data.SessionID, data.Messages); err != nil {
		logger.Warn("[MemoryCore] session memory extraction failed: %v", err)
		// Don't reset tracker on failure — retry next turn
		return nil
	}

	// 6. Reset tracker for next cycle.
	tracker.Reset()
	return nil
}

// llmExtractMemories performs LLM-driven memory extraction for a session.
func (p *memoryCorePlugin) llmExtractMemories(
	ctx context.Context,
	sessionID string,
	messages []*agentEntity.Message,
) error {
	chatModel, err := p.getChatModel(ctx)
	if err != nil {
		return fmt.Errorf("no chat model available: %w", err)
	}

	// Build conversation context from messages passed in AfterTurnData.
	var convCtx strings.Builder
	convCtx.WriteString("Recent conversation:\n\n")
	for _, msg := range messages {
		role := string(msg.Role)
		content := msg.Content
		if len([]rune(content)) > 500 {
			runes := []rune(content)
			content = string(runes[:400]) + "\n...[truncated]..."
		}
		convCtx.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	userPrompt := fmt.Sprintf(sessionMemoryUserPrompt, convCtx.String())

	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: sessionMemorySystemPrompt},
		{Role: schema.User, Content: userPrompt},
	})
	if err != nil {
		return fmt.Errorf("LLM extraction call failed: %w", err)
	}

	responseText := strings.TrimSpace(resp.Content)

	// Check sentinel.
	if strings.Contains(strings.ToUpper(responseText), nothingToStoreToken) {
		logger.Info("[MemoryCore] session memory: nothing to store")
		return nil
	}

	// Write extracted memories.
	now := time.Now()
	datePath := fmt.Sprintf("memory/%s.md", now.Format("2006-01-02"))
	entry := fmt.Sprintf("\n## %s (session memory)\n\n%s\n",
		now.Format("15:04:05"),
		responseText,
	)

	if err := p.manager.WriteMemory(ctx, datePath, entry, true); err != nil {
		return fmt.Errorf("write memory failed: %w", err)
	}

	logger.Info("[MemoryCore] session memory: stored to %s", datePath)
	return nil
}

// getChatModel retrieves the default chat model from the runtime API.
func (p *memoryCorePlugin) getChatModel(ctx context.Context) (einoModel.BaseChatModel, error) {
	if p.handle == nil {
		return nil, fmt.Errorf("no handle available")
	}
	runtimeAPI := p.handle.RuntimeAPI()
	if runtimeAPI == nil {
		return nil, fmt.Errorf("no runtime API available")
	}
	mm := runtimeAPI.ModelManager()
	if mm == nil {
		return nil, fmt.Errorf("no model manager available")
	}
	return mm.GetDefaultChatModel(ctx)
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

	// Return an explicit 'no result' message so the LLM knows to inform the
	// user instead of silently ending the turn with no follow-up text.
	if len(results) == 0 {
		return map[string]interface{}{
			"results:": []interface{}{},
			"message": "No matching memories found, the memory store is empty or has no relevant entries for this query" +
				"Tell the user you checked but found nothing.",
		}, nil
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

	// Legacy path: inject memory recall instruction as a user message.
	hookData, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}

	msg := agentEntity.NewUserMessage(memoryRecallInstruction)
	var injected []*agentEntity.Message

	// Preserve existing injected messages from other hooks.
	if existing, ok := hookData["injected_messages"].([]*agentEntity.Message); ok {
		injected = existing
	}
	injected = append(injected, msg)
	hookData["injected_messages"] = injected

	logger.Debug("[MemoryCore] injected memory recall instruction")

	return nil
}

// onBeforeCompaction performs an LLM-driven memory flush before compaction.
// This is the Echoryn equivalent of OpenClaw's pre-compaction memory flush:
//   - Triggered by the before_compaction hook (fired by the Compactor)
//   - Uses the ChatModel to let the LLM decide what memories to persist
//   - The LLM can call memory_write to store durable memories
//   - Frequency control: only once per compaction cycle (via session.ShouldMemoryFlush)
//
// Data expected: {"agent", "session", "chat_model"}
func (p *memoryCorePlugin) onBeforeCompaction(ctx context.Context, data interface{}) error {
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

	// Frequency control: only flush once per compaction cycle.
	if !session.ShouldMemoryFlush() {
		logger.Debug("[MemoryCore] memory flush skipped: already flushed in this compaction cycle")
		return nil
	}

	chatModel, _ := hookData["chat_model"].(einoModel.BaseChatModel)
	if chatModel == nil {
		logger.Debug("[MemoryCore] memory flush skipped: no chat model available, falling back to lightweight flush")
		p.lightweightFlush(ctx, session)
		return nil
	}

	// LLM-driven flush: let the model decide what to store.
	logger.Info("[MemoryCore] running LLM-driven pre-compaction memory flush...")

	// Build conversation context for the LLM to analyze.
	activeMessages := session.ActiveMessages()
	var conversationCtx strings.Builder
	conversationCtx.WriteString("Current conversation context to analyze for durable memories:\n\n")
	for _, msg := range activeMessages {
		role := string(msg.Role)
		content := msg.Content
		if len([]rune(content)) > 500 {
			runes := []rune(content)
			content = string(runes[:400]) + "\n...[truncated]..."
		}
		conversationCtx.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	userPrompt := memoryFlushPrompt + "\n\n" + conversationCtx.String()

	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: memoryFlushSystemPrompt},
		{Role: schema.User, Content: userPrompt},
	})
	if err != nil {
		logger.Warn("[MemoryCore] LLM-driven memory flush failed: %v, falling back to lightweight flush", err)
		p.lightweightFlush(ctx, session)
		session.RecordMemoryFlush()
		return nil
	}

	responseText := strings.TrimSpace(resp.Content)

	// Check if the LLM decided nothing needs to be stored.
	if strings.Contains(strings.ToUpper(responseText), nothingToStoreToken) {
		logger.Info("[MemoryCore] LLM-driven flush: nothing to store")
		session.RecordMemoryFlush()
		return nil
	}

	// The LLM returned content to store — write it as a memory entry.
	now := time.Now()
	datePath := fmt.Sprintf("memory/%s.md", now.Format("2006-01-02"))

	entry := fmt.Sprintf("\n## %s (pre-compaction flush)\n\n%s\n",
		now.Format("15:04:05"),
		responseText,
	)

	if err := p.manager.WriteMemory(ctx, datePath, entry, true); err != nil {
		logger.Warn("[MemoryCore] LLM-driven memory flush write failed: %v", err)
		return nil
	}

	session.RecordMemoryFlush()
	logger.Info("[MemoryCore] LLM-driven memory flush: stored memories to %s", datePath)
	return nil
}

// lightweightFlush is the fallback flush when no ChatModel is available.
// It mechanically extracts the last user+assistant pair (legacy behavior).
func (p *memoryCorePlugin) lightweightFlush(ctx context.Context, session *agentEntity.Session) {
	activeMessages := session.ActiveMessages()
	if len(activeMessages) < minFlushMessages {
		return
	}

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
		return
	}

	now := time.Now()
	datePath := fmt.Sprintf("memory/%s.md", now.Format("2006-01-02"))
	userSnippet := truncate(lastUserMsg, 200)
	assistantSnippet := truncate(lastAssistantMsg, 400)

	entry := fmt.Sprintf("\n## %s\n\n- **User**: %s\n- **Assistant**: %s\n",
		now.Format("15:04:05"),
		userSnippet,
		assistantSnippet,
	)

	if err := p.manager.WriteMemory(ctx, datePath, entry, true); err != nil {
		logger.Warn("[MemoryCore] lightweight memory flush failed: %v", err)
		return
	}

	logger.Info("[MemoryCore] lightweight memory flush: appended to %s", datePath)
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

// Enabled returns true when the memory manager is initialized
// Unlike the previous implementation that required ChunkCount > 0,
// we now enable the section as long as the manager exists (aligned with OpenClaw)
// so the Agent always sees the recall instruction even for a fresh memory store.
func (s *MemorySection) Enabled(_ context.Context, _ *prompt.PromptContext) bool {
	return s.plugin.manager != nil
}

// Render returns the memory recall instruction text.
// Aligned with OpenClaw's imperative "Memory Recall" prompt style
func (s *MemorySection) Render(_ context.Context, _ *prompt.PromptContext) (string, error) {
	if s.plugin.manager == nil {
		return "", nil
	}

	status := s.plugin.manager.Status()
	if status.ChunkCount == 0 {
		return "", nil
	}

	// Enhanced instruction with index stats for Agent awareness.
	return `## Memory Recall
Before answering anything about prior work, decisions, dates, people, preferences, or todos: 
run memory_search on MEMORY.md + memory/*.md; then use memory_read to pull only the needed lines. 
If low confidence after search, say you checked.`, nil
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
