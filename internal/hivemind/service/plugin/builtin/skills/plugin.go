// Package skills provides the skills built-in plugin for Hivemind.
//
// This plugin bridges the pkg/skills system to the Agent runtime, enabling:
//   - Skills discovery and loading from .echoryn/golem/skills/ directories
//   - System prompt injection of <available_skills> and <skills_instructions>
//   - list_skills / view_skill tools for agents to query skills on demand
//   - Hot-reload via fsnotify when SKILL.md files change
//
// Without this plugin, the Agent has no awareness of file-system skills
// and cannot leverage specialized domain knowledge or workflows.
package skills

import (
	"context"
	"fmt"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	pkgskills "github.com/kiosk404/echoryn/pkg/skills"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "skills"

	// Kind marks this as a "general" plugin (no slot exclusion).
	Kind = "general"
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Skills",
		Kind:        Kind,
		Description: "Bridges file-system skills (SKILL.md) to Agent runtime, providing skills discovery, prompt injection, and on-demand loading tools",
	}
}

// Config holds the configuration for the skills plugin.
type Config struct {
	Enabled bool `json:"enabled"`

	// GlobalSkillsDir overrides the global skills directory.
	// Default: paths.ResolveGolemSkillsDir() (~/.echoryn/golem/skills)
	GlobalSkillsDir string `json:"global_skills_dir,omitempty"`

	// ProjectSkillsDir overrides the project-level skills directory.
	// Default: .echoryn/skills
	ProjectSkillsDir string `json:"project_skills_dir,omitempty"`

	// HotReload enables automatic file watching (fsnotify).
	// Default: true
	HotReload bool `json:"hot_reload"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:   true,
		HotReload: true,
	}
}

// skillsPlugin is the runtime instance of the skills plugin.
type skillsPlugin struct {
	cfg      *Config
	registry *pkgskills.Registry
}

// Factory is the PluginFactory for skills.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfg := DefaultConfig()
	if c, ok := args["config"].(*Config); ok && c != nil {
		cfg = c
	}

	if !cfg.Enabled {
		logger.Info("[skills] plugin disabled via config")
		return &skillsPlugin{cfg: cfg}, nil
	}

	// Create Loader with configured directories.
	var loaderOpts []pkgskills.LoaderOption
	if cfg.GlobalSkillsDir != "" {
		loaderOpts = append(loaderOpts, pkgskills.WithGlobalSkillsDir(cfg.GlobalSkillsDir))
	}
	if cfg.ProjectSkillsDir != "" {
		loaderOpts = append(loaderOpts, pkgskills.WithProjectSkillsDir(cfg.ProjectSkillsDir))
	} else {
		// Default project skills dir: .echoryn/golem/skills (relative to CWD).
		// This is where skills are stored in the project repository, matching
		// the golem's skills directory layout.
		loaderOpts = append(loaderOpts, pkgskills.WithProjectSkillsDir(".echoryn/golem/skills"))
	}
	loader := pkgskills.NewLoader(loaderOpts...)

	// Create Registry with optional hot-reload.
	var regOpts []pkgskills.RegistryOption
	if cfg.HotReload {
		regOpts = append(regOpts, pkgskills.WithAutoWatch(true))
	}
	registry := pkgskills.NewRegistry(loader, regOpts...)

	// Initialize: scan directories, load metadata.
	if err := registry.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize skills registry: %w", err)
	}

	logger.Info("[skills] initialized with %d skill(s) (global=%s, project=%s)",
		registry.Count(), loader.GlobalDir(), loader.ProjectDir())

	return &skillsPlugin{
		cfg:      cfg,
		registry: registry,
	}, nil
}

func (p *skillsPlugin) Name() string { return PluginName }

// --- SkillsEnricher interface (registry.SkillsEnricher) ---

// Compile-time check that skillsPlugin implements SkillsEnricher.
var _ registry.SkillsEnricher = (*skillsPlugin)(nil)

// GetGlobalSkills returns all known skills as proto InstalledSkill messages.
// This is used by the Golem Registry to enrich node registrations with
// Hivemind-side skills when Golem nodes don't report their own.
func (p *skillsPlugin) GetGlobalSkills() []*pb.InstalledSkill {
	if p.registry == nil {
		return nil
	}

	metadata := p.registry.GetMetadata()
	if len(metadata) == 0 {
		return nil
	}

	skills := make([]*pb.InstalledSkill, len(metadata))
	for i, m := range metadata {
		skills[i] = &pb.InstalledSkill{
			Name:         m.Name,
			Description:  m.Description,
			Capabilities: m.Capabilities,
			Path:         m.Path,
		}
	}
	return skills
}

// --- InitPlugin interface ---

// Init registers list_skills and view_skill tools via the PluginAPI.
func (p *skillsPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled || p.registry == nil {
		return nil
	}

	api.RegisterTool(plugin.ToolDefinition{
		Name: "list_skills",
		Description: "List all available skills with their descriptions. Skills provide specialized workflows and domain knowledge. " +
			"Use this tool to discover what capabilities are available, find skills relevant to a specific domain or task, " +
			"or check if a skill exists before trying to use it.",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "filter",
				Type:        "string",
				Description: "Optional: filter skills by keyword in name or description.",
				Required:    false,
			},
			{
				Name:        "source",
				Type:        "string",
				Description: "Optional: filter by source - 'global' or 'project'.",
				Required:    false,
			},
		},
		Handler: p.handleListSkills,
	})

	api.RegisterTool(plugin.ToolDefinition{
		Name: "view_skill",
		Description: "View the full content of a skill's instructions. Use this tool when a task matches an available skill, " +
			"when you need detailed instructions for a specific workflow, or when the skill description indicates it's relevant to the current task.",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "name",
				Type:        "string",
				Description: "The name of the skill to view (must match a name from <available_skills>).",
				Required:    true,
			},
			{
				Name:        "section",
				Type:        "string",
				Description: "Optional: extract only a specific section by heading (e.g., 'Instructions', 'Examples').",
				Required:    false,
			},
			{
				Name:        "toc",
				Type:        "string",
				Description: "Optional: when 'true', returns only the table of contents (all headings with indentation).",
				Required:    false,
			},
		},
		Handler: p.handleViewSkill,
	})

	logger.Info("[skills] registered list_skills and view_skill tools")
	return nil
}

// --- PromptProvider interface ---

// PromptSections returns a SkillsSection that injects <available_skills>
// and <skills_instructions> into the system prompt.
func (p *skillsPlugin) PromptSections() []prompt.PromptSection {
	if !p.cfg.Enabled || p.registry == nil {
		return nil
	}
	return []prompt.PromptSection{
		&skillsPromptSection{plugin: p},
	}
}

// --- LifecyclePlugin interface ---

// Start is a no-op (the watcher is started during Factory via middleware.Create).
func (p *skillsPlugin) Start(_ context.Context) error {
	return nil
}

// Stop shuts down the file watcher if active.
func (p *skillsPlugin) Stop(_ context.Context) error {
	if p.registry != nil {
		return p.registry.StopWatching()
	}
	return nil
}

// --- Tool Handlers ---

func (p *skillsPlugin) handleListSkills(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.registry == nil {
		return "No skills available (registry not initialized).", nil
	}

	metadata := p.registry.GetMetadata()
	if len(metadata) == 0 {
		return "No skills available.", nil
	}

	// Apply filters.
	filterStr, _ := params["filter"].(string)
	sourceStr, _ := params["source"].(string)

	filtered := make([]pkgskills.SkillMetadata, 0, len(metadata))
	for _, m := range metadata {
		if sourceStr != "" && string(m.Source) != sourceStr {
			continue
		}
		if filterStr != "" {
			filter := strings.ToLower(filterStr)
			name := strings.ToLower(m.Name)
			desc := strings.ToLower(m.Description)
			if !strings.Contains(name, filter) && !strings.Contains(desc, filter) {
				continue
			}
		}
		filtered = append(filtered, m)
	}

	if len(filtered) == 0 {
		if filterStr != "" || sourceStr != "" {
			return "No skills match the specified filters.", nil
		}
		return "No skills available.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d skill(s):\n\n", len(filtered)))
	for _, m := range filtered {
		sb.WriteString(fmt.Sprintf("## %s\n", m.Name))
		sb.WriteString(fmt.Sprintf("- **Source**: %s\n", m.Source))
		sb.WriteString(fmt.Sprintf("- **Location**: %s/SKILL.md\n", m.Path))
		sb.WriteString(fmt.Sprintf("- **Description**: %s\n\n", m.Description))
	}

	return sb.String(), nil
}

func (p *skillsPlugin) handleViewSkill(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.registry == nil {
		return nil, fmt.Errorf("skills registry not initialized")
	}

	name, _ := params["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("skill name is required")
	}

	sectionParam, _ := params["section"].(string)
	tocParam, _ := params["toc"].(string)
	isTOC := strings.EqualFold(tocParam, "true")

	if isTOC && sectionParam != "" {
		return nil, fmt.Errorf("cannot specify both 'toc' and 'section' parameters")
	}

	content, err := p.registry.GetContent(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to load skill '%s': %w", name, err)
	}

	parser := pkgskills.NewParser()

	if isTOC {
		toc := parser.ExtractTOC(content)
		if toc == "" {
			return "No headings found in this skill.", nil
		}
		return toc, nil
	}

	if sectionParam != "" {
		sectionContent := parser.ExtractSection(content, sectionParam)
		if sectionContent == "" {
			return nil, fmt.Errorf("section '%s' not found in skill '%s'", sectionParam, name)
		}
		return sectionContent, nil
	}

	return content, nil
}

// --- SkillsPromptSection ---
//
// Injects <available_skills> and <skills_instructions> XML into the system prompt.
// Priority 250 places it between ToolingSection (200) and PersonaSection (300).

type skillsPromptSection struct {
	plugin *skillsPlugin
}

func (s *skillsPromptSection) Name() string  { return "skills" }
func (s *skillsPromptSection) Priority() int { return 250 }

func (s *skillsPromptSection) Enabled(_ context.Context, _ *prompt.PromptContext) bool {
	return s.plugin.registry != nil && s.plugin.registry.Count() > 0
}

func (s *skillsPromptSection) Render(_ context.Context, _ *prompt.PromptContext) (string, error) {
	reg := s.plugin.registry

	var sb strings.Builder
	sb.WriteString("## Skills\n\n")
	sb.WriteString("You have access to specialized skills that provide domain-specific knowledge and workflows.\n")
	sb.WriteString("When a task matches an available skill, use the `view_skill` tool to load its full instructions before proceeding.\n\n")
	sb.WriteString(reg.GenerateSystemPromptSection())
	sb.WriteString("\n")
	sb.WriteString(reg.GenerateSkillsInstructions())

	return sb.String(), nil
}
