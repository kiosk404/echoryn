# Echoryn TODO — 开发任务清单

> 按优先级和领域组织，聚焦可执行任务，去除冗余对比分析。
> 最后更新：2026-04-05

---

## 一、架构治理（P0 — 代码质量基础）

当前多个核心包存在**扁平结构 + 文件过长 + BC 边界泄漏**问题，需渐进重构。

### 1.1 Team 包 DDD 重构（渐进 4 Phase）

**现状**：`internal/hivemind/service/team/` 14 个文件 3285 行，全部平铺在同一目录。`orchestrator_impl.go` 906 行，直接 import 了 agents BC 的 4 个包（`entity`, `repo`, `subagent`, `messagebus`），破坏了 BC 边界。

**已具备的 DDD 元素**（做得好的）：
- `app_service.go` — 标准 Application Service Facade
- `execution_port.go` — 标准 DDD Port（依赖反转正确）
- `events.go` — 领域事件 + `TeamPublisher` 接口
- `registry.go` — Repository 接口与实现分离
- `WorkerRef` — 正确的 BC 边界值对象

**渐进路线**：

- [ ] **Phase 0: 移除 orchestrator_impl.go legacy path**
    - 删除 `subAgentManager`、`sessionRepo` 字段和所有 legacy 分支代码
    - 让 `ExecutionPort` 成为唯一执行路径
    - **效果**：消除 3 个 agents/ import（`entity`, `repo`, `subagent`）
    - 工作量：中（需确认 ExecutionPort 覆盖所有场景）

- [ ] **Phase 1: EventBridge 移至 integration 层**
    - 在 team 包中只保留 `LifecycleHook` 接口定义
    - 具体桥接实现（`OnSubAgentTerminal` + `entity.SubAgentRecord` 依赖）移到 `integration/`
    - **效果**：消除最后 1 个 agents/ import，Team BC 对 Agents BC 依赖 = 0
    - 工作量：小

- [ ] **Phase 2: 目录分层**
    - 拆分为 `domain/entity/`、`domain/service/`、`domain/repo/`、`domain/port/`、`infra/`、`app/`
    - 拆分 `TeamRegistry` 为 `TeamRepository` + `TemplateRepository`（两个聚合根独立 repo）
    - 工作量：中（14 文件 + 7 个外部导入者改路径）

- [ ] **Phase 3: 添加 module.go 入口**
    - 对齐 agents 模块的 `Config → Complete() → New()` 模式
    - 接入 `PolicyService`（当前定义但未使用）
    - 工作量：小

### 1.2 大文件拆分

以下文件超过 500 行，应按职责拆分：

| 文件 | 行数 | 建议拆分 |
|------|------|----------|
| `agents/domain/service/runtime/runner.go` | 900+ | 按阶段拆分（resolve/context/execute/postprocess） |
| `team/orchestrator_impl.go` | 906 | Phase 0 后减至 ~600 行；Phase 2 时拆为 orchestrator + member_manager |
| `team/registry_boltdb.go` | 415 | Phase 2 时拆为 team_store + template_store |
| `team/entity.go` | 482 | Phase 2 时拆为 team.go + template.go + value_objects.go |
| `agents/domain/service/runtime/executor.go` | 500+ | 按恢复分支拆分 |

### 1.3 PolicyService 接入

- [ ] `team/policy_service.go` 已定义 `PolicyService` 接口但 `orchestratorImpl` 未使用
    - `maxTeamMembers` 硬编码应改为从 `PolicyService.MaxTeamMembers()` 获取
    - 工作量：小

---

## 二、Team 模式增强（P0 — 当前开发重点）

> 详细设计见 `docs/ECHORYN_TEAM_ENHANCEMENT_SPEC.md`

### 2.1 SSE 实时推送 ✅ 已完成

- [x] `ChannelTeamPublisher` — per-team subscriber channels
- [x] SSE Handler `GET /v1/teams/:id/events`
- [x] 注入到 Orchestrator + 路由注册
- [x] `TeamEventSubscriber` 通用抽象接口（TUI/GUI 共用）
- [x] `TeamHTTPSubscriber` — HTTP SSE 客户端实现
- [x] TUI 后台事件监听（`teamEventWatcher` + `TeamEventHandler`）
- [x] `NotifyMemberCompleted` 补发事件

### 2.2 Team Agent 工具（待实现）

- [ ] 创建 `team-agent` 内置插件
    - `team_create` / `team_dissolve` / `team_status` / `team_message` 4 个工具
    - 让 Agent 能自主创建和管理 Team（类似 DeerFlow 的 `task()` 委派）
    - 工作量：中

- [ ] 注册到 `builtin/registry.go`
    - 工作量：小

- [ ] PromptSection 注入
    - 引导 Agent 选择 `sessions_spawn`（轻量单任务）vs `team_create`（复杂多角色协作）
    - 工作量：小

### 2.3 MessageBus → Agent Runtime 桥接（待实现）

- [ ] `SessionController.InjectTeamMessage()` — 通过 SteerChannel 注入团队消息到成员 Agent
    - 让 `team_message` 工具和 `/msg` 命令的消息真正到达成员 Agent
    - 工作量：中

### 2.4 Agent 决策引导

- [ ] 系统提示中注入 Team 感知信息
    - Team 成员使用 `PromptMode=minimal`，不注入 team 工具，防止递归创建
    - 工作量：小

---

## 三、TUI 交互增强（P1）

### 3.1 斜杠命令扩充

- [ ] `/model` / `/models` — 模型选择和切换
- [ ] `/session` / `/sessions` — 会话管理
- [ ] `/think` — Thinking Level 切换
- [ ] `/status` — 当前状态总览
- [ ] `/usage` — Token 用量显示
- [ ] `/abort` — 中止当前运行（后端 `AgentRunner.Abort()` 已实现，TUI 需接入 Escape 键绑定）
- 工作量：中

### 3.2 工具调用展示

- [ ] ToolExecutionComponent：emoji + 标题 + running/completed 标记 + 输出预览
- 工作量：中

### 3.3 对话中止

- [ ] TUI Escape 键 → HTTP `/abort` → `AgentRunner.Abort()`
- 后端已完成，仅需串联
- 工作量：小

### 3.4 Token 用量显示

- [ ] footer `tokens 12.5k/128k (10%)`
- 工作量：小

---

## 四、Agent Runner 增强（P1）

### 4.1 Auth Profile 轮换

- [ ] 3 种凭据类型（api_key / token / oauth）+ round-robin + 冷却
- [ ] 按失败原因分类（auth / rate_limit / billing / timeout 等）
- [ ] `CalcDynamicMaxAttempts()` 已预留接口
- 工作量：中，**P1 优先**

### 4.2 Stream 包装补全

- [ ] 请求侧层 4: model-specific sanitization
- [ ] 请求侧层 5: trace/logging middleware
- 工作量：小

### 4.3 Tool Result Session 级截断

- [ ] 单个 tool result 最多 30% context（绝对上限 400K chars）
- [ ] Context Guard：总预算 75% headroom
- 工作量：中

### 4.4 Compaction Safeguard 高级特性

- [ ] 文件操作跟踪（readFiles / modifiedFiles）
- [ ] 工具失败收集（MAX_TOOL_FAILURES=8）
- [ ] workspace 关键规则注入（AGENTS.md 的 "Session Startup" + "Red Lines"）
- [ ] 自适应分块（BASE_CHUNK_RATIO=0.4 → 按消息大小自动降低）
- 工作量：中

---

## 五、记忆系统增强（P1.5）

核心混合搜索（向量 0.7 + BM25 0.3 + MMR + 时间衰减 + 7 语言查询扩展 + LLM Memory Flush）已完成。

- [ ] **Session 记忆 Hook** — /new 或 /reset 时 LLM 摘要保存 + JSONL 解析 + 敏感信息脱敏
- [ ] **Embedding 模型限制表** — per-model max input tokens + 二分搜索截断
- [ ] **Voyage / Mistral Embedding Provider** — 扩展到 5 个 provider
- [ ] **Post-Compaction 恢复** — "Execute your Session Startup sequence now" 提醒
- 工作量：各项小-中

---

## 六、SubAgent Plan 机制（P2）

> 详细设计见本文第九章

### 6.1 阶段 1：Plan + Executor 抽象（当下实现）

- [ ] `SubAgentPlan` + `PlanItem` 实体
- [ ] `SubAgentExecutor` 接口 + `LocalExecutor`（从 manager 抽取）+ `RemoteExecutor`（占位）
- [ ] `ExecutorRouter` — 路由决策
- [ ] `sessions_plan` 工具 — LLM 结构化规划
- [ ] Plan 感知 Prompt 注入（`section_plan.go` Priority=350）
- [ ] Announce 防抖队列 + 合并播报 + 结果摘要
- [ ] 自动归档清扫器
- 预估：800-1000 行新增/改造

### 6.2 阶段 2-4：分布式 Golem（骨架已完成）

**已完成**：
- [x] `golem/registry/` — InMemoryRegistry + 健康检查
- [x] `golem/dispatcher/` — GRPCDispatcher + 连接池
- [x] `golem/scheduler/` — defaultScheduler + NodeSelector + PriorityQueue
- [x] Proto 定义 + gRPC 服务注册

**待实现**：
- [ ] `echoadm join` — Golem 通过 join token 注册到 Hivemind
- [ ] Golem Worker 运行时 — `internal/golem/app.go` 空壳实现
- [ ] `RemoteExecutor` 实装 — gRPC 下发任务 → 流式接收结果
- [ ] Skill 上报 + 节点调度匹配
- 工作量：大

---

## 七、Plugin 框架增强（P2）

- [ ] Hook 种类扩充（当前 8 种 → 按需增加 Message/Tool/Session/Subagent 类）
- [ ] Prompt Section 扩充（当前 ~10 → 补充 Safety/Skills/ReplyTags/DateTime 等）
- [ ] System Prompt 文件大小限制（单文件 20K / 总量 150K chars）
- 工作量：中，按需增量补充

---

## 八、IM Gateway 扩展 + CLI 工具链（P2）

### 8.1 IM Gateway

- [x] 飞书 + Telegram ✅
- [ ] Discord / Slack / 钉钉 / WhatsApp 等
- 工作量：每个渠道中等

### 8.2 CLI 工具链

- [ ] `echoadm join` — Golem 注册命令
- [ ] `echoadm doctor` — 诊断检查
- [ ] `echoctl models` — 模型管理 CLI
- [ ] `echoadm Factory` 实现 — 6 个方法待实现
- 工作量：各项小-中

---

## 九、SubAgent Plan 机制详细设计

> 融合 DeepAgent WriteTodos + OpenClaw 队列化 + Echoryn K8S Controller 模式

### 架构总览

```
LLM → sessions_plan(items=[...])
  → 按计划逐个 sessions_spawn
    → ExecutorRouter
      ├─ LocalExecutor (当前 Runner)
      └─ RemoteExecutor (gRPC → Golem, 阶段 3)
  → AnnounceQueue 合并播报
  → 主 Agent 看到 Plan 上下文 + 结果摘要
```

### 改造文件清单

| # | 操作 | 文件 | 说明 |
|---|------|------|------|
| 1 | 新增 | `entity/subagent_plan.go` | Plan + PlanItem 实体 |
| 2 | 新增 | `runtime/subagent_executor.go` | Executor 接口 |
| 3 | 新增 | `runtime/subagent_executor_local.go` | 本地执行器 |
| 4 | 新增 | `runtime/subagent_executor_remote.go` | 远程执行器（占位） |
| 5 | 新增 | `runtime/subagent_executor_router.go` | 路由器 |
| 6 | 改造 | `runtime/subagent_manager.go` | Spawn 增加 Plan 感知 |
| 7 | 改造 | `runtime/announce_controller.go` | 防抖 + 合并 + 摘要 |
| 8 | 改造 | `entity/subagent.go` | SpawnRequest 增加字段 |
| 9 | 改造 | `plugin/builtin/subagent/plugin.go` | 新增 sessions_plan 工具 |
| 10 | 新增 | `prompt/section_plan.go` | Plan Prompt 注入 |
| 11 | 新增 | `runtime/subagent_sweeper.go` | 归档清扫器 |

### 分布式平滑过渡

```
阶段 1 (单机): Router → LocalExecutor → Runner
阶段 3 (分布): Router → RemoteExecutor → gRPC → Golem
```

唯一变化点是 `Router.Route()` 返回值，上层 Plan/Manager/Announce/Store 全部不变。

---

## 十、开发路线总结

```
Phase 1 (当下):
  ├─ 架构治理: Team BC Phase 0-1（消除 agents/ 依赖）
  ├─ Team 增强: team-agent 插件 + MessageBus 桥接
  └─ TUI: 斜杠命令 + 对话中止 + Tool 展示

Phase 2:
  ├─ Agent Runner: Auth Profile + Compaction Safeguard
  ├─ 记忆: Session Hook + Embedding 增强
  └─ Team BC Phase 2-3（目录分层 + module.go）

Phase 3:
  ├─ SubAgent Plan 机制
  ├─ Plugin 框架增强
  └─ IM Gateway 扩展

Phase 4:
  ├─ Golem Worker 运行时 + echoadm join
  └─ RemoteExecutor + Skill 调度
```

---

## 附录 A：已完成模块对齐度

| 模块 | 对齐度 | 说明 |
|------|--------|------|
| Memory-Core | 95%+ ✅ | 混合搜索/MMR/时间衰减/查询扩展/Memory Flush 全部对齐 |
| Context Management | 90%+ ✅ | Window Guard/Compaction/Pruning 核心对齐，高级 Safeguard 待补 |
| Agent Runner | 85%+ ✅ | 主循环/3 分支恢复/动态预算/Hook/Fallback 完全对齐，缺 Auth Profile |
| Thinking Level | 100% ✅ | 6 级 + Strategy + 流式 Reasoning + 错误驱动降级 |
| Stream Sanitizer | 85% ✅ | 请求侧 3 层 + 响应侧对齐，缺 model-specific + trace |
| Skills 系统 | 100% ✅ | Loader + Registry + Watcher + Tools，Go 侧架构更完整 |
| IM Gateway | 首批 ✅ | 飞书 + Telegram，核心层（Manager/Dispatcher/Deliverer）完整 |
| Golem 架构 | 骨架 ✅ | Registry + Dispatcher + Scheduler 三层，Worker 运行时待实现 |
| Team SSE 推送 | ✅ | Publisher + SSE Handler + 客户端 Subscriber + TUI Watcher |
