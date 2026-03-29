// Package middleware provides the Skills middleware for integrating the skills
// system into Eino-based agents. It handles system prompt injection, auto-detection
// of relevant skills, and exposes skill-related tools.
package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/pkg/skills"
	skilltools "github.com/kiosk404/echoryn/pkg/skills/tools"
)

// SkillsMiddleware injects skills metadata into agent prompts
// and provides skill-related Eino tools.
type SkillsMiddleware struct {
	registry *skills.Registry
	tools    []tool.BaseTool
}

// New creates a new skills middleware.
func New(registry *skills.Registry) *SkillsMiddleware {
	return &SkillsMiddleware{
		registry: registry,
		tools:    skilltools.NewSkillTools(registry),
	}
}

// InjectPrompt appends skills information to the system prompt.
func (m *SkillsMiddleware) InjectPrompt(basePrompt string) string {
	skillsSection := m.registry.GenerateSystemPromptSection()
	instructions := m.registry.GenerateSkillsInstructions()

	if skillsSection == "" {
		return basePrompt
	}

	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")
	sb.WriteString(skillsSection)
	sb.WriteString("\n")
	sb.WriteString(instructions)

	return sb.String()
}

// GetTools returns skill-related tools to add to the agent.
func (m *SkillsMiddleware) GetTools() []tool.BaseTool {
	return m.tools
}

// ProcessMessages can modify messages before they reach the model.
// It auto-detects when to suggest relevant skills based on user input.
func (m *SkillsMiddleware) ProcessMessages(_ context.Context, messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return messages
	}

	lastMsg := messages[len(messages)-1]
	if lastMsg.Role != schema.User {
		return messages
	}

	match := m.registry.FindMatchingSkill(lastMsg.Content)
	if match == nil {
		return messages
	}

	hint := &schema.Message{
		Role:    schema.User,
		Content: fmt.Sprintf("[Hint: The '%s' skill may be relevant for this task. Consider loading %s/SKILL.md for specialized instructions.]", match.Name, match.Path),
	}

	result := make([]*schema.Message, 0, len(messages)+1)
	result = append(result, messages[:len(messages)-1]...)
	result = append(result, hint, lastMsg)
	return result
}

// --- convenience constructors ---

// Config holds configuration for the skills middleware.
type Config struct {
	// GlobalSkillsDir is the global skills directory.
	// Default: paths.ResolveGolemSkillsDir() (~/.echoryn/golem/skills)
	GlobalSkillsDir string

	// ProjectSkillsDir is the project-level skills directory.
	// Default: .echoryn/skills
	ProjectSkillsDir string

	// AutoWatch enables automatic file watching (hot-reload).
	AutoWatch bool
}

// DefaultConfig returns the default skills middleware configuration.
func DefaultConfig() *Config {
	return &Config{
		// empty means the Loader will use paths.ResolveGolemSkillsDir() as default
		GlobalSkillsDir:  "",
		ProjectSkillsDir: ".echoryn/skills",
		AutoWatch:        true,
	}
}

// Create creates a fully configured skills middleware.
func Create(ctx context.Context, config *Config) (*SkillsMiddleware, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var loaderOpts []skills.LoaderOption
	if config.GlobalSkillsDir != "" {
		loaderOpts = append(loaderOpts, skills.WithGlobalSkillsDir(config.GlobalSkillsDir))
	}
	if config.ProjectSkillsDir != "" {
		loaderOpts = append(loaderOpts, skills.WithProjectSkillsDir(config.ProjectSkillsDir))
	}

	loader := skills.NewLoader(loaderOpts...)

	var regOpts []skills.RegistryOption
	if config.AutoWatch {
		regOpts = append(regOpts, skills.WithAutoWatch(true))
	}

	registry := skills.NewRegistry(loader, regOpts...)
	if err := registry.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skills registry: %w", err)
	}

	return New(registry), nil
}
