# Agent Run：一次对话请求的奇幻漂流

> 本文深入剖析 Echoryn Hivemind 中 Agent 执行引擎的内部机制，
> 从一个用户消息进入系统开始，追踪它如何被处理、编排、最终生成回复的完整旅程。

---

## 引言：当用户按下回车

当用户在聊天界面按下回车，一条消息开始了它在系统内部的漫长旅程。这个旅程看似简单——不过是"发送消息，等待回复"——但在引擎内部，却是一场精心编排的多层协作：

```
用户消息 → AgentRunner → TurnExecutor → LLM → Tool Execution → Context Management → 最终回复
```

这条链路上，每一个环节都有其独特的职责：**编排**协调全局流程，**执行**处理重试降级，**上下文**管理 Token 预算，**压缩**在溢出时抢救会话。这些机制共同构成了一个健壮的 Agent 运行时。

本文将从核心概念出发，逐层揭开这个执行引擎的面纱。

---

## 一、核心抽象：三个关键实体

在深入执行流程之前，我们需要先理解三个核心实体的职责边界：

### 1.1 Agent：人格的载体

Agent 是对话的"人格配置"——它定义了**用哪个模型**、**有哪些能力**、**如何表现**：

```
Agent = {
    身份标识: ID, Name, Description
    模型配置: ModelRef (provider/model) + Fallback 链
    能力集合: Tools[] (插件工具) + MCPServers[] (MCP 工具)
    行为参数: MaxTurns, Temperature, MaxTokens, ThinkingLevel
    人格设定: SystemPrompt + Persona (Identity + WorkspaceDir)
}
```

**Fallback 链**是一个关键设计：当主模型失败时，系统会自动尝试备用模型。这不仅仅是简单的"换一个模型"，而是配合错误分类、冷却期管理、Thinking Level 降级的复杂决策链。

### 1.2 Session：记忆的容器

Session 是对话历史的持久化载体，但它的设计比简单的"消息列表"更精巧：

```
Session = {
    标识: ID, AgentID, ParentSessionID (子 Agent 场景)
    消息历史: Messages[] + FirstKeptIndex (压缩边界)
    压缩状态: CompactionSummary + CompactionCount
    Token 用量: Usage {Input, Output, Total}
}
```

**FirstKeptIndex** 是理解 Session 的关键：当会话过长需要压缩时，旧的对话会被 LLM 总结成摘要，`FirstKeptIndex` 标记了**从哪条消息开始是完整的原文**。这个设计实现了"摘要+原文"的混合记忆模式。

### 1.3 Run：一次执行的轨迹

Run 是单次请求执行的完整记录：

```
Run = {
    标识: ID, SessionID, AgentID
    状态机: Created → InProgress → Completed/Failed/Cancelled
    输入输出: Input, Output
    资源消耗: Usage, ModelRef (最终使用的模型)
    错误信息: Error {Code, Message}
}
```

Run 的状态机是严格的：一旦进入 `Completed` 或 `Failed`，就不可逆转。这个设计保证了执行记录的一致性。

---

## 二、AgentRunner：七层编排管线

`AgentRunner` 是整个执行流程的顶层编排器。它的设计借鉴了 OpenClaw 的 7 层管线思想，每一层都有明确的职责边界。

### 2.1 执行流程全景图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           AgentRunner.Run()                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Layer 1: 解析阶段                                                       │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐                   │
│  │ Resolve     │ → │ Load/Create │ → │ Create Run  │                   │
│  │ Agent       │   │ Session     │   │ Record      │                   │
│  └─────────────┘   └─────────────┘   └─────────────┘                   │
│                                                                          │
│  Layer 2: 初始化阶段                                                     │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐                   │
│  │ State       │ → │ Abort       │ → │ Stream Pipe │                   │
│  │ Machine     │   │ Controller  │   │ Creation    │                   │
│  └─────────────┘   └─────────────┘   └─────────────┘                   │
│                                                                          │
│  Layer 3: 预处理阶段 (异步)                                              │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐                   │
│  │ Fire Hooks  │ → │ Resolve     │ → │ Build Prompt│                   │
│  │ (memory)    │   │ Tools       │   │ Context     │                   │
│  └─────────────┘   └─────────────┘   └─────────────┘                   │
│                                                                          │
│  Layer 4: 上下文构建                                                     │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ ContextBuilder.Build()                                          │   │
│  │   System Prompt → Compaction Summary → Injected → History → Input│   │
│  │                              ↓                                   │   │
│  │                     ContextPruner.Prune()                        │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  Layer 5: 执行阶段                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ TurnExecutor.Execute()                                          │   │
│  │   (重试循环 + Fallback + 恢复分支)                               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  Layer 6: 后处理阶段                                                     │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐                   │
│  │ Persist     │ → │ Proactive   │ → │ Fire        │                   │
│  │ Session/Run │   │ Compaction  │   │ agent_end   │                   │
│  └─────────────┘   └─────────────┘   └─────────────┘                   │
│                                                                          │
│  Layer 7: 流式输出                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ StreamReader[AgentEvent] → 客户端消费                            │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2.2 关键设计点

**异步执行模式**

AgentRunner 采用"同步返回流，异步执行体"的模式：

```go
func (r *AgentRunner) Run(ctx, req) (*StreamReader[AgentEvent], error) {
    // 同步：解析 Agent、Session，创建 Run 记录
    // 同步：创建 StreamReader + StreamWriter
    // 异步：safego.Go(executeRun)
    // 立即返回 StreamReader
    return sr, nil
}
```

这种设计让调用方能够**立即获得流的控制权**，而执行体在后台 goroutine 中运行。客户端通过消费 `StreamReader` 实时接收事件。

**AbortController 机制**

外部可以通过 `Abort(runID)` 取消正在执行的 Run。实现原理是一个简单的 context 取消传播：

```
AbortController = parentCtx + cancelFunc + timeout
                    ↓
              childCtx = context.WithCancel(parentCtx)
                    ↓
              所有子操作检查 ctx.Done()
```

这个设计对超时控制和用户主动取消场景至关重要。

---

## 三、TurnExecutor：三分支恢复循环

`TurnExecutor` 是执行引擎中最复杂的组件，它实现了**多层容错**机制。理解它的关键是"三分支恢复循环"。

### 3.1 三层错误分类

并非所有错误都应该重试。系统将错误分为三类：

```
┌────────────────────────────────────────────────────────────────┐
│                        错误分类金字塔                           │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Layer 0: 瞬态网络错误                                          │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ connection reset, socket hang up, 502/503/504            │ │
│  │ 策略: 单次重试 (2.5s 延迟)                                │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  Layer 1: 上下文溢出                                            │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ context_length_exceeded, prompt too long, 上下文过长      │ │
│  │ 策略: 压缩会话，最多 3 次尝试                             │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  Layer 2: Thinking Level 不支持                                 │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ "reasoning_effort not supported", "supported values are"  │ │
│  │ 策略: 降级到支持的级别 (xhigh→high→medium→low)            │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  非恢复性错误: 格式错误、认证失败、内容过滤                      │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ 策略: 立即失败，不再重试                                  │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### 3.2 恢复循环流程图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    TurnExecutor.Execute() 恢复循环                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  for attempt < maxRetries:                                               │
│      │                                                                   │
│      ├─→ CheckAborted() ──→ 返回 ErrAborted                             │
│      │                                                                   │
│      ├─→ RunWithFallback(候选模型列表)                                   │
│      │       │                                                           │
│      │       ├─→ SkipOnCooldown? ──→ 跳过该模型                         │
│      │       │                                                           │
│      │       ├─→ executeSingleAttempt(model)                            │
│      │       │       │                                                   │
│      │       │       ├─→ SanitizerPipeline.Apply(messages)  [请求侧]    │
│      │       │       ├─→ AgentFlow.Stream(ctx, messages)                │
│      │       │       │       └─→ StreamMiddlewareChain [响应侧]         │
│      │       │       └─→ collectStreamResult()                          │
│      │       │                                                           │
│      │       ├─→ 成功 ──→ 返回 TurnResult                                │
│      │       └─→ 失败 ──→ 记录错误，尝试下一个候选                       │
│      │                                                                   │
│      ├─→ 所有候选都失败?                                                 │
│      │       │                                                           │
│      │       ├─→ 是 ContextOverflow 且 compactionAttempts < 3?          │
│      │       │       ├─→ Compactor.Compact(session)                     │
│      │       │       ├─→ ContextBuilder.Build() 重建上下文              │
│      │       │       └─→ continue (重试)                                │
│      │       │                                                           │
│      │       ├─→ 是 ThinkingLevelError 且有降级选项?                     │
│      │       │       ├─→ pickFallbackThinkingLevel()                    │
│      │       │       └─→ continue (降级重试)                            │
│      │       │                                                           │
│      │       ├─→ 是 TransientHTTPError 且未重试?                        │
│      │       │       ├─→ sleep(2.5s)                                    │
│      │       │       └─→ continue (单次重试)                            │
│      │       │                                                           │
│      │       └─→ 否则 ──→ 返回错误                                       │
│      │                                                                   │
│      └─→ attempt++                                                       │
│                                                                          │
│  返回 "max retries exceeded"                                             │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.3 动态重试预算

OpenClaw 采用动态公式计算重试预算：

```
maxRetries = BASE(24) + max(1, profileCount) * 8
maxRetries = clamp(maxRetries, 32, 160)
```

Echoryn 简化为静态默认值（3次），但保留了扩展点。核心思想是：**给恢复机制足够的尝试次数，但也要有明确的终止边界**。

---

## 四、ContextBuilder：上下文的精细管理

当消息历史越来越长，如何高效地构建 LLM 输入？`ContextBuilder` 实现了分层组装 + 智能裁剪的策略。

### 4.1 消息组装顺序

最终发送给 LLM 的消息列表，按以下顺序组装：

```
┌────────────────────────────────────────────────────────────┐
│                     消息组装顺序                           │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  1. System Prompt                                          │
│     └─→ PromptPipeline.Assemble() 或 agent.SystemPrompt   │
│                                                            │
│  2. Compaction Summary (如果存在)                          │
│     └─→ "[Conversation Summary]\n{摘要内容}"               │
│                                                            │
│  3. Injected Messages                                      │
│     └─→ 来自 before_agent_start 钩子的注入消息            │
│                                                            │
│  4. Session History                                        │
│     └─→ ActiveMessages()[FirstKeptIndex:]                 │
│     └─→ limitHistoryTurns(maxHistoryTurns) 裁剪           │
│                                                            │
│  5. Current User Input                                     │
│     └─→ 当前用户的最新消息                                │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

这个顺序的意义：
- **System Prompt 在最前**：定义 Agent 的基本行为
- **Summary 紧随其后**：为后续历史提供背景
- **Injected 中间插入**：如记忆检索结果
- **History 形成上下文**：保留最近的对话轮次
- **Input 在最后**：触发本次响应

### 4.2 两阶段裁剪策略

当估算 Token 超过可用预算时，`ContextPruner` 启动裁剪。关键是：**裁剪发生在深拷贝上，不修改原始消息**。

```
┌────────────────────────────────────────────────────────────────────────┐
│                        两阶段裁剪策略                                   │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  计算 ratio = estimatedTokens / usableTokens                           │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ ratio ≤ 0.3: 不裁剪                                              │  │
│  │   └─→ 直接返回原始消息                                           │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ 0.3 < ratio ≤ 0.5: Stage 1 - Soft Trim                          │  │
│  │   ├─→ 对保护边界前的 Tool 消息：                                 │  │
│  │   │     head(1500字符) + "\n... [N characters truncated] ...\n"  │  │
│  │   │     + tail(1500字符)                                        │  │
│  │   └─→ 保留首尾，裁剪中间                                         │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ ratio > 0.5: Stage 2 - Hard Clear                               │  │
│  │   └─→ 对保护边界前的 Tool 消息：                                 │  │
│  │         内容替换为 "[Old tool result content cleared]"           │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
│  保护机制：最后 3 个 Assistant 消息及之后的所有消息永远不会被裁剪      │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

**为什么保护最后 3 个 Assistant 消息？**

因为 Tool 调用的结果通常紧跟在 Assistant 消息之后。如果裁剪了这些 Tool 结果，当前的对话上下文就会断裂。保护最近几轮对话的完整性，是保证响应质量的关键。

---

## 五、PromptPipeline：Section 化的系统提示词

传统的系统提示词往往是硬编码的字符串拼接。Echoryn 采用了更灵活的**Section 化管线**设计，借鉴了 Kubernetes Admission Chain 的思想。

### 5.1 Section 的概念

一个 Section 是一个独立的提示词片段，具有**名称、优先级、启用条件、渲染逻辑**：

```go
type PromptSection interface {
    Name() string                              // 唯一标识
    Priority() int                             // 排序权重 (小数字先执行)
    Enabled(ctx, PromptContext) bool           // 动态启用判断
    Render(ctx, PromptContext) (string, error) // 生成文本
}
```

### 5.2 内置 Section 及优先级

```
┌────────────────────────────────────────────────────────────────────────┐
│                        Section 优先级列表                              │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  Priority │ Section Name       │ 内容                                 │
│  ─────────┼────────────────────┼────────────────────────────────────  │
│    100    │ IdentitySection    │ Agent 名称 + Vibe + 分布式架构意识   │
│    150    │ ClusterAwareness   │ Golem 集群拓扑 (如有)                │
│    200    │ ToolingSection     │ 可用工具列表 (Plugin + MCP 分组)     │
│    300    │ PersonaSection     │ 用户自定义 SystemPrompt              │
│    310    │ Workspace:soul     │ SOUL.md 内容                         │
│    320    │ Workspace:identity │ IDENTITY.md 内容                     │
│    330    │ Workspace:agents   │ AGENTS.md 内容                       │
│    350+   │ Workspace:extra    │ prompts/*.md 额外提示词              │
│    400    │ MemorySection      │ 记忆系统指令 (插件贡献)              │
│    900    │ RuntimeSection     │ 时间/模型/版本元信息                 │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

### 5.3 PromptMode 过滤

不同场景需要不同详细程度的提示词。`PromptMode` 通过优先级阈值控制包含范围：

| Mode | 阈值 | 包含的 Section |
|------|------|---------------|
| `none` | ≤100 | 仅 Identity |
| `minimal` | ≤500 | Identity + Cluster + Tooling + Persona + Workspace + Memory |
| `full` | 无限制 | 全部 Section |

**子 Agent 场景**通常使用 `minimal` 模式，减少不必要的上下文消耗。

### 5.4 Mutator 链

Section 组装完成后，还可以通过 **Mutator** 对结果进行后处理：

```go
type PromptMutator interface {
    Name() string
    Priority() int
    Mutate(ctx, PromptContext, assembled string) (string, error)
}
```

典型用途：注入动态数据、格式转换、敏感词过滤等。Mutator 按 Priority 顺序执行，形成处理链。

---

## 六、Compactor：会话压缩的艺术

当对话历史无限增长，终有一天会超出模型的上下文窗口。`Compactor` 负责在那一刻到来前或到来时，"压缩"历史以腾出空间。

### 6.1 触发时机

压缩有两种触发方式：

| 触发类型 | 时机 | 目的 |
|---------|------|------|
| **主动压缩** | 每轮对话后检查 | Token 使用率 > 阈值 (默认 0.8) 时提前压缩 |
| **被动压缩** | ContextOverflow 错误时 | 紧急压缩以恢复执行 |

### 6.2 压缩流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Compactor.Compact() 流程                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. Fire before_compaction 钩子                                         │
│     └─→ 允许插件执行预压缩操作 (如 memory flush)                        │
│                                                                         │
│  2. 确定分割点                                                           │
│     └─→ findCompactionSplitPoint()                                     │
│     └─→ 保留最近 N 轮 user→assistant 对话 (默认 3 轮)                  │
│                                                                         │
│  3. 分块摘要 (如果需要)                                                  │
│     ├─→ 消息总 Token < 40% 窗口? → 单次 LLM 摘要                        │
│     └─→ 否则 → 分块 → 逐块摘要 → 合并摘要                               │
│                                                                         │
│  4. 应用到 Session                                                       │
│     ├─→ session.CompactionSummary = summary                            │
│     ├─→ session.FirstKeptIndex = 压缩边界                              │
│     └─→ session.CompactionCount++                                      │
│                                                                         │
│  5. Fire after_compaction 钩子                                          │
│     └─→ 允许插件执行后压缩操作 (如 workspace context refresh)           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 6.3 多阶段摘要策略

当历史消息过长，无法一次性摘要时，采用**分块摘要 + 合并**策略：

```
消息序列 ──→ 分块 ──→ [Chunk1] ──→ LLM ──→ Summary1
                    [Chunk2] ──→ LLM ──→ Summary2
                    [Chunk3] ──→ LLM ──→ Summary3
                              │
                              ↓
                    [Summary1, Summary2, Summary3]
                              │
                              ↓
                           LLM 合并
                              │
                              ↓
                        最终 Summary
```

每个分块的摘要提示词会**携带之前累积的摘要**作为上下文，保证信息的连贯性。

---

## 七、Stream 包装管道：请求与响应的双重处理

在 TurnExecutor 中，消息在发送给 LLM 之前和接收流式响应时，都会经过处理管道。这个设计对齐了 OpenClaw 的 5 层 wrapper 架构。

### 7.1 请求侧：SanitizerPipeline

在消息发送给 LLM 之前，`SanitizerPipeline` 对消息进行预处理：

```
┌────────────────────────────────────────────────────────────────────────┐
│                     SanitizerPipeline (请求侧)                         │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  Messages ──→ [ThinkingBlockSanitizer] ──→ 移除 thinking 块           │
│                    │                                                   │
│                    ↓                                                   │
│              [ToolCallIDSanitizer] ──→ 规范化 tool_call.id             │
│                    │                                                   │
│                    ↓                                                   │
│              [ToolCallNameTrimSanitizer] ──→ 清理工具名前缀            │
│                    │                                                   │
│                    ↓                                                   │
│              Sanitized Messages ──→ 发送给 LLM                        │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

**为什么需要这些处理？**

- **ThinkingBlockSanitizer**：某些模型的响应中包含 `<thinking>` 块，如果被原样返回给模型，会干扰下一轮推理
- **ToolCallIDSanitizer**：不同 Provider 对 `tool_call.id` 格式要求不同，需要统一规范化
- **ToolCallNameTrimSanitizer**：清理工具名中可能的命名空间前缀，避免模型混淆

### 7.2 响应侧：StreamMiddlewareChain

在接收流式响应时，`StreamMiddlewareChain` 处理每个 chunk：

```
┌────────────────────────────────────────────────────────────────────────┐
│                   StreamMiddlewareChain (响应侧)                       │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  LLM Stream ──→ [TrimToolCallNamesMiddleware] ──→ 清理 chunk 中的工具名│
│                       │                                                │
│                       ↓                                                │
│                 [ChunkLoggerMiddleware] ──→ 调试日志记录               │
│                       │                                                │
│                       ↓                                                │
│                 Processed Chunks ──→ ReplayChunkCallback              │
│                                        │                               │
│                                        ↓                               │
│                                   AgentEvent 流                        │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

**双层管道的意义**：

请求侧和响应侧的处理分离，是因为它们的时机和目的不同：
- 请求侧：确保发送给 LLM 的数据是"干净的"
- 响应侧：处理流式数据中的异常情况和格式问题

这种分离也让测试更加清晰——可以单独测试每一层的处理逻辑。

---

## 八、错误码体系

系统定义了 9 个领域错误码，覆盖所有 Agent 执行相关的异常情况：

| 错误码 | 含义 | 典型场景 |
|--------|------|---------|
| `ErrAgentNotFound` | Agent 不存在 | 无效的 AgentID |
| `ErrSessionNotFound` | Session 不存在 | 无效的 SessionID |
| `ErrRunNotFound` | Run 不存在 | 查询不存在的 Run |
| `ErrRunAlreadyDone` | Run 已结束 | 尝试取消已完成的 Run |
| `ErrNoToolsAvailable` | 无可用工具 | Agent 未配置任何工具 |
| `ErrMaxTurnsExceeded` | 超过最大轮数 | 工具调用循环过多 |
| `ErrAborted` | 运行已中止 | 用户取消或超时 |
| `ErrContextOverflow` | 上下文溢出 | 压缩后仍超出窗口 |
| `ErrModelNotToolCapable` | 模型不支持工具 | 非 ToolCalling 模型配置了工具 |

这些错误码不仅用于内部判断，也会通过 API 返回给调用方，便于问题诊断。

---

## 九、与 OpenClaw 的架构对比

### 9.1 设计理念

| 维度 | Echoryn | OpenClaw |
|------|---------|----------|
| **语言** | Go | TypeScript |
| **编排引擎** | Eino ReAct DAG | 命令式循环 |
| **重试策略** | 3 分支恢复循环 | 类似 + Auth Profile 轮转 |
| **Prompt 管理** | Section 化管线 | 模板字符串拼接 |
| **错误恢复** | 基础 (overflow→compaction) | 复杂 (session reset/Gemini corruption) |
| **流式输出** | schema.Pipe[AgentEvent] | Block streaming pipeline |

### 9.2 核心对齐项

| 功能 | 对齐状态 | 说明 |
|------|---------|------|
| 3 层错误分类 + 恢复分支 | ✅ | Context Overflow / Transient HTTP / Thinking Level |
| 两阶段上下文裁剪 | ✅ | Soft-trim + Hard-clear |
| 会话压缩 | ✅ | 多块摘要 + 合并 |
| Stream 包装管道 | ✅ | 请求侧 Sanitizer + 响应侧 Middleware |
| 主动压缩检查 | ✅ | Post-turn threshold maintenance |
| Abort 机制 | ✅ | 外部取消 + 超时控制 |

### 9.3 差异项

| 功能 | Echoryn | OpenClaw |
|------|---------|----------|
| **Auth Profile 轮转** | ❌ | 支持多个 API Key 轮转 |
| **Session Reset** | ❌ | 支持 corruption 自动重置 |
| **Typing 信号** | ❌ | 打字指示器 |
| **媒体理解** | ❌ | 图片/链接/音频理解 |
| **指令系统** | ❌ | rich directive (model/thinking/verbose) |

---

## 十、总结：Agent Run 的核心设计原则

回顾整个执行引擎的设计，可以总结出几个核心原则：

**1. 分层职责，清晰边界**

AgentRunner 编排全局流程，TurnExecutor 处理执行细节，ContextBuilder 管理上下文，Compactor 负责压缩。每一层只关心自己的职责，通过接口而非实现依赖。

**2. 渐进式降级，永不放弃**

3 分支恢复循环确保在各种异常情况下都能尝试恢复：瞬态错误重试、上下文溢出压缩、Thinking Level 降级。只有真正无法恢复的错误才会失败。

**3. 流式优先，实时反馈**

从 AgentRunner 立即返回 StreamReader，到 TurnExecutor 流式处理 LLM 响应，整个系统设计为"推送"而非"拉取"模式，让客户端能够实时感知执行进度。

**4. 保护关键信息，智能裁剪**

两阶段裁剪策略优先保护最近几轮对话，确保当前上下文的连贯性。Tool 结果可以裁剪，但 user 和 assistant 的核心对话要保留。

**5. 插件化扩展，灵活组合**

PromptPipeline 的 Section 设计让提示词可以灵活组合，插件可以贡献自己的 Section。Hook 机制让插件能在关键节点注入逻辑。

这些原则共同构成了一个**健壮、可扩展、可维护**的 Agent 运行时引擎。

---

> 最后更新: 2026-02-01
