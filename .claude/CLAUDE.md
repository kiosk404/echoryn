# Echoryn — 开源分布式 AI 虚拟角色容器平台

Echoryn 是用 Go 编写的分布式 AI Agent 基础设施。不同于单体式 AI 框架，Echoryn 将**推理决策**与**任务执行**分离——Hivemind（蜂巢智心）负责思考与调度，Golem（傀儡）负责在边缘执行技能。一个统一的 AI 心智同时驱动分布在不同位置的多个执行体。

## 三大组件

- **Hivemind**：中央服务器，承载 Agent 运行时、LLM 调用、插件系统、团队协作、MCP 工具协议、Golem 调度等全部核心逻辑
- **Golem**：无自主意识的工作节点，通过 gRPC 双向流接收 Hivemind 下发的任务，在边缘执行技能（Shell、浏览器等）
- **Echoctl**：CLI 管理工具（BubbleTea TUI），通过 HTTP/SSE 与 Hivemind 交互

## 核心特性

- **Skills 技能系统**：Markdown 定义，按需渐进加载，内置 Shell 执行、文件操作等技能，支持 MCP Server 扩展第三方工具
- **Sub-Agents 子智能体**：复杂任务拆解为多个子智能体并行处理，独立上下文和生命周期
- **Team 团队协作**：多 Agent 协同框架，YAML 模板定义团队结构，内置并行/流水线/辩论/Leader 驱动等协作策略，MessageBus 异步通信
- **Plugin 插件框架**：Kubernetes 风格编译时插件，Slot 互斥机制，支持 Tool/Hook/Service/CLI/PromptSection 五种能力注入
- **Context Engineering**：两阶段裁剪（ContextBuilder → ContextPruner）+ Compaction 多轮摘要压缩 + TokenEstimator 精确估算
- **Memory 长期记忆**：SQLite FTS5 + 向量搜索混合检索，跨 Session 积累偏好与习惯，OpenAI/Gemini 双 Embedding Provider
- **LLM 多模型管理**：SPI 四层架构（Provider → ChatModel → Compat → Probe），8 个 Provider（OpenAI/Claude/DeepSeek/Gemini/Ollama/Qwen/GLM/Kimi），健康探测 + 自动 Fallback 降级
- **IM 渠道集成**：飞书（WebSocket）、Telegram（Bot API）、Web Chat（OpenAI 兼容接口）
- **可观测性**：Diagnostics 插件（指标收集 + LLM span 追踪）+ Langfuse 插件（LLM 全链路追踪 + Token 成本分析）

## 快速导航

| 类别 | 文档 |
|------|------|
| 项目总体规范 | [docs/ECHORYN_SPEC.md](docs/ECHORYN_SPEC.md) |
| 智能体引擎 | [docs/ECHORYN_HIVEMIND_AGENTS.md](docs/ECHORYN_HIVEMIND_AGENTS.md) |
| 插件框架 | [docs/ECHORYN_HIVEMIND_PLUGIN.md](docs/ECHORYN_HIVEMIND_PLUGIN.md) |
| LLM 多模型管理 | [docs/ECHORYN_HIVEMIND_LLM.md](docs/ECHORYN_HIVEMIND_LLM.md) |
| 记忆系统 | [docs/ECHORYN_HIVEMIND_MEMORY.md](docs/ECHORYN_HIVEMIND_MEMORY.md) |
| Golem 工作节点 | [docs/ECHORYN_GOLEM_SPEC.md](docs/ECHORYN_GOLEM_SPEC.md) |
| 团队协作 | [docs/ECHORYN_TEAM_SPEC.md](docs/ECHORYN_TEAM_SPEC.md) |
| 上下文管理研究 | [docs/BLOG_CONTEXT_SPEC.md](docs/BLOG_CONTEXT_SPEC.md) |
| 子智能体研究 | [docs/BLOG_SUBAGENT_SPEC.md](docs/BLOG_SUBAGENT_SPEC.md) |
| Harness 实践指南 | [docs/HARNESS_ENGINEERING_GUIDE.md](docs/HARNESS_ENGINEERING_GUIDE.md) |
| 开发任务清单 | [docs/TODO_SPEC.md](docs/TODO_SPEC.md) |

## 架构分层（依赖规则：只能向下依赖）

```
cmd/                        → internal/, pkg/         入口层
internal/hivemind/handler/  → internal/hivemind/service/  请求处理 → 业务服务
internal/hivemind/service/  → domain/                     应用服务 → 领域层
domain/service/             → domain/repo/, domain/entity/ 领域服务 → 仓储+实体
domain/entity/              ← domain/repo/                实体 ← 仓储接口（接口依赖实体）
pkg/                        ← internal/, cmd/             公共库被上层引用
```

横切关注点（所有层均可使用）：`pkg/logger`（日志）、`pkg/errorx`（错误码）、`pkg/paths`（路径解析）、`pkg/utils`（工具函数）

**禁止**：`pkg/` 不得导入 `internal/`；`handler/` 不得直接导入 `store/`；`domain/entity/` 不得导入 `domain/service/`。

## Hivemind 初始化流程

服务启动通过 `InitializerChain` 按序执行（见 `internal/hivemind/initializers.go`）：

```
InitInfrastructure → InitGolem → InitLLM → InitMCP → InitAgents → InitPluginLifecycle
```

每个 `InitFunc` 接收共享的 `Dependencies` 结构体，完成后将模块注入其中。

## Agents 模块（核心，DDD 分层）

位置：`internal/hivemind/service/agents/`

- **入口**：`module.go`，遵循 Config → Complete() → New() 模式
- **领域层**：`domain/entity/`（Agent, Session, Run, SubAgent, Message, Event）、`domain/repo/`（仓储接口）、`domain/service/`（AgentService 接口：CRUD + Run 执行）
- **运行时引擎**：`domain/service/runtime/`，核心组件包括 AgentRunner（执行中枢）、executor（工具调用）、context_builder（上下文构建）、compaction（压缩）、context_pruner（裁剪）、agentflow（Eino DAG 编排）、subagent（子智能体）、prompt（提示词管道）、toolloop（工具循环熔断）
- **持久化**：`store/`（BoltDB / 内存实现）
- **错误码**：`pkg/errno/`

**执行流程**：`RunRequest` → 解析/创建 Session → 构建上下文 → Eino 调用 LLM → 处理工具调用 → 流式返回 `AgentEvent`

## 插件框架

位置：`internal/hivemind/service/plugin/`

- **Plugin**：基础接口，只需 `Name() string`
- **InitPlugin**：通过 `PluginAPI` 注册 Tool/CLI/Hook/Service
- **LifecyclePlugin**：拥有 `Start(ctx)` / `Stop(ctx)` 生命周期
- **插槽机制**（`slots.go`）：确保同类型只有一个激活插件（如 `memory` 插槽绑定 `memory-core`）

内置插件：

| 插件 | Slot | 功能 |
|------|------|------|
| `memory-core` | `memory` | 核心记忆（SQLite FTS5 + 向量搜索 + 混合检索） |
| `diagnostics` | — | 运维级可观测性（指标收集 + LLM span 追踪） |
| `langfuse-tracing` | — | LLM 全链路追踪（Langfuse 集成） |
| `llm-task` | — | LLM 子任务执行 |
| `subagent` | — | 子智能体管理 |
| `skills` | — | 技能加载管理 |
| `golem-cluster` | — | Golem 集群管理 |
| `channel-feishu` | `channel` | 飞书 IM 渠道 |
| `channel-telegram` | `channel` | Telegram IM 渠道 |
| `web-search` | — | Web 搜索（Gemini） |

## 构建命令

```bash
make all        # tidy + format + lint + build（默认目标）
make build      # 构建所有二进制（hivemind, golem, echoctl）
make test       # 运行单元测试 + 覆盖率
make cover      # 测试 + 覆盖率阈值检查（≥60%）
make lint       # golangci-lint 代码检查
make format     # gofmt + goimports + golines
make proto      # Protobuf 代码生成
make run        # 运行 hivemind（开发模式）
make run.%      # 运行指定二进制，如 make run.golem
make clean      # 清理 output/
```

## 代码约定

- **初始化模式**：所有模块遵循 K8S 风格链式初始化：`cfg := &module.Config{...}` → `completed := cfg.Complete()` → `module, err := completed.New()`
- **文件名**：小写下划线 `agent_service.go`
- **接口**：动词或 -er 后缀 `Manager`, `Repository`, `Plugin`
- **插件名**：DNS 兼容，小写连字符 `memory-core`, `web-search`
- **错误码**：统一使用 `pkg/errorx`
- **日志**：统一使用 `pkg/logger`（不要直接用 `log` 或 `fmt` 打印日志）
- **路径**：统一使用 `pkg/paths` 解析 `~/.echoryn` 状态目录
- **配置文件**：`conf/hivemind-server.json`（服务配置）、`conf/golem-worker.json`（Golem 配置）、`conf/mcp.json`（MCP 配置）
- **环境变量**：API Key 通过 `${DEEPSEEK_API_KEY}` 等占位符引用

## 依赖技术栈

| 类别 | 技术 |
|------|------|
| AI/LLM 框架 | CloudWeGo Eino + 多模型扩展 |
| LLM 追踪 | Langfuse (eino-ext/callbacks/langfuse) |
| MCP | mark3labs/mcp-go |
| Web | Gin + pprof + SSE |
| RPC | gRPC + Protobuf |
| CLI | Cobra + Pflag + Viper |
| TUI | BubbleTea + Glamour + LipGloss |
| 存储 | SQLite3 (FTS5) + BoltDB + BBolt |
| IM | 飞书 SDK (oapi-sdk-go) · Telegram Bot |
| 日志 | Logrus |
| JSON | Bytedance Sonic |

## gRPC 服务

- `GolemNodeService`：节点注册、心跳、任务流
- `HivemindAdminService`：令牌管理、集群管理
- 默认端口：`11788`（gRPC）、`11789`（HTTP）
