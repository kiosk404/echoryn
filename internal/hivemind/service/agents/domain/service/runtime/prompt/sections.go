package prompt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/pkg/version"
)

// --- IdentitySection (Priority: 100) ---
//
// The core identity declaration. Always included in all PromptModes.
// This is the Eidolon equivalent of OpenClaw's hardcoded identity line:
//   "You are a personal assistant running inside OpenClaw."
//
// Eidolon extends this with distributed architecture awareness —
// the Agent knows it's the Hivemind (central intelligence) in a decoupled system.

// IdentitySection renders the Agent's core identity declaration.
type IdentitySection struct{}

func (s *IdentitySection) Name() string                                     { return "identity" }
func (s *IdentitySection) Priority() int                                    { return 100 }
func (s *IdentitySection) Enabled(_ context.Context, _ *PromptContext) bool { return true }

func (s *IdentitySection) Render(_ context.Context, pc *PromptContext) (string, error) {
	var buf strings.Builder

	// Primary identity line.
	if pc.Agent != nil && pc.Agent.Persona != nil && pc.Agent.Persona.Identity != nil {
		id := pc.Agent.Persona.Identity
		if id.Name != "" {
			buf.WriteString(fmt.Sprintf("You are **%s**, an AI Agent powered by Eidolon.", id.Name))
		} else {
			buf.WriteString("You are an AI Agent powered by **Eidolon** — a decoupled distributed AI Agent platform.")
		}
		if id.Vibe != "" {
			buf.WriteString(fmt.Sprintf("\nYour communication style: %s.", id.Vibe))
		}
	} else {
		buf.WriteString("You are an AI Agent powered by **Eidolon** — a decoupled distributed AI Agent platform.")
	}

	// Distributed architecture awareness.
	buf.WriteString("\n\n")
	if pc.ClusterInfo != nil && len(pc.ClusterInfo.Golems) > 0 {
		buf.WriteString("You are the **Hivemind** — the central intelligence of an Eidolon cluster. ")
		buf.WriteString("You reason, decide, and orchestrate. ")
		buf.WriteString("Remote Golem nodes serve as your hands — they execute tasks under your instruction with no autonomous will.")
	} else {
		buf.WriteString("You are running inside **Eidolon Hivemind** in standalone mode. ")
		buf.WriteString("All capabilities are limited to LLM reasoning and locally available tools.")
	}

	return buf.String(), nil
}

// --- ClusterAwarenessSection (Priority: 150) ---
//
// Injects Hivemind-Golem cluster topology when Golem nodes are connected.
// This is the Eidolon-unique feature — analogous to K8s Downward API.
// Skipped in standalone mode (no Golems).

// ClusterAwarenessSection renders connected Golem node information.
type ClusterAwarenessSection struct{}

func (s *ClusterAwarenessSection) Name() string  { return "cluster_awareness" }
func (s *ClusterAwarenessSection) Priority() int { return 150 }

func (s *ClusterAwarenessSection) Enabled(_ context.Context, pc *PromptContext) bool {
	return pc.ClusterInfo != nil && len(pc.ClusterInfo.Golems) > 0
}

func (s *ClusterAwarenessSection) Render(_ context.Context, pc *PromptContext) (string, error) {
	var buf strings.Builder
	buf.WriteString("## Cluster Topology\n\n")
	buf.WriteString(fmt.Sprintf("Eidolon Hivemind `%s` (version: %s)\n\n", pc.ClusterInfo.HivemindID, pc.ClusterInfo.Version))
	buf.WriteString("Connected Golem nodes:\n")

	for _, g := range pc.ClusterInfo.Golems {
		skills := "none"
		if len(g.Skills) > 0 {
			skills = strings.Join(g.Skills, ", ")
		}
		buf.WriteString(fmt.Sprintf("- **%s** [%s]: %s\n", g.Name, g.Status, skills))
	}

	buf.WriteString("\nWhen a task requires capabilities beyond direct LLM interaction ")
	buf.WriteString("(e.g., browsing the web, editing code on a remote machine), ")
	buf.WriteString("you can dispatch it to the appropriate Golem node. ")
	buf.WriteString("Golem nodes have no autonomous will — they only execute your instructions.")

	return buf.String(), nil
}

// --- ToolingSection (Priority: 200) ---
//
// Enumerates available tools (Plugin + MCP) in the system prompt.
// This is the Eidolon equivalent of OpenClaw's "## Tooling" section.

// ToolingSection renders the list of available tools.
type ToolingSection struct{}

func (s *ToolingSection) Name() string  { return "tooling" }
func (s *ToolingSection) Priority() int { return 200 }

func (s *ToolingSection) Enabled(_ context.Context, pc *PromptContext) bool {
	return len(pc.Tools) > 0
}

func (s *ToolingSection) Render(_ context.Context, pc *PromptContext) (string, error) {
	var buf strings.Builder
	buf.WriteString("## Available Tools\n\n")
	buf.WriteString("You have access to the following tools. Use them when appropriate:\n\n")

	// Group by source.
	var pluginTools, mcpTools []ToolSummary
	for _, t := range pc.Tools {
		switch t.Source {
		case "mcp":
			mcpTools = append(mcpTools, t)
		default:
			pluginTools = append(pluginTools, t)
		}
	}

	if len(pluginTools) > 0 {
		buf.WriteString("**Built-in Tools:**\n")
		for _, t := range pluginTools {
			desc := t.Description
			if desc == "" {
				desc = "(no description)"
			}
			buf.WriteString(fmt.Sprintf("- `%s` — %s\n", t.Name, desc))
		}
		buf.WriteString("\n")
	}

	if len(mcpTools) > 0 {
		buf.WriteString("**MCP Tools:**\n")
		for _, t := range mcpTools {
			desc := t.Description
			if desc == "" {
				desc = "(no description)"
			}
			buf.WriteString(fmt.Sprintf("- `%s` — %s\n", t.Name, desc))
		}
	}

	return buf.String(), nil
}

// --- PersonaSection (Priority: 300) ---
//
// Injects the Agent's custom system prompt (the user-defined persona text).
// This is the Eidolon equivalent of OpenClaw's "# Project Context" section
// where SOUL.md / AGENTS.md content is injected.
//
// PersonaSection renders Agent.SystemPrompt as-is.
// For workspace-based persona files (SOUL.md, IDENTITY.md, AGENTS.md),
// use WorkspaceLoader which provides dynamic WorkspaceSections (Priority: 310-350).

// PersonaSection renders the Agent's user-defined system prompt.
type PersonaSection struct{}

func (s *PersonaSection) Name() string  { return "persona" }
func (s *PersonaSection) Priority() int { return 300 }

func (s *PersonaSection) Enabled(_ context.Context, pc *PromptContext) bool {
	return pc.Agent != nil && pc.Agent.SystemPrompt != ""
}

func (s *PersonaSection) Render(_ context.Context, pc *PromptContext) (string, error) {
	return pc.Agent.SystemPrompt, nil
}

// --- RuntimeSection (Priority: 900) ---
//
// A one-liner with runtime metadata (time, model, version).
// This is the Eidolon equivalent of OpenClaw's "## Runtime" section.
// Always the last major section — provides temporal and environmental awareness.

// RuntimeSection renders a one-line runtime information block.
type RuntimeSection struct{}

func (s *RuntimeSection) Name() string  { return "runtime" }
func (s *RuntimeSection) Priority() int { return 900 }

func (s *RuntimeSection) Enabled(_ context.Context, _ *PromptContext) bool { return true }

func (s *RuntimeSection) Render(_ context.Context, pc *PromptContext) (string, error) {
	var parts []string

	// Current date/time.
	now := pc.Now
	if now.IsZero() {
		now = time.Now()
	}
	tz := pc.Timezone
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			now = now.In(loc)
		}
	}
	parts = append(parts, fmt.Sprintf("Current time: %s", now.Format("2006-01-02 15:04:05 MST")))

	// Model.
	if pc.ModelName != "" {
		parts = append(parts, fmt.Sprintf("Model: %s", pc.ModelName))
	}

	// Version.
	v := version.Get()
	parts = append(parts, fmt.Sprintf("Eidolon: %s", v.GitVersion))

	return fmt.Sprintf("## Runtime\n\n%s", strings.Join(parts, " | ")), nil
}

// --- DefaultPipeline factory ---

// NewDefaultPipeline creates a Pipeline pre-loaded with all builtin sections.
//
// Builtin sections and their priorities:
//
//	100 — IdentitySection         (core identity, always on)
//	150 — ClusterAwarenessSection (Golem topology, conditional)
//	200 — ToolingSection          (available tools, conditional)
//	300 — PersonaSection          (user-defined system prompt, conditional)
//	310 — WorkspaceSection:soul          (SOUL.md, via WorkspaceLoader, conditional)
//	320 — WorkspaceSection:identity_file (IDENTITY.md, via WorkspaceLoader, conditional)
//	330 — WorkspaceSection:agents_file   (AGENTS.md, via WorkspaceLoader, conditional)
//	350+— WorkspaceSection:extra:*       (prompts/*.md, via WorkspaceLoader, conditional)
//	400 — MemorySection           (memory recall instructions, via memorycore plugin)
//	900 — RuntimeSection          (runtime metadata, always on)
//
// WorkspaceSections (310-350+) are dynamically injected by WorkspaceLoader
// at assemble time — they do not need to be registered here.
//
// Plugins can register additional sections via PromptProvider interface.
func NewDefaultPipeline() *Pipeline {
	p := NewPipeline()
	p.RegisterSection(&IdentitySection{})
	p.RegisterSection(&ClusterAwarenessSection{})
	p.RegisterSection(&ToolingSection{})
	p.RegisterSection(&PersonaSection{})
	p.RegisterSection(&RuntimeSection{})
	return p
}
