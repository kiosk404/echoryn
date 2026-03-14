// Package team template loader provides filesystem-based template loading.
// Templates are loaded from YAML/JSON files at startup and cached in memory
// via TeamRegistry (design decision v1.4: startup-time loading to avoid repeated disk IO).
package team

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
	"gopkg.in/yaml.v3"
)

// TemplateLoader loads team templates from a directory of YAML/JSON files
// and registers them into a TeamRegistry.
//
// Template directories are resolved via paths.ResolveTemplatesDirs(), which
// searches in order:
//
//	./conf/templates/       (project-specific)
//	~/.echoryn/templates/   (user-level)
//	/etc/echoryn/templates/ (system-wide)
//
// Template files are expected to be in the format:
//
//	templates/
//	├── software-dev-team-v1.yaml
//	├── code-review-team-v1.yaml
//	└── custom-team.json
type TemplateLoader struct {
	registry TeamRegistry
	dirs     []string
}

// NewTemplateLoader creates a new loader that will scan the given directories.
func NewTemplateLoader(registry TeamRegistry, dirs ...string) *TemplateLoader {
	return &TemplateLoader{
		registry: registry,
		dirs:     dirs,
	}
}

// LoadAll scans all configured directories and loads templates into the registry.
// It returns the number of templates successfully loaded and any errors encountered.
func (l *TemplateLoader) LoadAll(ctx context.Context) (int, error) {
	loaded := 0
	var errs []string

	for _, dir := range l.dirs {
		n, err := l.loadDir(ctx, dir)
		if err != nil {
			errs = append(errs, fmt.Sprintf("dir %s: %v", dir, err))
			continue
		}
		loaded += n
	}

	if len(errs) > 0 {
		return loaded, fmt.Errorf("template loading errors: %s", strings.Join(errs, "; "))
	}
	return loaded, nil
}

// loadDir loads all YAML/JSON template files from a single directory.
func (l *TemplateLoader) loadDir(ctx context.Context, dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("[TemplateLoader] directory not found, skipping: %s", dir)
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read directory: %w", err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		template, err := l.loadFile(filePath)
		if err != nil {
			logger.Warn("[TemplateLoader] failed to load template from %s: %v", filePath, err)
			continue
		}

		// Save to registry (idempotent: overwrite if exists).
		if err := l.registry.SaveTemplate(ctx, template); err != nil {
			logger.Warn("[TemplateLoader] failed to save template %s: %v", template.ID, err)
			continue
		}

		loaded++
		logger.Info("[TemplateLoader] loaded template: id=%s, name=%s, version=%s, members=%d",
			template.ID, template.Name, template.Version, len(template.MemberSpecs))
	}

	return loaded, nil
}

// loadFile reads and parses a single template file.
func (l *TemplateLoader) loadFile(filePath string) (*TeamTemplate, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	template := &TeamTemplate{}

	// Parse based on file extension.
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, template); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	case ".json":
		// Use yaml.Unmarshal which also handles JSON (YAML is a superset of JSON).
		if err := yaml.Unmarshal(data, template); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	// Set timestamps if not present.
	now := time.Now()
	if template.CreatedAt.IsZero() {
		template.CreatedAt = now
	}
	if template.UpdatedAt.IsZero() {
		template.UpdatedAt = now
	}

	// Validate the loaded template.
	if err := template.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return template, nil
}
