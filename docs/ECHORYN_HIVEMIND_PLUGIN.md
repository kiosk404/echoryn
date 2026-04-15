# Echoryn Hivemind — 插件框架 (Plugin Framework) 详解

> 本文档是 `ECHORYN_SPEC.md` 的子文档，深入阐述 Hivemind 中 **Plugin Framework** 的完整实现逻辑。
>
> 代码位置: `internal/hivemind/service/plugin/`

---

## 一、模块概述

Plugin Framework 是 Echoryn 的编译时插件系统，为 Hivemind 提供模块化的能力扩展机制。它借鉴了 K8s Scheduler 的插件架构，实现了 **5 种能力注入**、**Slot 互斥** 和 **完整生命周期管理**。

核心特点：
1. **编译时安全** — Go 接口约束，非运行时动态加载
2. **Slot 互斥** — 同一功能槽只允许一个插件（如 "memory" slot）
3. **5 种能力注入** — Tool / Hook / Service / CLI / PromptProvider
4. **Interface Probe** — 自动探测插件实现的可选接口
5. **9 个内置插件** — memory-core / diagnostics / llm-task / subagent / golem-cluster / skills / web-search / channel-feishu / channel-telegram

---

## 二、架构概览

```
Plugin Framework (Config → Complete → New)
  │
  ├── 核心框架
  │   ├─ types.go           # Plugin/InitPlugin/LifecyclePlugin 基础接口
  │   ├─ framework.go       # Framework 核心 (注册→Slot→Init→Start→Stop)
  │   ├─ registry.go        # Registry 中央注册表 (线程安全)
  │   ├─ slots.go           # Slot 互斥机制
  │   ├─ api.go             # PluginAPI + RuntimeAPI + FireHooks
  │   ├─ status.go          # 状态码 (K8s scheduler 风格)
  │   ├─ intree.go          # InTreeRegistry (in-tree 注册)
  │   └─ prompt_provider.go # PromptProvider/MutatorProvider 接口
  │
  ├── 能力注入接口
  │   ├─ tools.go           # ToolProvider + ToolDefinition
  │   ├─ hooks.go           # HookProvider + HookHandler (8 种事件)
  │   ├─ services.go        # ServiceProvider + ServiceDefinition
  │   ├─ cli.go             # CLIProvider + CLIRegistrar
  │   └─ prompt_provider.go # PromptProvider + PromptMutatorProvider
  │
  └── 内置插件 (builtin/)
      ├─ registry.go        # InTree 注册 9 个插件
      ├─ memory-core/       # ★ 记忆系统 (详见 ECHORYN_HIVEMIND_MEMORY_SPEC.md)
      ├─ diagnostics/       # 诊断 + 可观测性 (OTEL)
      ├─ llmtask/           # 通用 JSON LLM 工具
      ├─ subagent/          # 子智能体管理
      ├─ golem-cluster/     # Golem 分布式集群
      ├─ skills/            # 技能加载框架
      ├─ web-search/        # Web 搜索 (Gemini Grounding)
      └─ channel/           # IM 渠道集成
          ├─ feishu/        #   飞书 Bot
          └─ telegram/      #   Telegram Bot
```

---

## 三、核心接口体系

### 3.1 基础接口 (3 层)

```go
// Layer 1: 最小接口 — 所有插件必须实现
type Plugin interface {
    Name() string
}

// Layer 2: 初始化接口 — 可选，在 Init 阶段注册能力
type InitPlugin interface {
    Plugin
    Init(api PluginAPI) error
}

// Layer 3: 生命周期接口 — 可选，支持 Start/Stop
type LifecyclePlugin interface {
    Plugin
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### 3.2 工厂模式

```go
type PluginFactory = func(args PluginArgs, handle Handle) (Plugin, error)
type PluginArgs = map[string]interface{}

type Handle interface {
    RuntimeAPI() RuntimeAPI
}
```

### 3.3 元数据定义

```go
type Definition struct {
    ID          string  // 唯一标识 (如 "memory-core")
    Name        string  // 显示名
    Kind        string  // 功能槽类型 (如 "memory")
    Description string
}
```

---

## 四、5 种能力注入

### 4.1 Tool — LLM 可调用工具

```go
type ToolDefinition struct {
    Name        string           // 工具名 (全局唯一)
    Description string           // LLM 可见的描述
    Parameters  []ParameterDef   // 参数定义
    Handler     ToolHandler      // 执行函数
}

type ToolHandler func(ctx context.Context, params map[string]interface{}) (interface{}, error)

// 声明式接口 (可选，自动探测)
type ToolProvider interface {
    Tools() []ToolDefinition
}
```

### 4.2 Hook — 生命周期钩子

```go
type HookEvent string

const (
    HookServerStart      HookEvent = "server_start"
    HookServerStop       HookEvent = "server_stop"
    HookBeforeAgentStart HookEvent = "before_agent_start"
    HookAgentEnd         HookEvent = "agent_end"
    HookBeforeGenerate   HookEvent = "before_generate"
    HookAfterGenerate    HookEvent = "after_generate"
    HookBeforeCompaction HookEvent = "before_compaction"
    HookAfterCompaction  HookEvent = "after_compaction"
)

type HookHandler func(ctx context.Context, data interface{}) error

// 声明式接口 (可选，自动探测)
type HookProvider interface {
    Hooks() map[HookEvent]HookHandler
}
```

### 4.3 Service — 后台服务

```go
type ServiceDefinition struct {
    Name  string
    Start func(ctx context.Context) error
    Stop  func(ctx context.Context) error
}

type ServiceProvider interface {
    Services() []ServiceDefinition
}
```

### 4.4 CLI — 命令行扩展

```go
type CLIRegistrar interface {
    RegisterCommands(parent *cobra.Command)
}

type CLIProvider interface {
    CLIRegistrars() []CLIRegistrar
}
```

### 4.5 Prompt — 系统提示词注入

```go
type PromptProvider interface {
    PromptSections() []prompt.PromptSection
}

type PromptMutatorProvider interface {
    PromptMutators() []prompt.PromptMutator
}
```

此能力允许插件向 Agent 的系统提示词管线注入 Section（如记忆上下文、知识库摘要等）或 Mutator（提示词动态转换）。

---

## 五、Slot 互斥机制

### 5.1 设计理念

Slot 机制确保同一功能类别只有一个插件被激活，避免冲突。例如 "memory" slot 只允许一个 Memory 实现。

### 5.2 配置

```go
type SlotConfig map[string]string  // kind → desired plugin name

// 默认值
var slotDefaults = SlotConfig{
    "memory": "memory-core",  // 默认使用 memory-core 插件
}
```

### 5.3 解析逻辑

```
ResolveSlot(def, activeSlots, config):
  ├─ Kind 为空或 "general" → 直接通过 (不参与 Slot 竞争)
  ├─ Config 中指定 "none" → 禁用所有该 Kind 的插件
  ├─ Config 中指定了其他插件 → 当前插件不匹配 → 跳过
  ├─ Slot 已被占据 → 跳过 (先到先得)
  └─ 通过 → 占据 Slot
```

---

## 六、Framework 核心生命周期

### 6.1 初始化流程

```
Framework.Init():
  │
  for each registeredFactory:
  │
  ├─ 1. Slot 解析
  │     └─ ResolveSlot(def, activeSlots, config) → 通过 or 跳过
  │
  ├─ 2. 实例化
  │     └─ factory(args, handle) → Plugin 实例
  │
  ├─ 3. 注册到 Registry
  │     └─ registry.addPlugin(name, plugin, def)
  │
  ├─ 4. Init 调用 (如果实现 InitPlugin)
  │     └─ plugin.Init(pluginAPI) → 命令式注册 Tool/Hook/CLI/Service
  │
  └─ 5. Interface Probe (自动探测)
        ├─ ToolProvider? → 注册 Tools
        ├─ HookProvider? → 注册 Hooks
        ├─ ServiceProvider? → 注册 Services
        ├─ CLIProvider? → 注册 CLI
        ├─ PromptProvider? → 注册 PromptSections
        └─ PromptMutatorProvider? → 注册 PromptMutators
```

### 6.2 启动流程

```
Framework.Start(ctx):
  ├─ 1. 启动 LifecyclePlugin — 按注册顺序
  ├─ 2. 启动注册的 Service — 按注册顺序
  └─ 3. 触发 HookServerStart — 按注册顺序
```

### 6.3 停止流程

```
Framework.Stop(ctx):
  ├─ 1. 触发 HookServerStop
  ├─ 2. 停止 Service — 反序
  └─ 3. 停止 LifecyclePlugin — 反序
```

---

## 七、Registry — 中央注册表

### 7.1 数据结构

```go
type Registry struct {
    mu           sync.RWMutex
    plugins      map[string]Plugin            // name → Plugin
    pluginOrder  []string                     // 注册顺序
    definitions  map[string]Definition        // name → 元数据
    tools        map[string]ToolDefinition    // toolName → 定义 (全局唯一)
    toolOwners   map[string]string            // toolName → pluginName
    cliRegistrars []CLIRegistrar              // 有序 CLI 列表
    hooks        map[HookEvent][]HookHandler  // event → 有序 handler 列表
    services     []ServiceDefinition          // 有序 Service 列表
    slots        map[string]string            // kind → active pluginName
}
```

### 7.2 线程安全

所有写操作使用写锁 (`Lock`)，查询操作使用读锁 (`RLock`)。

### 7.3 Hook 触发

```go
func FireHooks(ctx context.Context, reg *Registry, event HookEvent, data interface{}) error {
    handlers := reg.GetHooks(event)
    for _, h := range handlers {
        if err := h(ctx, data); err != nil {
            return err  // 首个错误即终止
        }
    }
    return nil
}
```

---

## 八、内置插件详解

Echoryn 随 Hivemind 二进制发行 **9 个内置插件**，在编译时链接，运行时由 Plugin Framework 管理。详见 [PLUGIN_BUILTIN_INVENTORY.md](./PLUGIN_BUILTIN_INVENTORY.md) 获取完整配置和最佳实践。

### 8.1 memory-core (记忆系统) ⭐

> 详见 [ECHORYN_HIVEMIND_MEMORY_SPEC.md](ECHORYN_HIVEMIND_MEMORY.md)

| 属性 | 值 |
|------|---|
| **ID** | `memory-core` |
| **Kind** | `memory` (Slot 独占) |
| **位置** | `internal/hivemind/service/plugin/builtin/memory-core/` |
| **大小** | 19.69 KB |
| **优先级** | P0 (Critical) |
| **接口** | Plugin + InitPlugin + LifecyclePlugin + ToolProvider + HookProvider + PromptProvider |
| **工具** | `memory_search` / `memory_write` / `memory_read` / `memory_delete` |
| **Hook** | `before_agent_start` + `agent_end` |
| **PromptSection** | 记忆上下文注入 (P:400) |

**功能**：提供 Echoryn 的核心记忆系统，支持混合搜索（向量 + FTS5）、快速检索（ANN）、知识蒸馏。

### 8.2 diagnostics (诊断与可观测性)

| 属性 | 值 |
|------|---|
| **ID** | `diagnostics` |
| **Kind** | `observability` (Slot 独占) |
| **位置** | `internal/hivemind/service/plugin/builtin/diagnostics/` |
| **大小** | 11.79 KB |
| **优先级** | P0 (Critical) |
| **接口** | Plugin + InitPlugin + LifecyclePlugin + HookProvider + ServiceProvider |
| **工具** | `diagnostics_status` (系统状态查询) |
| **Hook** | `server_start` + `server_stop` + `before_generate` + `after_generate` |
| **Service** | `diagnostics-collector` (后台指标收集) |

**功能**：集成 **OpenTelemetry (OTEL)** 提供链路追踪、指标收集、日志聚合。支持 Jaeger / Prometheus / OTLP 后端导出。

**关键指标**:
```
agent_run_total、agent_run_duration_ms、agent_toolcall_total
llm_request_total、llm_token_input_total、llm_token_output_total
context_compaction_total、agent_toolloop_detected_total
```

### 8.3 llm-task (JSON-only LLM 工具)

| 属性 | 值 |
|------|---|
| **ID** | `llm-task` |
| **Kind** | `tools` (Slot 多占) |
| **位置** | `internal/hivemind/service/plugin/builtin/llmtask/` |
| **大小** | 8.78 KB |
| **优先级** | P1 |
| **接口** | Plugin + InitPlugin + ToolProvider |
| **工具** | `llm_call` (JSON-only 工具调用) |

**功能**：提供通用 LLM 工具，支持流式响应、内置超时和重试。仅接受 JSON 输入/输出（安全性考虑）。

**工具参数**: `model` / `prompt` / `temperature` / `max_tokens` / `timeout_ms`

### 8.4 subagent (子智能体管理)

| 属性 | 值 |
|------|---|
| **ID** | `subagent` |
| **Kind** | `tools` (Slot 多占) |
| **位置** | `internal/hivemind/service/plugin/builtin/subagent/` |
| **大小** | 12.9 KB |
| **优先级** | P1 |
| **接口** | Plugin + InitPlugin + ToolProvider |
| **工具** | `subagent_spawn` / `subagent_status` / `subagent_wait` / `subagent_cancel` / `subagent_collect` |

**功能**：为 Agent 提供启动和监控子智能体的能力，支持多智能体协作、结果收集和取消操作。

### 8.5 golem-cluster (Golem 分布式集群)

| 属性 | 值 |
|------|---|
| **ID** | `golem-cluster` |
| **Kind** | `tools` (Slot 多占) |
| **位置** | `internal/hivemind/service/plugin/builtin/golem-cluster/` |
| **大小** | 25.9 KB |
| **优先级** | P1 |
| **特殊依赖** | GolemModule (post-init injection) |
| **接口** | Plugin + InitPlugin + ToolProvider |
| **工具** | `golem_list_nodes` / `golem_execute_remote` / `golem_get_cluster_status` |

**功能**：将 Golem 分布式工作节点集群暴露给 Agent，支持节点选择、远程任务执行、集群状态查询。

**配置**: `scheduler_type` (ai-aware / round-robin / least-loaded) / `node_selector_enabled` / 探活间隔 / 任务超时

### 8.6 skills (技能加载框架)

| 属性 | 值                                                      |
|------|--------------------------------------------------------|
| **ID** | `skills`                                               |
| **Kind** | `tools\general` (Slot 多占)                              |
| **位置** | `internal/hivemind/service/plugin/builtin/skills/`     |
| **大小** | 10.26 KB                                               |
| **优先级** | P2                                                     |
| **接口** | Plugin + InitPlugin + LifecyclePlugin + PromptProvider |
| **工具** | `skill_list` / `skill_view`                            |

**功能**：三源加载 Skills (Project > Hivemind > Golem) 在系统提示词中区分两类 Skills 语义：

- **Hivemind Skills** (`~/.echoryn/skills/`): 全局决策知识和工作流指导，帮助 Agent 理解系统能力并规划任务，Agent 通过 `list_skills` / `view_skills` 加载后，参考其中的指令和工作流决策，具体执行方式由 Skill 内容指导（可能是调用工具、编排 Golem、或直接 LLM 推理）
- **Golem Skills** (`~/.echoryn/golem/skills`): 本地执行能力，描述特定 Golem 节点可直接执行的任务。由 Golem 上报，驱动 Scheduler 节点选择，Agent 通过 `cluster_execute_skill` / `cluster_dispatch_task` 调度执行


### 8.7 web-search (Web 搜索)

| 属性 | 值 |
|------|---|
| **ID** | `web-search` |
| **Kind** | `tools` (Slot 多占) |
| **位置** | `internal/hivemind/service/plugin/builtin/web-search/gemini-web-search/` |
| **大小** | 3.59 KB |
| **优先级** | P2 |
| **接口** | Plugin + InitPlugin + ToolProvider |
| **工具** | `web_search` (通过 Gemini Grounding 实时搜索) |

**功能**：通过 **Gemini Grounding** 进行 Web 搜索，提供实时信息检索能力（需 Gemini 2.0+）。

**配置**: `gemini_api_key` / `search_results_limit` / `cache_ttl_minutes`

### 8.8 channel-feishu (飞书 IM 集成)

| 属性 | 值 |
|------|---|
| **ID** | `channel-feishu` |
| **Kind** | `channel` (Slot 多占) |
| **位置** | `internal/hivemind/service/plugin/builtin/channel/feishu/` |
| **大小** | 20.04 KB |
| **优先级** | P3 |
| **接口** | Plugin + InitPlugin + LifecyclePlugin + ChannelProvider |
| **Webhook** | `/feishu/webhook` |

**功能**：集成飞书（Lark）作为 Agent 的 IM 渠道。支持文本、卡片、交互式消息。

**配置**: `app_id` / `app_secret` / `bot_name` / `message_type` / `webhook_url`

### 8.9 channel-telegram (Telegram 机器人)

| 属性 | 值 |
|------|---|
| **ID** | `channel-telegram` |
| **Kind** | `channel` (Slot 多占) |
| **位置** | `internal/hivemind/service/plugin/builtin/channel/telegram/` |
| **大小** | 9.04 KB |
| **优先级** | P3 |
| **接口** | Plugin + InitPlugin + LifecyclePlugin + ChannelProvider |
| **Webhook** | `/telegram/webhook` |

**功能**：集成 Telegram Bot 作为 Agent 的 IM 渠道。支持 `/chat` 命令触发任务执行。

**配置**: `bot_token` / `webhook_url` / `allowed_user_ids` / `message_timeout_sec`

---

## 九、PluginAPI — 命令式注册

```go
type PluginAPI interface {
    RegisterTool(def ToolDefinition) error
    RegisterCLI(registrar CLIRegistrar)
    RegisterHook(event HookEvent, handler HookHandler)
    RegisterService(def ServiceDefinition)
}
```

插件在 `Init(api)` 阶段通过此 API 命令式注册能力。与声明式接口探测 (ToolProvider/HookProvider) 互补：
- **命令式**: 适合条件注册（根据配置决定是否注册某工具）
- **声明式**: 适合无条件注册（简化代码）

---

## 十、与 OpenClaw 对比

### 10.1 插件系统对比

| 维度 | Echoryn | OpenClaw |
|------|---------|----------|
| **语言** | Go (编译时) | TypeScript (运行时) |
| **加载方式** | In-Tree 编译注册 | npm/archive/path 动态加载 |
| **能力注入** | 5 种 (Tool/Hook/Service/CLI/Prompt) | 10+ 种 (含 HttpHandler/Channel/Gateway/Provider/Command) |
| **Hook 种类** | 8 种 | 13 种 |
| **Slot 互斥** | ✅ 完全对齐 | ✅ (相同机制) |
| **Slot 默认** | `{"memory": "memory-core"}` | `{"memory": "memory-core"}` |
| **Plugin SDK** | Go 接口 (编译时检查) | TypeScript API (运行时注册) |
| **Interface Probe** | ✅ K8s 风格自动探测 | ❌ 无 (显式 register) |
| **PromptProvider** | ✅ Section 注入到 Pipeline | ❌ 无 (Hook 方式) |
| **内置插件数** | 9 个 | 更多 |

### 10.2 对齐项

| 功能 | 状态 | 说明 |
|------|------|------|
| Slot 互斥 | ✅ 完全对齐 | slotDefaults 完全一致 |
| Tool 注册 | ✅ 对齐 | ToolDefinition 结构类似 |
| Hook 系统 | ✅ 大部分对齐 | 8/13 种 Hook |
| 内置插件 | 🟡 部分对齐 | 9 个 vs OpenClaw 更多 |
| 插件配置 | ✅ 对齐 | entries[id].config 结构 |

### 10.3 Hook 事件对齐表

| OpenClaw Hook | Echoryn 对应 | 状态 |
|---------------|-------------|------|
| `server_start` | ✅ `server_start` | 已对齐 |
| `server_stop` | ✅ `server_stop` | 已对齐 |
| `before_agent_start` | ✅ `before_agent_start` | 已对齐 |
| `agent_end` | ✅ `agent_end` | 已对齐 |
| `before_generate` | ✅ `before_generate` | 已对齐 |
| `after_generate` | ✅ `after_generate` | 已对齐 |
| `before_compaction` | ✅ `before_compaction` | **已实现** ✨ |
| `after_compaction` | ✅ `after_compaction` | **已实现** ✨ |
| `message_received` | ❌ 缺失 | |
| `message_sending` | ❌ 缺失 | |
| `message_sent` | ❌ 缺失 | |
| `before_tool_call` | ❌ 缺失 | |
| `after_tool_call` | ❌ 缺失 | |
| `tool_result_persist` | ❌ 缺失 | |
| `session_start` | ❌ 缺失 | |
| `session_end` | ❌ 缺失 | |
| `gateway_start` | ❌ 缺失 | |
| `gateway_stop` | ❌ 缺失 | |

### 10.4 内置插件对齐表

| Echoryn 插件 | 功能分类 | OpenClaw 对标 | 状态 |
|-------------|----------|-------------|------|
| memory-core | 记忆存储 | memory | ✅ 对齐 |
| diagnostics | 可观测性 | diagnostics | ✅ 对齐 |
| llm-task | LLM 工具 | llm (内置) | ✅ 对齐 |
| subagent | 多智能体 | subagent | ✅ 对齐 |
| golem-cluster | 分布式集群 | 无对应 | ✨ Echoryn 独有 |
| skills | 技能加载 | skills | ✅ 对齐 |
| web-search | Web 搜索 | web-search | ✅ 对齐 |
| channel-feishu | IM 集成 (飞书) | channel-feishu | ✅ 对齐 |
| channel-telegram | IM 集成 (Telegram) | channel-telegram | ✅ 对齐 |

### 10.5 缺失的能力注入通道 (vs OpenClaw)

| OpenClaw 能力 | Echoryn 对应 | 状态 |
|--------------|-------------|------|
| `registerTool` | ✅ `RegisterTool` | 已对齐 |
| `registerHook` | ✅ `RegisterHook` | 已对齐 |
| `registerCli` | ✅ `RegisterCLI` | 已对齐 |
| `registerService` | ✅ `RegisterService` | 已对齐 |
| `registerPrompt` | ✅ `PromptProvider` | **已对齐** ✨ |
| `registerHttpHandler` | ❌ 缺失 | 无 HTTP 扩展 |
| `registerHttpRoute` | ❌ 缺失 | |
| `registerChannel` | ❌ 缺失 | 依赖 Channels 模块 |
| `registerGatewayMethod` | ❌ 缺失 | |
| `registerProvider` | ❌ 缺失 | LLM Provider 注册走 SPI |
| `registerCommand` | ❌ 缺失 | |
| `on()` (typed hooks) | ❌ 缺失 | 无类型安全 Hook |

### 10.6 Echoryn 独有设计

1. **Interface Probe** — K8s 风格的自动接口探测，减少样板代码
2. **PromptProvider** — 独立的 Prompt 注入能力，将 System Prompt 组装管线化
3. **Status 码体系** — 借鉴 K8s scheduler 的 Success/Error/Skip/Unschedulable
4. **编译时安全** — Go 接口编译期检查，vs TypeScript 运行时 duck-typing
5. **InTreeRegistry** — 显式集中注册，避免 `init()` 隐式依赖
6. **BeforeCompaction/AfterCompaction** — 上下文压缩阶段的钩子，支持 LLM 驱动的记忆管理

---

## 十一、扩展新插件示例

```go
// 1. 定义插件
type MyPlugin struct{}

func (p *MyPlugin) Name() string { return "my-plugin" }

// 2. 实现 InitPlugin (命令式)
func (p *MyPlugin) Init(api plugin.PluginAPI) error {
    api.RegisterTool(plugin.ToolDefinition{
        Name:        "my_tool",
        Description: "My custom tool",
        Handler:     p.handleTool,
    })
    api.RegisterHook(plugin.HookBeforeAgentStart, p.onAgentStart)
    return nil
}

// 或 3. 实现声明式接口 (自动探测)
func (p *MyPlugin) Tools() []plugin.ToolDefinition { ... }
func (p *MyPlugin) Hooks() map[plugin.HookEvent]plugin.HookHandler { ... }
func (p *MyPlugin) PromptSections() []prompt.PromptSection { ... }

// 4. 注册到 InTreeRegistry
registry.Add(plugin.Definition{ID: "my-plugin", Kind: "general"},
    func(args plugin.PluginArgs, h plugin.Handle) (plugin.Plugin, error) {
        return &MyPlugin{}, nil
    }, nil)
```

---

> 最后更新: 2026-03-31
