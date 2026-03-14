package render

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ToolStatus represents the current state of a tool invocation.
type ToolStatus int

const (
	ToolRunning ToolStatus = iota
	ToolSuccess
	ToolFailed
)

// toolIcons maps tool names to their custom icons.
// Based on actual built-in plugins defined in internal/hivemind/service/plugin/builtin/.
// Tools not in this map will use the default "⏳" icon.
var toolIcons = map[string]string{
	// === memory-core plugin ===
	"memory_search": "🔍",
	"memory_read":   "📖",
	"memory_write":  "📝",
	"memory_delete": "🗑️",

	// === web-search plugin ===
	"web_search": "🌐",

	// === llm-task plugin ===
	"llm_task": "🧠",

	// === diagnostics plugin ===
	"diagnostics_status": "📊",

	// === subagent plugin ===
	"sessions_spawn":  "🚀",
	"sessions_status": "📋",

	// === skills plugin ===
	"list_skills": "📚",
	"view_skill":  "👁️",

	// === team plugin ===
	"team_create":         "👥",
	"team_list_templates": "📋",

	// === golem-cluster plugin ===
	"cluster_list_nodes":    "🖥️",
	"cluster_get_node":      "🔍",
	"cluster_dispatch_task": "📤",
	"cluster_execute_skill": "⚡",
}

// getToolIcon returns the custom icon for a tool, or the default if not found.
func getToolIcon(name string) string {
	// Normalize: try exact match first
	if icon, ok := toolIcons[name]; ok {
		return icon
	}
	// Try lowercase match
	if icon, ok := toolIcons[strings.ToLower(name)]; ok {
		return icon
	}
	// Default icon
	return "⏳"
}

// toolCallEntry is an individual tool invocation record.
type toolCallEntry struct {
	Name      string
	Args      string // summary of arguments (file path, etc.)
	Status    ToolStatus
	StartTime time.Time
	Duration  time.Duration
}

// ToolCallPanel manages the display of tool invocations during a
// streaming response. It renders Gemini-CLI style bordered panels:
//
//	┌──────────────────────────────────────────────────────┐
//	│ ✓  WriteFile Writing to hello.py                     │
//	│                                                      │
//	│  1 print("Hello, world!")                            │
//	└──────────────────────────────────────────────────────┘
type ToolCallPanel struct {
	output *termenv.Output
	width  int

	mu    sync.Mutex
	calls []toolCallEntry
	lines int // number of lines currently rendered
}

// NewToolCallPanel creates a new panel with default output.
func NewToolCallPanel() *ToolCallPanel {
	return &ToolCallPanel{
		output: termenv.NewOutput(os.Stdout),
		width:  80,
	}
}

// SetWidth updates the panel width.
func (p *ToolCallPanel) SetWidth(w int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.width = w
}

// Start records the beginning of a tool call and renders it.
func (p *ToolCallPanel) Start(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, toolCallEntry{
		Name:      name,
		Status:    ToolRunning,
		StartTime: time.Now(),
	})

	p.renderToolStart(name)
}

// StartWithArgs records a tool call with argument summary.
func (p *ToolCallPanel) StartWithArgs(name, args string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, toolCallEntry{
		Name:      name,
		Args:      args,
		Status:    ToolRunning,
		StartTime: time.Now(),
	})

	p.renderToolStart(name + " " + args)
}

// renderToolStart displays the tool call header with a custom icon.
func (p *ToolCallPanel) renderToolStart(display string) {
	// Extract tool name from display (first word before space)
	toolName := display
	if idx := strings.Index(display, " "); idx > 0 {
		toolName = display[:idx]
	}

	icon := lipgloss.NewStyle().
		Foreground(warningColor).
		Render(getToolIcon(toolName))

	label := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Render(display)

	fmt.Fprintf(os.Stdout, "\n  %s %s\n", icon, label)
	p.lines += 2
}

// Finish marks a tool call as completed (success or failure) and
// re-renders the line.
func (p *ToolCallPanel) Finish(name string, success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find the most recent running call with this name.
	for i := len(p.calls) - 1; i >= 0; i-- {
		if p.calls[i].Name == name && p.calls[i].Status == ToolRunning {
			p.calls[i].Duration = time.Since(p.calls[i].StartTime)
			if success {
				p.calls[i].Status = ToolSuccess
			} else {
				p.calls[i].Status = ToolFailed
			}
			break
		}
	}
}

// RenderToolResult renders a tool result in a bordered box (Gemini style).
func RenderToolResult(toolName, description, content string, width int, success bool) string {
	if width <= 10 {
		width = 80
	}

	innerWidth := width - 6 // border + padding

	// --- Header line ---
	var icon string
	if success {
		icon = lipgloss.NewStyle().
			Bold(true).
			Foreground(successColor).
			Render("✓")
	} else {
		icon = lipgloss.NewStyle().
			Bold(true).
			Foreground(errorColor).
			Render("✗")
	}

	toolLabel := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Render(toolName)

	desc := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Render(description)

	header := fmt.Sprintf("%s  %s %s", icon, toolLabel, desc)

	// --- Content with line numbers ---
	var body string
	if content != "" {
		lines := strings.Split(content, "\n")
		var numberedLines []string

		for i, line := range lines {
			lineNum := lipgloss.NewStyle().
				Foreground(subtleColor).
				Width(3).
				Align(lipgloss.Right).
				Render(fmt.Sprintf("%d", i+1))

			codeLine := lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Render(" " + line)

			numberedLines = append(numberedLines, lineNum+codeLine)
		}

		body = "\n" + strings.Join(numberedLines, "\n")
	}

	// --- Box ---
	boxContent := header + body

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(dimColor).
		Padding(0, 1).
		Width(innerWidth).
		Render(boxContent)
}

// RenderToolCallCompact renders a compact tool call line with custom icons.
func RenderToolCallCompact(name string, status ToolStatus, duration time.Duration) string {
	var icon string
	switch status {
	case ToolRunning:
		// Use custom icon for the tool when running
		icon = lipgloss.NewStyle().Foreground(warningColor).Render(getToolIcon(name))
	case ToolSuccess:
		icon = lipgloss.NewStyle().Bold(true).Foreground(successColor).Render("✓")
	case ToolFailed:
		icon = lipgloss.NewStyle().Bold(true).Foreground(errorColor).Render("✗")
	}

	toolLabel := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Render(name)

	var statusText string
	switch status {
	case ToolRunning:
		statusText = lipgloss.NewStyle().
			Foreground(subtleColor).
			Render("running...")
	case ToolSuccess:
		statusText = lipgloss.NewStyle().
			Foreground(successColor).
			Render(fmt.Sprintf("%.1fs", duration.Seconds()))
	case ToolFailed:
		statusText = lipgloss.NewStyle().
			Foreground(errorColor).
			Render("failed")
	}

	return fmt.Sprintf("  %s  %s  %s", icon, toolLabel, statusText)
}

// LineCount returns the number of terminal lines this panel has output.
func (p *ToolCallPanel) LineCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lines
}

// Reset clears all tracked tool calls.
func (p *ToolCallPanel) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = p.calls[:0]
	p.lines = 0
}
