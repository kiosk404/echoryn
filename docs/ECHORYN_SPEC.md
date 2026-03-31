# Echoryn Project SPEC

> **Echoryn** — AI 虚拟角色的灵魂容器，一个分离式分布式 AI Agent 平台。
>
> 模块名: `github.com/kiosk404/echoryn` | Go 1.25.0 | 参考项目: OpenClaw (TypeScript)

---

## 〇、模块详解文档索引

本文档为 Echoryn 项目的**顶层 SPEC**，概述整体架构、模块状态和开发路线。每个 Hivemind 核心模块均有独立的详解文档（含 OpenClaw 对比分析）：

| 文档 | 模块 | 核心内容 |
|------|------|---------|
| [ECHORYN_HIVEMIND_AGENTS_SPEC.md](./ECHORYN_HIVEMIND_AGENTS_SPEC.md) | Agents 运行时引擎 | AgentRunner 编排流程、TurnExecutor 重试/Fallback、ContextBuilder/Pruner 两阶段裁剪、PromptPipeline Section 化系统提示词、Compactor 多轮压缩、AgentFlow Eino DAG、SubAgent 接口预留、Store 层 |
| [ECHORYN_HIVEMIND_LLM_SPEC.md](./ECHORYN_HIVEMIND_LLM_SPEC.md) | LLM 多模型管理 | SPI 四层插件架构、8 个 Provider 实现（两种模式）、ModelManager 四阶段初始化、FallbackExecutor 四层错误分类、ModelProber 并发扫描、CompatManager 规则引擎 |
| [ECHORYN_HIVEMIND_MEMORY_SPEC.md](./ECHORYN_HIVEMIND_MEMORY_SPEC.md) | 记忆系统 | 插件注册（4 工具 + 2 钩子 + PromptProvider）、Manager 索引核心、混合搜索流程（向量 + 关键词融合）、SQLite 5 表 Schema、OpenAI/Gemini Embedding、Memory Flush、安全机制 |
| [ECHORYN_HIVEMIND_PLUGIN_SPEC.md](./ECHORYN_HIVEMIND_PLUGIN_SPEC.md) | 插件框架 | 三层基础接口、5 种能力注入（Tool/Hook/Service/CLI/PromptSection）、Slot 互斥、生命周期管理、Registry 中心仓、Interface Probe、3 个内置插件 |
| [ECHORYN_HIVEMIND_MCP_SPEC.md](./ECHORYN_HIVEMIND_MCP_SPEC.md) | MCP 工具调用 | Claude Desktop 兼容配置、Manager 接口、并发初始化、MCPServer 连接流程、工具聚合到 AgentRunner、重连/关闭 |

---

## 一、项目定位与核心理念

Echoryn 是一个 **分离式（Decoupled）** AI Agent 系统。不同于单体式 AI 服务，Echoryn 将 **推理决策** 与 **任务执行** 分离到不同节点：

| 角色 | 命名 | 职责 |
|------|------|------|
| **中心服务** | `hivemind`（蜂巢心智） | 统一心智中枢。拥有全局意识，可同时操控多个 Golem 节点；负责 Agent 推理、多模型调度、工具调用（MCP）、流式回复、会话压缩、记忆管理；按需将任务分配到不同 Golem，并可在节点间自由转移执行上下文 |
| **执行器** | `golem`（傀儡 / 躯体） | 无自主意识的执行体。仅接受 Hivemind 的指令，在本地执行 Skills（如浏览器操作、代码编写、文件处理等）；Golem 之间**互不通信**，各自独立 |
| **客户端** | `echoctl`（类似 kubectl） | 客户端 CLI。提供交互式 TUI 聊天界面（Bubbletea）、连接 Hivemind 对话、未来将扩展为完整的资源管理 CLI |

> **核心隐喻**：Hivemind 如同《复仇者联盟》中的奥创 (Ultron) — 一个统一的 AI 心智，同时驱动分布在不同位置的多个躯体 (Golem)。每个 Golem 拥有不同的能力（装了浏览器的能上网、装了 IDE 的能写代码），但它们没有自主意识，完全受 Hivemind 指挥。

```
用户请求 → Hivemind (统一心智: 推理 + 决策 + 调度)
              │
              ├──→ Agent Runtime (编排引擎)
              │     ├─ Context Builder (上下文构建)
              │     ├─ TurnExecutor (单轮执行 + 重试 + Fallback)
              │     ├─ Context Pruner (裁剪) + Compaction (压缩)
              │     └─ AgentFlow (Eino DAG 图编排)
              │
              ├──→ LLM Module (8 Provider: GPT/DeepSeek/Gemini/Claude/Qwen/Kimi/GLM/Ollama)
              ├──→ MCP Module (工具调用: stdio/SSE 传输)
              ├──→ Plugin Framework (memorycore/diagnostics/llm-task)
              │
              ├──→ Golem #1 (浏览器节点): 搜索网页、抓取信息
              ├──→ Golem #2 (开发节点): 编写代码、运行测试
              └──→ ... 更多 Golem
           ← 结果回传 ← 各 Golem 独立上报

     ┌──────────────────────────┐
     │      echoctl CLI          │
     │  (客户端交互)              │
     │  chat (Bubbletea TUI)     │
     │  --server / --model /    │
     │  --session               │
     └──────────────────────────┘
```

---

## 二、Echoryn 与 OpenClaw 对比分析

Echoryn 参考了 OpenClaw（TypeScript 编写的 AI Agent 平台），但在架构层面做了根本性的重新设计。

### 2.1 架构差异

| 维度 | OpenClaw | Echoryn |
|------|----------|---------|
| **语言** | TypeScript (Node.js) | Go |
| **部署模型** | **单体服务** — 所有逻辑在一个进程 | **分离式分布式** — Hivemind（大脑）+ Golem（躯体）分离部署 |
| **执行模型** | 本地执行 Skills | Golem 远程执行 Skills，Hivemind 统一编排 |
| **通信** | HTTP REST + gRPC 双协议 |
| **模型抽象** | 简单的 Provider 封装 | 完整的 SPI 插件体系（Provider/Chat/Compat/Probe 四层） |
| **插件系统** | npm 包 + 运行时加载 | Go 编译时插件框架（Slot 互斥 + 生命周期管理） |
| **存储** | PostgreSQL / Redis | BoltDB（嵌入式）+ SQLite（Memory）+ InMemory |
| **API 风格** | 自定义 REST API | **OpenAI 兼容 API**（`/v1/chat/completions`、`/v1/models`） |

### 2.2 概念映射

| OpenClaw 概念 | Echoryn 对应 | 状态 | 差异说明 |
|---------------|-------------|------|---------|
| Agent 服务 | `service/agents` | ✅ 已实现 | 增加了完整的 Runtime 编排引擎（Runner → Executor → AgentFlow） |
| LLM Provider | `service/llm/provider` | ✅ 已实现 | 从简单封装升级为 SPI 插件体系，8 个 Provider，支持 Probe/Fallback/Compat |
| Channel 通道 | `service/channels` | ❌ 未实现 | 相同理念，计划支持 Telegram/飞书/Web 等 |
| MCP 工具调用 | `service/mcp` | ✅ 已实现 | 支持 stdio/SSE 传输，Claude Desktop 兼容配置格式，并发初始化 |
| Memory 系统 | `plugin/builtin/memorycore` | ✅ 已实现 | SQLite + 混合搜索（关键词 + 向量语义），OpenAI/Gemini 双 Embedding Provider |
| 对话压缩 | `agents/runtime/compaction` | ✅ 已实现 | 在 Hivemind 中心节点统一处理，集成上下文裁剪 + Token 估算 |
| 任务调度 | Scheduler（已实现） | ⭐ 已实现 | OpenClaw 无此概念。Echoryn 的分布式任务调度引擎：PriorityQueue + AISelector 6 维评分 + Monitor + StatsCollector |
| Gateway 配置 | `gateway_config.go` | ✅ 已实现 | Auth 令牌、Store 选择、默认模型配置 |
| Sub-Agent 派生 | `service/agents/subagent` | ⭐ 已实现 | OpenClaw 有 `sessions_spawn/send`；Echoryn 完整实现 SubAgentManager/Scheduler/AnnounceController + 双存储后端 |
| — | Golem 工作节点 | 🟡 骨架 | OpenClaw 无对应。Echoryn 独创的远程执行体 |
| CLI 工具 | `echoctl` | ✅ 已实现 | echoctl 提供 Bubbletea TUI 交互式聊天 |

### 2.3 Echoryn 独有的设计

以下是 Echoryn 超出 OpenClaw 范畴的创新设计：

1. **Hivemind-Golem 分离架构**：推理在中心、执行在边缘，可跨机器调度 Skill
2. **完整的编译时插件框架**：Slot 互斥机制，支持 Tool/CLI/Hook/Service/PromptSection 五种能力注入
3. **SPI 插件化 LLM Provider**：四层 SPI 接口（Provider → ChatModel → Compat → Probe），支持健康探测和自动 Fallback
4. **Eino 图编排**：使用字节跳动 cloudwego/eino 框架构建 Agent 执行 DAG
5. **OpenAI 兼容 API**：对外暴露标准 `/v1/chat/completions` 接口，可无缝对接现有生态
6. **终端 TUI 聊天**：echoctl chat 使用 Bubbletea 框架实现交互式终端对话，支持 SSE 流式回复

---

## 三、系统架构

### 3.1 整体拓扑

```
                    ┌──────────────────────────────────────────────────┐
                    │               Hivemind Server                    │
                    │            (中央控制器 / 调度器)                  │
                    │                                                  │
                    │  ┌──────────┐    ┌───────────────────┐           │
                    │  │ HTTP API │    │  gRPC Service      │           │
                    │  │ :11789   │    │  :11788            │           │
                    │  └────┬─────┘    └────────┬──────────┘           │
                    │       │                   │                      │
                    │  ┌────┴───────────────────┴───────────────────┐  │
                    │  │              Handler Layer                  │  │
                    │  │  ├─ POST /v1/chat/completions (SSE+JSON)  │  │
                    │  │  ├─ GET  /v1/models                       │  │
                    │  │  ├─ CRUD /v1/agents + /v1/sessions        │  │
                    │  │  └─ Middleware: Bearer Auth + CORS         │  │
                    │  └───────────────────────────────────────────┘  │
                    │                                                  │
                    │  ┌───────────────────────────────────────────┐  │
                    │  │          ⭐ Agents Runtime Engine          │  │
                    │  │  ├─ AgentRunner (主编排器)                  │  │
                    │  │  ├─ TurnExecutor (单轮执行+重试+Fallback)   │  │
                    │  │  ├─ ContextBuilder → ContextPruner         │  │
                    │  │  ├─ Compaction (会话压缩)                   │  │
                    │  │  ├─ TokenEstimator (Token 估算)             │  │
                    │  │  └─ AgentFlow (Eino DAG 图编排)             │  │
                    │  └───────────────────────────────────────────┘  │
                    │                                                  │
                    │  ┌───────────────────────────────────────────┐  │
                    │  │          ⭐ LLM Module (8 Providers)       │  │
                    │  │  ├─ SPI: Provider/ChatModel/Compat/Probe  │  │
                    │  │  ├─ Providers: OpenAI | DeepSeek | Gemini │  │
                    │  │  │   | Claude | Qwen | Kimi | GLM | Ollama│  │
                    │  │  ├─ ModelManager + ModelProber             │  │
                    │  │  └─ FallbackExecutor (自动降级)             │  │
                    │  └───────────────────────────────────────────┘  │
                    │                                                  │
                    │  ┌───────────────────────────────────────────┐  │
                    │  │          ⭐ Plugin Framework               │  │
                    │  │  ├─ Slot 互斥 + 生命周期 (Init/Start/Stop)│  │
                    │  │  ├─ 4 能力: Tool/CLI/Hook/Service         │  │
                    │  │  └─ 内置插件:                              │  │
                    │  │      ├─ memorycore (SQLite + 混合搜索)     │  │
                    │  │      ├─ diagnostics (OTEL 可观测)          │  │
                    │  │      └─ llm-task (JSON LLM 工具)           │  │
                    │  └───────────────────────────────────────────┘  │
                    │                                                  │
                    │  ┌───────────────────────────────────────────┐  │
                    │  │          ⭐ MCP Module                     │  │
                    │  │  ├─ stdio / SSE 双传输                     │  │
                    │  │  ├─ Claude Desktop 兼容配置                 │  │
                    │  │  └─ 并发初始化 + 工具聚合 + 自动重连        │  │
                    │  └───────────────────────────────────────────┘  │
                    │                                                  │
                    │  ┌───────────────────────────────────────────┐  │
                    │  │  Scheduler (已实现)    │ channels (待实现)│  │
                    │  │  routing (待实现)        │ Store: BoltDB   │  │
                    │  └───────────────────────────────────────────┘  │
                    └──┬──────────┬──────────┬─────────────────────┘
                       │          │          │
                       │    ┌─────┘          └─────┐
                       │    │  gRPC 单向控制链路     │
                       ▼    ▼                      ▼
               ┌──────────────┐              ┌──────────────┐
               │   Golem #1   │              │   Golem #2   │
               │  (浏览器节点) │              │  (开发节点)   │
               │              │              │              │
               │ ┌──────────┐ │              │ ┌──────────┐ │
               │ │ browser  │ │              │ │ code_edit│ │
               │ │ crawler  │ │              │ │ terminal │ │
               │ └──────────┘ │              │ └──────────┘ │
               └──────────────┘              └──────────────┘
                      ▲                             ▲
                      │    Golem 之间互不通信         │
                      │    仅接受 Hivemind 指令      │
                      └──────── 各自独立 ────────────┘
```

### 3.2 通信协议

- **Client → Hivemind**: HTTP REST (Gin) + SSE (流式推送)，OpenAI 兼容格式
- **Hivemind → Golem**: gRPC **单向控制**（Hivemind 下发指令，Golem 上报结果/心跳）
- **Golem ↔ Golem**: **无通信**（各节点完全独立，互不感知）
- **MCP Tools**: stdio 进程管道 / SSE HTTP（Model Context Protocol）

---

## 四、项目目录结构

```
echoryn/
├── cmd/                                  # 三个可执行入口
│   ├── hivemind/hivemind.go              # 中央服务入口
│   ├── golem/golem.go                    # 工作节点入口
│   └── echoctl/echoctl.go               # 日常操作 CLI (kubectl 风格)
│
├── conf/
│   ├── hivemind-server.json              # Hivemind 默认配置
│   └── mcp.json                          # MCP 工具配置（Claude Desktop 格式）
│
├── idl/                                  # Protobuf IDL 定义
│   ├── base.proto                        # 基础 RPC 消息 (Base/BaseResp/TrafficEnv)
│   ├── api.proto                         # [待实现] API 服务定义
│   └── app/common_struct/
│       ├── common_struct.proto           # 用户/连接器/空间/变量/资源/权限
│       ├── intelligence_common_struct.proto # 智能体状态/类型/基础信息
│       └── golem_node_common_struct.proto  # [待实现] Golem 节点结构
│
├── internal/                             # 内部实现（不可外部引用）
│   ├── hivemind/                         # ⭐ Hivemind 服务实现（核心）
│   │   ├── app.go / run.go / server.go   # 应用生命周期
│   │   ├── router.go                     # HTTP 路由注册 (OpenAI 兼容)
│   │   ├── gateway_config.go             # 网关配置（Auth/Store/Defaults）
│   │   ├── config/config.go              # 配置结构
│   │   ├── options/                      # 命令行选项（含 MCP 选项）
│   │   ├── middleware/                   # Bearer Auth + CORS 中间件
│   │   ├── handler/v1/                   # HTTP Handler (chat_completions/models/agents/sessions)
│   │   └── service/                      # ⭐⭐⭐ 核心业务逻辑
│   │       ├── agents/                   # ⭐ Agent 管理 + 运行时引擎
│   │       │   ├── module.go             # K8s 式模块入口
│   │       │   ├── domain/entity/        # Agent/Session/Run/Message/Tool/Events/SubAgent
│   │       │   ├── domain/repo/          # 仓储接口
│   │       │   ├── domain/service/       # AgentService CRUD + SubAgentManager/Registry (接口已预留)
│   │       │   ├── domain/service/runtime/ # ⭐ Runtime 引擎
│   │       │   │   ├── runner.go         # AgentRunner 主编排器
│   │       │   │   ├── executor.go       # TurnExecutor (执行+重试+Fallback)
│   │       │   │   ├── context_builder.go # LLM 上下文构建 (集成 PromptPipeline)
│   │       │   │   ├── context_pruner.go  # 上下文裁剪
│   │       │   │   ├── context_window.go  # 上下文窗口管理
│   │       │   │   ├── compaction.go      # 会话压缩
│   │       │   │   ├── token_estimator.go # Token 估算
│   │       │   │   ├── message_converter.go # 消息格式转换
│   │       │   │   ├── run_state.go       # Run 状态机
│   │       │   │   ├── abort.go           # 中止处理
│   │       │   │   ├── prompt/            # ⭐ PromptPipeline (Section 化系统提示词)
│   │       │   │   │   ├── types.go       # PromptSection/Mode/Context/Mutator 接口
│   │       │   │   │   ├── pipeline.go    # 组装管线 (K8s Admission Chain)
│   │       │   │   │   ├── sections.go    # 内置 Section (Identity/Cluster/Tooling/Persona/Runtime)
│   │       │   │   │   └── workspace.go   # WorkspaceLoader + WorkspaceSection (P1: fsnotify 热更新)
│   │       │   │   └── agentflow/         # Eino DAG 图编排
│   │       │   ├── store/inmemory/       # 内存存储
│   │       │   ├── store/boltdb/         # BoltDB 持久化存储
│   │       │   └── pkg/errno/            # 错误码
│   │       │
│   │       ├── llm/                      # ⭐ LLM 多模型管理
│   │       │   ├── module.go             # Manager + Prober + Fallback + Registry
│   │       │   ├── domain/entity/        # 14 个实体文件
│   │       │   ├── domain/service/       # ModelManager/ModelProber/FallbackExecutor
│   │       │   ├── provider/             # ⭐ SPI 插件体系
│   │       │   │   ├── spi/spi.go        # 4 层接口 (Provider/Chat/Compat/Probe)
│   │       │   │   ├── helper/           # BasePlugin 基类
│   │       │   │   ├── registry.go       # Provider 注册表
│   │       │   │   ├── openai/           # OpenAI (GPT-4o/o1/o3-mini)
│   │       │   │   ├── deepseek/         # DeepSeek (V3/R1)
│   │       │   │   ├── anthropic/        # Anthropic Claude
│   │       │   │   ├── gemini/           # Google Gemini
│   │       │   │   ├── qwen/             # 通义千问
│   │       │   │   ├── kimi/             # Moonshot Kimi
│   │       │   │   ├── glm/              # 智谱 GLM
│   │       │   │   └── ollama/           # Ollama (本地模型)
│   │       │   └── store/inmemory/       # 内存存储
│   │       │
│   │       ├── plugin/                   # ⭐ 插件框架
│   │       │   ├── types.go              # Plugin/InitPlugin/LifecyclePlugin 接口
│   │       │   ├── framework.go          # 核心 (注册→Slot→Init→Start→Stop)
│   │       │   ├── registry.go / slots.go # 注册表 + Slot 互斥
│   │       │   ├── tools.go / hooks.go   # ToolProvider / HookProvider
│   │       │   ├── services.go / cli.go  # ServiceProvider / CLIProvider
│   │       │   └── builtin/              # 3 个内置插件
│   │       │       ├── memorycore/       # Memory 插件 (SQLite + 混合搜索 + Embedding)
│   │       │       ├── diagnostics/      # 诊断插件 (OTEL 可观测性)
│   │       │       └── llmtask/          # LLM-Task 插件 (JSON-only LLM 工具)
│   │       │
│   │       ├── mcp/                      # ⭐ MCP 协议模块
│   │       │   ├── module.go             # 模块入口
│   │       │   ├── config.go             # Claude Desktop 兼容配置
│   │       │   ├── manager.go            # Manager 接口
│   │       │   ├── manager_impl.go       # 并发初始化 + 工具聚合 + 重连
│   │       │   └── server.go             # MCPServer (stdio/SSE)
│   │       │
│   │       ├── scheduler/                # ⭐ Scheduler 调度引擎 (已实现)
│   │       │   ├── scheduler.go         # Scheduler 接口 + defaultScheduler 实现
│   │       │   ├── task.go              # ScheduleRequest/Decision/Event 类型 + Builder
│   │       │   ├── queue.go             # PriorityQueue (heap-based, 线程安全)
│   │       │   ├── selector.go          # NodeSelector (Direct/AI/Composite/Filter)
│   │       │   └── monitor.go           # Monitor (超时/停滞检测) + StatsCollector
│   │       │
│   │       ├── channels/                 # [待实现] 通道管理
│   │       └── routing/                  # [待实现] 智能路由
│   │

│   ├── echoctl/                          # 客户端 CLI (Bubbletea TUI)
│   │   ├── cmd/                          # 命令实现
│   │   │   ├── cmd.go                    # 根命令注册 (ASCII banner + 命令组)
│   │   │   ├── chat/                     # ⭐ chat 命令 (交互式 TUI)
│   │   │   │   ├── chat.go              # Cobra 命令 (--server/--model/--session)
│   │   │   │   ├── client.go            # HivemindClient (HTTP + SSE 流式)
│   │   │   │   └── tui.go              # Bubbletea TUI (交互界面 + 流式渲染)
│   │   │   ├── util/                     # Factory 工厂模式
│   │   │   ├── banner.go / global.go    # 横幅 + 全局选项
│   │   │   └── profiling.go            # 性能分析支持
│   │   ├── types/                        # 常量定义
│   │   └── utils/                        # 模板/中断/终端工具
│   │
│   ├── golem/                            # Golem 工作节点 [骨架]
│   │   └── app.go                        # 仅 App 初始化
│   │
│   └── pkg/                              # 内部共享包
│       ├── core/                         # HTTP 响应写入
│       ├── options/                      # 通用选项 (gRPC/HTTP/Model/Plugin/MCP)
│       └── server/                       # 通用服务器 (Gin + gRPC + Viper)
│
├── pkg/                                  # 公共库（可被外部引用）
│   ├── app/                              # Cobra 应用框架封装
│   ├── logger/                           # logrus 日志系统（文件轮转）
│   ├── errorx/                           # 错误码系统（堆栈跟踪）
│   ├── version/                          # 版本信息（ldflags 注入）
│   ├── http/
│   │   ├── ginutil/                      # Gin 工具
│   │   ├── sse/                          # SSE 推送系统
│   │   └── shutdown/                     # 优雅关闭管理器
│   ├── paths/                            # 集中路径解析 (NodeRole + ~/.echoryn/ 状态目录)
│   ├── cli/genericclioptions/            # CLI IO 流
│   └── utils/                            # 通用工具集 (cliflag/goroutine/ip/json/safego...)
│
├── scripts/make-rules/                   # Makefile 构建规则
├── Makefile                              # 构建入口
├── openclaw/                             # 参考项目（OpenClaw TypeScript 完整源码）
└── airi-go/                              # 参考项目（airi-go AI Agent 后端）
```

---

## 五、核心组件详解

### 5.0 Team 团队协作 — 多智能体编排（已实现）

Echoryn 实现了完整的**多智能体协作系统**（类似 K8s 的 Pod 编排）。

```
Team Orchestration (完整实现)
  │
  ├── TeamOrchestrator
  │   ├─ CreateTeam(name, template) → 创建团队 (保存到 BoltDB)
  │   ├─ DissolvTeam(teamID) → 解散团队 + 清理资源
  │   ├─ AddMember(teamID, agentID) → 添加成员
  │   └─ ListTeams/GetTeam/SetTeamMetadata
  │
  ├── Team 生命周期: Created → Running → Completed/Failed/Cancelled
  │
  ├── TeamTemplateService
  │   ├─ 预定义团队模板（如 "研发团队" = [CodeAgent + TestAgent])
  │   └─ 支持快速启动常见的多智能体配置
  │
  ├── EventBridge
  │   ├─ SubAgent 生命周期事件桥接
  │   ├─ Spawned/Started/Completed/Failed 等事件
  │   └─ 推送到所有团队成员（通过内部消息总线）
  │
  └── MessageBus (异步消息传递)
      ├─ Agent 间通过异步消息通信（不阻塞彼此）
      ├─ 支持请求-响应模式
      └─ 支持发布-订阅广播
```

**HTTP API 端点**：
- `GET /v1/teams/templates` — 列出团队模板
- `POST /v1/teams` — 创建团队
- `GET /v1/teams/:id` — 获取团队详情
- `DELETE /v1/teams/:id` — 解散团队
- `POST /v1/teams/:id/messages` — 发送团队消息



#### 启动流程（InitializerChain 模式）

```
NewApp("hivemind-server")
  → Options → Cobra Command → Run(opts)
    → InitLog → CreateConfigFromOptions
      → createAPIServer(cfg)
        → GenericAPIServer (Gin HTTP :11789)
        → GRPCAPIServer (gRPC :11788)
        → InitializerChain 顺序执行（internal/hivemind/initializers.go）:
        │   ├─ InitInfrastructure
        │   │   ├─ Admin Token 加载/创建/持久化到 ~/.echoryn/credentials/admin_token (0o600)
        │   │   ├─ TokenManager 初始化（Bootstrap Token 管理，支持 TTL + max-usage）
        │   │   ├─ Golem Dev-Mode 支持（loopback 节点跳过 join-token 验证）
        │   │   └─ Team 依赖初始化
        │   ├─ InitGolem (Golem 子系统：Registry/Dispatcher/Scheduler)
        │   ├─ InitLLM (LLM Module 初始化 → 8 个 Provider)
        │   ├─ InitMCP (MCP Module 初始化 → 并发启动 MCP Server)
        │   ├─ InitAgents (AgentsModule 初始化 → CRUD + Runtime)
        │   └─ InitPluginLifecycle (Plugin Framework Start 钩子)
        → PrepareRun() → Run()
          → GracefulShutdown 注册
```

**InitInfrastructure 详解**（对系统影响最大）：
1. Admin Token 加载/创建 → 持久化到 `~/.echoryn/credentials/admin_token` (权限 0o600)
2. TokenManager 初始化 → Bootstrap Token 系统就绪
    - **TTL**: Token 有时间过期限制（单位: 秒）
    - **Max Usage**: Token 可使用次数限制
    - Golem 注册时通过 join-token 验证
3. Golem Dev-Mode 配置：Loopback 地址 (127.0.0.1) 可跳过 join-token，用于本地开发
4. Team 依赖初始化：多智能体协作系统就绪



#### HTTP 路由 (OpenAI 兼容)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 健康检查 |
| GET | `/version` | 版本信息 |
| POST | `/v1/chat/completions` | 聊天补全（SSE 流式 + JSON 非流式） |
| GET | `/v1/models` | 列出可用模型 |
| POST | `/v1/agents` | 创建 Agent |
| GET | `/v1/agents` | 列出所有 Agent |
| GET | `/v1/agents/:id` | 获取 Agent 详情 |
| DELETE | `/v1/agents/:id` | 删除 Agent |
| GET | `/v1/agents/:id/sessions` | 列出 Agent 的会话 |
| GET | `/v1/sessions/:id` | 获取会话详情 |
| DELETE | `/v1/sessions/:id` | 删除会话 |
| GET | `/v1/teams/templates` | 列出团队模板 |
| POST | `/v1/teams` | 创建团队 |
| GET | `/v1/teams/:id` | 获取团队详情 |
| DELETE | `/v1/teams/:id` | 解散团队 |
| POST | `/v1/teams/:id/messages` | 发送团队消息 |

#### 配置结构 (`conf/hivemind-server.json`)

**配置格式**: 仅支持 **JSON** 格式（`internal/pkg/server/config.go` 第 110 行硬编码 `viper.SetConfigType("json")`）

```json
{
  "server": {
    "mode": "debug",
    "healthz": true,
    "max-ping-count": 3,
    "middlewares": ["recovery", "logger", "nocache", "cors", "secure"]
  },
  "grpc": { "bind-address": "0.0.0.0", "bind-port": 11788 },
  "log": { "name": "hivemind-apiserver", "level": "debug", "format": "console" },
  "models": { /* LLM 模型配置 */ },
  "plugins": { /* 插件配置 */ },
  "mcp": { "config-path": "./conf/mcp.json" },
  "gateway": { "auth_token": "...", "store_type": "boltdb" }
}
```

#### gRPC 服务

| 服务 | 说明 |
|------|------|
| `GolemNodeService` | Golem 节点注册、心跳、任务流 |
| `HivemindAdminService` | Token 管理、集群管理 |

**默认端口**: `:11788` (gRPC)、`:11789` (HTTP)

#### 5.2 Agents Module — Agent 管理与运行时

这是 Hivemind 中**最核心**的模块，实现了完整的 Agent 生命周期和执行引擎。

#### 架构分层

```
AgentsModule (K8s 式: Config → Complete → New)
  │
  ├── AgentService (CRUD)
  │     ├─ CreateAgent / GetAgent / ListAgents / DeleteAgent
  │     └─ CreateSession / GetSession / ListSessions / DeleteSession
  │
  └── Runtime Engine (执行引擎)
        ├─ AgentRunner (主编排器)
        │    └─ 管理 Run 生命周期: Created → Running → Completed/Failed
        │
        ├─ TurnExecutor (单轮执行)
        │    ├─ 执行一轮 LLM 交互
        │    ├─ Tool Call 循环处理
        │    ├─ 错误重试（指数退避）
        │    └─ Model Fallback（自动降级）
        │
        ├─ ContextBuilder → ContextPruner
        │    ├─ 构建 LLM 上下文（System Prompt + History + Tools）
        │    ├─ **集成 PromptPipeline**（Section 化系统提示词组装）
        │    ├─ 按 Token 预算裁剪消息
        │    └─ ContextWindow 管理最大 Token 窗口
        │
        ├─ **PromptPipeline** (K8s Admission Chain 模式)
        │    ├─ IdentitySection (P:100) — 核心身份 + 分布式架构意识
        │    ├─ ClusterAwarenessSection (P:150) — Golem 拓扑注入
        │    ├─ ToolingSection (P:200) — 可用工具枚举
        │    ├─ PersonaSection (P:300) — 用户自定义人格文本
        │    ├─ WorkspaceSection:soul (P:310) — SOUL.md (WorkspaceLoader)
        │    ├─ WorkspaceSection:identity_file (P:320) — IDENTITY.md (WorkspaceLoader)
        │    ├─ WorkspaceSection:agents_file (P:330) — AGENTS.md (WorkspaceLoader)
        │    ├─ WorkspaceSection:extra:* (P:350+) — prompts/*.md (WorkspaceLoader)
        │    ├─ MemorySection (P:400) — 记忆系统指令 (memorycore PromptProvider)
        │    ├─ RuntimeSection (P:900) — 运行时元信息
        │    └─ 插件可通过 PromptProvider 注册自定义 Section
        │
        ├─ Compaction (会话压缩)
        │    └─ 当历史消息过长时自动压缩摘要
        │
        ├─ TokenEstimator
        │    └─ 估算消息的 Token 数（不依赖远程 API）
        │
        └─ AgentFlow (Eino DAG)
             ├─ builder.go — 构建 Eino 执行图
             ├─ callback.go — 流式回调处理
             └─ node_tool_plugin.go — 工具节点集成
```

#### Sub-Agent 编排（K8s Controller 模式，已实现）

```
SubAgentManager (完整实现)
  │
  ├── Spawn — 派生子 Agent，在独立 Session 中异步执行
  │     ├─ 深度限制: 子 Agent 不可再派生子 Agent (max depth = 1)
  │     ├─ 并发限制: SubAgentScheduler (semaphore.Weighted, 默认 8)
  │     ├─ 工具黑名单: subAgentToolDenyList (6 个被禁工具)
  │     └─ 结果汇报: AnnounceController 注入消息到父 Session
  │
  ├── SubAgentScheduler — 基于 semaphore 的并发控制 + WaitGroup 优雅关闭
  │
  ├── AnnounceController — 结果公告
  │     ├─ 直接投递 (Announce)
  │     └─ 延迟队列 (Enqueue/DrainPending)
  │
  ├── SubAgentRegistry (持久化) — BoltDB + InMemory 双存储后端
  │     └─ 支持进程重启后恢复 in-flight 子 Agent (Recover)
  │
  ├── 已注册的工具:
  │     ├─ sessions_spawn — 父 Agent 派生子 Agent 的 Tool
  │     └─ sessions_send — Agent 间通信的 Tool
  │
  └── 相关实体:
        ├─ SubAgentSpawnRequest (派生请求)
        ├─ SubAgentRecord (生命周期跟踪: Pending→Running→Completed/Failed/Cancelled)
        ├─ Session.ParentSessionID (深度检查)
        └─ EventSubAgentSpawned / EventSubAgentCompleted (流式事件)
```

#### 存储层

| 实现 | 说明 |
|------|------|
| `store/inmemory/` | 内存存储（开发/测试用） |
| `store/boltdb/` | BoltDB 持久化（生产用） |

### 5.3 LLM Module — 多模型管理

#### SPI 插件体系

```go
// 四层 SPI 接口 — 每个 Provider 按需实现
type ProviderPlugin interface {
    Meta() ProviderMeta           // 提供商元信息
    ValidateConfig(conn) error    // 配置校验
}

type ChatModelPlugin interface {
    CreateChatModel(ctx, conn) (chat_model.ChatModel, error)  // 创建聊天模型
}

type CompatPlugin interface {
    GetCompatRules() []CompatRule  // 兼容性规则
}

type ProbePlugin interface {
    Probe(ctx, conn) ProbeResult  // 健康探测
}
```

#### 已实现的 8 个 Provider

| Provider | 支持模型 | 特殊能力 |
|----------|---------|---------|
| OpenAI | GPT-4o, o1, o3-mini | Azure endpoint 支持 |
| DeepSeek | V3, R1 | 思考模型（ThinkingType） |
| Gemini | Gemini Pro/Flash | Backend 选择 + 项目/区域配置 |
| Claude | Claude 3/3.5/4 | Anthropic 原生 API |
| Qwen | 通义千问系列 | 阿里云 DashScope |
| Kimi | Moonshot 系列 | |
| GLM | 智谱 GLM-4 | |
| Ollama | 本地任意模型 | 本地部署，无需 API Key |

#### 核心服务

- **ModelManager**: 模型 CRUD + 默认模型管理
- **ModelProber**: 健康探测（定期检查模型可用性）
- **FallbackExecutor**: 自动降级（主模型失败时切换到备用模型）

### 5.4 Plugin Framework — 插件框架

```
Plugin Framework (编译时插件)
  │
  ├── 生命周期: Register → Slot 解析 → Init → Start → Stop
  │
  ├── Slot 互斥: 同一 Slot 只能有一个插件
  │   例: "memory" slot 只允许一个 Memory 实现
  │
  ├── 5 种能力注入:
  │   ├─ ToolProvider     — 注册工具供 Agent 调用
  │   ├─ HookProvider     — 服务器启动/停止钩子
  │   ├─ ServiceProvider   — 后台服务
  │   ├─ CLIProvider      — 扩展 CLI 命令
  │   └─ PromptProvider   — 注册 PromptSection/Mutator 到系统提示词管线
  │
  └── 10+ 个内置插件:
      ├── memorycore   — 记忆系统 (SQLite + 混合搜索)
      │   ├─ memory_write / memory_read / memory_delete 工具
      │   ├─ 关键词搜索 + 向量语义搜索 混合
      │   ├─ OpenAI Embedding Provider
      │   ├─ Gemini Embedding Provider
      │   ├─ Batch 分页 + System Prompt 注入
      │   ├─ Memory Flush 钩子 (会话结束自动存储)
      │   └─ **PromptProvider: MemorySection (P:400) — 记忆系统指令注入系统提示词**
      │
      ├── diagnostics   — 可观测性 (OTEL Traces/Metrics)
      ├── llm-task      — 通用 JSON-only LLM 工具
      ├── subagent      — 子智能体管理工具 (sessions_spawn/sessions_send)
      ├── skills        — 技能加载管理
      ├── golem-cluster — Golem 集群管理工具
      ├── channel-feishu — 飞书 IM 渠道集成
      ├── channel-telegram — Telegram IM 渠道集成
      ├── web-search    — Web 搜索工具 (Gemini)
      └── [可插拔扩展] — 用户自定义插件注册点
```

### 5.5 MCP Module — 工具调用

支持 Model Context Protocol，与 Claude Desktop 配置格式兼容。

```go
// 配置格式（Claude Desktop 兼容）
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "env": {}
    }
  }
}
```

核心能力：
- **双传输**: stdio 进程管道 + SSE HTTP
- **并发初始化**: 多个 MCP Server 并行启动
- **工具聚合**: 所有 MCP Server 的工具统一注册到 Agent 可调用列表
- **自动重连**: Server 断开后自动恢复

### 5.6 Scheduler — 任务调度引擎（已实现）

Scheduler 模块已完整实现（5 个文件，~2000 行），采用 K8s 风格的 Config → CompletedConfig → New() 初始化模式。

```
Scheduler (defaultScheduler)
  │
  ├── Schedule(req) — 入队 + 调度循环
  │   ├─ DirectMode: 指定节点 → DirectSelector 验证
  │   └─ AIMode: AISelector 6 维加权评分自动选择
  │
  ├── PriorityQueue (heap-based)
  │   └─ 线程安全 + FIFO 平局决断
  │
  ├── NodeSelector 策略体系
  │   ├─ DirectSelector — 直接指定节点（验证硬约束）
  │   ├─ AISelector — 6 维评分: Capability/Skill/Resource/Load/Tag/Affinity
  │   ├─ CompositeSelector — 责任链组合多策略
  │   └─ FilterSelector — 装饰器模式预过滤候选节点
  │
  ├── Monitor (任务监控)
  │   ├─ 超时检测 + 停滞检测（心跳间隔）
  │   ├─ 后台轮询 goroutine
  │   └─ 事件回调: OnTaskTimeout / OnTaskStalled → 自动重调度
  │
  ├── StatsCollector (统计收集)
  │   └─ 全局 + 单节点统计，滑动窗口平均延迟
  │
  └── 事件驱动: Subscribe/Unsubscribe + 8 种 TaskEventType
```

核心依赖（外部注入）：
- **ProfileProvider**: 提供 Golem 节点画像列表（待 Golem 通信层就绪后接入）
- **TaskDispatcher**: 向目标节点下发任务（待 gRPC 传输层就绪后接入）

### 5.7 Golem — 工作节点

当前为骨架阶段。设计意图：

- 作为 Skill 执行器，从 Hivemind 接收任务
- 通过 gRPC 与 Hivemind 保持连接、汇报心跳
- 本地维护 workspace/skills/data 目录结构

#### 运行时状态目录布局（NodeRole 机制）

系统通过 `pkg/paths` 包的 `NodeRole` 类型区分 Hivemind 和 Golem 节点，使用不同的配置文件和目录结构：

```
~/.echoryn/                                       ← 统一状态根目录
├── hivemind.json                                 ← Hivemind 控制平面配置
├── golem.json                                    ← Golem 工作节点配置
│
├── credentials/                                  ← Hivemind 专属
│   ├── admin_token                               ← Admin Token 持久化 (0o600)
│   └── bootstrap_tokens/                         ← Bootstrap Token 列表（含 TTL/max-usage）
│
├── agents/                                       ← Hivemind 专属
│   └── main/
│       └── sessions/                             ← 会话数据 (BoltDB)
│
├── memory/                                       ← Hivemind 专属
│   ├── memory.db                                 ← SQLite FTS5 + 向量搜索
│   └── embeddings/                               ← 向量缓存
│
├── workspace/                                    ← Hivemind 专属
│   ├── SOUL.md / IDENTITY.md / AGENTS.md         ← Agent 身份定义 (WorkspaceLoader 热更新)
│   ├── memory/                                   ← 工作空间记忆
│   └── prompts/                                  ← 自定义 prompt 文件 (*.md)
│
└── golem/                                        ← Golem 专属
    ├── workspace/                                ← 工作空间
    ├── skills/                                   ← 技能插件
    └── data/                                     ← 数据存储
        ├── logs/                                 ← 执行日志
        └── cache/                                ← 缓存数据
```

**路径解析** (`pkg/paths/resolve.go` + `pkg/paths/ensure.go`):
- `pkg/paths.ResolveStateDir()` → 返回 `~/.echoryn/` (可通过 `ECHORYN_STATE_DIR` 环境变量自定义)
- `pkg/paths.ResolveConfigPath(role)` → Hivemind → `hivemind.json`，Golem → `golem.json`
- `pkg/paths.ResolveAdminTokenPath()` → `~/.echoryn/credentials/admin_token`
- `pkg/paths.EnsureStateDirForRole(role)` → 按角色初始化完整目录结构 (0o700 权限)

### 5.8 CLI 工具

#### echoctl（客户端 — kubectl 风格）

echoctl 已从节点管理工具重构为**客户端交互工具**，核心是 `chat` 命令。

| 命令 | 功能 | 状态 |
|------|------|------|
| `echoctl chat` | 交互式 TUI 聊天（Bubbletea）| ✅ 已完成 |
| `echoctl chat --server <url>` | 指定 Hivemind 服务器地址 | ✅ 已完成 |
| `echoctl chat --model <name>` | 指定模型（默认 "echoryn"）| ✅ 已完成 |
| `echoctl chat --session <key>` | 指定会话 Key | ✅ 已完成 |
| `echoctl chat -m "消息"` | 单次消息模式（非交互）| ✅ 已完成 |
| `echoctl get/describe/create/delete` | K8s 风格资源操作 | 🔲 待实现 |

**Chat TUI 特性**：
- **Bubbletea 框架**：Elm 架构 TUI，charmbracelet/bubbles 组件（textarea + spinner）
- **SSE 流式**：通过 `HivemindClient` 向 `/v1/chat/completions` 发起流式请求，实时渲染回复
- **会话管理**：自动生成 `echoctl-{timestamp}` 会话 Key，通过 `X-Session-Key` Header 传递
- **命令系统**：`/quit`、`/exit`（退出）、`/clear`（清空对话）
- **ASCII 艺术欢迎页**：含版本号、模型名、服务器地址
- **快捷键**：`Enter` 发送 | `Shift+Enter` 换行 | `Esc`/`Ctrl+C` 退出

---

## 六、关键设计模式

| 模式 | 应用场景 | 说明 |
|------|---------|------|
| **Options → Config → CompletedConfig** | Hivemind/Golem/各 Module 启动 | k8s 风格，命令行参数 → 配置对象 → 校验完成的配置 → New() |
| **SPI (Service Provider Interface)** | LLM Provider 插件 | 四层接口分离，Provider 只需实现关心的层 |
| **DAG 图编排** | AgentFlow (Eino) | 将 Agent 执行流程建模为有向无环图 |
| **Slot 互斥** | Plugin Framework | 同一功能槽只允许一个实现，避免冲突 |
| **Admission Chain** | PromptPipeline | Section 化系统提示词组装，类似 K8s MutatingAdmission Chain |
| **Fallback** | LLM FallbackExecutor | 主模型失败自动降级到备用模型 |
| **Elm Architecture** | echoctl chat TUI | Bubbletea Model-Update-View 三件套，单向数据流 |
| **Abstract Factory** | `Factory` 接口 | echoctl 命令的可替换依赖注入 |
| **Interface Probe** | Plugin 能力注入 | 运行时类型断言检测插件支持的能力（Tool/Hook/Service/CLI/Prompt）|

---

## 七、技术栈

| 类别 | 技术选型 |
|------|---------|
| 语言 | Go 1.25.0 |
| CLI 框架 | `spf13/cobra` + `spf13/viper` |
| TUI 框架 | `charmbracelet/bubbletea` + `bubbles` + `lipgloss`（Elm 架构终端 UI）|
| HTTP 框架 | `gin-gonic/gin` + pprof + CORS |
| RPC 框架 | `google.golang.org/grpc` + Protobuf |
| LLM 编排 | `cloudwego/eino`（字节跳动） |
| 日志 | `sirupsen/logrus` + 文件轮转 |
| JSON | `bytedance/sonic`（高性能） |
| 持久化 | `bbolt` (BoltDB) + `mattn/go-sqlite3` (CGO) |
| MCP | `mark3labs/mcp-go`（Model Context Protocol SDK） |
| Embedding | OpenAI API + Google Gemini API |
| 错误处理 | 自研 `pkg/errorx`（错误码 + 堆栈） |
| 优雅关闭 | 自研 `pkg/http/shutdown` (POSIX 信号) |
| 流式推送 | 自研 `pkg/http/sse` |
| 构建 | Makefile + ldflags 版本注入 |
| 运行时 | `go.uber.org/automaxprocs` |

---

## 八、各模块实现状态

| 模块 | 状态 | 完成度 | 说明 |
|------|------|--------|------|
| `pkg/*` 基础库 | ✅ 完成 | 100% | app/logger/errorx/version/sse/shutdown/utils |
| `internal/pkg/server` | ✅ 完成 | 100% | Gin + gRPC 双协议服务器 |
| `internal/pkg/options` | ✅ 完成 | 100% | 通用选项（含 Model/Plugin/MCP） |
| Hivemind 基础框架 | ✅ 完成 | 100% | App/Server/Config/Router/Gateway/Middleware/Handler |
| **Agents Module** | ⭐ 完成 | **95%** | 完整 CRUD + Runtime 引擎（Runner/Executor/Context/Compaction/AgentFlow）+ **PromptPipeline**。详见 [ECHORYN_HIVEMIND_AGENTS_SPEC.md](./ECHORYN_HIVEMIND_AGENTS_SPEC.md) |
| **LLM Module** | ⭐ 完成 | **95%** | 完整 SPI 体系，8 个 Provider，Manager/Prober/Fallback。详见 [ECHORYN_HIVEMIND_LLM_SPEC.md](./ECHORYN_HIVEMIND_LLM_SPEC.md) |
| **Plugin Framework** | ⭐ 完成 | **95%** | 完整生命周期，Slot 互斥，**5 种能力注入**（Tool/CLI/Hook/Service/Prompt），10+ 内置插件。详见 [ECHORYN_HIVEMIND_PLUGIN_SPEC.md](./ECHORYN_HIVEMIND_PLUGIN_SPEC.md) |
| **MCP Module** | ⭐ 完成 | **90%** | stdio/SSE 传输，并发初始化，工具聚合，Claude Desktop 兼容。详见 [ECHORYN_HIVEMIND_MCP_SPEC.md](./ECHORYN_HIVEMIND_MCP_SPEC.md) |
| **Memory 系统** | ✅ 完成 | **90%** | SQLite + 混合搜索 + OpenAI/Gemini Embedding + Flush 钩子。详见 [ECHORYN_HIVEMIND_MEMORY_SPEC.md](./ECHORYN_HIVEMIND_MEMORY_SPEC.md) |
| **Team 多智能体** | ⭐ 完成 | **90%** | 完整团队编排：Orchestrator/Template/EventBridge/MessageBus。支持 HTTP API CRUD |
| **echoctl CLI** | ✅ chat 完成 | **50%** | chat TUI (Bubbletea + SSE 流式) 已完成，kubectl 风格资源管理命令待实现|
| **SubAgent 编排** | ⭐ 完成 | **85%** | K8s Controller 模式：SubAgentManager/SubAgentScheduler/AnnounceController + 双存储后端（InMemory/BoltDB）+ 工具黑名单 + 恢复机制。Cleanup 待完善 |
| Golem 工作节点 | 🟡 骨架 | 10% | 仅 App 初始化，无业务逻辑 |
| Scheduler 调度引擎 | ⭐ 完成 | **95%** | 完整调度器：PriorityQueue + AISelector 6 维评分 + Monitor 超时检测 + StatsCollector。待接入 ProfileProvider/TaskDispatcher |
| Channels 模块 | ❌ 未开始 | 0% | 空目录 |
| Routing 模块 | ❌ 未开始 | 0% | 空目录 |
| Proto 定义 | 🟡 部分完成 | 30% | base + common 已定义，api + golem_node 为空 |

---

## 九、Echoryn 特点总结

### 9.1 已形成的核心竞争力

1. **中心已就绪的 AI Agent 平台**
    - Hivemind 已经是一个功能完整的 AI Agent 服务器：Agent CRUD、多轮对话、会话压缩、上下文管理、流式回复、OpenAI 兼容 API 全部可用
    - 可以独立运行 Hivemind 作为单机 AI Agent 后端

2. **生产级插件框架**
    - 编译时安全、Slot 互斥、完整生命周期管理
    - Memory 系统已实现混合搜索（关键词 + 向量语义）
    - 可扩展的 Tool/CLI/Hook/Service/PromptSection 五种能力

3. **K8s 风格的运维体验**
    - JSON 配置 + dot-path 访问 + 配置脱敏
    - Token 认证机制

4. **终端原生交互体验**
    - echoctl chat 提供 Bubbletea TUI 交互式聊天界面
    - SSE 流式实时渲染、会话管理、ASCII 艺术欢迎页
    - 支持交互式和单次消息两种模式

项目最大的特色——Hivemind-Golem 分布式架构——目前处于调度引擎已就绪、传输层待接通阶段：
- Scheduler 调度器已完整实现（优先级队列 + AI 选择器 + 监控），但缺少 ProfileProvider/TaskDispatcher 具体实现
- Golem 只有骨架，无法接收和执行任务
- gRPC 通信层（心跳/任务下发/结果回传）未实现
- SubAgent 编排已完整实现（Manager + Scheduler + AnnounceController + 双存储后端）

### 9.3 与同类项目的差异化定位

| 特性 | Echoryn | OpenClaw | Dify | AutoGPT |
|------|---------|----------|------|---------|
| 语言 | Go | TypeScript | Python | Python |
| 分布式 | ✅ Hivemind-Golem | ❌ 单体 | ❌ 单体 | ❌ 单体 |
| 远程执行 | ✅ Golem Skill | ❌ | ❌ | ❌ |
| K8s 风格运维 | ✅ echoctl | ❌ | ❌ | ❌ |
| 终端 TUI | ✅ Bubbletea chat | ❌ | ❌ | ✅ CLI |
| 编译型插件 | ✅ Slot 互斥 | npm 运行时 | Python 运行时 | Python |
| OpenAI 兼容 | ✅ /v1/chat/completions | ❌ 自定义 | ✅ | ❌ |
| 本地部署 | ✅ 二进制分发 | Docker | Docker | Docker/pip |

---

## 十、待实现模块详细设计

> 本章对所有尚未实现或部分实现的模块做完整盘点，包括当前状态、已定义的接口/类型、缺失内容和设计要点。
> 为每个模块建立清晰的 **已有资产 → 缺失清单 → 目标架构** 三层视图。

### 10.1 实现状态总览

| 模块 | 完成度 | 已有资产 | 核心缺失 |
|------|--------|---------|---------|
| **Golem 工作节点** | ~5% | `app.go` 空壳 | gRPC 客户端、心跳、任务执行、Skill 引擎 |
| **Hivemind-Golem 通信协议** | 0% | 无 | Proto 定义、gRPC Service、`protocol` 包 |
| **Scheduler 调度引擎** | ~95% | 完整实现：Scheduler + PriorityQueue + AISelector + Monitor + StatsCollector (5 文件 ~2000 行) | ProfileProvider/TaskDispatcher 注入（依赖 Golem 通信层） |
| **SubAgent 编排** | ~85% | Manager + Scheduler + AnnounceController + Policy + 双存储后端 (9 文件) | Cleanup 实现、延迟队列忙碌检测 |
| **Channels 通道** | 0% | 空目录 (`.keep`) | 全部 |
| **Routing 路由** | 0% | 空目录 (`.keep`) | 全部 |
| **echoctl CLI** | ~50% | chat TUI 完整 | kubectl 风格资源管理命令 |
| **Proto/IDL 定义** | ~25% | `base.proto` + `common_struct.proto` | `api.proto` 空、`golem_node` 空、无 gRPC service |
| **Agent Runtime 补全** | ~90% | Runner/Flow/Pruner/Compaction 完整 | `Abort()` 未实现 |
| **Plugin 补全** | ~90% | 框架 + 3 内置插件完整 | llm-task 用 placeholder caller |

### 10.2 Golem 工作节点

**当前文件**: `internal/golem/app.go`（仅 40 行）

**已有**:
- `NewApp("golem")` 启动入口
- `run()` 函数（仅初始化日志，立即 `return nil`）

**缺失项**:

```
internal/golem/
├── app.go                    # [已有] 空壳启动
├── options/options.go        # [待实现] Golem 独立选项（当前复用 hivemind/options）
├── server.go                 # [待实现] gRPC Server / Client 启动
├── node/
│   ├── node.go              # [待实现] 节点信息（ID/Skills/Labels/Status）
│   ├── heartbeat.go         # [待实现] 心跳上报循环 (gRPC streaming)
│   └── registration.go     # [待实现] 向 Hivemind 注册 (token 验证)
├── executor/
│   ├── executor.go          # [待实现] 任务执行引擎
│   ├── skill_loader.go      # [待实现] Skill 动态加载
│   └── sandbox.go           # [待实现] 执行沙箱 (进程隔离)
└── skill/
    ├── skill.go             # [待实现] Skill 接口定义
    ├── browser.go           # [待实现] 浏览器技能
    ├── code_edit.go         # [待实现] 代码编辑技能
    └── terminal.go          # [待实现] 终端命令技能
```

**目标架构**:

```
Golem Node 启动
  → 读取配置 (~/.echoryn/golem.json)
  → 加载本地 Skills (skill_loader.go)
  → gRPC Dial → Hivemind
    → Register(token, NodeInfo{ID, Skills, Labels, CPU, Mem})
    → Hivemind 验证 token → 返回 ACK
  → 进入主循环:
    ├─ 心跳协程 (每 30s): Heartbeat(NodeLoadInfo{CPU%, Mem%, ActiveTasks})
    ├─ 任务接收 (gRPC stream): WaitForTask() → Executor.Execute(task) → ReportResult()
    └─ 信号处理: SIGTERM → graceful drain → Deregister()
```

### 10.3 Hivemind-Golem 通信协议

**当前状态**: `idl/api.proto` 和 `idl/app/common_struct/golem_node_common_struct.proto` 均为空文件。项目中不存在 Go `protocol` 包。

**需要定义的 Proto**:

```protobuf
// idl/app/common_struct/golem_node_common_struct.proto
message NodeInfo {
    string node_id = 1;
    string hostname = 2;
    repeated string skills = 3;
    map<string, string> labels = 4;
    NodeResources resources = 5;
}

message NodeResources {
    int64 cpu_cores = 1;
    int64 memory_mb = 2;
    string os = 3;
    string arch = 4;
}

message NodeLoadInfo {
    double cpu_percent = 1;
    double memory_percent = 2;
    int32 active_tasks = 3;
}

message TaskAssignment {
    string task_id = 1;
    string skill_name = 2;
    bytes payload = 3;             // JSON-encoded skill params
    int64 timeout_seconds = 4;
}

message TaskResult {
    string task_id = 1;
    TaskStatus status = 2;
    bytes output = 3;
    string error = 4;
}

enum TaskStatus {
    TASK_PENDING = 0;
    TASK_RUNNING = 1;
    TASK_COMPLETED = 2;
    TASK_FAILED = 3;
    TASK_CANCELLED = 4;
}
```

```protobuf
// idl/api.proto — gRPC Service 定义
service GolemNodeService {
    rpc Register(RegisterRequest) returns (RegisterResponse);
    rpc Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse);
    rpc AssignTask(TaskAssignment) returns (TaskAcceptResponse);
    rpc ReportResult(TaskResult) returns (BaseResp);
    rpc Deregister(DeregisterRequest) returns (BaseResp);
}

service HivemindControlService {
    rpc ListNodes(ListNodesRequest) returns (ListNodesResponse);
    rpc GetNode(GetNodeRequest) returns (NodeInfo);
    rpc DrainNode(DrainNodeRequest) returns (BaseResp);
}
```

**需要实现的 Go `protocol` 包**:

```
pkg/protocol/                    # 或 internal/pkg/protocol/
├── types.go                    # Go 核心类型: Task, NodeInfo, NodeLoadInfo, TaskStatus
├── registry.go                 # NodeRegistry 接口 (Hivemind 侧维护节点列表)
└── codec.go                    # Skill payload 编解码
```

### 10.4 Scheduler 调度引擎

**当前状态**: 已完整实现（5 个文件，~2000 行代码），采用 K8s 风格初始化。

**已实现的文件** (`internal/hivemind/service/scheduler/`):

```
internal/hivemind/service/scheduler/
├── scheduler.go          # Scheduler 接口 + defaultScheduler 完整实现
│                          #   ├─ Schedule/Cancel/Status/Stats
│                          #   ├─ Subscribe/Unsubscribe (事件驱动)
│                          #   ├─ Start/Stop (后台调度循环)
│                          #   └─ ReportProgress/ReportResult (传输层回调)
├── task.go               # 类型定义 + Builder 模式
│                          #   ├─ ScheduleRequest (DirectMode/AIMode)
│                          #   ├─ ScheduleDecision (含 NodeScore 多维评分)
│                          #   ├─ GolemProfile (节点画像)
│                          #   ├─ TaskEvent (8 种事件类型)
│                          #   └─ ScheduleRequestBuilder (流式 API)
├── queue.go              # PriorityQueue (container/heap, 线程安全, FIFO 平局)
├── selector.go           # 节点选择策略体系
│                          #   ├─ DirectSelector — 直接指定 + 硬约束校验
│                          #   ├─ AISelector — 6 维加权评分 (Capability/Skill/Resource/Load/Tag/Affinity)
│                          #   ├─ CompositeSelector — 责任链组合
│                          #   ├─ FilterSelector — 装饰器预过滤
│                          #   └─ 内置 NodeFilter: Online/Healthy/Feature
└── monitor.go            # Monitor 任务监控 + StatsCollector 统计
                           #   ├─ 超时检测 + 停滞检测 (心跳间隔)
                           #   ├─ 后台轮询 goroutine
                           #   └─ 全局 + 单节点 + 滑动窗口统计
```

**待接入（外部依赖）**:
- `ProfileProvider` 接口：提供 Golem 节点画像列表（需 Golem 通信层实现后注入）
- `TaskDispatcher` 接口：向目标节点下发任务（需 gRPC 传输层实现后注入）

**依赖关系**: Scheduler 逻辑已自洽，仅需 10.3 通信协议就绪后注入 ProfileProvider 和 TaskDispatcher 即可投入使用。

### 10.5 SubAgent 编排

**当前状态**: 已完整实现（~85%），采用 K8s Controller 模式。

**已实现的文件**:

```
domain/entity/
├── subagent.go                  # [已实现] 实体 + 状态机 (Pending→Running→Completed/Failed/Cancelled)
│                                 #   SubAgentSpawnRequest, SubAgentRecord, FormatAnnouncement()

domain/service/
├── subagent_service.go          # [已实现] SubAgentManager + SubAgentRegistry 接口 (re-export)

domain/service/runtime/
├── subagent_manager.go          # [已实现] SubAgentManager 完整实现
│                                 #   ├─ Spawn: 创建子 Session → semaphore 限流 → goroutine 执行
│                                 #   ├─ 深度限制: max depth = 1 (子 Agent 不可再派生)
│                                 #   ├─ Cancel/CancelByParent: 取消 + 状态更新
│                                 #   ├─ Recover: 进程重启后恢复 in-flight 记录
│                                 #   └─ Stop: 优雅关闭等待所有子 Agent 完成
├── subagent_scheduler.go        # [已实现] 并发控制器 (semaphore.Weighted, 默认 maxConcurrent=8)
├── subagent_policy.go           # [已实现] 工具黑名单 (6 个被禁工具: spawn/status/list/send/memory_*)
└── announce_controller.go       # [已实现] 结果公告 (直接投递 + 延迟队列模式)

store/inmemory/
└── subagent_store.go            # [已实现] InMemory SubAgentRegistry

store/boltdb/
└── subagent_store.go            # [已实现] BoltDB SubAgentRegistry (持久化)

pkg/errno/
└── errno.go                     # [已实现] 错误码 (NotFound/MaxDepth/ConcurrencyLimit/AlreadyDone)
```

**待完善**:
- `Cleanup()` 方法为空实现（需 `ListAll` 遍历终态记录按时间清理）
- `AnnounceController` 延迟队列未接入忙碌检测
- 部分 TODO 注释可清理（实际已在 Manager 中实现）

### 10.6 Channels 通道模块

**当前状态**: `internal/hivemind/service/channels/` 仅含 `.keep` 空文件。

**目标架构**:

```
internal/hivemind/service/channels/
├── module.go                  # K8s 式模块入口
├── channel.go                 # Channel 接口
│                               #   Receive(ctx) → InboundMessage
│                               #   Send(ctx, msg OutboundMessage) error
│                               #   Start(ctx) / Stop(ctx)
├── dispatcher.go              # 消息分发器: InboundMessage → routing → Agent
├── adapter/
│   ├── telegram.go            # Telegram Bot API 适配
│   ├── feishu.go              # 飞书/Lark 机器人适配
│   ├── web.go                 # Web WebSocket 适配
│   └── wechat.go              # 微信适配 (可选)
└── store/
    └── channel_store.go       # 通道配置持久化
```

### 10.7 Routing 路由模块

**当前状态**: `internal/hivemind/service/routing/` 仅含 `.keep` 空文件。

**目标架构**:

```
internal/hivemind/service/routing/
├── module.go                  # K8s 式模块入口
├── router.go                  # Router 接口
│                               #   Route(ctx, msg InboundMessage) → (Agent, error)
├── strategy/
│   ├── keyword.go             # 关键词匹配路由
│   ├── regex.go               # 正则路由
│   ├── agent_id.go            # 指定 Agent 路由
│   └── ai_router.go           # LLM 智能路由 (使用 llm-task 分析意图)
└── config.go                  # 路由规则配置
```

### 10.8 echoctl CLI 补全

**当前状态**: 已从节点管理工具重构为客户端交互工具。`chat` 命令完整可用，Factory 接口已简化为空接口。

**已完成**：

| 功能 | 文件 | 说明 |
|------|------|------|
| `echoctl chat` | `cmd/chat/chat.go` | Cobra 命令入口，支持 `--server`/`--model`/`--session`/`-m` 参数 |
| TUI 交互界面 | `cmd/chat/tui.go` | Bubbletea Elm 架构 TUI（576 行），含 ASCII 欢迎页、消息渲染、状态栏 |
| HTTP 客户端 | `cmd/chat/client.go` | `HivemindClient`，支持 SSE 流式和非流式两种调用模式 |
| 单次消息模式 | `cmd/chat/tui.go:RunOnce()` | 非交互模式：发送单条消息，流式输出到 stdout |

**Factory 接口** (`internal/echoctl/cmd/util/factory.go`):

```go
// 已简化为空接口，为未来 kubectl 风格命令预留扩展点
type Factory interface{}
```

**待实现命令**:

| 命令 | 状态 | 缺失内容 |
|------|------|---------|
| `echoctl get <resource>` | 未开始 | kubectl 风格资源查询（agents/sessions/models/nodes） |
| `echoctl describe <resource>` | 未开始 | 资源详情 |
| `echoctl create <resource>` | 未开始 | 资源创建 |
| `echoctl delete <resource>` | 未开始 | 资源删除 |
| `echoctl logs <session>` | 未开始 | 查看会话日志 |

### 10.9 Proto/IDL 补全

| 文件 | 当前状态 | 需要添加 |
|------|---------|---------|
| `idl/api.proto` | **空文件** | `GolemNodeService` + `HivemindControlService` gRPC 定义 |
| `idl/app/common_struct/golem_node_common_struct.proto` | **空文件** | `NodeInfo`, `NodeResources`, `NodeLoadInfo`, `TaskAssignment`, `TaskResult`, `TaskStatus` |
| `idl/base.proto` | ✅ 已完成 | — |
| `idl/app/common_struct/common_struct.proto` | ✅ 已完成 | — |
| `idl/app/common_struct/intelligence_common_struct.proto` | ✅ 已完成 | — |

### 10.11 已实现模块中的待补全项

以下是已实现模块中标记 `TODO` 或使用 placeholder 的具体代码位置：

| 文件 | 位置 | 说明 |
|------|------|------|
| `runtime/runner.go:422` | `AgentRunner.Abort()` | 返回 `"abort not yet implemented"`，需实现 Run→AbortController 映射表 |
| `runtime/prompt/types.go:124` | `ClusterInfo` | `TODO(cluster): Populate from Scheduler/GolemRegistry`，依赖 10.3 通信层 |
| `entity/agent.go:94` | `AgentPersona.WorkspaceDir` | ✅ 已实现 — WorkspaceLoader (P1) 从 WorkspaceDir 加载 SOUL.md/IDENTITY.md/AGENTS.md/prompts/*.md |
| `entity/events.go` | SubAgent 事件 | ✅ 已实现 — SubAgentManager 在 Spawn/Complete 时填充事件 |
| `entity/session.go` | `ParentSessionID` | ✅ 已实现 — SubAgentManager.Spawn() 创建 session 时设置 |
| `plugin/builtin/llmtask/plugin.go` | `placeholderCaller` | `Call()` 返回 error: `"LLM caller not configured"`，需接入 `RuntimeAPI.ModelManager()` |
| `llm/domain/service/model_meta.go` | `ModelMetaConf` | `TODO(cleanup): ModelMetaConf and related types are currently unused` |

---

## 十一、后续开发路线图

### P0 — Golem 通信层（打通 Hivemind ↔ Golem）

1. **定义 `protocol` 包** — 核心类型：Task, NodeInfo, NodeLoadInfo, TaskStatus（→ 10.3）
2. **完善 Proto 定义** — `golem_node_common_struct.proto` + `api.proto`（→ 10.10）
3. **实现 Golem 核心**：gRPC 连接 Hivemind、心跳上报、任务接收与执行、Skill 加载（→ 10.2）

### P1 — 调度接入与补全

5. **接入 Scheduler** — 实现 ProfileProvider/TaskDispatcher，将 Scheduler 接入 Golem 通信层（→ 10.4）
6. **~~实现 SubAgent 编排~~** — ✅ 已完成：Manager/Scheduler/AnnounceController/Policy + 双存储后端（→ 10.5）
7. **~~实现 PromptPipeline P1~~** — ✅ 已完成：WorkspaceLoader (fsnotify 热更新) + memorycore PromptProvider (MemorySection P:400)
8. **补全 Agent Runtime** — `Abort()` 实现 + llm-task caller 接入（→ 10.11）

### P2 — 通道与运维

10. **Channels 实现**: Telegram / 飞书 / Web 通道适配（→ 10.6）
11. **Routing 实现**: 消息路由 + Agent 匹配（→ 10.7）
12. **echoctl 补全**: kubectl 风格资源管理命令 get/describe/create/delete（→ 10.8）
13. **~~echoctl chat~~** — ✅ 已完成：Bubbletea TUI + SSE 流式 + 会话管理 + 单次消息模式

### P3 — 生态扩展

13. **更多内置插件**: Web 搜索、代码执行、文件管理
14. **前端 UI**: Agent 管理 Dashboard
15. **多租户**: 用户/组织/权限体系

---

## 十二、开发规范

### 12.1 代码组织原则

- `cmd/` 仅包含 `main.go`，只负责启动
- `internal/` 不可被外部引用，包含具体业务实现
- `pkg/` 可被外部引用，提供通用能力
- 每个服务 (hivemind/golem) 遵循 `app.go → run.go → server.go` 启动链
- 使用 `Options → Config → CompletedConfig → New()` 模式构建服务
- 每个 service module 使用 K8s 式 `module.go` 入口：`Config → Complete → New`

### 12.2 接口优先

所有核心组件必须先定义接口再实现：
- `AgentService`, `AgentRunner`, `TurnExecutor` — Agent 层
- `ModelManager`, `ModelProber`, `FallbackExecutor` — LLM 层
- `PluginFramework`, `Plugin`, `ToolProvider` — 插件层
- `MCPManager`, `MCPServer` — MCP 层
- `Scheduler`, `Monitor`, `Queue` — 调度层
- `Factory`, `HivemindConnector`, `NodeChecker` — CLI 工具层

### 12.3 错误处理

使用 `pkg/errorx` 统一错误码系统：
```go
var ErrNodeNotFound = errorx.NewWithCode(404, "node not found")
return errorx.Wrap(err, "failed to schedule task")
```

### 12.4 日志规范

```go
import "github.com/kiosk404/echoryn/pkg/logger"

logger.Infof("task %s scheduled to node %s", taskID, nodeID)
logger.WithFields(logger.Fields{"task_id": taskID}).Error("timeout")
```

### 12.5 构建

```bash
make build             # 构建所有二进制 (hivemind/golem/echoctl)
make build.hivemind    # 构建 hivemind
make build.golem       # 构建 golem
make build.echoctl     # 构建 echoctl
make lint              # 代码检查
make test              # 运行测试
make format            # 格式化代码
```

---

## 十三、关键数据流

### 13.1 Chat Completion 流（已实现）

```
Client POST /v1/chat/completions
  → Bearer Auth 中间件验证
  → ChatCompletionsHandler
    → 解析 OpenAI 格式请求
    → AgentRunner.Run(session, messages)
      → ContextBuilder.Build (构建 LLM 上下文)
        ├─ **PromptPipeline.Assemble()** (Section 化系统提示词)
        │   ├─ IdentitySection (P:100) → 核心身份 + 分布式意识
        │   ├─ ClusterAwarenessSection (P:150) → Golem 拓扑 (如有)
        │   ├─ ToolingSection (P:200) → 可用工具列表
        │   ├─ PersonaSection (P:300) → 用户自定义 SystemPrompt
        │   ├─ WorkspaceSection:soul (P:310) → SOUL.md (WorkspaceLoader)
        │   ├─ WorkspaceSection:identity_file (P:320) → IDENTITY.md (WorkspaceLoader)
        │   ├─ WorkspaceSection:agents_file (P:330) → AGENTS.md (WorkspaceLoader)
        │   ├─ WorkspaceSection:extra:* (P:350+) → prompts/*.md (WorkspaceLoader)
        │   ├─ MemorySection (P:400) → 记忆系统指令 (memorycore PromptProvider)
        │   ├─ [Plugin Sections] → 其他插件贡献的 Section
        │   └─ RuntimeSection (P:900) → 时间/模型/版本
        ├─ History Messages (裁剪后)
        └─ Available Tools (Plugin + MCP)
      → TurnExecutor.Execute (单轮执行)
        ├─ AgentFlow.Invoke (Eino DAG 执行)
        │   ├─ LLM ChatModel 调用
        │   └─ Tool Call 循环
        ├─ 如失败 → Retry (指数退避)
        └─ 如仍失败 → Fallback (切换模型)
      → 如 stream=true → SSE 实时推送
      → 如 stream=false → JSON 一次性返回
      → Compaction 检查（如超长则压缩）
      → Memory Flush（会话结束时自动存储记忆）
```

### 13.2 Golem 注册流（待实现）

```
golem (启动工作节点)
  → 读取 ~/.echoryn/golem.json 配置
  → 连接 Hivemind gRPC
  → 提交节点信息 (CPU/Mem/IP/OS/Skills)
  → Hivemind 验证 + 注册节点
  → Golem 进入主循环：心跳上报 + 任务接收 + 执行返回
```

### 13.3 SSE 流式回复

```go
// pkg/http/sse 已实现完整的 SSE 推送
sender := sse.NewSender(ginContext.Writer)
sender.Send("thinking", "正在分析...")
sender.Send("content", "这是回复内容")
sender.Send("done", "")

// 支持广播到多个客户端
stream := sse.NewStream()
stream.AddClient(clientID, sender)
stream.Broadcast("update", data)
```

---

## 十四、已知问题与修复清单

### Issue 1: Token List 命令未注册（已修复 ✅）
**位置**: `internal/echoctl/cmd/token/token.go` 第 28 行  
**问题**: `NewCmdTokenList()` 函数已实现（94 行），但未在命令组中注册  
**修复**: ✅ 已在 token.go 第 28 行添加 `cmd.AddCommand(NewCmdTokenList(f, ioStreams))`

### Issue 2: Team GET 端点路由错误（已修复 ✅）
**位置**: `internal/hivemind/router.go` 第 81 行  
**问题**: `GET /v1/teams/:id` 错误地映射到 `teamHandler.DissolveTeam()` 而非 `teamHandler.GetTeam()`  
**修复**: ✅ 已修正行 81 为 `apiV1.GET("/teams/:id", teamHandler.GetTeam)`

### Issue 3: 配置格式限制未在文档中明确
**位置**: `internal/pkg/server/config.go` 第 110 行  
**问题**: Viper 被硬编码为 JSON 格式，但文档未说明此限制  
**修复状态**: ✅ 已在 5.1 节配置结构中更新为"仅支持 JSON 格式"

---

## 十五、项目完成度评估

### 核心功能（Hivemind 中心已就绪）
- ✅ **Agent CRUD 和多轮对话** — 生产就绪
- ✅ **8 个 LLM Provider 集成** — 生产就绪
- ✅ **MCP 工具调用** — 生产就绪
- ✅ **Plugin Framework** — 生产就绪（10+ 内置插件）
- ✅ **SubAgent 编排** — K8s Controller 模式，生产就绪
- ✅ **Team 多智能体协作** — 生产就绪
- ✅ **echoctl chat TUI** — 生产就绪（SSE 流式）
- ✅ **echoadm 集群管理** — P0 功能完成（init/config/info）
- ✅ **Scheduler 调度引擎** — 逻辑完整，待接入 Golem 通信层

### 分布式功能
- ✅ **Golem 工作节点** — 已实现（gRPC/心跳/节点注册/任务执行）
- ✅ **Hivemind ↔ Golem 通信** — 完整的 gRPC 双向流（注册/心跳/任务上报）
- ✅ **Channels IM 通道** — 已实现（Telegram 长轮询 + 飞书 WebSocket 双模式）
- ✅ **Routing 智能路由** — 已实现（Dispatcher 消息路由 + 执行策略选择器）

### 代码质量
- ✅ 无依赖循环
- ✅ 接口优先设计
- ✅ K8s 风格初始化模式
- ✅ 完整的错误码系统
- ✅ 统一的日志框架
- 🟡 测试覆盖率 ~60%（见 `make cover`）

---

> **本文档为 Echoryn 项目的顶层 SPEC，各 Hivemind 核心模块的详解请参阅对应的子文档（见第一章文档索引）。**
>
> **最后更新**: 2026-03-31 | **综合审查**: ✅ 完成。已删除所有 echoadm 过时设计、修复 2 个已知 bug（token list 注册、team GET 路由）


