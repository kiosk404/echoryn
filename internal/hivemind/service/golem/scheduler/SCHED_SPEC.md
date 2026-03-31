# Scheduler — HiveMind 任务调度引擎

`scheduler` 包实现了 HiveMind 的任务调度引擎。它负责接收任务请求，选择最优的 Golem 节点执行任务，调度任务的分发，并监控其整个生命周期。

设计遵循 Kubernetes 调度器的架构哲学：基于可插拔的 Strategy 选择管道，由优先级队列支持，后台进行协调循环，并通过事件驱动的观察者系统进行通知。

## 两种调度模式

调度器支持两种截然不同的调度模式，由每个 `ScheduleRequest` 中的 `ScheduleMode` 字段控制：

### DirectMode ("direct")

调用者显式指定一个目标 Golem 节点的 ID。这适用于人工操作员或上游系统已经知道应由哪个 Golem 执行任务的场景（例如，用户说"在 golem-3 上运行此任务"）。

调度器会验证目标节点是否：
- 存在
- 在线
- 满足所有硬约束（所需能力、已安装的技能、支持的功能、资源阈值）

如果验证通过，任务会立即分发；否则，请求会被拒绝并给出明确的原因。

### AIMode ("ai")

调度器自主选择最佳的 Golem 节点。这是默认模式，设计用于常见场景：调用者描述它需要什么（能力、技能、功能、资源最小值），让调度引擎判断哪个 Golem 最适合。

AI 选择器会根据多维加权评分模型评估每个在线 Golem，并选出评分最高的符合条件的节点。

## AI 选择器评分模型

在 AIMode 下运行时，`AISelector` 会从六个维度对每个候选 Golem 进行评分：

| 维度 | 评分逻辑 |
|------|----------|
| **Capability Score** | Golem 是否声明了任务所需的全部能力（例如 "bash"、"browser"、"coding"）？完全匹配得 1.0 分；部分匹配得按比例计算的分数。 |
| **Skill Score** | Golem 是否安装了所有必需的技能？调度器会将请求的 `RequiredSkills` 与 Golem 的 `InstalledSkills` 列表进行对比。完全匹配得 1.0 分。 |
| **Resource Score** | Golem 在 CPU、内存和磁盘方面有多少剩余空间？根据最新的心跳数据计算。一个完全空闲、资源充足的节点得分接近 1.0。 |
| **Load Score** | Golem 当前有多忙？通过正在运行和排队的任务数量来衡量，使用指数衰减函数，因此轻负载的节点会被强烈偏好。 |
| **Tag Score** | 有多少调用者偏好的标签（例如 "region=us-west"、"gpu=true"）与 Golem 的标签匹配？这是一个软偏好，不是硬约束。 |
| **Affinity Score** | Golem 是否匹配调用者的亲和性提示（例如用于会话粘性）或落在反亲和黑名单上？亲和性匹配得 1.0 分；反亲和性匹配得 0.0 分；中立节点得 0.5 分。 |

### 默认权重

每个维度都有可配置的权重（参见 `ScoringWeights`）。默认权重为：

```go
Capability: 0.25
Skill:      0.20
Resource:   0.20
Load:       0.20
Tag:        0.10
Affinity:   0.05
```

最终得分是加权总和。得分最高的节点获胜。

### 硬约束检查

在评分之前，每个候选节点必须通过一组硬约束检查：
- 在线状态
- 所需能力
- 所需技能
- 所需功能
- 资源最小值

未通过任何硬约束的节点会被标记为不符合条件并从排名中排除。

## GolemProfile — 调度器了解每个 Golem 的什么信息

调度器的决策质量取决于它对每个 Golem 掌握的数据丰富程度。`GolemProfile` 结构体聚合了：

| 字段 | 描述 |
|------|------|
| **NodeInfo** | 静态注册数据：操作系统、架构、CPU 核心数、总内存、磁盘空间、Go 版本、主机名，以及声明的能力列表 (`NodeCapability`)。 |
| **NodeLoadInfo** | 动态心跳数据：当前 CPU%、内存%、活动任务计数、排队任务计数。 |
| **InstalledSkills** | 当前安装在 Golem 上的技能（插件）集合，每个 skill 都有 ID、名称、版本以及它提供的能力。 |
| **SupportedFeatures** | 高级特性标志，如 "browser_automation"、"gpu_inference"、"sandbox_execution"。 |
| **Tags** | 用于过滤和软偏好匹配的任意键值对标签。 |
| **HealthScore** | 综合健康指标 (0.0–1.0)，由心跳新鲜度、错误率和资源余量导出。 |

## 架构和设计模式

调度器采用了多种知名的设计模式：

| 模式 | 说明 |
|------|------|
| **Strategy Pattern (策略模式)** | `NodeSelector` 是策略接口。`DirectSelector` 和 `AISelector` 是可互换的实现。调用者的 `ScheduleMode` 决定了运行时调用哪个策略。 |
| **Facade Pattern (门面模式)** | `Scheduler` 接口将队列、选择器、监控和统计收集器的复杂性隐藏在一个统一的 API 后面：`Schedule / Cancel / Status / Stats / Subscribe`。 |
| **Builder Pattern (构建器模式)** | `ScheduleRequestBuilder` 提供了一个流畅的 API 用于构建 `ScheduleRequest` 实例。 |
| **Observer Pattern (观察者模式)** | `TaskEventListener` 接收生命周期事件（已提交、已分配、进度、完成、失败、已取消、超时、已重新调度）。`TaskEventListenerFunc` 将普通函数适配到接口（类似 `http.HandlerFunc`）。 |
| **Decorator Pattern (装饰器模式)** | `FilterSelector` 用预过滤逻辑包装任意 `NodeSelector`，`CompositeSelector` 在责任链中链接多个选择器。 |
| **Options Pattern (选项模式，k8s 风格)** | `SchedulerConfig` → `CompletedSchedulerConfig` → `New()` 遵循 Kubernetes 的 `Config → Complete() → New()` 约定。 |

## 生命周期

典型的生命周期如下：

```go
cfg := scheduler.DefaultSchedulerConfig()
completed, err := cfg.Complete(profileProvider, dispatcher)
if err != nil { ... }

sched := completed.New()
sched.Start(ctx)
defer sched.Stop(ctx)

// Direct 模式：用户显式选择一个 Golem。
decision, err := sched.Schedule(ctx,
    scheduler.NewScheduleRequest(task).
        WithDirectMode("golem-3").
        WithRequiredCapabilities("bash", "file_ops").
        Build(),
)

// AI 模式：让调度器选择最佳的 Golem。
decision, err := sched.Schedule(ctx,
    scheduler.NewScheduleRequest(task).
        WithAIMode().
        WithRequiredSkills("web-scraper").
        WithRequiredFeatures("browser_automation").
        WithResourceRequirements(&scheduler.ResourceRequirements{
            MinMemoryMB: 2048,
            MaxCPUPercent: 80,
        }).
        WithHints(&scheduler.ScheduleHints{
            PreferLowLatency: true,
            Affinity:         "golem-7",
        }).
        Build(),
)
```
