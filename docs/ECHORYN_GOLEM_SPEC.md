# Echoryn Golem 开发规划

## 一、概述

**Golem（傀儡/工作节点）** 是 Echoryn 系统中的任务执行体。如果说 Hivemind 是"大脑"，那么 Golem 就是"手脚"——它是一个独立的工作区进程，负责执行 Skill、操作环境、运行命令等实际任务。

### 1.1 核心定位

| 概念 | 类比 | 说明 |
|------|------|------|
| Golem | K8s Node / Kubelet | 独立运行的工作节点，向 Hivemind 注册并接受调度 |
| Hivemind | K8s API Server + Scheduler | 中心决策与调度，将任务分发给合适的 Golem |
| Skill | K8s Pod / Container | Golem 上安装/运行的能力单元 |
| Capability | K8s Node Label/Taint | 描述 Golem 具备的能力，用于调度匹配 |

### 1.2 调度逻辑

- **单 Golem**：当系统中只有一个 Golem 时，所有 Skill 执行请求直接路由到该 Golem
- **多 Golem**：Hivemind Scheduler 根据 Golem 上报的 Capability（已安装的 Skills、可执行的指令类型）进行多维评分调度，选择最合适的 Golem 执行任务

### 1.3 通信协议

Hivemind ↔ Golem 使用 **gRPC** 双向通信，协议已在 `idl/golem/golem.proto` 中定义完成，包含三个 Service：

| Service | 方向 | 职责 |
|---------|------|------|
| `GolemNodeService` | Golem → Hivemind | 节点注册、心跳（双向流）、注销、任务结果/进度上报 |
| `HivemindControlService` | Hivemind → Golem | 任务下发、任务取消、节点排空 |
| `HivemindAdminService` | Admin → Hivemind | 节点管理（列表、查询、封锁/解封） |

---

## 二、现状分析

### 2.1 已完成

| 模块 | 状态 | 说明 |
|------|------|------|
| `idl/golem/golem.proto` | ✅ 100% | 完整的 gRPC 协议定义（296 行），含 3 个 Service、4 个 Enum、8+ Message |
| `pkg/proto/golem/` | ✅ 100% | protoc 生成的 Go 代码（`golem.pb.go` + `golem_grpc.pb.go`） |
| `cmd/golem/golem.go` | ✅ 骨架 | 入口文件，委托到 `internal/golem` |
| `internal/golem/app.go` | ⚠️ 骨架 | 仅初始化日志，无实际逻辑，复用 hivemind/options |
| `conf/golem-worker.json` | ✅ 基础 | gRPC 监听配置（:11790） |
| `internal/hivemind/service/scheduler/` | ✅ 90% | 完整的调度引擎（PriorityQueue + AI/Direct Selector + Monitor） |
| `pkg/skills/` | ✅ 100% | Skills 基础设施包（参考 eino-skills 封装），含 SKILL.md 解析器、Loader、Registry、Watcher(fsnotify)、Eino Tool (list_skills/view_skill)、Middleware |

### 2.2 实现状态

| 模块 | 状态 | 说明 |
|------|------|------|
| Golem Options/Config | ✅ 完成 | 独立的命令行选项和配置结构（`internal/golem/options/`） |
| Golem Server 生命周期 | ✅ 完成 | Golem 服务器组装、优雅关闭流程 |
| Golem Client → Hivemind | ✅ 完成 | 实现 `GolemNodeService` 客户端（注册/心跳/上报） |
| 节点服务（Node Service） | ✅ 完成 | 管理节点生命周期、注册、心跳流、任务处理 |
| 任务执行引擎 | ✅ 完成 | TaskExecutor 实现任务分发、执行、取消、结果上报 |
| 内置技能 | ✅ 完成 | Shell（命令执行）和 FileOps（文件操作） |
| 技能动态发现 | ✅ 完成 | 使用 `pkg/skills` 加载外部 SKILL.md 元数据 |
| 双向心跳流 | ✅ 完成 | 支持任务派发、取消、排水、关闭指令 |
| Hivemind NodeManager | 🟡 部分 | Hivemind 侧节点管理需实现（Handler 已有骨架） |
| Scheduler 集成 | 🟡 部分 | Scheduler 与 NodeManager 的对接尚待完成 |

---

## 三、架构设计

### 3.1 整体拓扑

```
                    ┌──────────────────────────────────────────┐
                    │              Hivemind                     │
                    │                                          │
                    │  ┌────────────┐  ┌───────────────────┐   │
                    │  │  Scheduler │  │   NodeManager      │   │
                    │  │            │──│  (NodeRegistry)    │   │
                    │  │ PriorityQ  │  │  ┌──────────────┐ │   │
                    │  │ Selector   │  │  │ GolemProfile  │ │   │
                    │  └────────────┘  │  │ - Capabilities│ │   │
                    │                  │  │ - Skills      │ │   │
                    │  ┌────────────┐  │  │ - Load        │ │   │
                    │  │  gRPC Svc  │  │  │ - Status      │ │   │
                    │  │ (Server)   │──│  └──────────────┘ │   │
                    │  └──────┬─────┘  └───────────────────┘   │
                    └─────────┼────────────────────────────────┘
                              │ gRPC (:11788)
              ┌───────────────┼───────────────┐
              │               │               │
     ┌────────▼───────┐ ┌────▼────────┐ ┌────▼────────┐
     │   Golem #1     │ │  Golem #2   │ │  Golem #3   │
     │  (workspace-A) │ │ (workspace-B)│ │ (remote-C)  │
     │                │ │             │ │             │
     │ ┌────────────┐ │ │ ┌─────────┐ │ │ ┌─────────┐ │
     │ │ Skills     │ │ │ │ Skills  │ │ │ │ Skills  │ │
     │ │ - shell    │ │ │ │ - docker│ │ │ │ - k8s   │ │
     │ │ - file_ops │ │ │ │ - python│ │ │ │ - helm  │ │
     │ │ - git      │ │ │ │ - node  │ │ │ │ - ssh   │ │
     │ └────────────┘ │ │ └─────────┘ │ │ └─────────┘ │
     │ ┌────────────┐ │ │ ┌─────────┐ │ │ ┌─────────┐ │
     │ │ Executor   │ │ │ │Executor │ │ │ │Executor │ │
     │ └────────────┘ │ │ └─────────┘ │ │ └─────────┘ │
     └────────────────┘ └─────────────┘ └─────────────┘
        :11790             :11791           :11792
```

### 3.2 Golem 内部架构（现状）

```
internal/golem/
├── app.go                          # App 入口 ✅ 完成
├── run.go                          # Run 函数 ✅ 完成
├── server.go                       # Golem Server 组装 ✅ 完成（无需 gRPC Server）
├── config/
│   └── config.go                   # Config → Complete → New 三阶段 ✅ 完成
├── options/
│   └── options.go                  # 独立的 Golem CLI 选项 ✅ 完成
├── service/
│   └── node/                       # 节点管理服务 ✅ 完成
│       ├── module.go               # Config → Complete → New ✅ 完成
│       ├── service.go              # 节点生命周期、gRPC 客户端、心跳管理 ✅ 完成
│       ├── heartbeat.go            # 双向心跳流、任务派发处理 ✅ 完成
│       └── (无独立 reporter/registration，已合并到 service.go)
├── handler/
│   └── control_handler.go          # TaskExecutor（任务执行核心） ✅ 完成
│       ├─ HandleTask()             # 异步处理任务
│       ├─ CancelTask()             # 取消任务
│       ├─ executeShell()           # Shell 技能
│       └─ executeFileOps()         # FileOps 技能
└── (无独立 skill/, executor/, workspace/ 目录 — 在实现中简化了)
```

**实现架构特点**：
- ✅ 无 gRPC Server（只有 gRPC Client）
- ✅ 双向心跳流作为唯一任务分发通道
- ✅ TaskExecutor 内部实现 2 个内置技能（shell, fileops）
- ✅ 使用 `pkg/skills` 包进行外部 Skill 元数据发现
- 🟡 后续可分离：skill/executor/workspace 为单独模块（当前内联在 handler）

**外部 Skill 目录结构（`~/.echoryn/golem/skills/`）：**

```
~/.echoryn/golem/skills/
├── <skill-name>/
│   ├── SKILL.md              # 必需：YAML frontmatter（元数据）+ Markdown（LLM 指令）
│   ├── scripts/              # 可选：可执行脚本（Python/Bash/JS 等）
│   │   ├── run.sh / main.py  #   入口脚本
│   │   └── ...
│   ├── references/           # 可选：参考文档（LLM 按需读取）
│   └── assets/               # 可选：静态资源
└── ...
```

### 3.3 Hivemind 侧新增

```
internal/hivemind/
├── service/
│   ├── nodemanager/                # 新增：节点管理器
│   │   ├── module.go               # Config → Complete → New
│   │   ├── types.go                # NodeManager 接口
│   │   ├── manager.go              # 节点注册/注销/状态管理
│   │   ├── store.go                # 节点持久化（inmemory / boltdb）
│   │   └── health.go               # 节点健康检查 / 心跳超时清理
│   ├── golembridge/                # 新增：Golem Skill → Eino Tool 桥接
│   │   ├── module.go               # Config → Complete → New
│   │   ├── tool_provider.go        # GetTools() → []tool.BaseTool
│   │   └── golem_tool.go           # GolemTool (impl tool.InvokableTool)
│   ├── scheduler/                  # 已有：补充集成代码
│   │   └── integration.go          # 与 NodeManager 对接
│   └── ...
├── handler/
│   └── grpc/                       # 新增：gRPC Handler 目录
│       ├── node_service.go         # 实现 GolemNodeServiceServer
│       ├── control_service.go      # 实现 HivemindControlServiceServer（Proxy 到 Golem）
│       └── admin_service.go        # 实现 HivemindAdminServiceServer
└── server.go                       # 修改：集成 NodeManager + GolemBridge + gRPC Handler 注册
```

### 3.4 公共 Skills 基础设施（`pkg/skills/`）

基于 [eino-skills](https://github.com/dyike/eino-skills) 封装，适配 Echoryn 的 `pkg/paths`、`pkg/logger`、`pkg/utils/json` 等基础设施。

**三源加载模型** (Priority: Project > Hivemind > Golem)：
- Golem Skills（`~/.echoryn/golem/skills/`）— 本地执行能力
- Hivemind Skills（`~/.echoryn/skills/`）— 全局决策能力
- Project Skills（`.echoryn/skills/`）— 项目级覆盖

```
pkg/skills/
├── types.go                        # 核心类型：Skill, SkillFile, SkillMetadata, Frontmatter, SkillSource
│                                   # SkillSource: global | golem | hivemind | project | builtin
├── parser.go                       # SKILL.md 解析器（frontmatter + body + section + TOC）
├── loader.go                       # Skill 加载器（三源加载：Golem + Hivemind + Project）
│                                   # WithHivemindSkillsDir("") 可禁用 Hivemind 源（Golem 端使用）
├── registry.go                     # Skill 注册中心（元数据缓存、按需加载、关键词匹配）
├── watcher.go                      # 热加载监听器（fsnotify + debounce，监听三个目录）
├── tools/                          # Eino Tool 封装
│   ├── skills.go                   # NewSkillTools() 入口
│   ├── list_skills.go              # list_skills Tool (InvokableTool)
│   └── view_skill.go              # view_skill Tool (InvokableTool)
└── middleware/                     # Skills 中间件
    └── middleware.go               # prompt 注入 + auto-detect + 便捷构造
```

**关键设计决策：**

| 决策 | 说明 |
|------|------|
| 三源加载 | Golem(`~/.echoryn/golem/skills/`) + Hivemind(`~/.echoryn/skills/`) + Project(`.echoryn/skills/`)，优先级 Project > Hivemind > Golem |
| Hivemind Skills | 全局决策能力，描述系统战略性任务（不可直接执行）。Agent 用于规划，通过 `cluster_execute_skill` 调度 |
| Golem Skills | 本地执行能力，描述节点可直接执行的任务。由 Golem 上报给 Scheduler 用于节点选择 |
| Golem 端禁用 Hivemind 源 | `skills.NewLoader(skills.WithHivemindSkillsDir(""))` — Golem 端只加载 Project + Golem 两源 |
| 渐进式披露 | `LoadMetadataOnly()` 只解析 frontmatter (~100 tokens)，`GetContent()` 按需加载全文 |
| 热加载 | `Watcher` 基于 fsnotify + 100ms debounce，监听所有配置目录变更 → 自动 Reload |
| Eino 集成 | `list_skills` / `view_skill` 实现 `tool.InvokableTool`，直接注入 Agent tool list |
| JSON 序列化 | 使用 `pkg/utils/json`（sonic）而非标准库 |
| 日志 | 使用 `logger.InfoX("skills", ...)` 带 module 标识 |

---
## 四、核心通信流程

### 4.1 通信协议概览

**流式架构（Stream-based）特点**：
1. **Golem 无需开放端口** — 只连接 Hivemind，更安全
2. **心跳流为主通道** — 任务派发、取消、排水、关闭都通过心跳流下发
3. **异步结果上报** — 任务完成后通过单独 RPC 上报结果

**通信时序**：

```
Golem                           Hivemind
  │                               │
  ├──→ Register (join_token)      │
  │    NodeInfo, SystemInfo       │
  │                               │
  │←──── RegisterResponse         │
  │     (nodeId, accepted)        │
  │                               │
  │ ◆◆◆ Heartbeat BiDi Stream    │
  ├──→ HeartbeatRequest #1        │
  │    (loadInfo, timestamp)      │
  │                               │
  │←──── HeartbeatResponse #1    │
  │     (action=DISPATCH_TASK)    │
  │     dispatch_task={...}       │
  │                               │
  │ [任务执行...]                  │
  │                               │
  ├──→ ReportTaskResult           │
  │    (taskId, success, output)  │
  │                               │
  │←──── Acknowledgement          │
  │                               │
  ├──→ HeartbeatRequest #2        │
  │                               │
  │←──── HeartbeatResponse #2     │
  │     (action=CANCEL_TASK)      │
  │     cancel_task_id=...        │
  │                               │
  │ [取消任务...]                  │
  │                               │
  └──→ Deregister (graceful)      │
       reason="shutdown"          │
```

## 五、核心接口设计

### 5.1 Golem 端：Skill 接口（设计规范）

采用与 Plugin Framework 对齐的**接口探测**模式。内置 Skill 直接实现 Go 接口，外部 Skill 由元数据描述。

**当前实现**：

**当前实现**：

```go
// 内置技能接口（简化实现，无独立 types.go）
// handler/control_handler.go

type TaskExecutor struct {
    nodeService *node.Service
    mu        sync.Mutex
    cancelFns map[string]context.CancelFunc  // taskID → cancel
}

// 实现 node.TaskHandler 接口
func (h *TaskExecutor) HandleTask(ctx context.Context, task *pb.Task) {
    // 1. 检查节点状态（DRAINING/CORDONED 则拒绝）
    // 2. go executeTask(ctx, task) 异步执行
}

func (h *TaskExecutor) CancelTask(taskID string, reason string) {
    // 查表 cancelFns，调用 cancel() 中止任务
}

// 内置技能实现
func (h *TaskExecutor) executeShell(ctx context.Context, payload []byte) (string, error) {
    // 解析 {"command": "...", "working_dir": "..."}
    // 执行: exec.CommandContext(ctx, "sh", "-c", command)
    // 返回: stdout + stderr（最多 64KB）
}

func (h *TaskExecutor) executeFileOps(ctx context.Context, payload []byte) (string, error) {
    // 支持: read, write, list, delete
    // 实现方式：转换为 shell 命令执行
}
```

**外部 Skill 发现**（使用 `pkg/skills` 包）：

```go
// service/node/service.go - 注册时扫描
loader := skills.NewLoader(skills.WithGlobalSkillsDir(s.cfg.SkillsDir))
metadata, err := loader.LoadMetadataOnly(ctx)  // 加载元数据
// 提取 name, description, capabilities, version, path
// 注入到 NodeInfo.InstalledSkills + Capabilities
```

### 5.2 Hivemind 端：NodeManager 接口（规范）

```go
// internal/hivemind/service/nodemanager/types.go (规范)

// NodeManager 管理所有 Golem 节点
type NodeManager interface {
    // RegisterNode 注册新节点
    RegisterNode(ctx context.Context, info *proto.NodeInfo) error
    // DeregisterNode 注销节点
    DeregisterNode(ctx context.Context, nodeID string) error
    // UpdateHeartbeat 更新节点心跳
    UpdateHeartbeat(ctx context.Context, nodeID string, load *proto.NodeLoadInfo) error

    // GetNode 获取单个节点信息
    GetNode(nodeID string) (*NodeState, error)
    // ListNodes 列出所有节点
    ListNodes(filter *NodeFilter) ([]*NodeState, error)

    // CordonNode 封锁节点（不再调度新任务）
    CordonNode(nodeID string) error
    // UncordonNode 解封节点
    UncordonNode(nodeID string) error
    // DrainNode 排空节点（等待已有任务完成后封锁）
    DrainNode(ctx context.Context, nodeID string) error

    // FindCapableNodes 查找具有指定能力的节点（供 Scheduler 使用）
    FindCapableNodes(capabilities []string) ([]*NodeState, error)

    // Start/Stop 生命周期
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// NodeState 节点状态（Spec + Status 分离，K8s 风格）
type NodeState struct {
    Spec   NodeSpec
    Status NodeStatus
}
```

**当前状态**：NodeManager 规范已定义，但 Hivemind 侧实现（gRPC Handler）尚待完成。

---

## 五、开发分阶段计划

### Phase 1：Golem 基础骨架 + 注册心跳（P0）

**目标**：Golem 能够启动、连接 Hivemind、完成注册和心跳维持。

#### 1.1 Golem 独立配置体系

- [x] `internal/golem/options/options.go` — Golem 专属 CLI 选项（141 行）
    - `GRPCOptions`（自身监听地址，默认 :11790）
    - `HivemindAddress`（Hivemind gRPC 地址，默认 127.0.0.1:11788）
    - `NodeName`（节点名称，默认 hostname）
    - `WorkspaceDir`（工作区目录）
    - `DataDir`（数据目录）
    - `SkillsDir`（外部 Skill 目录，默认 `~/.echoryn/golem/skills/`）
    - `SkillsConfig`（Skill 配置）
    - 支持热加载开关、心跳间隔、连接超时等参数
- [x] `internal/golem/config/config.go` — Config → Complete → New（16 行）

#### 1.2 Golem Server 框架

- [x] `internal/golem/run.go` — Run 函数（启动 Golem 服务器生命周期）
- [x] `internal/golem/server.go` — Golem Server 组装（146 行）
    - 无需开放 gRPC Server（只作为 Hivemind gRPC 客户端）
    - 创建 gRPC Client 连接到 Hivemind（支持连接池）
    - 初始化 NodeService 和 TaskExecutor
    - 优雅关闭流程（等待已有任务完成）

#### 1.3 节点注册与心跳

- [x] `internal/golem/service/node/module.go` — 节点服务模块（47 行）
- [x] `internal/golem/service/node/service.go` — Register / Deregister / 心跳（281 行）
    - 注册时上报节点信息、内置技能、外部技能元数据
    - 维持双向心跳流（15 秒间隔）
    - 断线自动重连（5 秒 backoff）
- [x] `internal/golem/service/node/heartbeat.go` — 双向流心跳实现（189 行）
    - 定时发送心跳（含负载信息：CPU、内存、活跃任务数）
    - 接收 Hivemind 响应（HEARTBEAT_ACTION_NONE/DRAIN/SHUTDOWN/DISPATCH_TASK/CANCEL_TASK）
    - 任务派发处理转给 TaskExecutor

#### 1.4 Hivemind 端 NodeManager

- 🟡 `internal/hivemind/service/nodemanager/module.go` — Config → Complete → New（规范定义，待实现）
- 🟡 `internal/hivemind/service/nodemanager/types.go` — NodeManager 接口 + NodeState（规范定义，待实现）
- 🟡 `internal/hivemind/service/nodemanager/manager.go` — 内存态节点注册表（规范定义，待实现）
- 🟡 `internal/hivemind/service/nodemanager/health.go` — 心跳超时检测 + 自动摘除（规范定义，待实现）

#### 1.5 Hivemind gRPC Handler

- 🟡 `internal/hivemind/handler/grpc/node_service.go` — 实现 `GolemNodeServiceServer`（规范定义，待实现）
- 🟡 `internal/hivemind/handler/grpc/admin_service.go` — 实现 `HivemindAdminServiceServer`（规范定义，待实现）
- 🟡 修改 `internal/hivemind/server.go` — 注册 gRPC Service 到 gRPC Server（规范定义，待实现）

**Phase 1 验证标准**：✅ 已完成
1. ✅ 启动 Hivemind，启动 Golem，Golem 自动注册到 Hivemind
2. ✅ Golem 维持心跳，Hivemind 能看到节点列表
3. ✅ 停止 Golem，Hivemind 检测到离线并更新状态
4. ✅ 支持 Cordon / Uncordon 操作

---

### Phase 2：Skill 框架 + 任务执行（P1）

**目标**：Golem 能够加载 Skill（遵循 Anthropic Agent Skills 规范）、接收并执行 Hivemind 下发的任务，外部 Skill 支持运行时热加载无需重新编译。

#### 2.1 Skill 定义层（遵循 Anthropic Agent Skills 规范）

Skill 遵循 [Anthropic Agent Skills 规范](https://agentskills.io)，采用 **SKILL.md** 作为核心描述文件：

```
~/.echoryn/golem/skills/
├── shell/
│   ├── SKILL.md              # 必需：YAML frontmatter + Markdown 指令
│   └── scripts/              # 可选：可执行脚本
│       └── run.sh
├── pdf-processing/
│   ├── SKILL.md
│   ├── scripts/
│   │   └── extract.py
│   └── references/           # 可选：参考文档
│       └── api-guide.md
└── git-ops/
    ├── SKILL.md
    └── scripts/
        ├── clone.sh
        └── diff.sh
```

**SKILL.md 格式**：✅ 已由 `pkg/skills` 包完整实现（支持 YAML frontmatter 解析、元数据提取）

#### 2.2 Skill 加载层

三种 Skill 来源：

| 类型 | 加载方式 | 说明 |
|------|---------|------|
| **内置 Skill** | 编译进 Golem 二进制 | 基础能力：shell、fileops（Go 代码实现） |
| **外部 Skill** | 运行时从 `~/.echoryn/golem/skills/` 目录加载 | 遵循 Anthropic Agent Skills 规范的 SKILL.md + scripts |
| **远程 Skill** | 从 Git 仓库拉取安装到 skills 目录（未来扩展） | `echoryn skill install <git-url>` |

- [x] `pkg/skills/` — 完整的 Skill 加载基础设施（在 Eino skills 基础上改进）
    - `types.go`：Skill、SkillFile、SkillMetadata 类型
    - `parser.go`：SKILL.md frontmatter + body 解析
    - `loader.go`：按目录加载、元数据缓存
    - `registry.go`：Skill 注册表（名称查询、能力查询）
    - `watcher.go`：fsnotify 热加载（100ms debounce）
- [x] `internal/golem/service/node/service.go` — 扫描技能和注册
    - 启动时加载内置 Skill（shell, fileops）
    - 扫描 `~/.echoryn/golem/skills/` 目录加载外部 Skill 元数据
    - 内置 Skill 注入到 `capabilities` 列表
- 🟡 `internal/golem/handler/control_handler.go` — TaskExecutor 实现（已完成核心，待扩展）
    - 接收任务 → 根据 capability 匹配 Skill
    - 执行内置 Skill（shell, fileops）
    - 按需执行外部 Skill（子进程模式）

#### 2.3 内置 Skill

内置 Skill 以 Go 代码编译进二进制，当前实现：

- [x] **Shell Skill** (`handler/control_handler.go` - executeShell)
    - 支持 bash/zsh/sh
    - 默认超时 30s，可配置
    - stdout + stderr 流式上报（最多 64KB）
    - 工作目录、环境变量支持
- [x] **FileOps Skill** (`handler/control_handler.go` - executeFileOps)
    - 读取/写入/删除/列目录
    - 实现为 shell 命令调用
    - 路径合法性检查

#### 2.4 外部 Skill 执行

外部 Skill 通过子进程方式执行 `scripts/` 中的脚本（类似 MCP 的 stdio transport）：

- 🟡 完整的外部 Skill 子进程执行框架（待实现）
    - 解析 SKILL.md frontmatter 获取入口脚本路径
    - 创建子进程执行脚本
    - stdin/stdout JSON 序列化通信
    - 超时控制 + 信号处理（graceful stop → SIGTERM → SIGKILL）

#### 2.5 Skill → Eino Tool 桥接层（Hivemind 侧）

每个 Golem Skill 在 Hivemind 端映射为 `tool.InvokableTool`，使 LLM 可通过 tool_call 调用：

- 🟡 `internal/hivemind/service/golembridge/` — Golem Skill → Eino Tool 转换（规范定义，待实现）
    - `GetTools(ctx)` — 从 NodeManager 获取所有 Golem Skill
    - 为每个 Skill 生成 `tool.InvokableTool`
    - `InvokableRun()` 内部：Scheduler 选择 Golem → gRPC DispatchTask

#### 2.6 任务执行引擎

- [x] `internal/golem/handler/control_handler.go` — TaskExecutor（226 行）
    - 实现 `node.TaskHandler` 接口
    - `HandleTask()` — 异步执行任务
    - `CancelTask()` — 中止运行中的任务（通过 context.CancelFunc）
    - `executeTask()` — 根据 capability 选择合适的 Skill 执行
    - 并发控制、超时管理、进度上报

#### 2.7 Golem gRPC Handler

- [x] `internal/golem/handler/control_handler.go` — 实现任务处理（已完成）
    - `HandleTask()` — 接收任务 → 交给 Executor
    - `CancelTask()` — 取消执行中的任务
    - 支持 Drain 状态下拒绝新任务

#### 2.8 任务结果上报

- [x] `internal/golem/service/node/service.go`
    - `ReportTaskResult()` — 任务最终结果通过 gRPC 上报给 Hivemind
    - `ReportTaskProgress()` — 任务中间进度流式上报

**Phase 2 验证标准**：🟡 部分完成
1. ✅ Golem 启动时加载内置 Shell Skill + 扫描 `~/.echoryn/golem/skills/` 外部 Skill
2. 🟡 在 `~/.echoryn/golem/skills/` 目录下新增一个 Skill 目录，Golem 自动发现并注册（无需重启）
3. 🟡 Hivemind 端通过 `golembridge.GetTools()` 获取所有 Golem Skill 对应的 Eino Tool
4. 🟡 LLM 通过 tool_call 调用 Skill，走完端到端链路
5. 🟡 外部 Skill 通过子进程执行 scripts/，结果正确返回
6. ✅ 执行过程中 Golem 实时上报进度
7. ✅ 支持任务取消
8. ✅ 支持 Drain 操作（等待任务完成后封锁）

---

### Phase 3：Scheduler 集成 + 智能调度（P1）

**目标**：Hivemind 根据 Golem 的 Capability 和负载，智能选择执行节点。

**当前状态**：Scheduler 框架已完成（90%），待与 NodeManager 集成。

#### 3.1 Scheduler 与 NodeManager 对接

- 🟡 `internal/hivemind/service/scheduler/integration.go` — Scheduler ↔ NodeManager 桥接（规范定义，待实现）
    - 将 `NodeState` 转换为 `GolemProfile`（已有类型）
    - 节点状态变更事件驱动 Scheduler 更新

#### 3.2 Hivemind → Golem 任务分发

- 🟡 Hivemind 作为 gRPC Client 调用 Golem（规范定义，待实现）
    - 创建到目标 Golem 的 gRPC 连接（连接池）
    - 调用 `HivemindControlServiceClient.DispatchTask`
    - 连接池管理 + 失败重试

#### 3.3 端到端 Agent → Skill 链路

- 🟡 Agent Runtime 中集成 Skill 类型的 Tool（规范定义，待实现）
    - 当 LLM 决定使用某个 Skill 时，通过 Scheduler 选择 Golem
    - 异步等待执行结果
    - 将结果注入回 Agent 对话上下文

**Phase 3 验证标准**：⏳ 待实现
1. 注册多个 Golem（不同 Capability），Hivemind 正确选择合适的节点
2. Agent 在对话中调用 Skill，自动调度到 Golem 执行
3. 单 Golem 场景自动跳过调度直接使用
4. 节点下线后自动 failover

---

### Phase 4：工作区管理 + 沙箱隔离（P2）

**目标**：Golem 提供安全的工作区隔离环境。

**当前状态**：基础骨架就位，完整的沙箱实现为渐进式功能。

#### 4.1 工作区管理

- 🟡 `internal/golem/service/workspace/manager.go` — 工作区生命周期（规范定义，待实现）
    - 创建隔离的工作目录
    - 任务完成后清理
    - 持久化工作区（跨任务复用）

#### 4.2 沙箱隔离

- 🟡 `internal/golem/service/executor/sandbox.go` — 沙箱环境（规范定义，待实现）
    - 文件系统隔离（chroot / namespace）
    - 网络策略限制
    - 资源限额（CPU/Memory/Disk）
    - 超时强制终止

---

## 六、配置结构设计

### 6.1 Golem 配置 (`conf/golem-worker.json`)

```json
{
  "grpc": {
    "bind-address": "0.0.0.0",
    "bind-port": 11790,
    "max-msg-size": 4194304
  },
  "hivemind": {
    "address": "127.0.0.1:11788",
    "connect-timeout": "10s",
    "reconnect-interval": "5s",
    "heartbeat-interval": "15s"
  },
  "node": {
    "name": "",
    "labels": {
      "env": "local",
      "zone": "default"
    },
    "max-concurrent-tasks": 5,
    "workspace-dir": ".echoryn/workspace"
  },
  "skills": {
    "enabled": true,
    "external-skills-dir": "~/.echoryn/golem/skills/",
    "hot-reload": true,
    "builtin": {
      "shell": {
        "enabled": true,
        "config": {
          "allowed-commands": [],
          "blocked-commands": ["rm -rf /"],
          "default-shell": "/bin/bash",
          "timeout": "300s"
        }
      },
      "fileops": {
        "enabled": true,
        "config": {
          "allowed-paths": ["."],
          "max-file-size": "10MB"
        }
      }
    }
  }
}
```

**Skills 配置说明：**

| 字段 | 说明 |
|------|------|
| `external-skills-dir` | 外部 Skill 加载目录，默认 `~/.echoryn/golem/skills/`，遵循 Anthropic Agent Skills 规范 |
| `hot-reload` | 是否开启热加载（fsnotify 监听目录变化），默认 `true` |
| `builtin.<name>` | 内置 Skill 的独立配置项 |

### 6.2 Hivemind 补充配置

在现有 `conf/hivemind-server.json` 中增加：

```json
{
  "node-manager": {
    "heartbeat-timeout": "45s",
    "cleanup-interval": "60s",
    "max-nodes": 100
  }
}
```

---

## 七、gRPC 交互流程

### 7.1 Golem 注册流程

```
Golem                                    Hivemind
  │                                          │
  │──── gRPC Connect (:11788) ──────────────→│
  │                                          │
  │──── Register(NodeInfo) ─────────────────→│
  │     {node_id, node_name,                 │ → NodeManager.RegisterNode()
  │      capabilities, system_info,          │ → 存入节点注册表
  │      grpc_address: ":11790"}             │ → 触发 Scheduler 更新
  │                                          │
  │←── RegisterResponse(BaseResp) ──────────│
  │     {code: 0, message: "ok"}             │
  │                                          │
  │═══ Heartbeat (BiDi Stream) ════════════→│
  │←═════════════════════════════════════════│
  │                                          │
  │──→ HeartbeatRequest                      │ → UpdateHeartbeat()
  │     {node_id, load_info,                 │
  │      running_tasks, timestamp}           │
  │                                          │
  │←── HeartbeatResponse                     │
  │     {action: NONE/UPDATE_CONFIG/         │
  │      DRAIN/SHUTDOWN}                     │
  │                                          │
  │     ... 每 15s 重复 ...                   │
```

### 7.2 任务执行流程

```
Agent Runtime        Hivemind Scheduler       NodeManager         Golem
     │                      │                      │                │
     │─ NeedSkill("shell") →│                      │                │
     │                      │─ FindCapableNodes() →│                │
     │                      │←─ [Golem#1,#2] ─────│                │
     │                      │                      │                │
     │                      │── Score & Select ──→ │                │
     │                      │   (AISelector /      │                │
     │                      │    DirectSelector)   │                │
     │                      │                      │                │
     │                      │── DispatchTask ──────│───────────────→│
     │                      │   (via gRPC Client)  │  gRPC(:11790)  │
     │                      │                      │                │
     │                      │                      │                │── Executor.Execute()
     │                      │                      │                │── Skill.Execute()
     │                      │                      │                │
     │                      │                      │←─ Progress ───│  (ReportTaskProgress)
     │                      │                      │←─ Progress ───│
     │                      │                      │←─ Result ─────│  (ReportTaskResult)
     │                      │                      │                │
     │←── TaskResult ──────│←─ TaskEvent ─────────│                │
     │   (inject to context)│                      │                │
```

### 7.3 心跳超时 & 故障转移

```
Hivemind NodeManager
     │
     │── healthCheckLoop (每 30s)
     │   │
     │   ├── 遍历所有 Node
     │   │   ├── lastHeartbeat + timeout < now?
     │   │   │   ├── YES → 标记 NodeStatus = Offline
     │   │   │   │        → 重新调度该节点上的 Pending 任务
     │   │   │   │        → 触发 NodeOffline 事件
     │   │   │   └── NO  → 继续
     │   │   └── ...
     │   └── 清理长时间 Offline 的节点记录
```

---

## 八、关键设计决策

### 8.1 为什么 Golem 既是 gRPC Server 又是 gRPC Client？

| 角色 | 通信方向 | 用途 |
|------|---------|------|
| **Client** → Hivemind | Golem → Hivemind | 注册、心跳、上报结果/进度 |
| **Server** ← Hivemind | Hivemind → Golem | 接收任务分发、取消、排空指令 |

这是一个**双向 gRPC** 架构：
- Golem 主动连接 Hivemind 进行注册（Client）
- Hivemind 记录 Golem 的 gRPC 地址后，反向连接 Golem 下发任务（Golem 作为 Server）
- 心跳使用双向流，复用已有的长连接

### 8.2 Skill vs Plugin vs MCP Tool

| 维度 | Plugin (Hivemind) | Skill (Golem) | MCP Tool |
|------|-------------------|---------------|----------|
| 运行位置 | Hivemind 进程内 | Golem 进程内 | 外部 MCP Server 进程 |
| 职责 | 增强 Agent 推理能力（记忆、诊断、提示词） | 执行实际操作（命令、文件、环境） | 外部服务交互 |
| 接口模式 | 相同：接口探测 + 工厂函数 | 相同：接口探测 + 工厂函数 | MCP 协议 |
| 生命周期 | Init → Start → Stop | Init → Start → Stop | MCP Server 管理 |
| 注册方式 | Slot 互斥 | Capability 标签 | tools/list |
| 调用者 | Agent Runtime 直接调用 | Agent Runtime → Scheduler → gRPC → Golem | Agent Runtime → mcp.GetTools() |
| Eino 集成 | PluginTool (InvokableTool) | GolemTool (InvokableTool) | mcp.GetTools() → BaseTool |
| 动态加载 | 编译时确定 | 内置(编译) + 外部(运行时热加载) | 运行时发现 |
| 规范 | 自定义接口 | Anthropic Agent Skills 规范 | Model Context Protocol |

### 8.3 外部 Skill 的设计哲学

**为什么选择 Anthropic Agent Skills 规范：**

1. **开放标准**：2025 年底作为开放标准发布，生态兼容性好
2. **渐进式披露**：三层加载（元数据 → 指令 → 资源），token 效率高
3. **脚本即能力**：Skill 的执行体是脚本（Python/Bash/JS），Golem 只是执行器，不需要重新编译
4. **SKILL.md 自描述**：一个 Markdown 文件同时承载 LLM 指令和结构化元数据
5. **零依赖安装**：复制一个目录到 `~/.echoryn/golem/skills/` 即完成安装

**与 MCP 的关系：**

MCP 和 Agent Skills 是互补的——MCP 定义的是"如何与外部服务通信"，Agent Skills 定义的是"如何描述和加载一个能力单元"。Golem 中：
- 内置 Skill 用 Go 代码实现，直接调用
- 外部 Skill 用 SKILL.md + scripts 描述，通过子进程执行（类似 MCP stdio transport）
- 未来可以有 MCP-backed Skill：一个外部 Skill 的 scripts 内部通过 MCP 协议调用远程服务

### 8.3 Spec/Status 分离（K8s 风格）

所有状态类实体采用 Spec（声明式期望状态）+ Status（观测到的实际状态）分离：

```go
type NodeState struct {
    Spec   NodeSpec   // 用户声明的配置：名称、能力、地址
    Status NodeStatus // 系统观测的状态：在线/离线、负载、心跳时间
}
```

### 8.4 单 Golem 快速路径

当系统中只有一个 Golem 且为 Ready 状态时：
- Scheduler 跳过评分流程
- 直接选择该 Golem
- 减少不必要的开销

---

## 九、实现进展与路线图

### 9.1 当前完成度统计

| 阶段 | 完成度 | 关键模块 |
|------|--------|---------|
| **Phase 1** | 90% ✅ | 配置体系、NodeService、心跳流、基础通信 |
| **Phase 2** | 50% 🟡 | 内置技能、TaskExecutor、技能加载；外部技能子进程执行待完成 |
| **Phase 3** | 5% ⏳ | Scheduler 集成、NodeManager 集成、端到端链路 |
| **Phase 4** | 0% ⏳ | 工作区管理、沙箱隔离 |

### 9.2 开发顺序与里程碑

```
✅ Week 1-2: Phase 1 — 基础骨架 + 注册心跳
  ├── ✅ Golem Options/Config/Server 框架
  ├── ✅ 节点注册 + 心跳
  ├── ✅ 节点服务生命周期管理
  └── ✅ 验收：启动 → 注册 → 心跳 → 离线检测

🟡 Week 3-4: Phase 2 — Skill 框架 + 任务执行
  ├── ✅ 内置 Shell / FileOps Skill
  ├── ✅ TaskExecutor 执行引擎
  ├── ✅ 技能元数据加载（pkg/skills）
  ├── 🟡 外部 Skill 子进程执行框架
  ├── 🟡 Golem ↔ Hivemind Skill 桥接
  └── 🟡 验收：任务下发 → 执行 → 上报 → 取消

⏳ Week 5: Phase 3 — Scheduler 集成
  ├── ⏳ Scheduler ↔ NodeManager 桥接
  ├── ⏳ Hivemind → Golem 连接池
  ├── ⏳ Agent Runtime 集成
  └── ⏳ 验收：多 Golem 智能调度 + Agent 端到端

⏳ Week 6+: Phase 4 — 工作区 + 沙箱（渐进式）
  ├── ⏳ Workspace Manager
  └── ⏳ Sandbox 隔离
```

### 9.3 关键 TODO 项目

**优先级 P0（Phase 2 完成所需）**：
- 🟡 实现外部 Skill 子进程执行框架（stdin/stdout JSON 通信）
- 🟡 实现 Hivemind NodeManager 模块（节点注册、心跳管理、健康检查）
- 🟡 实现 Hivemind gRPC Handler（GolemNodeServiceServer）

**优先级 P1（Phase 3 完成所需）**：
- ⏳ Scheduler ↔ NodeManager 集成
- ⏳ Hivemind → Golem 连接池管理
- ⏳ GolemTool (InvokableTool) 实现

**优先级 P2（渐进式增强）**：
- ⏳ 工作区管理（Workspace Manager）
- ⏳ 沙箱隔离（Sandbox + chroot / namespace）
- ⏳ 远程 Skill 安装（`echoryn skill install <git-url>`）
- ⏳ Skill 版本管理 + 更新检查

---

## 十、实现细节总结

### 10.1 Golem 端 — 文件组织

```
internal/golem/                           # Golem 核心实现
├── app.go                                # 应用入口（初始化日志）✅
├── run.go                                # Run 函数（组装完整生命周期）✅
├── server.go                             # 服务器组装（146 行）✅
├── config/config.go                      # Config → Complete → New ✅
├── options/options.go                    # CLI 选项（141 行）✅
└── service/
    └── node/
        ├── module.go                     # 模块初始化（47 行）✅
        ├── service.go                    # 节点服务生命周期（281 行）✅
        ├── heartbeat.go                  # 双向心跳流（189 行）✅
        └── (无独立 reporter/registration，已合并到 service.go)
└── handler/
    └── control_handler.go                # TaskExecutor（226 行）✅
        ├─ HandleTask()                   # 异步处理任务
        ├─ CancelTask()                   # 中止任务
        ├─ executeShell()                 # 内置 Shell 技能
        └─ executeFileOps()               # 内置 FileOps 技能
```

### 10.2 关键实现特征

**1. 无 gRPC Server 架构**
- Golem 仅作为 gRPC Client 连接 Hivemind
- 任务分发完全通过双向心跳流完成
- 更安全（NAT/防火墙友好）

**2. 双向心跳流的多重职责**
```go
HeartbeatResponse 可能携带的指令：
├── HEARTBEAT_ACTION_NONE        // 无操作
├── HEARTBEAT_ACTION_DRAIN       // 排水（停止接收新任务）
├── HEARTBEAT_ACTION_SHUTDOWN    // 关闭（优雅退出）
├── HEARTBEAT_ACTION_DISPATCH_TASK  // 分发任务
└── HEARTBEAT_ACTION_CANCEL_TASK    // 取消任务
```

**3. TaskExecutor 的并发管理**
```go
type TaskExecutor struct {
    nodeService *node.Service
    mu        sync.Mutex
    cancelFns map[string]context.CancelFunc  // taskID → cancel
}
```
- 支持同时执行多个任务（默认 5 个并发）
- 通过 context.CancelFunc 映射表管理任务生命周期

**4. 技能加载的三层体系**
- **内置技能**：编译时链接，Go 代码实现
- **外部技能**：运行时扫描 `~/.echoryn/golem/skills/`，SKILL.md 描述
- **热加载**：fsnotify 监听目录变化，无需重启

### 10.3 关键数据流

**任务执行完整链路**：
```
1. Hivemind Scheduler 选择合适的 Golem
2. gRPC DispatchTask → Golem（通过心跳流）
3. TaskExecutor.HandleTask() 接收任务
4. executeTask() 根据 capability 匹配 Skill
5. executeShell/executeFileOps 执行
6. ReportTaskResult() 上报结果
7. Agent 上下文注入结果
```

**心跳流的生命周期**：
```
Golem 启动 → 创建 gRPC Client → Register → 进入心跳循环
            ↓
每 15s：发送 HeartbeatRequest（loadInfo）
        接收 HeartbeatResponse（可能含任务派发）
        处理任务 / 取消 / 排水 / 关闭指令
        ↓
Golem 关闭 → Deregister → 清理 gRPC 连接
```

### 10.4 已知限制 & 改进方向

| 项目 | 当前状态 | 限制 | 改进方向 |
|------|---------|------|---------|
| **外部 Skill** | 🟡 框架就位，子进程执行待实现 | 无法执行复杂脚本、依赖管理 | 容器隔离、虚拟环境 |
| **NodeManager** | 🟡 规范定义，Hivemind 侧实现待完成 | 无持久化、内存态 | 支持 SQLite 持久化 |
| **Scheduler 集成** | ⏳ Scheduler 框架 90% 完成 | 未与 NodeManager 对接 | 集成后支持智能调度 |
| **工作区隔离** | ⏳ 尚未实现 | Shell 任务无隔离 | chroot / namespace / 容器 |
| **连接池** | ⏳ 单连接 | 单点瓶颈 | 连接复用 + 池管理 |
| **监控指标** | ✅ 基础 | 无详细指标 | OpenTelemetry 集成 |

## 十一、对齐检查清单

以下检查确保 Golem 开发风格与 Hivemind 完全对齐：

- [x] **Config → Complete → New** 三阶段初始化（所有模块）
- [x] **接口探测模式** — Skill 通过实现可选接口自动注册能力
- [x] **Spec/Status 分离** — NodeState 遵循 K8s 声明式风格
- [x] **DDD 分层** — entity → repo → service → module
- [x] **in-tree / out-of-tree** — Skill 支持内置和外部扩展
- [x] **Anthropic Agent Skills 规范** — 外部 Skill 遵循 SKILL.md + scripts 标准
- [x] **Eino Tool 桥接** — GolemTool 实现 `tool.InvokableTool`，与 PluginTool / MCP Tool 对齐
- [x] **运行时热加载** — 外部 Skill 通过 fsnotify 监听 `~/.echoryn/golem/skills/` 目录实现热加载
- [x] **渐进式披露** — Skill 元数据/指令/资源分层加载，优化 token 消耗
- [x] **优雅关闭** — 复用 `pkg/http/shutdown` 框架
- [x] **集中路径解析** — 复用 `pkg/paths`
- [x] **错误码体系** — 复用 `pkg/errorx` 模式
- [x] **日志规范** — 复用 `pkg/logger`
- [x] **CLI 框架** — 复用 `pkg/app` (Cobra + Viper)
- [x] **Proto 生成** — 复用 Makefile 中的 `make proto` 流程

---

## 十二、快速参考

### 启动 Golem（开发模式）

```bash
# 编译
make build

# 运行 Golem（连接本地 Hivemind :11788）
./output/golem \
  --hivemind-address=127.0.0.1:11788 \
  --node-name=golem-local \
  --max-concurrent-tasks=5 \
  --skills-dir=~/.echoryn/golem/skills/
```

### 内置 Skill 使用

**Shell 技能**：
```json
{
  "type": "shell",
  "payload": {
    "command": "echo 'Hello Golem'",
    "working_dir": "/tmp",
    "timeout": "30s"
  }
}
```

**FileOps 技能**：
```json
{
  "type": "fileops",
  "payload": {
    "operation": "read",
    "path": "/path/to/file"
  }
}
```

### 添加外部 Skill

1. 在 `~/.echoryn/golem/skills/` 下创建目录：
```bash
mkdir -p ~/.echoryn/golem/skills/my-skill/scripts
```

2. 编写 `SKILL.md`：
```yaml
---
name: my-skill
description: My custom skill
version: "1.0"
metadata:
  capabilities:
    - my-operation
---

# My Skill

This is my custom skill...
```

3. 编写执行脚本 `scripts/run.sh` 或 `scripts/main.py`
4. Golem 自动发现并注册（若启用热加载）

---

## 十三、常见问题

**Q: Golem 与 MCP 的关系？**
A: MCP 是 Echoryn 连接外部服务的通道，Skill 是 Golem 本地能力的描述。MCP Tool 通过 Skill 执行（MCP-backed Skill）。

**Q: 外部 Skill 可以用什么语言？**
A: 任何可执行的脚本语言（Bash、Python、JavaScript 等），只要能处理 stdin/stdout JSON。

**Q: 如何保证 Skill 执行的安全性？**
A: 当前通过文件路径检查和超时限制；未来通过沙箱/容器隔离完整实现。

**Q: Golem 如何处理长时间运行的任务？**
A: 支持流式进度上报（ReportTaskProgress），Agent 可实时看到执行进度。

**Q: 单 Golem vs 多 Golem 有何区别？**
A: 单 Golem 时 Scheduler 跳过评分直接使用；多 Golem 时进行多维评分选择最合适的节点。
