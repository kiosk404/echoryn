# Echoryn Harness Engineering 实践指南

> **Harness Engineering = 为 AI Agent 构建可靠运行环境的工程方法论**
>
> 核心转变：从"优化模型"到"设计让 Agent 高效工作的支撑系统"。

---

## 目录

- [1. 什么是 Harness Engineering](#1-什么是-harness-engineering)
- [2. 核心理念与四大动词](#2-核心理念与四大动词)
- [3. 六大工程支柱](#3-六大工程支柱)
- [4. Echoryn 项目现状诊断](#4-echoryn-项目现状诊断)
- [5. 落地实践路线图](#5-落地实践路线图)
- [6. Phase 1：基础设施层](#6-phase-1基础设施层)
- [7. Phase 2：架构约束层](#7-phase-2架构约束层)
- [8. Phase 3：自验证循环](#8-phase-3自验证循环)
- [9. Phase 4：上下文工程](#9-phase-4上下文工程)
- [10. Phase 5：熵治理与垃圾回收](#10-phase-5熵治理与垃圾回收)
- [11. Phase 6：可拆卸性与可观测性](#11-phase-6可拆卸性与可观测性)
- [12. 参考资料](#12-参考资料)

---

## 1. 什么是 Harness Engineering

### 起源

2026 年 2 月，OpenAI 工程师 Ryan Lopopolo 发表文章《Harness Engineering: Harnessing Codex in an Agent-First World》。OpenAI 内部团队用 5 个月时间，**零行手写代码**，完全由 Codex Agent 生成了约 100 万行代码的产品，开发速度是传统方式的 10 倍。

这标志着软件工程从 **Prompt Engineering → Context Engineering → Harness Engineering** 的第三次范式跃迁。

### 核心公式

```
Agent = Model（引擎）+ Harness（驾驭系统 = 指南针 + 刹车 + 方向盘）
```

**Harness** 不是一个具体的工具或框架，而是**围绕 AI Agent 构建的一整套工程环境和机制**，使 Agent 能在可控边界内持续自主工作，并能被自动验证和纠偏。

### 与 Prompt/Context Engineering 的关系

| 维度 | Prompt Engineering | Context Engineering | **Harness Engineering** |
|------|-------------------|--------------------|-----------------------|
| 关注点 | 单次提示优化 | 上下文信息组织 | 整个运行环境设计 |
| 作用范围 | 一次 API 调用 | 一次会话 | 完整任务生命周期 |
| 核心动作 | 措辞、Few-shot | 检索、注入、裁剪 | 约束、告知、验证、纠正 |
| 比喻 | 写好一句话 | 准备好资料 | 设计整条赛道 |

---

## 2. 核心理念与四大动词

### 2.1 核心理念

来自 OpenAI Harness 团队的核心实践原则：

1. **人类掌舵，Agent 执行**：人类的角色从"写代码"变为"设计环境、明确意图、构建反馈回路"
2. **不手动写代码**：遇到问题不亲自写代码，而是改进 Agent 的工具、指导或约束条件
3. **Agent 可读性优先**：代码库的设计优先考虑 Agent 的可读性和可导航性
4. **给地图，不给说明书**：避免巨大的指导文件，使用渐进式信息披露
5. **代码仓库作为知识记录系统**：所有知识版本化存储在仓库中

### 2.2 四大动词：Constrain → Inform → Verify → Correct

| 动词 | 含义 | 实践手段 |
|------|------|---------|
| **Constrain（约束）** | 用工具和代码强制执行规则 | Linter、架构测试、工具白名单、权限分层 |
| **Inform（告知）** | 精准设计进入 Agent 上下文的信息 | 结构化文档、渐进式披露、延迟加载 |
| **Verify（验证）** | 在执行流程中内置验证检查点 | 自动测试、构建验证、进展检测、终止条件 |
| **Correct（纠正）** | 建立自动纠偏和熵治理机制 | 自动重构 PR、doc-gardening、状态蒸馏 |

---

## 3. 六大工程支柱

### 支柱 1：上下文架构

**核心**：精准设计进入模型上下文的信息，避免信息过载（研究表明上下文利用率超过 40% 时推理质量显著下降）。

- 信息优先级分层（关键决策信息 vs 辅助信息）
- 动态裁剪与滚动窗口管理
- 结构化上下文（XML 标签、分隔符）
- 延迟加载（仅在需要时注入）
- 上下文健康监控（token 利用率追踪）

### 支柱 2：架构约束

**核心**：用工具和代码强制执行规则，而非依赖 prompt 软约束。

- 工具白名单（只暴露安全工具集合）
- 参数强类型约束（拒绝非法输入）
- 权限分层（读操作自动→写操作确认→删除双重确认）
- 严格分层架构（Types → Config → Repo → Service → Runtime → UI）
- 自定义 Linter 强制执行架构边界

### 支柱 3：自验证循环

**核心**：在执行流程中内置验证检查点，防止死循环与静默失败。

- 前置条件验证（环境就绪、权限充足）
- 步骤后验证（输出符合预期、状态变更正确）
- 进展检测（状态指纹比对，防止原地踏步）
- 终止条件（最大迭代次数、时间预算）
- 错误升级链（自我校正→换策略→人工确认→中止并诊断）

### 支柱 4：上下文隔离

**核心**：多 Agent 协作时，保持每个 Agent 上下文的纯净性。

- 任务边界隔离（每个子任务使用全新上下文）
- 信息接口化（Agent 间通过结构化消息传递）
- 错误隔离（一个 Agent 的错误不传播到下游）
- 角色上下文分离（系统角色 vs 任务信息严格分层）

### 支柱 5：熵治理

**核心**：建立自维护机制，对抗 Agent 系统中状态的自然熵增。

- 定期上下文蒸馏（压缩历史为结构化摘要）
- 状态清理检查点（阶段性任务完成后清除中间状态）
- 规则冲突检测（识别并告警矛盾指令）
- 知识蒸馏沉淀（有价值经验迁移到持久知识库）
- Doc-gardening Agent（自动扫描修复过时文档）

### 支柱 6：可拆卸性

**核心**：模块化设计，使 Harness 系统能随模型迭代优雅适配。

- 抽象模型接口（屏蔽不同模型 API 差异）
- 工具模型无关定义（通过适配器转换）
- Prompt 模板分离（外置为配置文件）
- 能力特性标记（动态识别模型能力）
- 三层架构：应用层（业务逻辑）→ Harness 核心层（状态机）→ 模型适配层

---

## 4. Echoryn 项目现状诊断

基于项目代码的全面审计，以下是 Echoryn 在 Harness Engineering 六大支柱上的现状评估：

### ✅ 已有优势（得分高的领域）

| 领域 | 现状 | 评价 |
|------|------|------|
| **架构分层** | DDD 分层清晰：entity/repo/service/runtime | ⭐⭐⭐⭐ 优秀 |
| **可拆卸性** | LLM Provider SPI 抽象、Plugin 框架解耦 | ⭐⭐⭐⭐ 优秀 |
| **上下文管理** | context_builder/compaction/pruner/window 已实现 | ⭐⭐⭐ 良好 |
| **上下文隔离** | SubAgent 独立 Session、Team MessageBus 隔离 | ⭐⭐⭐ 良好 |
| **插件系统** | K8S 风格 Plugin Framework，接口驱动 | ⭐⭐⭐⭐ 优秀 |
| **初始化链** | InitializerChain，依赖注入清晰 | ⭐⭐⭐⭐ 优秀 |

### ❌ 关键缺口（需要建设的领域）

| 领域 | 现状 | 影响 |
|------|------|------|
| **测试覆盖** | internal/ 和 pkg/ 下**零测试文件** | 🔴 致命 |
| **Linter 配置** | `.golangci.yaml` 不存在，`make lint` 会失败 | 🔴 严重 |
| **AGENTS.md** | 项目根目录无 AI 引导文件 | 🟡 重要 |
| **架构约束强制** | 无依赖方向检测、无架构测试 | 🟡 重要 |
| **自动化验证** | 无 CI/CD Pipeline、无 pre-commit hooks | 🟡 重要 |
| **文档治理** | 文档分散，无自动化过时检测 | 🟠 中等 |
| **熵治理** | 无后台清理 Agent、无自动重构机制 | 🟠 中等 |

---

## 5. 落地实践路线图

```
Phase 1: 基础设施层          ← 你首先要做的事（1-2 周）
   ├── AGENTS.md（AI 引导文件）
   ├── .golangci.yaml（Linter 配置）
   ├── 架构测试骨架
   └── Makefile 增强

Phase 2: 架构约束层          ← 建立护栏（2-3 周）
   ├── 依赖方向检测（import-restrict linter）
   ├── 命名约定 Linter
   ├── 架构不变量测试
   └── 接口契约验证

Phase 3: 自验证循环          ← 建立信心（3-4 周）
   ├── 核心模块单元测试
   ├── 集成测试框架
   ├── CI Pipeline（GitHub Actions）
   └── Pre-commit hooks

Phase 4: 上下文工程          ← 优化 Agent 体验（2-3 周）
   ├── 渐进式文档索引
   ├── 模块级 README
   ├── API 文档自动生成
   └── 上下文健康监控

Phase 5: 熵治理              ← 持续维护（持续进行）
   ├── Doc-gardening 自动化
   ├── 代码质量漂移检测
   ├── 过时依赖清理
   └── 技术债务追踪

Phase 6: 可观测性增强        ← 高级进阶（按需）
   ├── Agent 执行追踪大盘
   ├── LLM 调用成本监控
   ├── 工具调用成功率统计
   └── Agent 决策审计日志
```

---

## 6. Phase 1：基础设施层

### 6.1 创建 AGENTS.md

在项目根目录创建 `AGENTS.md`，作为 AI Agent 理解项目的入口。遵循 OpenAI 的"给地图，不给说明书"原则——保持简短，指向详细文档。

```markdown
# Echoryn Agent 指南

## 项目概述
Echoryn 是一个用 Go 编写的分布式 AI 虚拟角色容器平台。
架构模型：Hivemind（蜂巢智心，中央大脑）+ Golem（傀儡，工作节点）。

## 快速导航
- 架构规范: docs/ECHORYN_SPEC.md
- 智能体引擎: docs/ECHORYN_HIVEMIND_AGENTS_SPEC.md
- 插件系统: docs/ECHORYN_HIVEMIND_PLUGIN_SPEC.md
- 团队协作: docs/ECHORYN_TEAM_SPEC.md
- 开发任务: docs/TODO_SPEC.md

## 架构分层（依赖规则：只能向下依赖）
```
cmd/           → internal/      （入口层 → 实现层）
internal/      → pkg/           （实现层 → 公共库）
handler/       → service/       （HTTP/gRPC → 业务逻辑）
service/       → domain/        （应用服务 → 领域模型）
domain/entity  ← domain/repo   （实体 ← 仓储接口）
domain/service → domain/repo    （领域服务 → 仓储接口）
```

## 构建命令
- `make all`        : tidy + format + lint + build
- `make build`      : 构建所有二进制
- `make test`       : 运行测试
- `make lint`       : 代码检查
- `make format`     : 代码格式化

## 代码约定
- 使用 K8S 风格初始化链：Config → Complete() → New()
- 插件实现 plugin.Plugin 接口
- 错误码使用 pkg/errorx
- 日志使用 pkg/logger（logrus 封装）
- 路径解析使用 pkg/paths
```

### 6.2 创建 .golangci.yaml

```yaml
run:
  timeout: 5m
  go: "1.25"
  skip-dirs:
    - temp
    - output
    - vendor

linters:
  enable:
    # 基础质量
    - errcheck        # 检查未处理的错误
    - gosimple        # 简化代码
    - govet           # Go vet 检查
    - ineffassign     # 无效赋值
    - staticcheck     # 静态分析
    - unused          # 未使用的代码
    # 代码风格
    - gofmt           # 格式化检查
    - goimports       # import 排序
    - misspell        # 拼写检查
    # 架构约束（Harness 核心）
    - depguard        # 依赖守卫（强制架构边界）
    - gocritic        # 高级代码审查
    - revive          # 可配置 linter
    - gosec           # 安全检查

linters-settings:
  depguard:
    rules:
      main:
        deny:
          - pkg: "log"
            desc: "使用 pkg/logger 代替标准 log 包"
          - pkg: "fmt"
            desc: "日志输出请使用 pkg/logger，fmt 仅用于字符串格式化"
  revive:
    rules:
      - name: exported
        severity: warning
      - name: var-naming
        severity: warning

issues:
  max-issues-per-linter: 50
  max-same-issues: 5
  exclude-rules:
    - path: ".*\\.pb\\.go"
      linters: [all]
    - path: "_test\\.go"
      linters: [errcheck]
```

### 6.3 增强 Makefile

在 Makefile 中添加 Harness 相关目标：

```makefile
## harness: Run full harness validation (lint + test + architecture check).
.PHONY: harness
harness: lint test arch-check
	@echo "===========> Harness validation passed ✓"

## arch-check: Validate architecture invariants.
.PHONY: arch-check
arch-check:
	@echo "===========> Checking architecture invariants"
	@# 检查 internal/ 不被 cmd/ 以外的包直接导入
	@# 检查依赖方向是否正确
	@$(GO) vet ./...
```

### 6.4 创建模块级 docs/ 索引

在每个核心模块目录下创建简短的 `README.md`：

```
internal/hivemind/service/agents/README.md
internal/hivemind/service/llm/README.md
internal/hivemind/service/plugin/README.md
internal/hivemind/service/team/README.md
internal/hivemind/service/golem/README.md
```

---

## 7. Phase 2：架构约束层

### 7.1 依赖方向不变量

Echoryn 的架构分层已经非常清晰，需要通过工具**强制执行**而非仅靠约定：

```
层级依赖规则（只能向下依赖，不能向上或交叉）：

cmd/             ← 最顶层（入口）
  ↓
internal/hivemind/handler/    ← 请求处理层
  ↓
internal/hivemind/service/    ← 业务服务层
  ↓
internal/hivemind/service/*/domain/service/  ← 领域服务
  ↓
internal/hivemind/service/*/domain/entity/   ← 领域实体
internal/hivemind/service/*/domain/repo/     ← 仓储接口

横切关注点（所有层都可以使用）：
  pkg/logger, pkg/errorx, pkg/paths, pkg/utils
```

**实践方法**：使用 `depguard` linter 或自定义架构测试来机械化强制执行。

### 7.2 架构测试示例

创建 `internal/architecture_test.go`（架构不变量测试）：

```go
package internal_test

import (
    "go/parser"
    "go/token"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestArchitectureInvariants 验证架构分层约束
func TestArchitectureInvariants(t *testing.T) {
    t.Run("handler_does_not_import_store", func(t *testing.T) {
        assertNoImport(t, "internal/hivemind/handler", "store/boltdb")
        assertNoImport(t, "internal/hivemind/handler", "store/inmemory")
    })

    t.Run("entity_does_not_import_service", func(t *testing.T) {
        assertNoImport(t, "internal/hivemind/service/agents/domain/entity", "domain/service")
    })

    t.Run("pkg_does_not_import_internal", func(t *testing.T) {
        assertNoImport(t, "pkg", "internal/")
    })
}

func assertNoImport(t *testing.T, dir, forbidden string) {
    t.Helper()
    err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if err != nil || !strings.HasSuffix(path, ".go") {
            return err
        }
        fset := token.NewFileSet()
        f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
        if err != nil {
            return nil
        }
        for _, imp := range f.Imports {
            importPath := strings.Trim(imp.Path.Value, `"`)
            if strings.Contains(importPath, forbidden) {
                t.Errorf("%s imports forbidden package %q (contains %q)", path, importPath, forbidden)
            }
        }
        return nil
    })
    if err != nil {
        t.Fatalf("walk error: %v", err)
    }
}
```

### 7.3 命名约定

通过 `revive` linter 强制执行：

| 类型 | 约定 | 示例 |
|------|------|------|
| 插件名 | DNS 兼容，小写连字符 | `memory-core`, `web-search` |
| 文件名 | 小写下划线 | `agent_service.go`, `model_provider.go` |
| 接口名 | 以动词或 -er 结尾 | `Manager`, `Repository`, `Plugin` |
| 配置结构 | Config → Complete() → New() | K8S 风格链式调用 |
| 错误码 | `pkg/errorx` 统一管理 | `ErrNotFound`, `ErrPermissionDenied` |

### 7.4 工具权限分层

在 Agent 执行环境中实施权限分级（已部分实现于 Plugin Framework）：

```
Level 0 (读操作) : 自动执行，无需确认
  - 查询 Agent/Session/Model 列表
  - 读取配置文件
  - 搜索记忆

Level 1 (写操作) : 需要单次确认
  - 创建/更新 Agent
  - 发送消息
  - 调用外部 API

Level 2 (破坏性操作) : 需要双重确认
  - 删除 Agent/Session
  - 修改系统配置
  - 清除记忆数据
```

---

## 8. Phase 3：自验证循环

### 8.0 Echoryn 已实现的自验证机制

Echoryn 在运行时已内置以下关键的自验证机制，是 Harness Engineering 自验证循环的优秀实现：

#### 8.0.1 工具循环检测（Tool Loop Detection）

**位置**：`internal/hivemind/service/agents/domain/service/runtime/toolloop/detector.go`

工具循环检测通过 4 层检测器替代硬性的 `MaxStep` 限制，实现智能熔断：

```
Detector 1: Global Circuit Breaker（全局熔断器）
  ↓ (任何工具重复 ≥ 30 次无进展)
  → Result{Stuck=true, Level=Critical}

Detector 2: Known Poll Tools（已知轮询工具检测）
  ↓ (process_poll, command_status 等无进展 ≥ 20 次)
  → Result{Stuck=true, Level=Critical}

Detector 3: Ping-Pong Detection（乒乓球循环）
  ↓ (两个工具交替 A→B→A→B ≥ 20 次无进展)
  → Result{Stuck=true, Level=Critical}

Detector 4: Generic Repeat（通用重复）
  ↓ (相同工具+参数 ≥ 10 次)
  → Result{Stuck=true, Level=Warning}
```

**关键配置** (`toolloop.Config`)：
- `WarningThreshold`: 10 - 触发警告的重复次数
- `CriticalThreshold`: 20 - 触发熔断的重复次数
- `GlobalCircuitBreakerThreshold`: 30 - 全局无进展累计触发熔断
- `HistorySize`: 30 - 保留最近 30 次工具调用

**运行时集成** (在 `runner.go` 中)：

```go
// 在工具执行前检测循环
loopResult := agent.toolLoopDetector.Check(toolName, args)
if loopResult.Stuck {
    if loopResult.Level == toolloop.LevelCritical {
        return fmt.Errorf("tool loop breaker: %s", loopResult.Message)
    }
    // LevelWarning: 记录但继续执行
    log.Warnf("tool loop warning: %s", loopResult.Message)
}

// 执行工具
output := executeToolCall(toolName, args)

// 记录执行结果和进展状态
hasProgress := !isErrorOutput(output)
agent.toolLoopDetector.Record(toolName, args)
agent.toolLoopDetector.RecordOutcome(hasProgress)
```

这是 Harness 自验证循环的**进展检测**和**终止条件**的实现。

#### 8.0.2 SubAgent 观察者系统（Observer Pattern）

**位置**：`internal/hivemind/service/subagent/observer/`

当 Agent 启动 SubAgent 时，自动收集执行指标和生命周期事件：

```
SubAgentObserver 职责链：

┌─────────────────────────────────────────┐
│ 1. Lifecycle Events (生命周期事件)       │
│    - Spawned     : SubAgent 启动          │
│    - Running     : 运行中                  │
│    - Completed   : 完成                    │
│    - Failed      : 失败                    │
└─────────────────────────────────────────┘
              ↓
┌─────────────────────────────────────────┐
│ 2. Metrics Collection (指标收集)         │
│    - execution_duration   : 执行耗时      │
│    - token_usage          : Token 消耗    │
│    - tool_call_count      : 工具调用数    │
│    - error_count          : 错误计数      │
└─────────────────────────────────────────┘
              ↓
┌─────────────────────────────────────────┐
│ 3. Event Reporting (事件上报)            │
│    - 发送到 Team MessageBus              │
│    - 记录到本地指标系统                   │
│    - 触发告警规则（可选）                 │
└─────────────────────────────────────────┘
```

**用途**：
- Parent Agent 可实时监控 SubAgent 执行状态
- 自动检测 SubAgent 异常（超时、无进展、错误率高）
- 支持动态决策（继续等待 vs 超时中止）

#### 8.0.3 上下文健康检查

**位置**：`internal/hivemind/service/agents/domain/service/runtime/context_builder.go` 和 `compaction.go`

Agent 在构建上下文时自动执行以下检查：

```
前置验证 (Pre-Condition Checks):
  ├── Model 连接可达性检查
  ├── 上下文 token 预计算（是否超限）
  ├── Session 状态校验
  └── 历史消息完整性检查

上下文压缩 (Dynamic Compaction):
  ├── Token 利用率 > 80% → 触发动态压缩
  ├── 保留最近 3 个 turn 完整信息
  ├── 历史压缩为结构化摘要
  └── 异常：在压缩前主动刷新 token 计数

步骤后验证 (Post-Step Checks):
  ├── LLM 输出非空
  ├── 工具调用参数合法性校验
  ├── 工具调用无重复（toolloop detector）
  └── 返回消息格式符合预期
```

这些机制通过**架构约束**而非 Prompt 软约束实现。

---

### 8.1 测试策略

当前项目零测试覆盖，需要按优先级逐步建设：

#### Priority 1：核心领域逻辑（必须先做）

```
internal/hivemind/service/agents/domain/entity/      → 实体单元测试
internal/hivemind/service/agents/domain/service/      → 服务单元测试
internal/hivemind/service/llm/domain/                 → LLM 管理测试
internal/hivemind/service/agents/store/               → 存储层测试
```

#### Priority 2：运行时引擎

```
internal/hivemind/service/agents/domain/service/runtime/
  ├── context_builder_test.go     → 上下文构建
  ├── compaction_test.go          → 上下文压缩
  ├── context_pruner_test.go      → 上下文裁剪
  ├── toolloop/detector_test.go   → 工具循环检测
  └── executor_test.go            → 执行器
```

#### Priority 3：基础设施

```
pkg/errorx/        → 错误码系统
pkg/skills/         → 技能加载器
pkg/paths/          → 路径解析
pkg/utils/          → 工具函数
```

### 8.2 CI Pipeline（GitHub Actions）

```yaml
# .github/workflows/harness.yaml
name: Harness Validation
on: [push, pull_request]
jobs:
  harness:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - name: Install SQLite dev
        run: sudo apt-get install -y libsqlite3-dev
      - name: Tidy
        run: make tidy
      - name: Format Check
        run: |
          make format
          git diff --exit-code
      - name: Lint
        run: make lint
      - name: Test
        run: make test
      - name: Architecture Check
        run: make arch-check
```

### 8.3 Pre-commit Hooks

```bash
#!/bin/bash
# .githooks/pre-commit

echo "==> Running Harness pre-commit checks..."

# 1. Format check
make format 2>/dev/null
if ! git diff --quiet; then
    echo "❌ Code formatting issues detected. Run 'make format' and commit again."
    exit 1
fi

# 2. Lint
make lint 2>/dev/null
if [ $? -ne 0 ]; then
    echo "❌ Lint errors detected. Fix them before committing."
    exit 1
fi

# 3. Quick test (only changed packages)
CHANGED_PKGS=$(git diff --cached --name-only --diff-filter=ACMR '*.go' | \
    xargs -r dirname | sort -u | sed 's|^|./|')
if [ -n "$CHANGED_PKGS" ]; then
    go test -short -count=1 $CHANGED_PKGS 2>/dev/null
fi

echo "✅ Harness pre-commit checks passed"
```

### 8.4 Agent 运行时自验证

在 AgentRunner 中增加检查点（Echoryn 已部分实现了 toolloop 检测）：

```
Agent 执行验证链：
  ┌─────────────────────────────────────────┐
  │ 前置验证                                  │
  │  ├── 模型连接可达                         │
  │  ├── 上下文 token 未超限                   │
  │  └── Session 状态正常                      │
  ├─────────────────────────────────────────┤
  │ 步骤后验证                                 │
  │  ├── 输出非空                             │
  │  ├── 工具调用参数合法                      │
  │  └── 无重复工具调用（toolloop detector）   │
  ├─────────────────────────────────────────┤
  │ 进展检测                                   │
  │  ├── 状态指纹变化                          │
  │  └── 连续空操作计数 < 阈值                 │
  ├─────────────────────────────────────────┤
  │ 终止条件                                   │
  │  ├── 最大迭代次数                          │
  │  ├── 时间预算（RunTimeout）                │
  │  └── 全局熔断器（circuit breaker）         │
  └─────────────────────────────────────────┘
```

---

## 9. Phase 4：上下文工程

### 9.1 渐进式信息披露

遵循 OpenAI 的"给地图，不给说明书"原则：

```
项目根目录/
├── AGENTS.md                    ← 入口地图（<50行）
├── docs/
│   ├── INDEX.md                 ← 文档索引
│   ├── ARCHITECTURE.md          ← 架构概览
│   ├── ECHORYN_SPEC.md          ← 完整规范（按需阅读）
│   └── ...
├── internal/hivemind/service/
│   ├── agents/README.md         ← 模块级指南
│   ├── llm/README.md
│   ├── plugin/README.md
│   ├── team/README.md
│   └── golem/README.md
```

### 9.2 代码即文档

在关键接口和入口文件中使用结构化注释，让 Agent 能快速理解意图：

```go
// AgentRunner 是 Echoryn 的核心运行时引擎。
//
// 架构角色: runtime/runner.go 是 Agent 执行的中枢。
// 依赖关系: AgentStore, SessionStore, RunStore, LLM Module, Plugin Framework
// 初始化方式: agents.Module.New() → runtime.NewAgentRunner()
//
// 执行流程:
//   1. 接收 RunRequest
//   2. 解析或创建 Session
//   3. 构建上下文（context_builder.go）
//   4. 通过 Eino 流编排调用 LLM（agentflow/）
//   5. 处理工具调用（executor.go）
//   6. 流式返回 AgentEvent
//
// 关键配置: AgentRunnerConfig（RunTimeout, MaxRetries, CompactionThreshold）
```

### 9.3 上下文健康监控（已有基础）

Echoryn 已实现 `context_window.go` 和 `compaction.go`，可以在此基础上增加监控指标：

```go
// 上下文健康指标（集成到 diagnostics 插件）
type ContextHealthMetrics struct {
    TokenUtilization    float64 // 当前 token 利用率
    CompactionCount     int     // 压缩触发次数
    PrunedMessageCount  int     // 被裁剪的消息数
    AvgContextBuildTime time.Duration // 平均上下文构建耗时
}
```

---

## 10. Phase 5：熵治理与垃圾回收

### 10.1 代码质量漂移检测

创建定期运行的"黄金规则"检查（可作为 CI 任务或 cron job）：

```bash
# scripts/harness/golden-rules-check.sh

echo "==> Golden Rules Check"

# 规则 1: 所有导出函数必须有注释
MISSING_DOC=$(grep -rn "^func [A-Z]" internal/ pkg/ --include="*.go" | \
    while read line; do
        file=$(echo "$line" | cut -d: -f1)
        linenum=$(echo "$line" | cut -d: -f2)
        prevline=$((linenum - 1))
        prev=$(sed -n "${prevline}p" "$file")
        if [[ ! "$prev" =~ ^// ]]; then
            echo "$line"
        fi
    done)

if [ -n "$MISSING_DOC" ]; then
    echo "⚠️  Missing documentation for exported functions:"
    echo "$MISSING_DOC" | head -20
fi

# 规则 2: 无 TODO/FIXME 超过 30 天
echo "==> Checking stale TODOs..."
grep -rn "TODO\|FIXME\|HACK" internal/ pkg/ --include="*.go" | head -20

# 规则 3: 大文件检测（>500行可能需要拆分）
echo "==> Large files (>500 lines):"
find internal/ pkg/ -name "*.go" -exec wc -l {} + | \
    sort -rn | awk '$1 > 500 {print}' | head -10
```

### 10.2 文档新鲜度检查

```bash
# scripts/harness/doc-freshness.sh

echo "==> Doc Freshness Check"

# 检查 docs/ 中超过 60 天未更新的文件
find docs/ -name "*.md" -mtime +60 | while read f; do
    echo "⚠️  Stale doc (>60 days): $f"
done

# 检查空文档
find docs/ -name "*.md" -empty | while read f; do
    echo "❌ Empty doc: $f"
done
```

### 10.3 上下文蒸馏策略

```
蒸馏原则：保留决策，丢弃推理
  ✅ 保留: 决策结论、关键事实、错误规避方案、架构约定
  ❌ 丢弃: 冗长推理链、失败路径细节、重复的中间步骤

蒸馏时机:
  - 任务完成边界（SubAgent 完成时）
  - 上下文利用率 > 40% 时
  - Session 达到 MaxHistoryTurns 时

蒸馏产物:
  - 压缩后的摘要消息（compaction.go 已实现）
  - 持久化的记忆条目（memory-core 插件已实现）
```

---

## 11. Phase 6：可拆卸性与可观测性

### 11.1 Echoryn 的三层架构（已良好实现）

```
┌──────────────────────────────────────────────┐
│  应用层 (Application Layer)                    │
│  handler/v1/ → service/ → domain/service/     │
│  纯业务逻辑，与 LLM 提供商无耦合              │
├──────────────────────────────────────────────┤
│  Harness 核心层 (Harness Core Layer)          │
│  runtime/ → context_builder, compaction       │
│  agentflow/ → Eino 流编排                      │
│  subagent/ → 子智能体管理                      │
│  plugin/ → 插件框架                            │
│  与模型无关的状态管理和执行控制                 │
├──────────────────────────────────────────────┤
│  模型适配层 (Model Adapter Layer)              │
│  llm/provider/ → openai, claude, deepseek...  │
│  llm/provider/helper/adapter.go               │
│  llm/provider/thinking/ → 思维链策略           │
│  强耦合于具体 LLM API                          │
└──────────────────────────────────────────────┘
```

### 11.2 可观测性增强

Echoryn 已有 diagnostics 插件（OpenTelemetry 集成）和 SubAgent Observer，可以在此基础上构建 Harness Dashboard：

```
Harness 可观测性指标：

[Agent 执行]
  - agent_run_total           : 总执行次数
  - agent_run_duration        : 执行耗时分布
  - agent_run_error_total     : 错误次数
  - agent_toolcall_total      : 工具调用次数
  - agent_toolloop_detected   : 循环检测触发次数
  - agent_toolloop_breaker    : 熔断器触发次数（Critical 级）

[工具循环检测（新增）]
  - toolloop_warning_total           : Warning 级警告次数
  - toolloop_critical_total          : Critical 级熔断次数
  - toolloop_detector_kind           : 按检测器分类
    - global_circuit_breaker_total   : 全局熔断器触发
    - known_poll_no_progress_total   : 轮询工具检测
    - ping_pong_total                : 乒乓循环检测
    - generic_repeat_total           : 通用重复检测
  - toolloop_avg_stuck_depth         : 平均卡住深度（调用次数）

[SubAgent 协作（新增）]
  - subagent_spawn_total            : 子智能体启动数
  - subagent_running_concurrent     : 当前并发运行数
  - subagent_duration               : 子智能体运行耗时
  - subagent_lifecycle_events       : 生命周期事件计数
    - spawned_total                 : 启动数
    - running_total                 : 运行中数
    - completed_total               : 完成数
    - failed_total                  : 失败数
  - subagent_error_rate             : 失败率
  - subagent_observer_metrics       : 观察者收集的指标
    - execution_duration_percentiles: 耗时百分位数
    - token_usage_histogram         : Token 消耗分布
    - tool_call_distribution        : 工具调用分布

[上下文健康]
  - context_token_utilization : 上下文利用率
  - context_compaction_total  : 压缩触发次数
  - context_build_duration    : 上下文构建耗时
  - context_prune_total       : 消息裁剪触发次数
  - context_pre_check_failed  : 前置检查失败数

[LLM 成本]
  - llm_request_total         : LLM 调用次数
  - llm_token_input_total     : 输入 token 总量
  - llm_token_output_total    : 输出 token 总量
  - llm_cost_total            : 总成本（USD）
  - llm_fallback_total        : 降级触发次数
  - llm_thinking_token_total  : Extended Thinking Token 总量
```

**仪表板查询示例**：

```promql
# 检测工具循环的 Agent 成功率
sum(rate(agent_run_error_total{reason="toolloop_breaker"}[5m])) /
sum(rate(agent_run_total[5m]))

# SubAgent 平均执行时间
histogram_quantile(0.95, 
  rate(subagent_duration_bucket[5m]))

# 上下文压缩频率（token 超限时）
sum(rate(context_compaction_total[1h])) / 
sum(rate(agent_run_total[1h]))

# 熔断器类型分布（识别最常见的循环类型）
sum by (detector_kind) (rate(toolloop_critical_total[1h]))
```



### 11.3 模型适配层的可拆卸性

Echoryn 已通过 `llm/provider/spi/spi.go` 实现了 LLM Provider SPI 抽象，支持：

- 新增 Provider 只需实现 `ModelProvider` 接口
- `helper/adapter.go` 统一适配不同 API 格式
- `thinking/` 策略模式处理不同模型的思维链
- 运行时通过配置文件动态切换模型

这是 Harness Engineering "可拆卸性"支柱的优秀实践。

---

## 12. 参考资料

### 核心文献

1. **OpenAI 原文**：[Harness Engineering: Harnessing Codex in an Agent-First World](https://openai.com/index/harness-engineering/) (2026.02)
2. **六大支柱详解**：[Harness Engineering：构建高可靠 AI Agent 的工程方法论 - John's Blog](https://johng.cn/ai/harness-engineering)
3. **实战案例**：[一些 Harness Engineering 的实践 - 阿里云开发者社区](https://developer.aliyun.com/article/1718179)
4. **方法论蒸馏**：[从 OpenAI Harness Engineering 蒸馏出四个 Skill](https://cloud.tencent.com/developer/news/3707733)

### 行业实践

5. **LangChain 实践**：通过 Harness 优化，排名从第 30 跃升至第 5
6. **Anthropic Claude Code**：CLAUDE.md + SubAgents 隔离 + /compact 蒸馏
7. **Cursor**：Self-driving codebases 范式
8. **nxcode 完整指南**：[Harness Engineering: The Complete Guide](https://www.nxcode.io/resources/news/harness-engineering-complete-guide-ai-agent-codex-2026)

### Echoryn 内部文档

9. `docs/ECHORYN_SPEC.md` — 项目总体规范
10. `docs/ECHORYN_HIVEMIND_AGENTS_SPEC.md` — 智能体引擎规范
11. `docs/ECHORYN_HIVEMIND_PLUGIN_SPEC.md` — 插件框架规范
12. `docs/BLOG_CONTEXT_SPEC.md` — 上下文管理研究
13. `docs/BLOG_SUBAGENT_SPEC.md` — 子智能体研究

---

## 附录 A：快速检查清单

在每次重大开发前，用此清单检查 Harness 健康度：

- [ ] **AGENTS.md** 是否最新？是否反映了当前架构？
- [ ] **Linter** 是否通过？`make lint` 无错误？
- [ ] **测试** 核心模块是否有覆盖？覆盖率 > 60%？
- [ ] **架构约束** 是否有新增的越层依赖？
- [ ] **文档** 是否有超过 60 天未更新的文档？
- [ ] **大文件** 是否有超过 500 行的文件需要拆分？
- [ ] **TODO/FIXME** 是否有积压超过 30 天的待办？
- [ ] **依赖** 是否有已知漏洞的过时依赖？

## 附录 B：术语对照表

| 英文术语 | 中文翻译 | Echoryn 对应 |
|---------|---------|------------|
| Harness | 驾驭系统 | 整体工程环境 |
| Context Architecture | 上下文架构 | context_builder, compaction, pruner |
| Architecture Constraints | 架构约束 | .golangci.yaml, 架构测试 |
| Self-Verification Loop | 自验证循环 | toolloop detector, CI pipeline |
| Context Isolation | 上下文隔离 | SubAgent Session, Team MessageBus |
| Entropy Governance | 熵治理 | doc-gardening, golden rules check |
| Detachability | 可拆卸性 | LLM Provider SPI, Plugin Framework |
| Agent-Readable | Agent 可读 | AGENTS.md, 模块级 README |
| Doc-Gardening | 文档园艺 | 过时文档自动检测 |
| Golden Rules | 黄金规则 | 代码质量不变量检查 |
| Progressive Disclosure | 渐进式披露 | AGENTS.md → docs/ → 模块 README |

---

> **Remember**: Harness Engineering 的核心不在于代码本身，而在于**构建一个能让 Agent 高效、可靠、可持续工作的支撑系统**。
>
> 就像赛车工程师不是在跑道上开车的人，而是设计转向、刹车和悬挂系统的人。
