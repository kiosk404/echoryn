<h1 align="center">Echoryn 项目</h1>

<p align="center">用 Go 重构 Openclaw，打造 AI 虚拟角色的灵魂容器。</p>

<p align="center">
  <strong>🚧 开发进行中 (WIP) – 项目尚未完成。</strong>
</p>

<div align="center">

[![Go 版本](https://img.shields.io/badge/Go-1.25.0-blue)](https://golang.org/)
[![许可证](https://img.shields.io/badge/许可证-MIT-green)](../LICENSE)
[![Go 报告卡](https://goreportcard.com/badge/github.com/kiosk404/echoryn)](https://goreportcard.com/report/github.com/kiosk404/echoryn)
[![zread](https://img.shields.io/badge/Ask_Zread-_.svg?style=flat&color=00b0aa&labelColor=000000&logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB3aWR0aD0iMTYiIGhlaWdodD0iMTYiIHZpZXdCb3g9IjAgMCAxNiAxNiIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTQuOTYxNTYgMS42MDAxSDIuMjQxNTZDMS44ODgxIDEuNjAwMSAxLjYwMTU2IDEuODg2NjQgMS42MDE1NiAyLjI0MDFWNC45NjAxQzEuNjAxNTYgNS4zMTM1NiAxLjg4ODEgNS42MDAxIDIuMjQxNTYgNS42MDAxSDQuOTYxNTZDNS4zMTUwMiA1LjYwMDEgNS42MDE1NiA1LjMxMzU2IDUuNjAxNTYgNC45NjAxVjIuMjQwMUM1LjYwMTU2IDEuODg2NjQgNS4zMTUwMiAxLjYwMDEgNC45NjE1NiAxLjYwMDFaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00Ljk2MTU2IDEwLjM5OTlIMi4yNDE1NkMxLjg4ODEgMTAuMzk5OSAxLjYwMTU2IDEwLjY4NjQgMS42MDE1NiAxMS4wMzk5VjEzLjc1OTlDMS42MDE1NiAxNC4xMTM0IDEuODg4MSAxNC4zOTk5IDIuMjQxNTYgMTQuMzk5OUg0Ljk2MTU2QzUuMzE1MDIgMTQuMzk5OSA1LjYwMTU2IDE0LjExMzQgNS42MDE1NiAxMy43NTk5VjExLjAzOTlDNS42MDE1NiAxMC42ODY0IDUuMzE1MDIgMTAuMzk5OSA0Ljk2MTU2IDEwLjM5OTlaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik0xMy43NTg0IDEuNjAwMUgxMS4wMzg0QzEwLjY4NSAxLjYwMDEgMTAuMzk4NCAxLjg4NjY0IDEwLjM5ODQgMi4yNDAxVjQuOTYwMUMxMC4zOTg0IDUuMzEzNTYgMTAuNjg1IDUuNjAwMSAxMS4wMzg0IDUuNjAwMUgxMy43NTg0QzE0LjExMTkgNS42MDAxIDE0LjM5ODQgNS4zMTM1NiAxNC4zOTg0IDQuOTYwMVYyLjI0MDFDMTQuMzk4NCAxLjg4NjY0IDE0LjExMTkgMS42MDAxIDEzLjc1ODQgMS42MDAxWiIgZmlsbD0iI2ZmZiIvPgo8cGF0aCBkPSJNNCAxMkwxMiA0TDQgMTJaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00IDEyTDEyIDQiIHN0cm9rZT0iI2ZmZiIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIvPgo8L3N2Zz4K&logoColor=ffffff)](https://zread.ai/kiosk404/echoryn)

</div>

## ✨ 概述

**Echoryn** 是一个分布式 AI 虚拟角色容器平台，旨在为 AI 智能体提供一个"灵魂容器"，将它们带入我们的世界。受 Openclaw 项目和《复仇者联盟》中奥创的启发，Echoryn 采用了中心智能体（Hivemind）与可更换机器身体（Golem）的架构设计，类似 Kubernetes 的协调模式。

如同奥创可以随时切换不同的机械躯体执行任务，Echoryn 的 Hivemind（蜂巢智心）作为中心智能体，能够协调多个 Golem（傀儡）工作节点，实现智能体的分布式任务执行。Hivemind 负责决策、记忆和调度，而 Golem 则作为可互换的"身体"执行具体操作。

该平台为 AI 智能体提供了完整的基础设施，包括 LLM 集成、插件系统、内存管理和分布式任务执行。

## 🚀 功能特性

### 核心架构
- **Hivemind**: 中央协调服务器，用于节点管理和任务调度
- **Golem**: 工作节点，本地执行技能和任务
- **Echoctl**: 命令行管理工具（类似 kubectl）

### AI 能力
- **多 LLM 提供商支持**: OpenAI、Claude、DeepSeek、Gemini、Ollama 等
- **模型上下文协议 (MCP)**: 标准化的工具和资源集成
- **CloudWeGo Eino 集成**: 具有推理能力的高级 LLM 框架
- **内存系统**: 向量搜索、语义内存和上下文管理

### 插件系统
- **Kubernetes 风格的插件框架**: 接口驱动的可扩展性
- **插槽机制**: 确保特定类型只有一个插件处于激活状态
- **多种集成类型**: 工具、钩子、服务、CLI 命令、提示词
- **自动发现**: 框架自动检测插件接口

### 开发者体验
- **基于 gRPC 的通信**: 双向流式传输实现实时任务分发
- **全面配置**: 支持 JSON、环境变量和命令行标志
- **内置可观测性**: OpenTelemetry 集成用于监控和追踪
- **生产就绪设计**: 优雅关闭、健康检查和生命周期管理
- **现代化 TUI**: 使用 BubbleTea 和 LipGloss 的漂亮终端界面

## 🏗️ 架构设计

### 系统组件

```
┌─────────────────────────────────────────────────────────────┐
│                       Hivemind (大脑)                      │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐  │
│  │   节点管理   │  │  任务调度   │  │  插件注册中心     │  │
│  └─────────────┘  └─────────────┘  └───────────────────┘  │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐  │
│  │  LLM 代理   │  │    内存     │  │   API 网关        │  │
│  └─────────────┘  └─────────────┘  └───────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                           │
             gRPC (双向流式传输)
                           │
┌─────────────────────────────────────────────────────────────┐
│                       Golem (工作者)                        │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐  │
│  │  技能执行    │  │  本地资源   │  │   心跳保持        │  │
│  └─────────────┘  └─────────────┘  └───────────────────┘  │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐  │
│  │  工具运行器  │  │  任务队列   │  │   状态同步        │  │
│  └─────────────┘  └─────────────┘  └───────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 目录结构

```
echoryn/
├── cmd/                    # 可执行程序入口点
│   ├── hivemind/          # 主服务器 (Hivemind)
│   ├── golem/             # 工作节点 (Golem)
│   └── echoctl/           # 命令行管理工具
├── internal/              # 内部包
│   ├── hivemind/          # Hivemind 实现
│   ├── golem/             # Golem 实现
│   └── echoctl/           # CLI 实现
├── pkg/                   # 公共库包
│   ├── app/               # 应用框架
│   ├── cli/               # CLI 工具
│   ├── http/              # HTTP 工具
│   ├── logger/            # 日志系统
│   └── utils/             # 工具函数
├── idl/                   # 接口定义语言
│   └── golem/             # Golem gRPC 协议定义
├── conf/                  # 配置文件
├── docs/                  # 文档
├── scripts/               # 构建脚本
└── golem-worker/          # Golem 工作空间目录
```

## 🚀 快速开始

### 先决条件

- **Go 1.25.0** 或更高版本
- **Git** 用于版本控制
- **Make** 用于构建自动化
- **SQLite** (可选，用于本地存储)

### 安装

```bash
# 克隆仓库
git clone https://github.com/kiosk404/echoryn.git
cd echoryn

# 安装依赖并构建
make all
```

这将：
1. 运行 `go mod tidy` 管理依赖
2. 格式化代码
3. 运行代码检查
4. 构建所有二进制文件

### 运行 Echoryn

#### 1. 启动 Hivemind 服务器

```bash
# 使用 make (开发环境)
make run.hivemind

# 或直接使用配置
./output/platforms/linux/amd64/hivemind --config conf/hivemind-server.json
```

#### 2. 启动 Golem 工作节点

```bash
# 在另一个终端中
make run.golem

# 或直接使用配置
./output/platforms/linux/amd64/golem --config conf/golem-worker.json
```

#### 3. 使用 Echoctl 进行管理

```bash
# 列出可用令牌
./output/platforms/linux/amd64/echoctl token list

# 创建新令牌
./output/platforms/linux/amd64/echoctl token create --name "admin"

# 获取系统信息
./output/platforms/linux/amd64/echoctl info
```

## 📦 构建选项

Echoryn 使用全面的 Makefile 进行构建自动化：

```bash
# 构建所有二进制文件
make build

# 构建特定二进制文件
make build BINS="hivemind echoctl"

# 运行测试
make test

# 运行测试并生成覆盖率报告
make cover

# 格式化代码
make format

# 运行代码检查
make lint

# 生成 Protobuf 代码
make proto

# 清理构建输出
make clean

# 显示帮助
make help
```

## ⚙️ 配置

### Hivemind 配置 (`conf/hivemind-server.json`)

```json
{
  "grpc": {
    "bind-address": "0.0.0.0",
    "bind-port": 11788,
    "max-msg-size": 4194304
  },
  "serving": {
    "mode": "debug",
    "healthz": true,
    "bind-address": "0.0.0.0",
    "bind-port": 11789
  },
  "models": {
    "mode": "merge",
    "default-provider": "deepseek",
    "default-model": "deepseek-chat",
    "providers": {
      "deepseek": {
        "base-url": "https://api.deepseek.com/v1",
        "api-key": "${DEEPSEEK_API_KEY}",
        "models": [...]
      }
    }
  },
  "plugins": {
    "enabled": true,
    "slots": {
      "memory": "memory-core"
    },
    "entries": {...}
  }
}
```

### Golem 配置 (`conf/golem-worker.json`)

```json
{
  "hivemind": {
    "address": "localhost:11788",
    "token": "${GOLEM_TOKEN}",
    "heartbeat-interval": "30s"
  },
  "skills": {
    "enabled": true,
    "workspace-dir": ".echoryn/golem"
  }
}
```

### 环境变量

```bash
# LLM API 密钥
export DEEPSEEK_API_KEY="your-api-key"
export OPENAI_API_KEY="your-api-key"
export ANTHROPIC_API_KEY="your-api-key"

# Golem 配置
export GOLEM_TOKEN="your-golem-token"

# 日志级别
export LOG_LEVEL="info"
```

## 🔌 插件系统

Echoryn 拥有一个强大的插件系统，灵感来自 Kubernetes 调度器框架：

### 插件类型

- **工具**: 用新功能扩展智能体能力
- **钩子**: 在关键点拦截和修改系统行为
- **服务**: 长时间运行的后台服务
- **CLI 命令**: 向 echoctl 添加新命令
- **提示词部分**: 用动态内容扩展系统提示词
- **运行时 API**: 为智能体运行时提供 API

### 创建插件

1. **实现插件接口**:
   ```go
   package myplugin

   import (
       "context"
       "github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
   )

   type MyPlugin struct{}

   func (p *MyPlugin) Name() string { return "my-plugin" }
   func (p *MyPlugin) Init(ctx context.Context) error { return nil }
   func (p *MyPlugin) Start(ctx context.Context) error { return nil }
   func (p *MyPlugin) Stop(ctx context.Context) error { return nil }
   ```

2. **注册你的插件**:
   ```go
   func init() {
       plugin.Register(&MyPlugin{})
   }
   ```

3. **在 Hivemind 中配置**:
   ```json
   {
     "plugins": {
       "enabled": true,
       "entries": {
         "my-plugin": {
           "config": {
             "enabled": true,
             "my-setting": "value"
           }
         }
       }
     }
   }
   ```

查看 [插件系统规范](ECHORYN_MEMORY_SPEC.md) 获取详细信息。

## 🛠️ 开发

### 设置开发环境

```bash
# 克隆仓库
git clone https://github.com/kiosk404/echoryn.git
cd echoryn

# 安装 Go 依赖
go mod download

# 安装开发工具
make tools.install

# 构建并运行测试
make all
```

### 项目结构

- **`cmd/`**: 主要应用入口点
- **`internal/`**: 私有应用代码
- **`pkg/`**: 公共库
- **`idl/`**: 协议定义 (gRPC)
- **`conf/`**: 配置示例
- **`docs/`**: 文档
- **`scripts/`**: 构建和开发脚本

### 代码风格

- 使用 `gofmt`、`goimports` 和 `golines` 进行格式化
- 遵循标准 Go 约定
- 编写全面的测试
- 文档化公共 API

### 测试

```bash
# 运行所有测试
make test

# 运行测试并生成覆盖率报告
make cover

# 运行特定测试
go test ./internal/hivemind/...
```

## 📚 文档

### 核心规范
- [Echoryn 项目规范](../.vibe/ECHORYN_SPEC.md) - 整体项目架构和愿景
- [插件系统规范](ECHORYN_MEMORY_SPEC.md) - 详细的插件架构
- [调度器规范](../internal/hivemind/service/golem/scheduler/SCHED_SPEC.md) - 任务调度引擎设计
- `conf/` 中的配置文件 - 配置示例

### 模块规范 (在 `.vibe/` 目录中)
- [ECHORYN_HIVEMIND_AGENTS_SPEC.md](../.vibe/ECHORYN_HIVEMIND_AGENTS_SPEC.md) - 智能体运行时引擎
- [ECHORYN_HIVEMIND_LLM_SPEC.md](../.vibe/ECHORYN_HIVEMIND_LLM_SPEC.md) - LLM 多模型管理
- [ECHORYN_HIVEMIND_MEMORY_SPEC.md](../.vibe/ECHORYN_HIVEMIND_MEMORY_SPEC.md) - 内存系统
- [ECHORYN_HIVEMIND_PLUGIN_SPEC.md](../.vibe/ECHORYN_HIVEMIND_PLUGIN_SPEC.md) - 插件框架
- [ECHORYN_HIVEMIND_MCP_SPEC.md](../.vibe/ECHORYN_HIVEMIND_MCP_SPEC.md) - MCP 工具调用

### 代码文档
- 代码注释中的全面 API 文档
- GoDoc 生成的文档 (即将推出)

## 🤝 贡献

欢迎贡献！以下是你可以帮助的方式：

1. **报告问题**: 使用 GitHub 问题跟踪器报告错误或请求功能
2. **提交 Pull Request**: Fork 仓库并提交改进的 PR
3. **改进文档**: 帮助增强文档和示例
4. **分享想法**: 在问题中讨论潜在的改进

### 开发工作流程

1. Fork 仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m '添加了很棒的功能'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 打开 Pull Request

## 📄 许可证

本项目基于 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件获取详细信息。

## 🙏 致谢

- **Openclaw**: 灵感和架构模式
- **CloudWeGo Eino**: 高级 LLM 框架
- **Kubernetes**: 插件系统设计灵感
- **模型上下文协议 (MCP)**: 标准化的工具集成

## 🔗 相关项目

- [Openclaw](https://github.com/openclaw/openclaw) - 原始灵感来源
- [CloudWeGo Eino](https://github.com/cloudwego/eino) - LLM 框架
- [模型上下文协议](https://spec.modelcontextprotocol.org/) - AI 工具标准

---

<div align="center">
  <p>由 Echoryn 贡献者用 ❤️ 制作</p>
  <p>将 AI 虚拟角色带入我们的世界，一次一个容器</p>
</div>