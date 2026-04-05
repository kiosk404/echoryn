// Package skills provides the skills built-in plugin for Hivemind.
//
// This plugin bridges the pkg/skills system to the Agent runtime, enabling:
//   - Three-source skill loading: Project > Hivemind > Golem
//   - System prompt injection with clear semantic distinction between:
//   - Hivemind Skills (global decision-making abilities)
//   - Golem Skills (local execution abilities on specific nodes)
//   - list_skills / view_skill tools for agents to query skills on demand
//   - Hot-reload via fsnotify when SKILL.md files change
//
// Semantic design:
//   - Hivemind Skills provide decision knowledge and workflow guidance.
//     The Agent loads them via list_skills/view_skill, then follows
//     the instructions within — which may involve tool calls, Golem
//     orchestration, LLM reasoning, or any combination thereof.
//   - Golem Skills describe WHAT a specific node can execute.
//     They are reported by Golem nodes and used by the Scheduler for node selection.
//     Agent interacts with them via cluster_dispatch_task / cluster_execute_skill.
package skills

import (
	"context"
	"fmt"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
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
		Description: "Bridges file-system skills (SKILL.md) to Agent runtime with three-source loading (Hivemind + Golem + Project), prompt injection, and on-demand tools",
	}
}

// Config holds the configuration for the skills plugin.
type Config struct {
	Enabled bool `json:"enabled"`

	// GlobalSkillsDir overrides the Golem-level global skills directory.
	// Default: paths.ResolveGolemSkillsDir() (~/.echoryn/golem/skills)
	GlobalSkillsDir string `json:"global_skills_dir,omitempty"`

	// HivemindSkillsDir overrides the Hivemind-level decision skills directory.
	// Default: paths.ResolveHivemindSkillsDir() (~/.echoryn/skills)
	// Set to empty string to disable Hivemind skills loading.
	HivemindSkillsDir string `json:"hivemind_skills_dir,omitempty"`

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

	// Build Loader options from config.
	loaderOpts := buildLoaderOptions(cfg)
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

	logger.Info("[skills] initialized with %d skill(s) (golem=%s, hivemind=%s, project=%s)",
		registry.Count(), loader.GlobalDir(), loader.HivemindDir(), loader.ProjectDir())

	return &skillsPlugin{
		cfg:      cfg,
		registry: registry,
	}, nil
}

// buildLoaderOptions constructs LoaderOptions from the plugin Config.
func buildLoaderOptions(cfg *Config) []pkgskills.LoaderOption {
	var opts []pkgskills.LoaderOption

	if cfg.GlobalSkillsDir != "" {
		opts = append(opts, pkgskills.WithGlobalSkillsDir(cfg.GlobalSkillsDir))
	}
	if cfg.HivemindSkillsDir != "" {
		opts = append(opts, pkgskills.WithHivemindSkillsDir(cfg.HivemindSkillsDir))
	}
	if cfg.ProjectSkillsDir != "" {
		opts = append(opts, pkgskills.WithProjectSkillsDir(cfg.ProjectSkillsDir))
	} else {
		// Default project skills dir for Hivemind: .echoryn/golem/skills (relative to CWD).
		opts = append(opts, pkgskills.WithProjectSkillsDir(".echoryn/golem/skills"))
	}

	return opts
}

func (p *skillsPlugin) Name() string { return PluginName }

// --- InitPlugin interface ---

// Init registers list_skills and view_skill tools via the PluginAPI.
func (p *skillsPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled || p.registry == nil {
		return nil
	}

	api.RegisterTool(plugin.ToolDefinition{
		Name: "list_skills",
		Description: "List all available Hivemind Skills (decision knowledge and workflow guidance). " +
			"These skills describe system capabilities and provide specialized workflows. " +
			"Use this to discover what skills are available, then use view_skill to load detailed instructions.",
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
				Description: "Optional: filter by source — 'hivemind', 'global' (golem), or 'project'.",
				Required:    false,
			},
		},
		Handler: p.handleListSkills,
	})

	api.RegisterTool(plugin.ToolDefinition{
		Name: "view_skill",
		Description: "View the full content of a skill's instructions. Use this to load a skill's " +
			"detailed workflow, parameters, and step-by-step guidance before executing the task.",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "name",
				Type:        "string",
				Description: "The name of the skill to view.",
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
				Description: "Optional: when 'true', returns only the table of contents.",
				Required:    false,
			},
		},
		Handler: p.handleViewSkill,
	})

	logger.Info("[skills] registered list_skills and view_skill tools")
	return nil
}

// --- PromptProvider interface ---

// PromptSections returns a SkillsSection that injects Hivemind Skills metadata
// into the system prompt with clear semantic distinction from Golem Skills.
func (p *skillsPlugin) PromptSections() []prompt.PromptSection {
	if !p.cfg.Enabled || p.registry == nil {
		return nil
	}
	return []prompt.PromptSection{
		&hivemindSkillsPromptSection{plugin: p},
	}
}

// --- LifecyclePlugin interface ---

// Start is a no-op (the watcher is started during Factory).
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

func (p *skillsPlugin) handleListSkills(_ context.Context, params map[string]interface{}) (interface{}, error) {
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
		if sourceStr != "" && !matchSource(m.Source, sourceStr) {
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
		sb.WriteString(fmt.Sprintf("- **Source**: %s\n", sourceLabel(m.Source)))
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

// --- HivemindSkillsPromptSection ---
//
// Injects Hivemind Skills metadata into the system prompt with clear semantic
// distinction from Golem Skills. Priority 250 places it between
// ToolingSection (200) and PersonaSection (300).
//
// Key design: Hivemind Skills are "global decision-making abilities" that describe
// what the system CAN DO by orchestrating Golem nodes. They are NOT directly
// executable. The Agent uses them for task planning, then dispatches execution
// to Golem nodes via cluster_execute_skill.
//
// Golem Skills (local execution abilities) are rendered by ClusterAwarenessSection
// (Priority 150) from live NodeManager data — NOT from this section.

type hivemindSkillsPromptSection struct {
	plugin *skillsPlugin
}

func (s *hivemindSkillsPromptSection) Name() string  { return "hivemind_skills" }
func (s *hivemindSkillsPromptSection) Priority() int { return 250 }

func (s *hivemindSkillsPromptSection) Enabled(_ context.Context, _ *prompt.PromptContext) bool {
	return s.plugin.registry != nil && s.plugin.registry.Count() > 0
}

func (s *hivemindSkillsPromptSection) Render(_ context.Context, _ *prompt.PromptContext) (string, error) {
	reg := s.plugin.registry
	metadata := reg.GetMetadata()
	if len(metadata) == 0 {
		return "", nil
	}

	// Partition skills by source for semantic clarity.
	var hivemindSkills, otherSkills []pkgskills.SkillMetadata
	for _, m := range metadata {
		if m.Source == pkgskills.SourceHivemind {
			hivemindSkills = append(hivemindSkills, m)
		} else {
			otherSkills = append(otherSkills, m)
		}
	}

	var sb strings.Builder

	// --- Hivemind Skills: global decision-making abilities ---
	if len(hivemindSkills) > 0 {
		sb.WriteString("## Hivemind Skills (Decision Knowledge & Workflows)\n\n")
		sb.WriteString("These skills provide **specialized knowledge and workflow guidance** for accomplishing tasks.\n")
		sb.WriteString("Load a skill's full instructions before proceeding — the skill will guide you on what to do.\n\n")
		sb.WriteString("**How to use:**\n")
		sb.WriteString("1. Use `list_skills` to discover available Hivemind Skills\n")
		sb.WriteString("2. Use `view_skill` to load the skill's detailed instructions and workflow\n")
		sb.WriteString("3. Follow the loaded instructions to accomplish the task\n\n")
		sb.WriteString("<hivemind_skills>\n")
		for _, m := range hivemindSkills {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", m.Name, m.Description))
		}
		sb.WriteString("</hivemind_skills>\n")
	}

	// --- Other Skills (Project/Global/Builtin) ---
	if len(otherSkills) > 0 {
		if len(hivemindSkills) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Available Skills (Workflow & Domain Knowledge)\n\n")
		sb.WriteString("These skills provide specialized workflows and domain knowledge.\n")
		sb.WriteString("Use `view_skill` to load full instructions before proceeding with a matching task.\n\n")
		sb.WriteString("<available_skills>\n")
		for _, m := range otherSkills {
			sb.WriteString(fmt.Sprintf("- **%s** (source: %s): %s\n", m.Name, m.Source, m.Description))
		}
		sb.WriteString("</available_skills>\n")
	}

	// --- Skills instructions ---
	sb.WriteString("\n")
	sb.WriteString(reg.GenerateSkillsInstructions())

	return sb.String(), nil
}

// --- Helpers ---

// matchSource checks if a skill source matches the filter string.
// Handles the "global" ↔ "golem" aliasing for backward compatibility.
func matchSource(source pkgskills.SkillSource, filter string) bool {
	f := strings.ToLower(filter)
	s := string(source)
	if f == s {
		return true
	}
	// "golem" and "global" are aliases.
	if (f == "golem" && s == "global") || (f == "global" && s == "golem") {
		return true
	}
	return false
}

// sourceLabel returns a human-readable label for the source.
func sourceLabel(source pkgskills.SkillSource) string {
	switch source {
	case pkgskills.SourceHivemind:
		return "hivemind (global decision ability)"
	case pkgskills.SourceGlobal, pkgskills.SourceGolem:
		return "golem (local execution ability)"
	case pkgskills.SourceProject:
		return "project"
	case pkgskills.SourceBuiltin:
		return "builtin"
	default:
		return string(source)
	}
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*skillsPlugin)(nil)
	_ plugin.InitPlugin      = (*skillsPlugin)(nil)
	_ plugin.LifecyclePlugin = (*skillsPlugin)(nil)
)
