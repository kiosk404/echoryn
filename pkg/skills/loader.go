package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
)

const logModule = "skills"

// Loader handles discovering and loading skills from the filesystem.
//
// Three-source loading model (priority: Project > Hivemind > Golem):
//   - globalDir:   Golem-local execution skills   (~/.echoryn/golem/skills)
//   - hivemindDir: Hivemind global decision skills (~/.echoryn/skills)
//   - projectDir:  Project-level skills            (.echoryn/skills)
//
// When hivemindDir is empty, Hivemind source is skipped (for Golem-side use).
type Loader struct {
	globalDir   string // Golem execution skills
	hivemindDir string // Hivemind decision skills (empty = disabled)
	projectDir  string
	parser      *Parser
}

// LoaderOption configures the loader.
type LoaderOption func(*Loader)

// WithGlobalSkillsDir sets the global skills directory.
// Default: paths.ResolveGolemSkillsDir() (~/.echoryn/golem/skills)
func WithGlobalSkillsDir(dir string) LoaderOption {
	return func(loader *Loader) {
		loader.globalDir = expandPath(dir)
	}
}

// WithProjectSkillsDir sets the project-level skills directory.
// Default: .echoryn/skills
func WithProjectSkillsDir(dir string) LoaderOption {
	return func(loader *Loader) {
		loader.projectDir = dir
	}
}

// WithHivemindSkillsDir sets the Hivemind global decision skills directory.
// Default: paths.ResolveHivemindSkillsDir() (~/.echoryn/skills).
// Pass an empty string to disable Hivemind skill loading (for Golem-side use).
func WithHivemindSkillsDir(dir string) LoaderOption {
	return func(loader *Loader) {
		if dir == "" {
			loader.hivemindDir = ""
		} else {
			loader.hivemindDir = expandPath(dir)
		}
	}
}

// NewLoader creates a new skills loader with the given options.
//
// Default directories:
//   - globalDir:   paths.ResolveGolemSkillsDir()    (~/.echoryn/golem/skills)
//   - hivemindDir: paths.ResolveHivemindSkillsDir() (~/.echoryn/skills)
//   - projectDir:  .echoryn/skills
func NewLoader(opts ...LoaderOption) *Loader {
	l := &Loader{
		globalDir:   paths.ResolveGolemSkillsDir(),
		hivemindDir: paths.ResolveHivemindSkillsDir(),
		projectDir:  ".echoryn/skills",
		parser:      NewParser(),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// GlobalDir returns the Golem global skills directory path.
func (l *Loader) GlobalDir() string { return l.globalDir }

// HivemindDir returns the Hivemind decision skills directory path.
// Empty string means Hivemind source is disabled.
func (l *Loader) HivemindDir() string { return l.hivemindDir }

// ProjectDir returns the project-level skills directory path.
func (l *Loader) ProjectDir() string { return l.projectDir }

// LoadAll loads all skills from Golem, Hivemind, and Project directories.
// Priority (last wins): Golem(global) < Hivemind < Project.
// When hivemindDir is empty, the Hivemind source is skipped (Golem-side use).
func (l *Loader) LoadAll(ctx context.Context) ([]*Skill, error) {
	skills := make(map[string]*Skill)

	// 1. Load Golem (global) skills — lowest priority.
	globalSkills, err := l.loadFromDir(ctx, l.globalDir, SourceGlobal)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load golem skills: %w", err)
	}
	for _, s := range globalSkills {
		skills[s.Name] = s
	}

	// 2. Load Hivemind skills — overrides Golem if same name.
	if l.hivemindDir != "" {
		hivemindSkills, err := l.loadFromDir(ctx, l.hivemindDir, SourceHivemind)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load hivemind skills: %w", err)
		}
		for _, s := range hivemindSkills {
			skills[s.Name] = s
		}
	}

	// 3. Load project skills — highest priority, overrides all.
	projectSkills, err := l.loadFromDir(ctx, l.projectDir, SourceProject)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load project skills: %w", err)
	}
	for _, s := range projectSkills {
		skills[s.Name] = s
	}

	result := make([]*Skill, 0, len(skills))
	for _, s := range skills {
		result = append(result, s)
	}

	return result, nil
}

// LoadMetadataOnly loads only skill metadata for system prompt injection.
// This is more efficient as it doesn't load full content.
// Priority: Golem(global) < Hivemind < Project.
func (l *Loader) LoadMetadataOnly(ctx context.Context) ([]SkillMetadata, error) {
	metadata := make(map[string]SkillMetadata)

	// 1. Golem (global) — lowest priority.
	if err := l.loadMetadataFromDir(ctx, l.globalDir, SourceGlobal, metadata); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	// 2. Hivemind — overrides Golem.
	if l.hivemindDir != "" {
		if err := l.loadMetadataFromDir(ctx, l.hivemindDir, SourceHivemind, metadata); err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
		}
	}

	// 3. Project — highest priority.
	if err := l.loadMetadataFromDir(ctx, l.projectDir, SourceProject, metadata); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	result := make([]SkillMetadata, 0, len(metadata))
	for _, m := range metadata {
		result = append(result, m)
	}

	return result, nil
}

// LoadSkill loads a specific skill by name.
// Search order: Project → Hivemind → Golem(global).
func (l *Loader) LoadSkill(ctx context.Context, name string) (*Skill, error) {
	// Try project first (highest priority).
	projectPath := filepath.Join(l.projectDir, name)
	if skill, err := l.loadSingleSkill(ctx, projectPath, SourceProject); err == nil {
		return skill, nil
	}

	// Try hivemind (if enabled).
	if l.hivemindDir != "" {
		hivemindPath := filepath.Join(l.hivemindDir, name)
		if skill, err := l.loadSingleSkill(ctx, hivemindPath, SourceHivemind); err == nil {
			return skill, nil
		}
	}

	// Try global (Golem).
	globalPath := filepath.Join(l.globalDir, name)
	if skill, err := l.loadSingleSkill(ctx, globalPath, SourceGlobal); err == nil {
		return skill, nil
	}

	return nil, &SkillError{
		SkillPath: name,
		Message:   "skill not found",
	}
}

// LoadSkillContent loads the full content of a skill's SKILL.md.
// Use this for on-demand loading when the skill is triggered.
func (l *Loader) LoadSkillContent(_ context.Context, skill *Skill) (string, error) {
	if skill.Content != "" {
		return skill.Content, nil
	}

	_, content, err := l.parser.ParseFile(skill.SkillMDPath())
	if err != nil {
		return "", err
	}

	skill.Content = content
	return content, nil
}

// ListSkills returns a formatted list of available skills.
func (l *Loader) ListSkills(ctx context.Context) (string, error) {
	metadata, err := l.LoadMetadataOnly(ctx)
	if err != nil {
		return "", err
	}

	if len(metadata) == 0 {
		return "No skills found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Available Skills:\n\n")

	for _, m := range metadata {
		sb.WriteString(fmt.Sprintf("- %s (%s)\n", m.Name, m.Source))
		sb.WriteString(fmt.Sprintf("  %s\n\n", truncate(m.Description, 100)))
	}

	return sb.String(), nil
}

// --- internal helpers ---

func (l *Loader) loadFromDir(ctx context.Context, dir string, source SkillSource) ([]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		skillPath := filepath.Join(dir, entry.Name())
		skill, err := l.loadSingleSkill(ctx, skillPath, source)
		if err != nil {
			logger.WarnX(logModule, "failed to load skill %s: %v", entry.Name(), err)
			continue
		}

		result = append(result, skill)
	}

	return result, nil
}

func (l *Loader) loadMetadataFromDir(ctx context.Context, dir string, source SkillSource, metadata map[string]SkillMetadata) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		skillPath := filepath.Join(dir, entry.Name())
		skillMDPath := filepath.Join(skillPath, SkillFileName)

		fm, err := l.parser.ParseMetadataOnly(skillMDPath)
		if err != nil {
			continue // skip invalid skills silently for metadata loading
		}

		metadata[fm.Name] = SkillMetadata{
			Name:         fm.Name,
			Description:  fm.Description,
			Source:       source,
			Path:         skillPath,
			Capabilities: fm.Capabilities,
		}
	}

	return nil
}

func (l *Loader) loadSingleSkill(_ context.Context, skillPath string, source SkillSource) (*Skill, error) {
	skillMDPath := filepath.Join(skillPath, SkillFileName)

	if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
		return nil, ErrMissingSkillMD
	}

	fm, content, err := l.parser.ParseFile(skillMDPath)
	if err != nil {
		return nil, err
	}

	files, err := l.discoverFiles(skillPath)
	if err != nil {
		return nil, err
	}

	return &Skill{
		Name:        fm.Name,
		Description: fm.Description,
		Path:        skillPath,
		Content:     content,
		Files:       files,
		Source:      source,
		LoadedAt:    time.Now(),
	}, nil
}

func (l *Loader) discoverFiles(skillPath string) ([]SkillFile, error) {
	var files []SkillFile

	err := filepath.WalkDir(skillPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == skillPath || d.IsDir() || d.Name() == SkillFileName {
			return nil
		}

		relPath, _ := filepath.Rel(skillPath, path)
		files = append(files, SkillFile{
			RelPath: relPath,
			AbsPath: path,
			Type:    determineFileType(relPath),
		})

		return nil
	})

	return files, err
}

func determineFileType(relPath string) FileType {
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) == 0 {
		return FileTypeOther
	}

	switch parts[0] {
	case "scripts":
		return FileTypeScript
	case "references":
		return FileTypeReference
	case "assets":
		return FileTypeAsset
	default:
		return FileTypeOther
	}
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
