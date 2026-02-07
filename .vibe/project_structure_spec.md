# Ultronix — 项目目录结构规范

> 本文档定义 Ultronix 的完整目录结构，作为 AI Agent spec code 的唯一依据。
> 遵循 Go 社区工程规范（[golang-standards/project-layout](https://github.com/golang-standards/project-layout)）。
> 灵感来源于 OpenClaw（TypeScript 版），但 Ultronix 是独立的分布式架构。

---

## 零、核心概念

Ultronix 由三个独立模块构成：

| 模块 | 角色 | 比喻 |
|------|------|------|
| **ultron_cli** | 命令行工具 | 遥控器 — 初始化节点、管理集群、与 HiveMind/Golem 交互 |
| **HiveMind** | 中控服务 | 大脑 — 调度算力、中心知识库、MCP Hub、任务编排 |
| **Golem** | 工作节点 | 肢体 — 安装 Skills、被 HiveMind 调度执行具体操作 |

### 运行模式

```
模式 A: 单机部署（HiveMind + Golem 同机）
┌──────────────────────────────────────┐
│              单台机器                 │
│  ┌──────────┐    ┌──────────────┐    │
│  │ HiveMind │◄──►│    Golem     │    │
│  │ (中控)    │    │ (本地执行)    │    │
│  └──────────┘    └──────────────┘    │
│        ▲                             │
│        │ gRPC                        │
│  ┌─────┴──────┐                      │
│  │ ultron_cli │                      │
│  └────────────┘                      │
└──────────────────────────────────────┘

模式 B: 分布式部署
┌──────────────┐
│  ultron_cli  │──────┐
└──────────────┘      │
                      ▼
              ┌──────────────┐
              │   HiveMind   │ (中控节点)
              │  知识库 / MCP │
              └──────┬───────┘
                     │ gRPC
          ┌──────────┼──────────┐
          ▼          ▼          ▼
   ┌──────────┐ ┌──────────┐ ┌──────────┐
   │ Golem A  │ │ Golem B  │ │ Golem C  │
   │ (shell)  │ │ (browser)│ │ (coding) │
   └──────────┘ └──────────┘ └──────────┘

模式 C: 纯对话（无 Golem）
┌──────────────┐      ┌──────────────┐
│  ultron_cli  │─────►│   HiveMind   │  只有对话/知识库能力
└──────────────┘      └──────────────┘  无法执行操作
```

---

## 一、顶层目录总览

```
ultronix/
├── cmd/                        # 可执行入口（3 个 binary）
│   ├── ultron/                 #   CLI 工具
│   ├── hivemind/               #   中控服务
│   └── golem/                  #   工作节点
│
├── internal/                   # 私有应用代码
│   ├── cli/                    #   CLI 框架层（ultron_cli 专属）
│   ├── hivemind/               #   HiveMind 核心逻辑
│   ├── golem/                  #   Golem 核心逻辑
│   └── shared/                 #   三模块共享代码
│
├── pkg/                        # 公开库代码（可被外部项目导入）
│   ├── protocol/               #   HiveMind ↔ Golem 通信协议
│   └── pluginsdk/              #   插件开发 SDK
│
├── api/                        # API 协议定义（protobuf / OpenAPI / JSON Schema）
├── skills/                     # Agent 技能定义（Markdown + 脚本）
├── configs/                    # 默认配置模板
├── scripts/                    # 构建、部署、CI 脚本
├── deployments/                # 部署配置（Docker、systemd 等）
├── test/                       # 集成测试、E2E 测试
├── docs/                       # 项目文档
├── assets/                     # 静态资源
│
├── go.mod                      # Go module 定义
├── go.sum                      # 依赖校验
├── Makefile                    # 构建入口
├── .goreleaser.yml             # GoReleaser 发布配置
└── .golangci.yml               # golangci-lint 配置
```

---

## 二、`cmd/` — 三个可执行入口

Go 标准约定：每个 `cmd/<name>/main.go` 编译为一个独立二进制。

```
cmd/
├── ultron/                     # CLI 工具 (binary: ultron)
│   └── main.go
├── hivemind/                   # 中控服务 (binary: hivemind)
│   └── main.go
└── golem/                      # 工作节点 (binary: golem)
    └── main.go
```

### `cmd/ultron/main.go` — CLI 入口

```go
package main

import (
    "os"
    "github.com/kiosk404/ultronix/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

用户使用方式：
```bash
ultron hivemind start              # 启动 HiveMind 中控
ultron golem init                  # 初始化当前机器为 Golem 节点
ultron golem join <hivemind-addr>  # 加入 HiveMind 集群
ultron skills install github       # 为 Golem 安装 skill
ultron chat "你好"                 # 直接对话（走 HiveMind）
ultron status                      # 查看集群状态
```

### `cmd/hivemind/main.go` — 中控服务入口

```go
package main

import (
    "context"
    "os/signal"
    "syscall"

    "github.com/kiosk404/ultronix/internal/hivemind"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    server := hivemind.NewServer()
    if err := server.Run(ctx); err != nil {
        os.Exit(1)
    }
}
```

### `cmd/golem/main.go` — 工作节点入口

```go
package main

import (
    "context"
    "os/signal"
    "syscall"

    "github.com/kiosk404/ultronix/internal/golem"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    node := golem.NewNode()
    if err := node.Run(ctx); err != nil {
        os.Exit(1)
    }
}
```

> **三个 binary 也可以合并为一个**：`ultron` 二进制通过子命令 `ultron hivemind run` / `ultron golem run` 来启动不同角色。独立 binary 的好处是部署时更轻量。两种方案都可以，初期建议先用**单 binary + 子命令**方式。

---

## 三、`internal/` — 核心私有代码

`internal/` 下的包不会被外部项目引用（Go 编译器强制保证）。

```
internal/
├── cli/                        # [ultron_cli] CLI 框架层
├── hivemind/                   # [HiveMind]  中控服务核心
├── golem/                      # [Golem]     工作节点核心
└── shared/                     # [共享]       三模块共用代码
```

---

### 3.1 `internal/cli/` — CLI 框架（ultron_cli 专属）

```
internal/cli/
├── root.go                     # Cobra root command + Execute()
├── flags.go                    # 全局 flags (--profile, --verbose, --json, --output)
├── banner.go                   # CLI banner / tagline 输出
├── version.go                  # 版本信息 (ldflags 注入)
├── register.go                 # 子命令注册入口
│
├── cmd_hivemind.go             # ultron hivemind start/stop/status
├── cmd_golem.go                # ultron golem init/join/leave/status
├── cmd_chat.go                 # ultron chat <message> / ultron tui
├── cmd_skills.go               # ultron skills list/install/uninstall
├── cmd_status.go               # ultron status (集群概览)
├── cmd_config.go               # ultron config set/get/edit/show
├── cmd_channels.go             # ultron channels status/pair
├── cmd_memory.go               # ultron memory search/import/clear
├── cmd_model.go                # ultron model list/pick
├── cmd_doctor.go               # ultron doctor (诊断检查)
├── cmd_update.go               # ultron update
└── cmd_setup.go                # ultron setup (交互式配置向导)
```

设计要点：
- 使用 `spf13/cobra` 作为 CLI 框架
- 子命令按模块分组：`hivemind`、`golem`、`chat`、`skills`、`config`、`channels`
- `--profile <name>` 支持多套配置并行
- 所有子命令通过 gRPC/WebSocket 与 HiveMind 或 Golem 通信

---

### 3.2 `internal/hivemind/` — 中控服务核心

HiveMind 是 Ultronix 的大脑，负责：
- 接收用户消息（对话/命令）
- AI 推理与对话管理
- 任务编排与 Golem 调度
- 中心知识库 / Memory / RAG
- MCP (Model Context Protocol) Hub
- 频道接入（Telegram/Discord/Slack 等）

```
internal/hivemind/
├── server.go                   # HiveMind 主服务结构体 + Run/Shutdown
├── server_options.go           # ServerOptions (bind, port, tls, auth)
├── http.go                     # HTTP 路由注册
├── websocket.go                # WebSocket 运行时（CLI/Web UI 连接）
├── grpc.go                     # gRPC 服务端（Golem 连接用）
│
├── scheduler/                  # 任务调度器（核心！）
│   ├── scheduler.go            #   调度器主逻辑：选择 Golem、分配任务
│   ├── task.go                 #   任务定义 (Task, TaskStatus, TaskResult)
│   ├── queue.go                #   任务队列
│   ├── selector.go             #   Golem 选择策略（能力匹配、负载均衡）
│   └── monitor.go              #   任务执行监控
│
├── cluster/                    # 集群管理
│   ├── registry.go             #   Golem 注册表（在线节点追踪）
│   ├── heartbeat.go            #   心跳检测（Golem 存活探测）
│   ├── discovery.go            #   节点发现（mDNS / 手动注册）
│   └── topology.go             #   集群拓扑（节点能力图）
│
├── autoreply/                  # 消息处理管道（对话核心）
│   ├── dispatch.go             #   消息分发入口
│   ├── debounce.go             #   去抖动
│   ├── envelope.go             #   消息信封（标准化消息结构）
│   ├── commands/               #   内联命令系统
│   │   ├── registry.go         #     命令注册表
│   │   ├── detection.go        #     命令检测 (/model, /status, /golem ...)
│   │   └── handlers.go         #     命令处理器
│   └── reply/                  #   核心回复引擎
│       ├── get_reply.go        #     回复编排入口
│       ├── agent_runner.go     #     Agent 执行协调
│       ├── agent_exec.go       #     Agent 实际执行 + 模型回退
│       ├── streaming.go        #     流式响应处理
│       ├── model_selection.go  #     运行时模型选择
│       ├── session.go          #     会话加载/更新
│       └── golem_dispatch.go   #     需要执行操作时 → 派发到 Golem
│
├── agents/                     # AI Agent 引擎
│   ├── runner/                 #   Agent Runner
│   │   ├── run.go              #     主执行入口
│   │   ├── system_prompt.go    #     System Prompt 构建
│   │   ├── history.go          #     对话历史管理
│   │   ├── compact.go          #     上下文压缩
│   │   └── tool_dispatch.go    #     工具调用 → 本地执行 or 派发到 Golem
│   │
│   ├── models/                 #   模型管理
│   │   ├── catalog.go          #     模型目录
│   │   ├── fallback.go         #     模型故障转移
│   │   ├── selection.go        #     模型选择策略
│   │   └── providers.go        #     内置 Provider 注册表
│   │
│   └── auth/                   #   Auth Profile 管理
│       ├── profiles.go         #     多 API Key 轮换
│       ├── credentials.go      #     凭据存储
│       └── cooldown.go         #     失败冷却策略
│
├── memory/                     # 中心知识库 / RAG 系统
│   ├── manager.go              #   记忆管理器
│   ├── embeddings.go           #   嵌入接口抽象
│   ├── embeddings_openai.go    #   OpenAI 嵌入实现
│   ├── hybrid.go               #   混合搜索 (向量 + BM25)
│   ├── sqlite.go               #   SQLite 存储层
│   ├── sqlite_vec.go           #   sqlite-vec 向量扩展
│   ├── schema.go               #   数据库 schema / migration
│   └── types.go                #   MemoryEntry, SearchResult 等
│
├── mcp/                        # MCP Hub (Model Context Protocol)
│   ├── hub.go                  #   MCP 服务端（暴露工具给 Agent）
│   ├── registry.go             #   MCP Tool 注册表
│   ├── transport.go            #   MCP 传输层 (stdio / SSE / WebSocket)
│   └── types.go                #   MCP 协议类型定义
│
├── channels/                   # 频道接入层
│   ├── registry.go             #   频道注册表
│   ├── plugin.go               #   ChannelPlugin 接口定义
│   ├── types.go                #   ChannelID, ChannelMeta, Capabilities
│   └── outbound/               #   出站消息处理
│       ├── send.go
│       └── format.go
│
├── routing/                    # 消息路由
│   ├── resolve.go              #   路由解析 (channel + peer → agentId + sessionKey)
│   └── session_key.go          #   SessionKey 生成
│
├── sessions/                   # 会话存储
│   ├── store.go                #   Session Store (JSONL)
│   ├── metadata.go             #   会话元数据
│   └── transcript.go           #   对话记录
│
├── plugins/                    # 插件系统
│   ├── registry.go             #   PluginRegistry
│   ├── types.go                #   插件类型定义
│   ├── loader.go               #   插件加载器
│   ├── discovery.go            #   插件发现
│   └── hooks.go                #   钩子系统
│
├── hooks/                      # Hook 系统
│   ├── loader.go               #   Hook 加载器
│   └── builtin.go              #   内置 Hooks
│
├── cron/                       # 定时任务
│   ├── scheduler.go            #   Cron 调度器
│   └── store.go                #   任务持久化
│
├── openai/                     # OpenAI 兼容 API 端点
│   ├── handler.go              #   /v1/chat/completions
│   └── models.go               #   /v1/models
│
├── protocol/                   # HiveMind WebSocket/gRPC 协议
│   ├── messages.go             #   消息类型定义
│   └── codec.go                #   编解码
│
├── auth.go                     # HiveMind 认证 (token / device)
├── config_reload.go            # 配置热重载 (fsnotify)
└── discovery.go                # 服务发现 (mDNS / Bonjour)
```

---

### 3.3 `internal/golem/` — 工作节点核心

Golem 是 Ultronix 的执行者。任意一台机器通过 `ultron golem init` 初始化后变为 Golem 节点。
Golem 的职责：
- 注册到 HiveMind
- 汇报自身能力（已安装的 Skills）
- 接收并执行 HiveMind 分配的任务
- 返回执行结果

```
internal/golem/
├── node.go                     # Golem 节点主结构体 + Run/Shutdown
├── node_options.go             # NodeOptions (hivemind-addr, port, name, tags)
├── registration.go             # 向 HiveMind 注册（上报能力清单）
├── heartbeat.go                # 心跳上报
├── capabilities.go             # 能力清单（已安装 Skills + 系统信息）
│
├── executor/                   # 任务执行器
│   ├── executor.go             #   执行器主逻辑：接收任务 → 选择 Skill → 执行
│   ├── sandbox.go              #   沙盒执行环境
│   ├── result.go               #   执行结果封装
│   └── stream.go               #   流式输出转发（执行过程实时回传 HiveMind）
│
├── skills/                     # Skill 运行时
│   ├── manager.go              #   Skill 安装/卸载/更新管理
│   ├── loader.go               #   Skill 加载器
│   ├── registry.go             #   本地已安装 Skill 注册表
│   └── runner.go               #   Skill 执行适配器
│
├── tools/                      # 内置工具（不依赖 Skill 也可执行）
│   ├── bash.go                 #   Bash/Shell 执行
│   ├── bash_process.go         #   进程管理
│   ├── file_ops.go             #   文件操作
│   ├── web_search.go           #   Web 搜索
│   └── registry.go             #   工具注册表
│
├── browser/                    # 浏览器自动化（可选能力）
│   ├── session.go              #   浏览器会话
│   ├── cdp.go                  #   Chrome DevTools Protocol
│   ├── chrome.go               #   Chrome 实例管理
│   └── tools.go                #   浏览器工具 (截图, 点击, 输入)
│
├── grpc_client.go              # gRPC 客户端（连接 HiveMind）
├── state.go                    # 节点状态持久化
└── security.go                 # 执行安全策略（命令白名单、目录限制）
```

---

### 3.4 `internal/shared/` — 三模块共享代码

```
internal/shared/
├── config/                     # 配置系统（三模块共用）
│   ├── schema.go               #   主配置 struct
│   ├── schema_hivemind.go      #   HiveMind 配置
│   ├── schema_golem.go         #   Golem 配置
│   ├── schema_agents.go        #   Agent 配置
│   ├── schema_auth.go          #   Auth 配置
│   ├── schema_channels.go      #   频道配置
│   ├── io.go                   #   配置文件读写 (JSON5)
│   ├── paths.go                #   路径解析 (~/.ultronix/)
│   ├── defaults.go             #   默认值填充
│   ├── validation.go           #   配置校验
│   └── env_substitution.go     #   ${ENV_VAR} 环境变量替换
│
├── logging/                    # 日志子系统
│   ├── logger.go               #   slog 封装
│   ├── handler.go              #   自定义 slog handler
│   └── redact.go               #   敏感信息脱敏
│
├── terminal/                   # 终端 UI 工具
│   ├── palette.go              #   颜色调色板
│   ├── table.go                #   表格输出
│   ├── progress.go             #   进度条/Spinner
│   └── ansi.go                 #   ANSI 转义工具
│
├── media/                      # 媒体处理
│   ├── store.go                #   媒体存储
│   ├── fetch.go                #   媒体下载
│   ├── mime.go                 #   MIME 类型检测
│   └── image.go                #   图片处理
│
├── process/                    # 子进程管理
│   ├── exec.go                 #   命令执行
│   ├── spawn.go                #   后台进程
│   └── queue.go                #   命令队列
│
├── security/                   # 安全审计
│   ├── audit.go                #   审计日志
│   └── permissions.go          #   文件权限检查
│
├── infra/                      # 基础设施
│   ├── network/                #     网络工具 (ports, dns)
│   ├── device/                 #     设备身份
│   ├── retry/                  #     重试策略
│   ├── tls/                    #     TLS 证书管理
│   └── migration/              #     状态迁移
│
├── daemon/                     # 守护进程管理（HiveMind/Golem 共用）
│   ├── service.go              #   通用服务抽象
│   ├── launchd.go              #   macOS launchd
│   ├── systemd.go              #   Linux systemd
│   └── schtasks.go             #   Windows 计划任务
│
└── tui/                        # 终端 UI（对话界面）
    ├── app.go                  #   TUI 主程序 (bubbletea.Model)
    ├── commands.go             #   命令处理器
    ├── events.go               #   事件处理器
    ├── components/             #   UI 组件
    │   ├── chat_log.go         #     聊天日志
    │   ├── input.go            #     文本输入框
    │   └── status_bar.go       #     状态栏
    └── theme/
        └── theme.go            #   主题 (lipgloss)
```

---

### 3.5 频道适配器（`internal/hivemind/` 下的独立子包 or 顶层）

频道适配器挂载在 HiveMind 上（消息接入是 HiveMind 的职责）。
为了避免 `internal/hivemind/` 过于臃肿，频道适配器放在 `internal/` 顶层：

```
internal/
├── telegram/                   # Telegram 频道
│   ├── plugin.go               #   ChannelPlugin 注册入口
│   ├── bot.go                  #   Bot 实例管理
│   ├── monitor.go              #   消息监听
│   ├── send.go                 #   消息发送
│   ├── webhook.go              #   Webhook 管理
│   ├── format.go               #   消息格式化
│   └── types.go                #   Telegram 特定类型
│
├── discord/                    # Discord 频道
│   ├── plugin.go
│   ├── bot.go
│   ├── send.go
│   └── types.go
│
├── slack/                      # Slack 频道
│   ├── plugin.go
│   ├── bot.go
│   ├── send.go
│   └── types.go
│
├── whatsapp/                   # WhatsApp 频道
│   ├── plugin.go
│   ├── session.go
│   ├── send.go
│   └── types.go
│
├── signal/                     # Signal 频道
│   ├── plugin.go
│   ├── send.go
│   └── types.go
│
└── imessage/                   # iMessage 频道 (macOS only)
    ├── plugin.go
    ├── send.go
    └── types.go
```

---

## 四、`pkg/` — 公开库代码

`pkg/` 下的包可被外部项目 import。

```
pkg/
├── protocol/                   # HiveMind ↔ Golem 通信协议
│   ├── task.go                 #   Task, TaskStatus, TaskResult
│   ├── node.go                 #   NodeInfo, NodeCapability
│   ├── messages.go             #   协议消息类型
│   └── codec.go                #   编解码
│
└── pluginsdk/                  # 插件开发 SDK
    ├── sdk.go                  #   公共 API
    ├── types.go                #   插件接口定义
    ├── context.go              #   ToolContext, HookContext
    └── skill.go                #   Skill 接口定义
```

外部开发者 import 方式：
```go
import "github.com/kiosk404/ultronix/pkg/pluginsdk"
import "github.com/kiosk404/ultronix/pkg/protocol"
```

---

## 五、`api/` — API 协议定义

```
api/
├── proto/                      # Protobuf 定义（HiveMind ↔ Golem gRPC）
│   ├── hivemind.proto          #   HiveMind 服务定义
│   ├── golem.proto             #   Golem 服务定义
│   ├── task.proto              #   任务相关消息
│   └── common.proto            #   公共类型
│
├── openai/                     # OpenAI 兼容 API
│   ├── types.go                #   ChatCompletion, Message, etc.
│   └── schema.json             #   OpenAPI spec
│
└── mcp/                        # MCP (Model Context Protocol)
    ├── types.go
    └── schema.json
```

---

## 六、`skills/` — Agent 技能

Skills 安装在 Golem 上，定义 Agent 可以调用的能力。

```
skills/
├── github/
│   ├── skill.md                # 技能定义 (System Prompt 片段 + 工具声明)
│   └── scripts/                # 辅助脚本
│       └── pr_review.sh
├── shell/
│   └── skill.md                # Shell 执行技能
├── coding/
│   └── skill.md                # 代码编写技能
├── browser/
│   └── skill.md                # 浏览器操作技能
├── slack/
│   └── skill.md
├── notion/
│   └── skill.md
├── weather/
│   └── skill.md
└── ...
```

---

## 七、其他顶层目录

```
configs/
├── ultronix.default.json       # 默认配置模板 (JSON5)
├── ultronix.schema.json        # 配置 JSON Schema
└── examples/
    ├── single-node.json        # 单机部署配置
    ├── cluster.json            # 集群配置
    └── minimal.json            # 最小配置

deployments/
├── docker/
│   ├── Dockerfile.hivemind     # HiveMind 镜像
│   ├── Dockerfile.golem        # Golem 镜像
│   └── docker-compose.yml      # 完整集群编排
├── systemd/
│   ├── ultronix-hivemind.service
│   └── ultronix-golem.service
└── launchd/
    ├── com.ultronix.hivemind.plist
    └── com.ultronix.golem.plist

scripts/
├── build.sh                    # 构建脚本
├── release.sh                  # 发布脚本
├── proto-gen.sh                # Protobuf 代码生成
└── dev-cluster.sh              # 本地开发集群启动

test/
├── integration/                # 集成测试
│   ├── cluster_test.go         # 集群通信测试
│   ├── task_dispatch_test.go   # 任务调度测试
│   └── skill_exec_test.go      # Skill 执行测试
├── e2e/                        # E2E 测试
└── fixtures/                   # 测试数据
```

---

## 八、关键设计决策

### 8.1 为什么分三个模块？

| 决策 | 理由 |
|------|------|
| 分离 HiveMind 和 Golem | HiveMind 是有状态服务（知识库、会话），Golem 是无状态执行器，职责清晰 |
| ultron_cli 作为统一入口 | 用户只需学一个命令，不同子命令操作不同模块 |
| Golem 可独立部署 | 任意机器 `ultron golem init` 即可，不需要在同一台机器运行所有组件 |
| 无 Golem 时退化为纯对话 | 最小部署只需 HiveMind，渐进式扩展能力 |

### 8.2 HiveMind ↔ Golem 通信

| 方案 | 选择 | 理由 |
|------|------|------|
| **gRPC** | 主选 | 强类型、流式、高性能、Protobuf 生态 |
| WebSocket | 备选 | 简单场景或 Web 客户端接入 |

通信流程：
```
1. Golem 启动 → 连接 HiveMind gRPC
2. Golem 发送 Register(NodeInfo) → HiveMind 记录到 cluster.registry
3. HiveMind 定期发送 Ping → Golem 回复 Pong（心跳）
4. 用户发消息 → HiveMind Agent 判断需要执行操作
5. HiveMind scheduler 选择合适的 Golem → 发送 ExecuteTask(Task)
6. Golem executor 执行 → 流式返回 TaskProgress → 最终返回 TaskResult
```

### 8.3 频道适配器为独立顶层包

与 OpenClaw TS 版保持一致的设计：
- `internal/telegram/` 而不是 `internal/hivemind/channels/telegram/`
- 避免过深嵌套，Go 包路径直接映射 import path
- 每个频道自包含，仅依赖 `internal/hivemind/channels/` 的抽象接口

### 8.4 共享代码放 `internal/shared/`

三个模块需要共享的代码（config、logging、terminal、media 等）放在 `internal/shared/` 下，避免重复。任何模块都可以 import：

```go
import "github.com/kiosk404/ultronix/internal/shared/config"
import "github.com/kiosk404/ultronix/internal/shared/logging"
```

### 8.5 配置系统

配置文件格式：JSON5，路径：`~/.ultronix/ultronix.json`

```go
// internal/shared/config/schema.go
type UltronixConfig struct {
    Meta     MetaConfig     `json:"meta"`
    HiveMind HiveMindConfig `json:"hivemind"`
    Golem    GolemConfig    `json:"golem"`
    Agents   AgentsConfig   `json:"agents"`
    Auth     AuthConfig     `json:"auth"`
    Channels ChannelsConfig `json:"channels"`
    Plugins  PluginsConfig  `json:"plugins"`
    Memory   MemoryConfig   `json:"memory"`
}
```

---

## 九、依赖选型速查

| 功能 | Go 依赖 | 说明 |
|------|---------|------|
| CLI | `github.com/spf13/cobra` | 业界标准 CLI 框架 |
| 配置 | `github.com/titanous/json5` | JSON5 解析 |
| 配置验证 | `github.com/go-playground/validator` | struct tag 验证 |
| HTTP 服务 | 标准库 `net/http` | 轻量优先 |
| gRPC | `google.golang.org/grpc` | HiveMind ↔ Golem 通信 |
| Protobuf | `google.golang.org/protobuf` | 协议序列化 |
| WebSocket | `github.com/gorilla/websocket` | CLI/Web 连接 |
| WhatsApp | `go.mau.fi/whatsmeow` | Go 最成熟 WhatsApp 库 |
| Telegram | `github.com/go-telegram-bot-api/telegram-bot-api/v5` | 或 `gotd/td` |
| Discord | `github.com/bwmarrin/discordgo` | 社区标准 |
| Slack | `github.com/slack-go/slack` | 官方推荐 |
| SQLite | `github.com/mattn/go-sqlite3` | CGO, 最成熟 |
| 向量搜索 | `github.com/asg017/sqlite-vec` | sqlite-vec Go 绑定 |
| TUI | `github.com/charmbracelet/bubbletea` | Elm 架构终端 UI |
| TUI 样式 | `github.com/charmbracelet/lipgloss` | 终端样式 |
| 浏览器 | `github.com/chromedp/chromedp` | CDP 协议驱动 |
| 日志 | 标准库 `log/slog` | Go 1.21+ 结构化日志 |
| 测试 | `testing` + `github.com/stretchr/testify` | 断言和 mock |
| 文件监听 | `github.com/fsnotify/fsnotify` | 配置热重载 |
| 服务发现 | `github.com/hashicorp/mdns` | mDNS 局域网发现 |
| 发布 | `goreleaser` | 跨平台编译 + 打包 |

---

## 十、构建命令

```makefile
# Makefile

MODULE  := github.com/user/ultronix
VERSION := $(shell git describe --tags --always)
LDFLAGS := -s -w -X main.version=$(VERSION)

# === 构建 ===

build: build-ultron build-hivemind build-golem

build-ultron:
	go build -ldflags "$(LDFLAGS)" -o bin/ultron ./cmd/ultron

build-hivemind:
	go build -ldflags "$(LDFLAGS)" -o bin/hivemind ./cmd/hivemind

build-golem:
	go build -ldflags "$(LDFLAGS)" -o bin/golem ./cmd/golem

# === 开发 ===

dev-hivemind:
	go run ./cmd/hivemind $(ARGS)

dev-golem:
	go run ./cmd/golem $(ARGS)

dev-cli:
	go run ./cmd/ultron $(ARGS)

# === 测试 ===

test:
	go test ./internal/... -count=1

test-cover:
	go test ./internal/... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html

# === 代码质量 ===

lint:
	golangci-lint run ./...

fmt:
	gofumpt -l -w .

# === Protobuf ===

proto:
	protoc --go_out=. --go-grpc_out=. api/proto/*.proto

# === 跨平台 ===

build-all:
	@for bin in ultron hivemind golem; do \
		GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$$bin-linux-amd64   ./cmd/$$bin; \
		GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$$bin-linux-arm64   ./cmd/$$bin; \
		GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$$bin-darwin-arm64  ./cmd/$$bin; \
		GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$$bin-windows.exe   ./cmd/$$bin; \
	done

# === Docker ===

docker-hivemind:
	docker build -t ultronix-hivemind:latest -f deployments/docker/Dockerfile.hivemind .

docker-golem:
	docker build -t ultronix-golem:latest -f deployments/docker/Dockerfile.golem .

# === 清理 ===

clean:
	rm -rf bin/ coverage.out coverage.html
```

---

## 十一、import 路径示例

假设 module 名为 `github.com/kiosk404/ultronix`：

```go
import (
    // CLI
    "github.com/kiosk404/ultronix/internal/cli"

    // HiveMind
    "github.com/kiosk404/ultronix/internal/hivemind"
    "github.com/kiosk404/ultronix/internal/hivemind/scheduler"
    "github.com/kiosk404/ultronix/internal/hivemind/cluster"
    "github.com/kiosk404/ultronix/internal/hivemind/autoreply"
    "github.com/kiosk404/ultronix/internal/hivemind/autoreply/reply"
    "github.com/kiosk404/ultronix/internal/hivemind/agents"
    "github.com/kiosk404/ultronix/internal/hivemind/agents/runner"
    "github.com/kiosk404/ultronix/internal/hivemind/agents/models"
    "github.com/kiosk404/ultronix/internal/hivemind/memory"
    "github.com/kiosk404/ultronix/internal/hivemind/mcp"
    "github.com/kiosk404/ultronix/internal/hivemind/channels"
    "github.com/kiosk404/ultronix/internal/hivemind/plugins"

    // Golem
    "github.com/kiosk404/ultronix/internal/golem"
    "github.com/kiosk404/ultronix/internal/golem/executor"
    "github.com/kiosk404/ultronix/internal/golem/skills"
    "github.com/kiosk404/ultronix/internal/golem/tools"
    "github.com/kiosk404/ultronix/internal/golem/browser"

    // 频道适配器
    "github.com/kiosk404/ultronix/internal/telegram"
    "github.com/kiosk404/ultronix/internal/discord"
    "github.com/kiosk404/ultronix/internal/slack"
    "github.com/kiosk404/ultronix/internal/whatsapp"

    // 共享
    "github.com/kiosk404/ultronix/internal/shared/config"
    "github.com/kiosk404/ultronix/internal/shared/logging"
    "github.com/kiosk404/ultronix/internal/shared/terminal"
    "github.com/kiosk404/ultronix/internal/shared/media"

    // 公开 SDK
    "github.com/kiosk404/ultronix/pkg/protocol"
    "github.com/kiosk404/ultronix/pkg/pluginsdk"
)
```

---

## 十二、核心数据流

### 12.1 一条消息的完整生命周期

```
用户发送消息 (Telegram/Discord/CLI/...)
  │
  ▼
[频道适配器] internal/telegram/monitor.go
  │ 标准化为 InboundMessage
  ▼
[消息分发] internal/hivemind/autoreply/dispatch.go
  │ 去抖 → 命令检测 → 路由解析
  ▼
[回复引擎] internal/hivemind/autoreply/reply/get_reply.go
  │ 加载会话 → 选择模型 → 构建 Agent
  ▼
[Agent Runner] internal/hivemind/agents/runner/run.go
  │ 构建 System Prompt → 调用 LLM
  ▼
  ├── LLM 直接回复文本 → 流式返回给用户
  │
  └── LLM 调用工具 (tool_call)
      │
      ▼
      [工具调度] internal/hivemind/agents/runner/tool_dispatch.go
      │
      ├── 本地工具 (web_search, memory_search)
      │   → 直接在 HiveMind 执行
      │
      └── 需要 Golem 的工具 (bash, file_edit, browser)
          │
          ▼
          [调度器] internal/hivemind/scheduler/scheduler.go
          │ 选择合适的 Golem (能力匹配 + 负载均衡)
          ▼
          [gRPC] → Golem 节点
          │
          ▼
          [执行器] internal/golem/executor/executor.go
          │ 加载 Skill → 执行命令 → 流式返回结果
          ▼
          结果回传 HiveMind → Agent 继续推理或输出给用户
```

### 12.2 Golem 节点生命周期

```
ultron golem init
  │ 生成节点 ID → 创建 ~/.ultronix/golem/
  ▼
ultron golem join <hivemind-addr>
  │ gRPC 连接 → Register(NodeInfo{id, name, skills, system})
  ▼
HiveMind cluster/registry.go 记录节点
  │
  ▼
[心跳循环] Golem heartbeat.go ←→ HiveMind cluster/heartbeat.go
  │ 每 30s 互 ping
  ▼
[等待任务] Golem executor 监听 gRPC stream
  │
  ▼ (收到 ExecuteTask)
[执行] Golem executor → skill runner → bash/browser/...
  │
  ▼
[回传] TaskProgress (流式) → TaskResult (完成)
```

---

## 十三、实施优先级建议

AI Agent 实现本项目时，建议按以下顺序推进：

### Phase 1: 骨架搭建（可编译运行）
1. `cmd/` 三个 main.go
2. `internal/cli/` Cobra 根命令 + 基础子命令
3. `internal/shared/config/` 配置加载
4. `internal/shared/logging/` 日志初始化

### Phase 2: HiveMind 核心（单机对话能力）
5. `internal/hivemind/server.go` HTTP + WS 服务启动
6. `internal/hivemind/autoreply/` 消息处理管道
7. `internal/hivemind/agents/` Agent 引擎 + 模型调用
8. `internal/hivemind/sessions/` 会话存储

### Phase 3: Golem 节点（执行能力）
9. `internal/golem/node.go` 节点启动
10. `pkg/protocol/` + `api/proto/` gRPC 协议
11. `internal/hivemind/cluster/` 集群管理
12. `internal/hivemind/scheduler/` 任务调度
13. `internal/golem/executor/` 任务执行
14. `internal/golem/tools/` 内置工具

### Phase 4: 频道接入
15. `internal/hivemind/channels/` 频道抽象
16. `internal/telegram/` 第一个频道实现

### Phase 5: 高级功能
17. `internal/hivemind/memory/` 知识库 / RAG
18. `internal/hivemind/mcp/` MCP Hub
19. `internal/golem/skills/` Skill 管理
20. `internal/golem/browser/` 浏览器自动化
21. `internal/hivemind/plugins/` 插件系统
