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
type Loader struct {
	globalDir  string
	projectDir string
	parser     *Parser
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

// NewLoader creates a new skills loader with the given options.
func NewLoader(opts ...LoaderOption) *Loader {
	l := &Loader{
		globalDir:  paths.ResolveGolemSkillsDir(),
		projectDir: ".echoryn/skills",
		parser:     NewParser(),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// GlobalDir returns the global skills directory path.
func (l *Loader) GlobalDir() string { return l.globalDir }

// ProjectDir returns the project-level skills directory path.
func (l *Loader) ProjectDir() string { return l.projectDir }

// LoadAll loads all skills from both global and project directories.
// Project skills take precedence over global skills with the same name.
func (l *Loader) LoadAll(ctx context.Context) ([]*Skill, error) {
	skills := make(map[string]*Skill)

	// Load global skills first.
	globalSkills, err := l.loadFromDir(ctx, l.globalDir, SourceGlobal)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load global skills: %w", err)
	}
	for _, s := range globalSkills {
		skills[s.Name] = s
	}

	// Load project skills (override global).
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
func (l *Loader) LoadMetadataOnly(ctx context.Context) ([]SkillMetadata, error) {
	metadata := make(map[string]SkillMetadata)

	if err := l.loadMetadataFromDir(ctx, l.globalDir, SourceGlobal, metadata); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

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
func (l *Loader) LoadSkill(ctx context.Context, name string) (*Skill, error) {
	// Try project first.
	projectPath := filepath.Join(l.projectDir, name)
	if skill, err := l.loadSingleSkill(ctx, projectPath, SourceProject); err == nil {
		return skill, nil
	}

	// Try global.
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
