# Echoryn 内置插件完整清单

> **这是 Echoryn 项目的权威参考，记录所有随 Echoryn 发行的内置插件定义、配置和加载优先级。**

**文档版本**：v1.0  
**最后更新**：2026-02-01  
**维护者**：@kiosk404

---

## 目录

1. [概述](#概述)
2. [加载流程](#加载流程)
3. [10 个内置插件详解](#10-个内置插件详解)
4. [插件槽位系统](#插件槽位系统)
5. [配置解析](#配置解析)
6. [最佳实践](#最佳实践)

---

## 概述

### 什么是内置插件？

Echoryn 的**内置插件**是指在项目编译时直接链接的插件，随 `hivemind` 二进制一起发行。与之对比，**外置插件**是在运行时通过配置文件动态加载的插件。

```
编译时（Build Time）        →  运行时（Runtime）
  ┌─────────────────┐          ┌──────────────────┐
  │ In-Tree Registry │          │ Plugin Framework │
  │  (内置插件工厂)  │          │  (动态加载)      │
  └─────────────────┘          └──────────────────┘
         ↓                              ↑
    10个Go包              合并 in-tree + 外置插件
    (builtin/)            (来自 PluginsOptions)
```

### 为什么有内置插件？

1. **启动体验**：开箱即用的核心功能（记忆、诊断、子智能体）
2. **架构完整性**：避免循环依赖（例如 Golem 模块需要注入到 golem-cluster 插件）
3. **版本绑定**：确保内置插件与当前代码版本匹配

---

## 加载流程

### 初始化顺序

```go
// 在 internal/hivemind/server.go 中
inTreeRegistry := builtin.NewInTreeRegistry(opts, golemModule)
allPlugins := plugin.NewRegistry()

// 合并: 内置 + 配置文件中的外置
allPlugins.Merge(inTreeRegistry)        // ← 内置插件（优先级高）
allPlugins.LoadExternalPlugins(opts)    // ← 外置插件

// 插件加载顺序（由 Plugin Framework 负责）
for _, pluginID := range loadOrder {
    if err := allPlugins.Init(pluginID, deps); err != nil {
        return err
    }
}
```

### 加载优先级

| 优先级 | 插件名 | 依赖 | 加载时机 |
|--------|--------|------|----------|
| P0 (Critical) | **memory-core** | 无 | InitAgents 阶段 |
| P0 | **diagnostics** | 无 | InitAgents 阶段 |
| P1 | **llm-task** | LLM Module | InitPluginLifecycle |
| P1 | **subagent** | Agents Module | InitPluginLifecycle |
| P1 | **golem-cluster** | Golem Module | InitPluginLifecycle |
| P2 | **skills** | Plugin API | InitPluginLifecycle |
| P2 | **web-search** | LLM Module | InitPluginLifecycle |
| P3 (Channels) | **channel-feishu** | Gateway | InitChannelGateway |
| P3 | **channel-telegram** | Gateway | InitChannelGateway |

---

## 10 个内置插件详解

### 1. memory-core（记忆核心）

**位置**：`internal/hivemind/service/plugin/builtin/memory-core/`  
**大小**：19.69 KB  
**优先级**：P0 (Critical)  
**槽位**：`memory` (独占槽，每次只有一个激活)

#### 功能

提供 Echoryn 的核心记忆系统，支持：

- **混合搜索**：向量搜索 + 全文搜索 (FTS5)
- **快速检索**：近似最近邻 (ANN) 加速向量查询
- **知识蒸馏**：自动将执行历史转化为持久化知识

#### 技术栈

- **数据库**：SQLite3 + FTS5 全文索引
- **向量存储**：Embedding 维度可配（默认 1536）
- **保留策略**：TTL 自动清理 + 大小上限

#### 配置参数

```json
{
  "memory": {
    "entries": {
      "memory-core": {
        "enabled": true,
        "config": {
          "db_path": "${DATA_DIR}/memory/default.db",
          "embedding_model": "text-embedding-3-small",
          "vector_dim": 1536,
          "top_k": 5,
          "ttl_days": 90,
          "max_size_mb": 500,
          "fts_language": "en"
        }
      }
    }
  }
}
```

#### 关键接口

```go
// MemoryStore 接口（由此插件实现）
type MemoryStore interface {
    Save(ctx context.Context, memory *Memory) error
    Query(ctx context.Context, q *Query) ([]Memory, error)
    Delete(ctx context.Context, id string) error
    Clear(ctx context.Context) error
}

// Memory 实体
type Memory struct {
    ID          string
    AgentID     string
    Content     string          // 记忆内容
    Embedding   []float32       // 向量嵌入
    Tags        []string        // 分类标签
    Importance  float32         // 重要度 0~1
    CreatedAt   time.Time
    ExpiresAt   time.Time       // TTL
}
```

#### 使用场景

1. **Agent 知识积累**：执行完成后，自动蒸馏重要信息到记忆库
2. **上下文注入**：构建新 Agent 上下文时，检索相关记忆
3. **长期学习**：跨多个 Session/Run 的知识持久化

---

### 2. diagnostics（诊断与可观测性）

**位置**：`internal/hivemind/service/plugin/builtin/diagnostics/`  
**大小**：11.79 KB  
**优先级**：P0 (Critical)  
**槽位**：`observability` (独占槽)

#### 功能

集成 **OpenTelemetry (OTEL)** 完整链路追踪和指标收集：

- **追踪（Tracing）**：记录 Agent 执行的每一步调用栈
- **指标（Metrics）**：Prometheus 兼容的时间序列指标
- **日志（Logging）**：结构化日志聚合

#### 配置参数

```json
{
  "plugins": {
    "entries": {
      "diagnostics": {
        "enabled": true,
        "config": {
          "otel_exporter": "jaeger",        // 导出后端：jaeger / prometheus / otlp
          "jaeger_endpoint": "http://localhost:14268/api/traces",
          "prometheus_port": 9090,
          "sampling_rate": 0.1,             // 采样率
          "enable_metrics": true,
          "enable_tracing": true,
          "enable_logging": true
        }
      }
    }
  }
}
```

#### 关键指标

```
# Agent 执行
agent_run_total{status="success|error"}
agent_run_duration_ms{p50,p95,p99}
agent_toolcall_total
agent_error_total{error_type}

# 工具循环
agent_toolloop_detected_total{detector_kind}
agent_toolloop_breaker_triggered_total

# 上下文
context_compaction_total
context_token_usage_histogram

# LLM 调用
llm_request_total{model}
llm_token_input_total{model}
llm_token_output_total{model}
llm_latency_ms{model}
```

#### 集成方式

- Jaeger UI 可视化链路：`http://localhost:16686`
- Prometheus 拉取指标：`http://localhost:9090`
- 结合 Grafana 构建实时仪表板

---

### 3. llm-task（JSON-only LLM 工具）

**位置**：`internal/hivemind/service/plugin/builtin/llmtask/`  
**大小**：8.78 KB  
**优先级**：P1  
**槽位**：`tools` (多槽，可同时存在多个工具提供者)

#### 功能

为 Agent 提供通用的 LLM 工具：

```
llm_call {
  "model": "deepseek-chat",
  "prompt": "分析这段代码的复杂度",
  "temperature": 0.7,
  "max_tokens": 2000
}
```

**特点**：
- 仅接受 JSON 输入和输出（安全性考虑）
- 支持流式响应
- 内置超时和重试

#### 配置参数

```json
{
  "plugins": {
    "entries": {
      "llm-task": {
        "enabled": true,
        "config": {
          "model_whitelist": ["openai:gpt-4o", "deepseek:chat"],
          "default_model": "deepseek-chat",
          "timeout_seconds": 60,
          "max_retries": 3
        }
      }
    }
  }
}
```

---

### 4. subagent（子智能体管理）

**位置**：`internal/hivemind/service/plugin/builtin/subagent/`  
**大小**：12.9 KB  
**优先级**：P1  
**槽位**：`tools`

#### 功能

为 Agent 提供启动和监控子智能体的能力：

```
subagent_spawn {
  "agent_id": "researcher",
  "task": "搜索最新的 AI 论文",
  "timeout_ms": 30000
}
```

#### 提供的工具

| 工具 | 功能 |
|------|------|
| `subagent_spawn` | 启动子智能体 |
| `subagent_status` | 查询子智能体状态 |
| `subagent_wait` | 阻塞等待完成 |
| `subagent_cancel` | 取消执行 |
| `subagent_collect` | 收集结果 |

#### Observer 集成

自动收集 SubAgent 的执行指标：

```go
type SubAgentMetrics struct {
    ExecutionDuration   time.Duration
    TokenUsage          int
    ToolCallCount       int
    ErrorCount          int
    IsSuccess           bool
}
```

---

### 5. golem-cluster（Golem 集群管理）

**位置**：`internal/hivemind/service/plugin/builtin/golem-cluster/`  
**大小**：25.9 KB  
**优先级**：P1  
**槽位**：`tools`  
**特殊依赖**：GolemModule (post-init injection)

#### 功能

将 Golem 分布式工作节点集群暴露给 Agent：

```
golem_list_nodes {}           // 列出所有工作节点

golem_execute_remote {
  "node_selector": "region=us-west",
  "task": { "type": "compute", "cmd": "..." }
}

golem_get_cluster_status {}   // 集群状态
```

#### 配置参数

```json
{
  "plugins": {
    "entries": {
      "golem-cluster": {
        "enabled": true,
        "config": {
          "scheduler_type": "ai-aware",    // ai-aware / round-robin / least-loaded
          "node_selector_enabled": true,
          "liveness_probe_interval_sec": 5,
          "task_timeout_sec": 300
        }
      }
    }
  }
}
```

#### 架构集成

```
Agent Runtime
    ├── golem-cluster Plugin
    ├── → GolemModule.Registry       (节点注册中心)
    ├── → GolemModule.Dispatcher     (任务分发器)
    ├── → GolemModule.Scheduler      (调度器)
    └── → GolemModule.TokenManager   (认证)
```

---

### 6. skills（技能加载器）

**位置**：`internal/hivemind/service/plugin/builtin/skills/`  
**大小**：10.26 KB  
**优先级**：P2  
**槽位**：`tools`

#### 功能

从文件系统动态加载 `.skill.md` 文件定义的技能：

```markdown
# Skill: image-analysis
version: 1.0
description: 分析图像内容

## Parameters
- image_url: string (required)

## Execution
使用 Claude Vision 分析图像...

## Examples
- 分析这张图片的主要内容
```

#### 技能加载路径

```
~/.echoryn/workspace/
├── skills/
│   ├── image-analysis.skill.md
│   ├── text-summarize.skill.md
│   └── code-review.skill.md
```

#### 配置参数

```json
{
  "plugins": {
    "entries": {
      "skills": {
        "enabled": true,
        "config": {
          "skills_dir": "${DATA_DIR}/workspace/skills",
          "auto_reload": true,
          "reload_interval_sec": 5
        }
      }
    }
  }
}
```

---

### 7. web-search（Web 搜索）

**位置**：`internal/hivemind/service/plugin/builtin/web-search/gemini-web-search/`  
**大小**：3.59 KB  
**优先级**：P2  
**槽位**：`tools`

#### 功能

通过 **Gemini Grounding** 进行 Web 搜索（实时信息检索）：

```
web_search {
  "query": "latest AI news March 2026",
  "num_results": 5
}
```

#### 配置参数

```json
{
  "plugins": {
    "entries": {
      "web-search": {
        "enabled": true,
        "config": {
          "gemini_api_key": "${GEMINI_API_KEY}",
          "search_results_limit": 10,
          "cache_ttl_minutes": 60
        }
      }
    }
  }
}
```

#### 限制条件

- 仅在 Gemini 2.0 及以上版本支持 Grounding
- 需要独立的 Gemini API Key
- 搜索结果自动缓存 1 小时

---

### 8. channel-feishu（飞书 IM 集成）

**位置**：`internal/hivemind/service/plugin/builtin/channel/feishu/`  
**大小**：20.04 KB  
**优先级**：P3  
**槽位**：`channel` (多槽，可同时接入多个 IM)

#### 功能

集成飞书（Lark）作为 Agent 的 IM 渠道：

```
Agent ← MessageBus → Feishu Channel Plugin → Feishu Bot
                  ↓
              用户收到消息
```

#### 配置参数

```json
{
  "plugins": {
    "entries": {
      "channel-feishu": {
        "enabled": true,
        "config": {
          "app_id": "${FEISHU_APP_ID}",
          "app_secret": "${FEISHU_APP_SECRET}",
          "bot_name": "EchorynBot",
          "message_type": "text|card|interactive",
          "webhook_url": "https://your-domain/feishu/webhook"
        }
      }
    }
  }
}
```

#### 消息流向

```
User Feishu Chat
    ↓
Feishu Webhook
    ↓
ChannelManager (Dispatcher)
    ↓
Route to Agent
    ↓
Agent Runtime
    ↓
Response via ChannelManager
    ↓
Feishu Bot Reply
```

---

### 9. channel-telegram（Telegram 机器人）

**位置**：`internal/hivemind/service/plugin/builtin/channel/telegram/`  
**大小**：9.04 KB  
**优先级**：P3  
**槽位**：`channel`

#### 功能

集成 Telegram 作为 Agent 的 IM 渠道：

```
Telegram User
    ↓
/chat 任务描述
    ↓
Agent Execution
    ↓
Telegram Message
```

#### 配置参数

```json
{
  "plugins": {
    "entries": {
      "channel-telegram": {
        "enabled": true,
        "config": {
          "bot_token": "${TELEGRAM_BOT_TOKEN}",
          "webhook_url": "https://your-domain/telegram/webhook",
          "allowed_user_ids": [123456789, 987654321],
          "message_timeout_sec": 30
        }
      }
    }
  }
}
```

---

### 10. 预留位置（未来扩展）

当前共 9 个实现的内置插件，第 10 个位置预留给未来需要内置的新插件。

| 潜在候选 | 优先级 | 原因 |
|----------|--------|------|
| web-crawler | P2 | 网页抓取与内容提取 |
| email-sender | P3 | 邮件集成 |
| knowledge-graph | P1 | 知识图谱生成 |
| cost-optimizer | P2 | LLM 成本优化 |

---

## 插件槽位系统

### 槽位分类

Echoryn 使用"槽位"约束插件的加载方式：

#### 独占槽（Singleton Slot）

一次只有一个插件可以激活。用于**单一职责**的系统组件：

| 槽位 | 当前插件 | 说明 |
|------|----------|------|
| `memory` | memory-core | Agent 记忆系统（一次一个） |
| `observability` | diagnostics | 可观测性后端（一次一个） |

#### 多槽（Multi-Slot）

允许多个插件同时激活。用于**扩展能力**（工具、渠道）：

| 槽位 | 支持的插件 | 说明 |
|------|-----------|------|
| `tools` | llm-task, subagent, golem-cluster, skills, web-search | 提供各种工具 |
| `channel` | channel-feishu, channel-telegram, ... | 支持多个 IM 渠道 |

### 配置示例

```json
{
  "plugins": {
    "entries": {
      // 独占槽：memory
      "memory-core": {
        "enabled": true,
        "slots": ["memory"],
        "config": { ... }
      },
      // 独占槽：observability
      "diagnostics": {
        "enabled": true,
        "slots": ["observability"],
        "config": { ... }
      },
      // 多槽：tools
      "llm-task": {
        "enabled": true,
        "slots": ["tools"],
        "config": { ... }
      },
      "subagent": {
        "enabled": true,
        "slots": ["tools"],
        "config": { ... }
      },
      // 多槽：channel
      "channel-feishu": {
        "enabled": true,
        "slots": ["channel"],
        "config": { ... }
      },
      "channel-telegram": {
        "enabled": true,
        "slots": ["channel"],
        "config": { ... }
      }
    }
  }
}
```

---

## 配置解析

### 配置源（Configuration Sources）

内置插件的配置来自以下优先级顺序：

```
优先级 1 (最高)：命令行参数
  → --plugin-config memory-core.db_path=/custom/path

优先级 2：环境变量（前缀 ECHORYN_）
  → ECHORYN_PLUGIN_MEMORY_CORE_DB_PATH=/custom/path

优先级 3：配置文件（JSON）
  → conf/hivemind-server.json → plugins.entries[pluginID].config

优先级 4 (最低)：代码内默认值
  → plugin 包中的 DefaultConfig()
```

### 占位符解析

支持的占位符（在配置文件中）：

| 占位符 | 解析为 |
|--------|--------|
| `${DATA_DIR}` | `~/.echoryn/` |
| `${WORKSPACE}` | `~/.echoryn/workspace/` |
| `${OPENAI_API_KEY}` | 环境变量 `OPENAI_API_KEY` |
| `${DEEPSEEK_API_KEY}` | 环境变量 `DEEPSEEK_API_KEY` |
| `${GEMINI_API_KEY}` | 环境变量 `GEMINI_API_KEY` |

### 配置文件示例

```json
{
  "plugins": {
    "entries": {
      "memory-core": {
        "enabled": true,
        "config": {
          "db_path": "${DATA_DIR}/memory/default.db",
          "embedding_model": "text-embedding-3-small",
          "vector_dim": 1536,
          "top_k": 5,
          "ttl_days": 90
        }
      },
      "diagnostics": {
        "enabled": true,
        "config": {
          "otel_exporter": "jaeger",
          "jaeger_endpoint": "http://localhost:14268/api/traces",
          "prometheus_port": 9090,
          "sampling_rate": 0.1
        }
      },
      "llm-task": {
        "enabled": true,
        "config": {
          "model_whitelist": ["openai:gpt-4o", "deepseek:chat"],
          "default_model": "deepseek-chat"
        }
      },
      "channel-feishu": {
        "enabled": true,
        "config": {
          "app_id": "${FEISHU_APP_ID}",
          "app_secret": "${FEISHU_APP_SECRET}",
          "bot_name": "EchorynBot"
        }
      },
      "channel-telegram": {
        "enabled": true,
        "config": {
          "bot_token": "${TELEGRAM_BOT_TOKEN}",
          "webhook_url": "https://your-domain/telegram/webhook",
          "allowed_user_ids": [123456789]
        }
      }
    }
  }
}
```

---

## 最佳实践

### 1. 插件启用决策

**何时启用何时禁用**：

| 插件 | 启用条件 | 禁用条件 |
|------|----------|----------|
| memory-core | 始终启用 | 无（必须） |
| diagnostics | 生产环境需要追踪 | 仅开发调试时可禁用 |
| llm-task | 需要通用 LLM 调用 | 已有专用工具 |
| subagent | 需要多智能体协作 | 单 Agent 场景 |
| golem-cluster | 部署分布式 Golem | 单机模式 |
| channel-* | 支持 IM 渠道 | 无 IM 需求 |

### 2. 配置管理

**建议做法**：

```bash
# 生产环境：使用环境变量管理敏感信息
export FEISHU_APP_ID=xxx
export FEISHU_APP_SECRET=yyy
export GEMINI_API_KEY=zzz

# 启动 Hivemind
./hivemind --config conf/hivemind-server-prod.json
```

### 3. 性能调优

**内存优化**：

```json
{
  "memory-core": {
    "config": {
      "max_size_mb": 500,        // 限制数据库大小
      "embedding_batch_size": 32, // 批量嵌入
      "ttl_days": 30              // 更激进的清理
    }
  }
}
```

**诊断采样**：

```json
{
  "diagnostics": {
    "config": {
      "sampling_rate": 0.1  // 仅采样 10% 的链路（高流量环境）
    }
  }
}
```

### 4. 插件顺序依赖

确保加载顺序满足依赖关系：

```
内置插件自动按以下顺序初始化：
1. memory-core       (无依赖)
2. diagnostics       (无依赖)
3. llm-task          (依赖 LLM Module)
4. subagent          (依赖 Agents Module)
5. golem-cluster     (依赖 Golem Module)
6. skills            (依赖 Plugin API)
7. web-search        (依赖 LLM Module)
8-9. channels        (依赖 Gateway)
```

**注意**：框架会自动处理顺序，无需手动配置。

### 5. 问题排查

**常见问题**：

| 问题 | 原因 | 解决 |
|------|------|------|
| 插件加载失败 | 配置文件格式错误 | 检查 JSON 语法，验证占位符替换 |
| 无法连接外部服务 | API Key 未设置 | 检查环境变量或配置文件 |
| 内存占用持续增长 | memory-core 未清理过期数据 | 检查 TTL 和 max_size_mb 设置 |
| IM 消息无响应 | Channel 插件未正确初始化 | 检查 webhook URL 和认证信息 |

---

## 附录 A：插件API参考

### PluginDefinition

```go
type PluginDefinition struct {
    ID          string   // 插件唯一标识，e.g. "memory-core"
    Name        string   // 显示名称
    Version     string   // 语义版本
    Description string   // 功能描述
    Slots       []string // 支持的槽位: ["memory"], ["tools"], ["channel"]
}
```

### Plugin Interface

所有内置插件必须实现：

```go
type Plugin interface {
    // 返回定义信息
    Definition() PluginDefinition
    
    // 初始化（接收配置和依赖）
    Init(ctx context.Context, args PluginArgs) error
    
    // 可选生命周期钩子
    // Start/Stop、GetTool、GetHook 等
}
```

### PluginArgs

```go
type PluginArgs map[string]interface{}

// 常见键：
// "config"      : plugin config object
// "registry"    : node registry (for golem-cluster)
// "dispatcher"  : task dispatcher (for golem-cluster)
// "llm_manager" : LLM manager (for llm-task, web-search)
```

---

## 附录 B：版本历史

| 版本 | 日期 | 变更 |
|------|------|------|
| v1.0 | 2026-03-31 | 初始版本，记录 9 个内置插件 |

---

## 附录 C：快速参考表

### 按功能分类

| 功能 | 插件 |
|------|------|
| 记忆存储 | memory-core |
| 可观测性 | diagnostics |
| 工具执行 | llm-task, llm-task, subagent, golem-cluster, skills, web-search |
| IM 集成 | channel-feishu, channel-telegram |

### 按大小排序

| 排序 | 插件 | 大小 |
|------|------|------|
| 1 | web-search | 3.59 KB |
| 2 | llm-task | 8.78 KB |
| 3 | channel-telegram | 9.04 KB |
| 4 | skills | 10.26 KB |
| 5 | diagnostics | 11.79 KB |
| 6 | subagent | 12.9 KB |
| 7 | memory-core | 19.69 KB |
| 8 | channel-feishu | 20.04 KB |
| 9 | golem-cluster | 25.9 KB |

---

> **记住**：这份清单是 Echoryn 项目的权威参考。如有新增内置插件，请同步更新此文档。
