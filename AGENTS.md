# Echoryn Agent Guide

Echoryn 是用 Go 编写的分布式 AI 虚拟角色容器平台。
架构模型：**Hivemind**（蜂巢智心，中央大脑）+ **Golem**（傀儡，可更换的工作节点）+ **Echoctl**（CLI 管理工具）。

## 快速导航

| 类别 | 文档 |
|------|------|
| 项目总体规范 | [docs/ECHORYN_SPEC.md](docs/ECHORYN_SPEC.md) |
| 智能体引擎 | [docs/ECHORYN_HIVEMIND_AGENTS_SPEC.md](docs/ECHORYN_HIVEMIND_AGENTS_SPEC.md) |
| 插件框架 | [docs/ECHORYN_HIVEMIND_PLUGIN_SPEC.md](docs/ECHORYN_HIVEMIND_PLUGIN_SPEC.md) |
| LLM 多模型管理 | [docs/ECHORYN_HIVEMIND_LLM_SPEC.md](docs/ECHORYN_HIVEMIND_LLM_SPEC.md) |
| 记忆系统 | [docs/ECHORYN_HIVEMIND_MEMORY_SPEC.md](docs/ECHORYN_HIVEMIND_MEMORY_SPEC.md) |
| Golem 工作节点 | [docs/ECHORYN_GOLEM_SPEC.md](docs/ECHORYN_GOLEM_SPEC.md) |
| 团队协作 | [docs/ECHORYN_TEAM_SPEC.md](docs/ECHORYN_TEAM_SPEC.md) |
| 上下文管理研究 | [docs/BLOG_CONTEXT_SPEC.md](docs/BLOG_CONTEXT_SPEC.md) |
| 子智能体研究 | [docs/BLOG_SUBAGENT_SPEC.md](docs/BLOG_SUBAGENT_SPEC.md) |
| Harness 实践指南 | [docs/HARNESS_ENGINEERING_GUIDE.md](docs/HARNESS_ENGINEERING_GUIDE.md) |
| 开发任务清单 | [docs/TODO_SPEC.md](docs/TODO_SPEC.md) |

## 目录结构

```
echoryn/
├── cmd/                          # 可执行入口
│   ├── hivemind/                 #   中央服务器
│   ├── golem/                    #   工作节点
│   └── echoctl/                  #   CLI 管理工具
├── internal/                     # 内部实现（不对外暴露）
│   ├── hivemind/                 #   Hivemind 核心
│   │   ├── handler/              #     请求处理层（gRPC + HTTP）
│   │   ├── service/              #     业务服务层
│   │   │   ├── agents/           #       智能体模块（DDD）
│   │   │   ├── llm/              #       LLM 多模型管理
│   │   │   ├── plugin/           #       插件框架
│   │   │   ├── team/             #       团队协作
│   │   │   ├── golem/            #       Golem 节点管理
│   │   │   ├── mcp/              #       MCP 工具协议
│   │   │   ├── gateway/          #       IM 渠道网关
│   │   │   ├── messagebus/       #       消息总线
│   │   │   └── subagent/         #       子智能体基础设施
│   │   ├── config/               #     配置加载
│   │   ├── options/              #     命令行选项
│   │   ├── initializers.go       #     初始化链
│   │   ├── server.go             #     服务器组装
│   │   └── router.go             #     HTTP 路由
│   ├── golem/                    #   Golem 工作节点实现
│   ├── echoctl/                  #   CLI 实现
│   └── pkg/                      #   内部共享包
├── pkg/                          # 公共库（可被外部引用）
│   ├── app/                      #   应用框架（cobra 封装）
│   ├── cli/                      #   CLI TUI 框架（BubbleTea）
│   ├── errorx/                   #   错误码系统
│   ├── http/                     #   HTTP/SSE/Shutdown 工具
│   ├── logger/                   #   日志系统（logrus）
│   ├── paths/                    #   路径解析（~/.echoryn）
│   ├── proto/                    #   protobuf 生成代码
│   ├── skills/                   #   技能加载/解析
│   ├── utils/                    #   通用工具函数
│   └── version/                  #   版本管理
├── idl/                          # Protobuf 协议定义
├── conf/                         # 配置文件示例
├── docs/                         # 项目文档
└── scripts/                      # 构建脚本
```

## 架构分层（依赖规则：只能向下依赖）

```
cmd/                        → internal/, pkg/         入口层
internal/hivemind/handler/  → internal/hivemind/service/  请求处理 → 业务服务
internal/hivemind/service/  → domain/                     应用服务 → 领域层
domain/service/             → domain/repo/, domain/entity/ 领域服务 → 仓储+实体
domain/entity/              ← domain/repo/                实体 ← 仓储接口（接口依赖实体）
pkg/                        ← internal/, cmd/             公共库被上层引用

横切关注点（所有层均可使用）：
  pkg/logger     日志
  pkg/errorx     错误码
  pkg/paths      路径解析
  pkg/utils      工具函数
```

**禁止**：`pkg/` 不得导入 `internal/`；`handler/` 不得直接导入 `store/`；`domain/entity/` 不得导入 `domain/service/`。

## 核心模块指南

### Hivemind 初始化流程

服务启动通过 `InitializerChain` 按序执行（见 `internal/hivemind/initializers.go`）：

```
InitInfrastructure → InitGolem → InitLLM → InitMCP → InitAgents → InitPluginLifecycle
```

每个 `InitFunc` 接收共享的 `Dependencies` 结构体，完成后将模块注入其中。

### Agents 模块（最核心，DDD 分层）

位置：`internal/hivemind/service/agents/`

```
agents/
├── module.go                   Config → Complete() → New() 入口
├── domain/
│   ├── entity/                 Agent, Session, Run, SubAgent, Message, Event
│   ├── repo/                   AgentRepository, SessionRepository, RunRepository
│   └── service/
│       ├── agent_service.go    AgentService 接口（CRUD + Run 执行）
│       └── runtime/            运行时引擎
│           ├── runner.go       AgentRunner — 执行中枢（33KB）
│           ├── executor.go     工具调用执行器
│           ├── context_builder.go  上下文构建
│           ├── compaction.go   上下文压缩（token 超限时摘要化）
│           ├── context_pruner.go   消息裁剪
│           ├── agentflow/      Eino 流编排（LLM 调用链）
│           ├── subagent/       子智能体管理器
│           ├── prompt/         提示词管道
│           └── toolloop/       工具循环检测（熔断器）
├── store/
│   ├── boltdb/                 BoltDB 持久化实现
│   └── inmemory/               内存存储实现
└── pkg/errno/                  模块错误码
```

**执行流程**：`RunRequest` → 解析/创建 Session → 构建上下文 → Eino 调用 LLM → 处理工具调用 → 流式返回 `AgentEvent`。

### LLM 多模型管理

位置：`internal/hivemind/service/llm/`

- **Provider SPI**：`llm/provider/spi/spi.go` 定义 `ProviderPlugin`、`ChatModelPlugin`、`CompatPlugin`、`ProbePlugin` 接口
- **已接入**：OpenAI、Claude、DeepSeek、Gemini、Ollama、Qwen、GLM、Kimi
- **ModelManager**：统一的模型获取/降级/健康检查接口
- **Thinking 策略**：`llm/provider/thinking/` 按模型类型处理思维链

新增 LLM Provider 只需：实现 `spi.ChatModelPlugin` → 在 `llm/provider/registry.go` 注册。

### 插件框架

位置：`internal/hivemind/service/plugin/`

**核心接口**（`types.go`）：
- `Plugin`：基础接口，只需 `Name() string`
- `InitPlugin`：扩展，可通过 `PluginAPI` 注册 Tool/CLI/Hook/Service
- `LifecyclePlugin`：扩展，有 `Start(ctx)` / `Stop(ctx)` 生命周期

**PluginAPI** 注册能力（`api.go`）：
- `RegisterTool(tool)`：注册 Agent 可调用的工具
- `RegisterCLI(registrar)`：注册 echoctl 子命令
- `RegisterHook(event, handler)`：注册生命周期钩子
- `RegisterService(svc)`：注册后台服务

**插槽机制**（`slots.go`）：确保某类型只有一个激活插件（如 `memory` 插槽绑定 `memory-core`）。

**内置插件**（`plugin/builtin/`）：
| 插件 | 功能 |
|------|------|
| `memory-core` | 核心记忆（SQLite FTS5 + 向量搜索 + 混合检索） |
| `diagnostics` | OpenTelemetry 追踪 |
| `llm-task` | LLM 子任务执行 |
| `subagent` | 子智能体管理工具 |
| `skills` | 技能加载管理 |
| `golem-cluster` | Golem 集群管理工具 |
| `channel-feishu` | 飞书 IM 渠道 |
| `channel-telegram` | Telegram IM 渠道 |
| `web-search` | Web 搜索（Gemini） |

### Team 团队协作

位置：`internal/hivemind/service/team/`

- `TeamOrchestrator`：团队编排（创建/解散/成员管理）
- `TeamTemplateService`：团队模板管理
- `EventBridge`：SubAgent 生命周期事件桥接
- `MessageBus`：Agent 间异步消息传递

### Golem 分布式调度

位置：`internal/hivemind/service/golem/`

- `registry/`：节点注册中心（心跳、健康检查）
- `scheduler/`：任务调度器（选择器、队列、监控）
- `dispatcher/`：gRPC 双向流任务分发
- `tokenmanager/`：认证令牌管理

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

### 初始化模式

所有模块遵循 K8S 风格链式初始化：

```go
cfg := &module.Config{...}
completed := cfg.Complete()     // 填充默认值、校验
module, err := completed.New()  // 创建实例
```

### 命名规范

- **文件名**：小写下划线 `agent_service.go`
- **接口**：动词或 -er 后缀 `Manager`, `Repository`, `Plugin`
- **插件名**：DNS 兼容，小写连字符 `memory-core`, `web-search`
- **错误码**：统一使用 `pkg/errorx`
- **日志**：统一使用 `pkg/logger`（不要直接用 `log` 或 `fmt` 打印日志）
- **路径**：统一使用 `pkg/paths` 解析 `~/.echoryn` 状态目录

### 配置文件

- `conf/hivemind-server.json`：Hivemind 服务配置（gRPC、HTTP、模型、插件）
- `conf/golem-worker.json`：Golem 连接配置（地址、心跳、并发数）
- `conf/mcp.json`：MCP 服务器配置
- 环境变量：API Key 通过 `${DEEPSEEK_API_KEY}` 等占位符引用

### 依赖技术栈

| 类别 | 技术 |
|------|------|
| AI/LLM 框架 | CloudWeGo Eino + 多模型扩展 |
| MCP | mark3labs/mcp-go |
| Web | Gin + pprof + SSE |
| RPC | gRPC + Protobuf |
| CLI | Cobra + Pflag + Viper |
| TUI | BubbleTea + Glamour + LipGloss |
| 存储 | SQLite3 (FTS5) + BoltDB + BBolt |
| IM | 飞书 SDK (oapi-sdk-go) |
| 日志 | Logrus |
| 可观测 | OpenTelemetry |
| JSON | Bytedance Sonic |

## 运行时状态目录

Echoryn 的运行时状态统一存储在 `~/.echoryn/`（可通过 `--data-dir` 自定义）：

```
~/.echoryn/
├── agents/{id}/sessions/     # Agent 会话数据（BoltDB）
├── credentials/              # 认证凭据
├── memory/                   # 记忆数据库
├── workspace/                # Agent 工作空间
│   ├── IDENTITY.md           # Agent 身份定义
│   ├── SOUL.md               # Agent 灵魂设定
│   └── memory/               # 工作空间记忆
└── hivemind-server.json      # 运行时配置
```

## HTTP API（兼容 OpenAI 格式）

```
POST   /v1/chat/completions          流式对话（SSE）
GET    /v1/models                     模型列表

POST   /v1/agents                     创建 Agent
GET    /v1/agents                     列出 Agent
GET    /v1/agents/:id                 获取 Agent
DELETE /v1/agents/:id                 删除 Agent

GET    /v1/agents/:id/sessions        列出 Agent 的会话
GET    /v1/sessions/:id               获取会话
DELETE /v1/sessions/:id               删除会话

GET    /v1/teams/templates            列出团队模板
POST   /v1/teams                      创建团队
GET    /v1/teams/:id                  获取团队
DELETE /v1/teams/:id                  解散团队
POST   /v1/teams/:id/messages         发送团队消息
```

## gRPC 服务

- `GolemNodeService`：节点注册、心跳、任务流
- `HivemindAdminService`：令牌管理、集群管理
- 默认端口：`11788`（gRPC）、`11789`（HTTP）
