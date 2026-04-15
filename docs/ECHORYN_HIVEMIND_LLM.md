# Echoryn Hivemind — LLM 多模型管理详解

> 本文档是 `ECHORYN_SPEC.md` 的子文档，深入阐述 Hivemind 中 **LLM Module** 的完整实现逻辑。
>
> 代码位置: `internal/hivemind/service/llm/`

---

## 一、模块概述

LLM Module 是 Hivemind 的模型管理中枢，负责管理 8 个 LLM Provider 的注册、配置、调用、探测和故障转移。它采用 **DDD（领域驱动设计）+ K8s Scheduler Plugin Pattern** 的架构。

核心能力：
1. **SPI 四层插件体系** — Provider/ChatModel/Compat/Probe 分层接口
2. **8 个内置 Provider** — OpenAI/DeepSeek/Gemini/Claude/Qwen/Kimi/GLM/Ollama
3. **ModelManager** — 模型注册、查询、ChatModel 缓存
4. **FallbackExecutor** — 多模型故障转移
5. **ModelProber** — 并发健康探测
6. **CompatManager** — 模型兼容性规则引擎

---

## 二、架构分层

```
LLM Module (Config → Complete → New)
  │
  ├── Provider Layer (SPI 插件体系)
  │   ├─ spi/spi.go         — 4 层 SPI 接口定义
  │   ├─ registry.go         — Provider 注册中心 (线程安全)
  │   ├─ plugins.go          — In-Tree 注册表 (8 个内置 Provider)
  │   ├─ helper/helper.go    — BasePlugin 基类 + 环境变量解析
  │   ├─ helper/eino_adapter  — OpenAI 兼容 ChatModel 适配器
  │   └─ 8 个 Provider 实现目录
  │
  ├── Domain Layer
  │   ├─ entity/             — 14 个实体文件 (核心值对象)
  │   ├─ repository/         — ModelRepository + ProviderRepository
  │   └─ service/
  │       ├─ manager.go      — ModelManager 接口
  │       ├─ manager_impl.go — ModelManager 实现 (缓存 + 初始化)
  │       ├─ model_compat.go — CompatManager (规则引擎)
  │       ├─ model_fallback.go — FallbackExecutor (泛型)
  │       ├─ model_prober.go — ModelProber (并发探测)
  │       └─ model_meta.go   — ModelMetaConf (未使用)
  │
  └── Store Layer
      └─ inmemory/           — 内存存储实现
```

---

## 三、SPI 四层接口架构

### 3.1 接口定义

```go
// Layer 1: 基础插件 (必须实现)
type ProviderPlugin interface {
    Name() string
    DefaultConfig() *options.ModelProviderConfig  // 默认配置 (含环境变量 APIKey)
    BuildProvider(cfg) (*entity.ModelProvider, error)
    BuildModels(provider, cfg) ([]*entity.ModelInstance, error)
}

// Layer 2: ChatModel 构建 (推荐实现)
type ChatModelPlugin interface {
    BuildChatModel(ctx, instance, provider, params) (chat_model.ChatModel, error)
}

// Layer 3: 兼容性规则 (可选)
type CompatPlugin interface {
    CompatRules() []entity.ModelCompatRule
}

// Layer 4: 健康探测 (可选)
type ProbePlugin interface {
    Probe(ctx, instance, provider) *entity.ProbeResult
}
```

### 3.2 设计灵感

此架构借鉴 K8s Scheduler 的 `framework.Plugin` 扩展模式：
- **Layer 1** = 基础 Plugin（必须）
- **Layer 2~4** = 可选扩展点（按需实现）
- 每个 Provider 只需实现关心的层

---

## 四、Provider 注册与初始化

### 4.1 Registry — Provider 注册中心

```go
type Registry struct {
    mu        sync.RWMutex
    factories map[string]spi.PluginFactory  // name → factory
}
```

**关键特性**：
- 线程安全（RWMutex）
- 名称冲突检测（Register 返回 error）
- 支持 `Merge(other)` 合并 out-of-tree Registry

### 4.2 In-Tree 注册

`NewInTreeRegistry()` 集中注册 8 个内置 Provider，采用显式注册而非 `init()` 隐式注册：

```go
func NewInTreeRegistry() *Registry {
    r := NewRegistry()
    r.MustRegister("openai", func() spi.ProviderPlugin { return openai.New() })
    r.MustRegister("deepseek", ...)
    r.MustRegister("anthropic", ...)
    // ... 共 8 个
    return r
}
```

### 4.3 初始化流程 (4 Phase)

```
ModelManager.Initialize(ctx):
  │
  ├─ Phase 1: registerFromRegistry() — 自动发现
  │   └─ Registry.Range() → 每个 Plugin:
  │       ├─ 跳过用户已配置的 Provider
  │       ├─ DefaultConfig() → ResolveEnvValue(APIKey)
  │       │   例: "${OPENAI_API_KEY}" → 读取环境变量
  │       ├─ APIKey 为空 → 跳过 (无 Key 不注册)
  │       ├─ BuildProvider(cfg) → RegisterProvider()
  │       └─ BuildModels(provider, cfg) → RegisterModel() × N
  │
  ├─ Phase 2: registerProviderFromConfig() — 用户配置
  │   └─ 优先使用 Registry 中的 Plugin，否则回退到 generic BasePlugin
  │
  ├─ Phase 3: SetDefault(provider, model) — 设置默认模型
  │
  └─ Phase 4: CompatManager.Refresh() — 刷新兼容性规则
```

---

## 五、8 个 Provider 实现

### 5.1 实现模式分类

| Provider | 实现方式 | SDK | 环境变量 | BaseURL | 内置模型 |
|----------|---------|-----|----------|---------|----------|
| **OpenAI** | 继承 BasePlugin | eino-ext/openai | `OPENAI_API_KEY` | `api.openai.com/v1` | gpt-4o, gpt-4o-mini, o1, o1-mini, o3-mini |
| **DeepSeek** | 重写 BuildChatModel | eino-ext/deepseek | `DEEPSEEK_API_KEY` | `api.deepseek.com/v1` | deepseek-chat (V3), deepseek-reasoner (R1) |
| **Anthropic** | 重写 BuildChatModel | eino-ext/claude | `ANTHROPIC_API_KEY` | `api.anthropic.com` | claude-sonnet-4, claude-3.5-sonnet, claude-3.5-haiku |
| **Gemini** | 重写 BuildChatModel | eino-ext/gemini + google/genai | `GOOGLE_API_KEY` | `generativelanguage.googleapis.com` | gemini-2.5-pro, gemini-2.5-flash, gemini-2.0-flash |
| **Qwen** | 重写 BuildChatModel | eino-ext/qwen | `DASHSCOPE_API_KEY` | `dashscope.aliyuncs.com` | qwen-plus, qwen-turbo, qwen-max, qwq-plus |
| **Kimi** | 继承 BasePlugin | eino-ext/openai (兼容) | `MOONSHOT_API_KEY` | `api.moonshot.cn/v1` | moonshot-v1-auto, k2-0711-chat |
| **GLM** | 继承 BasePlugin | eino-ext/openai (兼容) | `ZHIPU_API_KEY` | `open.bigmodel.cn/api/paas/v4` | glm-4-plus, glm-4-flash |
| **Ollama** | 重写 BuildChatModel | eino-ext/ollama | `OLLAMA_API_KEY` | `localhost:11434/v1` | 空 (用户自行配置) |

### 5.2 两种实现模式

**OpenAI 兼容型** (openai/kimi/glm)：
- 嵌入 `helper.BasePlugin`
- 不重写 `BuildChatModel`
- 使用默认的 OpenAI 兼容适配器 (`helper.NewOpenAICompatibleChatModel`)

**专用 SDK 型** (deepseek/anthropic/gemini/qwen/ollama)：
- 嵌入 `helper.BasePlugin`
- **重写 `BuildChatModel`** 使用各自专用 SDK
- 有各自的 `applyParamsToXxxConfig()` 方法映射 LLMParams

### 5.3 BasePlugin 基类

```go
type BasePlugin struct {
    name string
}

// 通用构建流程
func (b *BasePlugin) BuildProvider(cfg) (*ModelProvider, error)
func (b *BasePlugin) BuildModels(provider, cfg) ([]*ModelInstance, error)
func (b *BasePlugin) BuildChatModel(ctx, instance, provider, params) (ChatModel, error)

// 环境变量解析
func ResolveEnvValue(s string) string  // "${ENV_VAR}" → os.Getenv("ENV_VAR")
```

`BuildModels` 是核心方法，负责将 `options.ModelDefinition` 转换为 `entity.ModelInstance`，设置 Connection/Capability/Cost 信息。

---

## 六、ModelManager — 模型管理器

### 6.1 接口

```go
type ModelManager interface {
    // Provider 管理
    RegisterProvider(ctx, *ModelProvider) error
    GetProvider(ctx, providerID) (*ModelProvider, error)
    ListProviders(ctx) ([]*ModelProvider, error)

    // Model 管理
    RegisterModel(ctx, *ModelInstance) error
    GetModelByRef(ctx, ModelRef) (*ModelInstance, error)
    GetDefaultModel(ctx) (*ModelInstance, error)
    SetDefaultModel(ctx, providerID, modelID) error
    ListAllModels(ctx) ([]*ModelInstance, error)

    // ChatModel 构建 (Eino)
    GetChatModel(ctx, ModelRef) (ChatModel, error)      // 带缓存
    BuildChatModel(ctx, ModelRef, *LLMParams) (ChatModel, error)  // 每次新建

    // 兼容性
    ResolveCompat(ctx, ModelRef) (*ModelCompatConfig, error)

    // 状态
    SetModelStatus(ctx, ModelRef, ModelStatus) error
    Initialize(ctx) error
}
```

### 6.2 缓存策略

```
GetChatModel(ref):
  └─ sync.Map 快速路径 → miss → BuildChatModel(ref, nil) → LoadOrStore

BuildChatModel(ref, params):
  └─ FindByRef → FindByID(provider) → getChatPlugin(providerID) → plugin.BuildChatModel()

getChatPlugin(providerID):
  └─ pluginCache sync.Map → miss → registry.Get(factory) → 断言 ChatModelPlugin → 缓存
```

使用双层 `sync.Map` 缓存：
- `chatModelCache`: ModelRef → ChatModel
- `pluginCache`: providerID → ChatModelPlugin

`SetModelStatus()` 会同时删除对应的 ChatModel 缓存。

---

## 七、FallbackExecutor — 故障转移

### 7.1 配置

```go
type FallbackConfig struct {
    Primary        ModelRef      // 主模型
    Fallbacks      []ModelRef    // 备用模型列表
    MaxAttempts    int           // 最大尝试次数 (0=候选数)
    SkipOnCooldown bool          // 是否跳过 Cooldown 状态的模型
}
```

### 7.2 执行流程

```go
// 泛型 Fallback 执行器
func RunWithFallback[T any](ctx, executor, config, params, run, onError) *FallbackResult[T]
```

```
RunWithFallback(候选列表):
  │
  for each candidate in [primary, fallback1, fallback2...]:
  │
  ├─ SkipOnCooldown → 检查模型状态 → 跳过
  ├─ BuildChatModel(ref, params) → 失败 → 记录, continue
  ├─ run(ctx, chatModel) → 成功 → return {OK: true, Value, Ref}
  └─ run 失败:
      ├─ ClassifyError(err) → FailoverError
      ├─ ShouldFailover?
      │   ├─ true → continue (下一个候选)
      │   └─ false (Format 错误) → break (中断)
      └─ onError(err, candidate) — 回调
```

### 7.3 错误分类 (4 层管道)

```
ClassifyError(err):
  ├─ Layer 1: 类型断言 — 已是 FailoverError → 直接返回
  ├─ Layer 2: HTTP 状态码 — 401→Auth, 429→RateLimit, 402→Billing, 500+→ServerError
  ├─ Layer 3: 错误码 — insufficient_quota, model_not_found 等
  └─ Layer 4: 消息模式匹配 — timeout, rate_limit, unauthorized 等关键字
```

**FailoverReason 行为**：

| Reason | 可重试 | 触发 Failover |
|--------|--------|-------------|
| Auth | 否 | 是 |
| RateLimit | 是 | 是 |
| Billing | 否 | 是 |
| Timeout | 是 | 是 |
| **Format** | 否 | **否** — 格式错误不降级 |
| Unavailable | 是 | 是 |
| ServerError | 是 | 是 |

---

## 八、ModelProber — 健康探测

### 8.1 探测类型

| ProbeType | 方法 | 说明 |
|-----------|------|------|
| Chat | `Generate("Reply with OK.")` | 最小聊天请求 |
| ToolCall | `WithTools([ping]) → Generate` | 工具调用能力 |
| Vision | 检查 ImageUnderstanding → probeChat | 视觉理解 |
| Streaming | `Stream + Recv 至少一个 chunk` | 流式能力 |

### 8.2 探测流程

```
ProbeModel(spec):
  ├─ probeViaPlugin() — Provider 实现了 ProbePlugin?
  │   └─ 是 → plugin.Probe(ctx, instance, provider) → 返回
  └─ 否 → probeByType() for each ProbeType:
      └─ BuildChatModel(ref, {MaxTokens: 16}) → 执行探测
```

### 8.3 并发扫描

```
ScanModels(specs, onProgress):
  └─ goroutine × len(specs) + semaphore(concurrency=3) + WaitGroup
```

---

## 九、CompatManager — 兼容性规则引擎

### 9.1 架构

```go
type CompatManager struct {
    rules []ModelCompatRule
    cache sync.Map  // ModelRef → *ModelCompatConfig
}
```

### 9.2 规则匹配

```go
type ModelCompatMatcher struct {
    ProviderIDs    []string     // 匹配 Provider ID
    ModelIDs       []string     // 匹配 Model ID
    ModelClasses   []ModelClass // 匹配模型家族
    APITypes       []ModelAPI   // 匹配 API 协议
    BaseURLContains string      // 匹配 BaseURL
}
```

### 9.3 内置兼容性规则 (6 条)

| 规则名 | 匹配条件 | 补丁 |
|--------|---------|------|
| `o1-no-system-role` | ModelID 含 "o1" | SupportsDeveloperRole=true, SupportsSystemRole=false |
| `o3-use-developer-role` | ModelID 含 "o3" | SupportsDeveloperRole=true |
| `anthropic-no-developer-role` | Provider=anthropic | SupportsDeveloperRole=false, SupportsSystemRole=true |
| `anthropic-requires-max-tokens` | Provider=anthropic | RequiresMaxTokens=true |
| `gemini-temperature-range` | Provider=gemini | TemperatureRange: 0.0~2.0 |
| `ollama-no-developer-role` | Provider=ollama | SupportsDeveloperRole=false |

规则按序应用，非 nil 字段覆盖，带缓存。

---

## 十、核心实体

### 10.1 ModelInstance (核心聚合根)

```go
type ModelInstance struct {
    ID          int64
    ModelID     string          // 如 "gpt-4o"
    ProviderID  string          // 如 "openai"
    Type        ModelType       // LLM=0 | TextEmbedding=1 | Rerank=2
    DisplayInfo *DisplayInfo
    Connection  *Connection     // 连接信息 (聚合)
    Capability  *ModelAbility   // 模型能力
    Cost        *ModelCostInfo  // 成本 (每百万 token)
    Status      ModelStatus     // Ready=0 | Disabled | Error | Cooldown
    IsDefault   bool
}
```

### 10.2 Connection (连接信息聚合)

```go
type Connection struct {
    Base    BaseConnectionInfo  // BaseURL/APIKey/Model/ThinkingType
    OpenAI  *OpenAIConnInfo     // Azure 支持
    Gemini  *GeminiConnInfo     // Vertex AI 支持
    DeepSeek *DeepseekConnInfo
    Qwen    *QwenConnInfo
    Ollama  *OllamaConnInfo
    Claude  *ClaudeConnInfo
}
```

### 10.3 ModelRef (模型引用)

```go
type ModelRef struct {
    ProviderID string
    ModelID    string
}
// String() → "openai/gpt-4o"
```

### 10.4 ModelClass (模型家族枚举)

```go
const (
    ModelClassGPT      = 1   // "openai" / "gpt"
    ModelClassQWen     = 2   // "qwen" / "dashscope"
    ModelClassGemini   = 3   // "gemini" / "google"
    ModelClassDeepSeek = 4
    ModelClassOllama   = 5
    ModelClassClaude   = 6   // "claude" / "anthropic"
    ModelClassKimi     = 7   // "kimi" / "moonshot"
    ModelClassGLM      = 8   // "glm" / "zhipu"
    ModelClassOther    = 999
)
```

---

## 十一、与 OpenClaw 对比

### 11.1 LLM 管理对比

| 维度 | Echoryn | OpenClaw |
|------|---------|----------|
| **Provider 数量** | 8 个 (Go) | 不固定 (通过 SDK 动态) |
| **架构模式** | SPI 四层插件 (K8s Scheduler 风格) | 简单 Provider 封装 |
| **模型发现** | 环境变量自动发现 + 配置注册 | 配置文件列表 |
| **Fallback** | 泛型 FallbackExecutor + 错误分类 | `runWithModelFallback` + auth profile |
| **健康探测** | ModelProber (并发 + 4 种探测类型) | 无专门探测 |
| **兼容性** | CompatManager 规则引擎 (6 条内置) | 无 (硬编码处理) |
| **ChatModel 缓存** | sync.Map 双层缓存 | 无缓存 |
| **思维链** | ThinkingType (Enable/Disable/Auto) | thinking directive |
| **Azure 支持** | 内置 (ByAzure + APIVersion) | 配置字段 |
| **Vertex AI** | 内置 (Backend + Project + Location) | 无 |

### 11.2 对齐项

| 功能 | 状态 | 说明 |
|------|------|------|
| 多 Provider 支持 | ✅ 对齐 | Echoryn 8 个 vs OpenClaw 动态 |
| 模型配置 | ✅ 对齐 | 温度/最大 token/TopP 等参数 |
| Model Fallback | ✅ 对齐 | 候选列表 + 错误分类 |
| 错误分类 | ✅ 对齐 | 4 层管道 vs OpenClaw 的 error handler |
| 模型元数据 | ✅ 对齐 | ContextWindow/MaxTokens/能力 |
| Auth Profile 轮转 | ❌ 未对齐 | OpenClaw 支持多 API Key 轮转 |
| CLI Provider | ❌ 未对齐 | OpenClaw 支持 CLI 模式的 Provider |
| 模型别名 | 🟡 部分 | Echoryn 有 `echoryn` 虚拟模型 |

### 11.3 Echoryn 独有设计

1. **SPI 四层架构** — 新增 Provider 只需实现接口，无需修改框架代码
2. **In-Tree/Out-of-Tree 合并** — 支持外部注册自定义 Provider
3. **ModelProber** — OpenClaw 没有专门的模型健康探测机制
4. **CompatManager** — 声明式规则引擎，OpenClaw 通过硬编码处理兼容性
5. **ModelStatus 状态机** — Ready/Disabled/Error/Cooldown 四态，支持自动降级

---

> 最后更新: 2026-01-13
