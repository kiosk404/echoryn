<h1 align="center">Project Echoryn</h1>

<p align="center">Re-creating Openclaw Via Golang, a soul container of AI virtual characters to bring them into our world.</p>

<p align="center">
  <strong>🚧 Work in progress (WIP) – this project is not finished yet.</strong>
</p>

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-1.25.0-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/kiosk404/echoryn)](https://goreportcard.com/report/github.com/kiosk404/echoryn)

</div>

## ✨ Overview

**Echoryn** is a distributed AI virtual character container platform designed to provide AI agents with a "soul container" that brings them into our world. Inspired by the Openclaw project, Echoryn enables the creation, deployment, and management of AI virtual characters through a Kubernetes-like architecture with Hivemind (central coordination) and Golem (worker nodes).

The platform provides a complete infrastructure for AI agents, including LLM integration, plugin systems, memory management, and distributed task execution.

## 🚀 Features

### Core Architecture
- **Hivemind**: Central coordination server for node management and task scheduling
- **Golem**: Worker nodes that execute skills and tasks locally
- **Echoctl**: Command-line management tool (similar to kubectl)

### AI Capabilities
- **Multi-LLM Provider Support**: OpenAI, Claude, DeepSeek, Gemini, Ollama, and more
- **Model Context Protocol (MCP)**: Standardized tool and resource integration
- **CloudWeGo Eino Integration**: Advanced LLM framework with reasoning capabilities
- **Memory System**: Vector search, semantic memory, and context management

### Plugin System
- **Kubernetes-inspired Plugin Framework**: Interface-driven extensibility
- **Slot Mechanism**: Ensures only one plugin of a specific type is active
- **Multiple Integration Types**: Tools, hooks, services, CLI commands, prompts
- **Automatic Discovery**: Framework auto-detects plugin interfaces

### Developer Experience
- **gRPC-based Communication**: Bidirectional streaming for real-time task distribution
- **Comprehensive Configuration**: Support for JSON, environment variables, and flags
- **Built-in Observability**: OpenTelemetry integration for monitoring and tracing
- **Production-ready Design**: Graceful shutdown, health checks, and lifecycle management
- **Modern TUI**: Beautiful terminal interfaces using BubbleTea and LipGloss

## 🏗️ Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                       Hivemind (Brain)                      │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐    │
│  │  Node Mgmt  │  │ Task Sched  │  │  Plugin Registry  │    │
│  └─────────────┘  └─────────────┘  └───────────────────┘    │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐    │
│  │  LLM Proxy  │  │  Memory     │  │   API Gateway     │    │
│  └─────────────┘  └─────────────┘  └───────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                           │
            gRPC (bidirectional streaming)
                           │
┌─────────────────────────────────────────────────────────────┐
│                       Golem (Worker)                        │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐    │
│  │  Skill Exec │  │  Local Res  │  │   Heartbeat       │    │ 
│  └─────────────┘  └─────────────┘  └───────────────────┘    │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐    │
│  │  Tool Runner│  │  Task Queue │  │   State Sync      │    │
│  └─────────────┘  └─────────────┘  └───────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### Directory Structure

```
echoryn/
├── cmd/                    # Executable entry points
│   ├── hivemind/          # Main server (Hivemind)
│   ├── golem/             # Worker node (Golem)
│   └── echoctl/           # Command-line management tool
├── internal/              # Internal packages
│   ├── hivemind/          # Hivemind implementation
│   ├── golem/             # Golem implementation
│   └── echoctl/           # CLI implementation
├── pkg/                   # Public library packages
│   ├── app/               # Application framework
│   ├── cli/               # CLI utilities
│   ├── http/              # HTTP utilities
│   ├── logger/            # Logging system
│   └── utils/             # Utility functions
├── idl/                   # Interface definition language
│   └── golem/             # Golem gRPC protocol definitions
├── conf/                  # Configuration files
├── docs/                  # Documentation
├── scripts/               # Build scripts
└── golem-worker/          # Golem workspace directory
```

## 🚀 Quick Start

### Prerequisites

- **Go 1.25.0** or higher
- **Git** for version control
- **Make** for build automation
- **SQLite** (optional, for local storage)

### Installation

```bash
# Clone the repository
git clone https://github.com/kiosk404/echoryn.git
cd echoryn

# Install dependencies and build
make all
```

This will:
1. Run `go mod tidy` to manage dependencies
2. Format the code
3. Run lint checks
4. Build all binaries

### Running Echoryn

#### 1. Start Hivemind Server

```bash
# Using make (development)
make run.hivemind

# Or directly with configuration
./output/platforms/linux/amd64/hivemind --config conf/hivemind-server.json
```

#### 2. Start Golem Worker

```bash
# In a separate terminal
make run.golem

# Or directly with configuration
./output/platforms/linux/amd64/golem --config conf/golem-worker.json
```

#### 3. Use Echoctl for Management

```bash
# List available tokens
./output/platforms/linux/amd64/echoctl token list

# Create a new token
./output/platforms/linux/amd64/echoctl token create --name "admin"

# Get system information
./output/platforms/linux/amd64/echoctl info
```

## 📦 Build Options

Echoryn uses a comprehensive Makefile for build automation:

```bash
# Build all binaries
make build

# Build specific binaries
make build BINS="hivemind echoctl"

# Run tests
make test

# Run tests with coverage
make cover

# Format code
make format

# Run linter
make lint

# Generate Protobuf code
make proto

# Clean build output
make clean

# Show help
make help
```

## ⚙️ Configuration

### Hivemind Configuration (`conf/hivemind-server.json`)

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

### Golem Configuration (`conf/golem-worker.json`)

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

### Environment Variables

```bash
# LLM API Keys
export DEEPSEEK_API_KEY="your-api-key"
export OPENAI_API_KEY="your-api-key"
export ANTHROPIC_API_KEY="your-api-key"

# Golem Configuration
export GOLEM_TOKEN="your-golem-token"

# Logging
export LOG_LEVEL="info"
```

## 🔌 Plugin System

Echoryn features a powerful plugin system inspired by Kubernetes scheduler framework:

### Plugin Types

- **Tools**: Extend agent capabilities with new functions
- **Hooks**: Intercept and modify system behavior at key points
- **Services**: Long-running background services
- **CLI Commands**: Add new commands to echoctl
- **Prompt Sections**: Extend system prompts with dynamic content
- **Runtime APIs**: Provide APIs for agent runtime

### Creating a Plugin

1. **Implement the Plugin Interface**:
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

2. **Register Your Plugin**:
   ```go
   func init() {
       plugin.Register(&MyPlugin{})
   }
   ```

3. **Configure in Hivemind**:
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

See the [Plugin System Specification](docs/ECHORYN_HIVEMIND_MEMORY.md) for detailed information.

## 🛠️ Development

### Setting Up Development Environment

```bash
# Clone the repository
git clone https://github.com/kiosk404/echoryn.git
cd echoryn

# Install Go dependencies
go mod download

# Install development tools
make tools.install

# Build and run tests
make all
```

### Project Structure

- **`cmd/`**: Main application entry points
- **`internal/`**: Private application code
- **`pkg/`**: Public libraries
- **`idl/`**: Protocol definitions (gRPC)
- **`conf/`**: Configuration examples
- **`docs/`**: Documentation
- **`scripts/`**: Build and development scripts

### Code Style

- Use `gofmt`, `goimports`, and `golines` for formatting
- Follow standard Go conventions
- Write comprehensive tests
- Document public APIs

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make cover

# Run specific test
go test ./internal/hivemind/...
```

## 📚 Documentation

### Language Versions
- [English Version](../README.md) - English documentation (main)
- [中文版本](docs/README_ZH.md) - Chinese documentation

### Core Specifications
- [Plugin System Specification](docs/ECHORYN_HIVEMIND_MEMORY.md) - Detailed plugin architecture
- [Scheduler Specification](internal/hivemind/service/golem/scheduler/SCHED_SPEC.md) - Task scheduling engine design
- Configuration files in `conf/` - Example configurations

### Module Specifications (in `.docs/` directory)
- `ECHORYN_HIVEMIND_AGENTS.md` - Agents runtime engine
- `ECHORYN_HIVEMIND_LLM.md` - LLM multi-model management
- `ECHORYN_HIVEMIND_MEMORY.md` - Memory system
- `ECHORYN_HIVEMIND_PLUGIN.md` - Plugin framework
- `ECHORYN_HIVEMIND_MCP.md` - MCP tool calling

### Code Documentation
- Comprehensive API documentation in code comments
- GoDoc generated documentation (coming soon)

## 🤝 Contributing

Contributions are welcome! Here's how you can help:

1. **Report Issues**: Use the GitHub issue tracker to report bugs or request features
2. **Submit Pull Requests**: Fork the repository and submit PRs with improvements
3. **Improve Documentation**: Help enhance documentation and examples
4. **Share Ideas**: Discuss potential improvements in issues

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- **Openclaw**: Inspiration and architectural patterns
- **CloudWeGo Eino**: Advanced LLM framework
- **Kubernetes**: Plugin system design inspiration
- **Model Context Protocol (MCP)**: Standardized tool integration

## 🔗 Related Projects

- [Openclaw](https://github.com/openclaw/openclaw) - Original inspiration
- [CloudWeGo Eino](https://github.com/cloudwego/eino) - LLM framework
- [Model Context Protocol](https://spec.modelcontextprotocol.org/) - Standard for AI tools

---

<div align="center">
  <p>Made with ❤️ by the Echoryn contributors</p>
  <p>Bringing AI virtual characters into our world, one container at a time</p>
</div>