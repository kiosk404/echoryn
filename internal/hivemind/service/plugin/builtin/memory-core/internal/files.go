package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
)

// IsMemoryPath checks whether a relative path is recognized as a memory file.
// Matches OpenClaw's isMemoryPath logic.
func IsMemoryPath(relPath string) bool {
	normalized := NormalizeRelPath(relPath)
	if normalized == "" {
		return false
	}
	if normalized == "MEMORY.md" || normalized == "memory.md" {
		return true
	}
	return strings.HasPrefix(normalized, "memory/")
}

// NormalizeRelPath trims leading dots/slashes and normalizes separators.
func NormalizeRelPath(value string) string {
	trimmed := strings.TrimLeft(strings.TrimSpace(value), "./")
	return strings.ReplaceAll(trimmed, "\\", "/")
}

// NormalizeExtraMemoryPaths resolves extra memory paths to absolute paths.
func NormalizeExtraMemoryPaths(workspaceDir string, extraPaths []string) []string {
	if len(extraPaths) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var result []string
	for _, p := range extraPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var resolved string
		if filepath.IsAbs(p) {
			resolved = filepath.Clean(p)
		} else {
			resolved = filepath.Clean(filepath.Join(workspaceDir, p))
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		result = append(result, resolved)
	}
	return result
}

// ListMemoryFiles scans the workspace directory for memory Markdown files.
// It looks for MEMORY.md, memory.md, and memory/*.md, plus any extra paths.
// Matches OpenClaw's listMemoryFiles.
func ListMemoryFiles(workspaceDir string, extraPaths []string) ([]string, error) {
	var result []string

	addMarkdownFile := func(absPath string) {
		info, err := os.Lstat(absPath)
		if err != nil {
			return
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return
		}
		if !strings.HasSuffix(absPath, ".md") {
			return
		}
		result = append(result, absPath)
	}

	// Check MEMORY.md and memory.md in workspace root.
	addMarkdownFile(filepath.Join(workspaceDir, "MEMORY.md"))
	addMarkdownFile(filepath.Join(workspaceDir, "memory.md"))

	// Walk memory/ directory.
	memoryDir := filepath.Join(workspaceDir, "memory")
	if info, err := os.Lstat(memoryDir); err == nil {
		if info.Mode()&os.ModeSymlink == 0 && info.IsDir() {
			_ = walkDir(memoryDir, &result)
		}
	}

	// Walk extra paths.
	normalizedExtra := NormalizeExtraMemoryPaths(workspaceDir, extraPaths)
	for _, inputPath := range normalizedExtra {
		info, err := os.Lstat(inputPath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.IsDir() {
			_ = walkDir(inputPath, &result)
			continue
		}
		if info.Mode().IsRegular() && strings.HasSuffix(inputPath, ".md") {
			result = append(result, inputPath)
		}
	}

	// Dedup by real path.
	if len(result) <= 1 {
		return result, nil
	}
	seen := make(map[string]struct{})
	var deduped []string
	for _, entry := range result {
		key := entry
		if real, err := filepath.EvalSymlinks(entry); err == nil {
			key = real
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, entry)
	}
	return deduped, nil
}

// BuildFileEntry builds a MemoryFileEntry from an absolute file path.
func BuildFileEntry(absPath, workspaceDir string) (*entity.MemoryFileEntry, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	hash := HashText(string(content))

	relPath, err := filepath.Rel(workspaceDir, absPath)
	if err != nil {
		relPath = absPath
	}
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	return &entity.MemoryFileEntry{
		Path:    relPath,
		AbsPath: absPath,
		MtimeMs: info.ModTime().UnixMilli(),
		Size:    info.Size(),
		Hash:    hash,
	}, nil
}

// walkDir recursively collects .md files from a directory, skipping symlinks.
func walkDir(dir string, result *[]string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") {
			*result = append(*result, path)
		}
		return nil
	})
}
