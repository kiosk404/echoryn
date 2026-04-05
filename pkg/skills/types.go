package skills

import (
	"path/filepath"
	"time"
)

// Skill represents a loaded skill with its metadata and content.
type Skill struct {
	// Name is the skill identifier (from YAML front matter).
	Name string `json:"name" yaml:"name"`

	// Description describes what the skill does and when to use it.
	Description string `json:"description" yaml:"description"`

	// Path is the absolute path to the skill directory.
	Path string `json:"path"`

	// Content is the full markdown body (loaded on demand).
	Content string `json:"-"`

	// Files are additional files bundled with the skill.
	Files []SkillFile `json:"files,omitempty"`

	// Source indicates where the skill was loaded from.
	Source SkillSource `json:"source"`

	// LoadedAt is when the skill was loaded.
	LoadedAt time.Time `json:"loaded_at"`
}

// SkillFile represents an additional file bundled with a skill.
type SkillFile struct {
	// RelPath is the path relative to skill directory.
	RelPath string `json:"rel_path"`

	// AbsPath is the absolute filesystem path
	AbsPath string `json:"abs_path"`

	// Type indicates the file category
	Type FileType `json:"type"`
}

// FileType categorizes bundled files.
type FileType string

const (
	FileTypeScript    FileType = "script"    // executable scripts (scripts/)
	FileTypeReference FileType = "reference" // documentation (references/)
	FileTypeAsset     FileType = "asset"     // templates, icons, etc (assets/)
	FileTypeOther     FileType = "other"     // uncategorized files
)

// SkillSource indicates where a skill was loaded from.
type SkillSource string

const (
	// SourceGlobal is the legacy name for Golem-level global skills.
	// Retained for backward compatibility; new code should prefer SourceGolem.
	// Directory: ~/.echoryn/golem/skills/
	SourceGlobal SkillSource = "global"

	// SourceGolem explicitly denotes Golem-local execution skills.
	// These describe what a specific Golem node can directly execute.
	// Directory: ~/.echoryn/golem/skills/
	SourceGolem SkillSource = "golem"

	// SourceHivemind denotes Hivemind-level global decision skills.
	// These describe strategic capabilities the system can accomplish
	// by orchestrating one or more Golem nodes — they are NOT directly executable.
	// Directory: ~/.echoryn/skills/
	SourceHivemind SkillSource = "hivemind"

	// SourceProject denotes project-level skills local to a workspace.
	// These take highest priority and override same-named skills from other sources.
	// Directory: <project>/.echoryn/skills/
	SourceProject SkillSource = "project"

	// SourceBuiltin denotes skills compiled into the binary.
	SourceBuiltin SkillSource = "builtin"
)

// Frontmatter represents the YAML frontmatter of a SKILL.md file.
type Frontmatter struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`

	// Optional fields
	Version      string   `json:"version,omitempty" yaml:"version,omitempty"`
	Author       string   `json:"author,omitempty" yaml:"author,omitempty"`
	License      string   `json:"license,omitempty" yaml:"license,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty" yaml:"allowed_tools,omitempty"`

	// Capabilities lists the capability tags these skills provides (used for scheduler matching)
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

// Validate checks if the frontmatter is valid.
func (f *Frontmatter) Validate() error {
	if f.Name == "" {
		return ErrMissingName
	}
	if len(f.Name) > MaxNameLength {
		return ErrNameTooLong
	}
	if f.Description == "" {
		return ErrMissingDescription
	}
	if len(f.Description) > MaxDescriptionLength {
		return ErrDescriptionTooLong
	}
	return nil
}

// SkillMetadata is the lightweight metadata loaded at startup.
// Only name and description are included to minimize context usage.
type SkillMetadata struct {
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Source       SkillSource `json:"source"`
	Path         string      `json:"path"`
	Capabilities []string    `json:"capabilities,omitempty"`
}

// ToMetaData extracts lightweight metadata from a full skill.
func (s *Skill) ToMetaData() SkillMetadata {
	return SkillMetadata{
		Name:        s.Name,
		Description: s.Description,
		Source:      s.Source,
		Path:        s.Path,
	}
}

// SkillMDPath returns the path to SKILL.md within the skill directory.
func (s *Skill) SkillMDPath() string {
	return filepath.Join(s.Path, SkillFileName)
}

// --- Constants and limits ---
const (
	// MaxNameLength is the maximum length for skill names.
	MaxNameLength = 64

	// MaxDescriptionLength is the maximum length for descriptions.
	MaxDescriptionLength = 1024

	// SkillFileName is the required filename for skill definitions.
	SkillFileName = "SKILL.md"
)

// --- Errors ---

// SkillError is the base error type for skill operations.
type SkillError struct {
	SkillPath string
	Message   string
	Err       error
}

func (e *SkillError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *SkillError) Unwrap() error {
	return e.Err
}

// Predefined errors
var (
	ErrMissingName        = &SkillError{Message: "skill name is required"}
	ErrNameTooLong        = &SkillError{Message: "skill name exceeds maximum length"}
	ErrMissingDescription = &SkillError{Message: "skill description is required"}
	ErrDescriptionTooLong = &SkillError{Message: "skill description exceeds maximum length"}
	ErrSkillNotFound      = &SkillError{Message: "skill not found"}
	ErrInvalidFrontmatter = &SkillError{Message: "invalid YAML frontmatter"}
	ErrMissingSkillMD     = &SkillError{Message: "SKILL.md file not found"}
)
