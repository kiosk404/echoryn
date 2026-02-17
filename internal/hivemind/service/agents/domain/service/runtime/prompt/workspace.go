package prompt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// conventionFile defines a well-known workspace file and its section metadata
type conventionFile struct {
	Filename string // e.g. "SOUL.md"
	Section  string // section name, e.g. "soul"
	Heading  string // Markdown heading injected before content
	Priority int
}

// conventionFiles lists the well-known workspace files in priority order
var conventionFiles = []conventionFile{
	{Filename: "SOUL.md", Section: "soul", Heading: "## Soul", Priority: 310},
	{Filename: "IDENTITY.md", Section: "identity_file", Heading: "## Identity", Priority: 320},
	{Filename: "AGENTS.md", Section: "agents_file", Heading: "## Collaborating Agents", Priority: 330},
}

// extraSectionBasePriority is the starting priority for extra .md files
// found under the `prompts/` subdirectory.
const extraSectionBasePriority = 350

// WorkspaceLoader watches a workspace directory and provides PromptSections
// from convention files (SOUL.md, IDENTITY.md, AGENTS.md) and extra prompts.
type WorkspaceLoader struct {
	mu      sync.RWMutex
	dir     string
	content map[string]string // section name -> file content
	watcher *fsnotify.Watcher
	closeCh chan struct{}
	closed  bool
}

// NewWorkspaceLoader creates a WorkspaceLoader for the given directory.
// It performs an initial scan and starts a background fsnotify watcher.
// If dir is empty or does not exist, returns nil (no-op).
func NewWorkspaceLoader(dir string) *WorkspaceLoader {
	if dir == "" {
		return nil
	}

	// Resolve to absolute path.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		logger.Warn("[WorkspaceLoader] failed to resolve path %q: %v", dir, err)
		return nil
	}

	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		logger.Debug("[WorkspaceLoader] directory %q does not exist, skipping", absDir)
		return nil
	}

	wl := &WorkspaceLoader{
		dir:     absDir,
		content: make(map[string]string),
		closeCh: make(chan struct{}),
	}

	// Initial scan.
	wl.reload()

	// Start watcher.
	if err := wl.startWatcher(); err != nil {
		logger.Warn("[WorkspaceLoader] failed to start watcher: %v, content loaded statically", err)
	}

	return wl
}

// Sections returns PromptSections for all loaded workspace files.
// Returns nil if no files were loaded.
func (wl *WorkspaceLoader) Sections() []PromptSection {
	if wl == nil {
		return nil
	}

	wl.mu.RLock()
	defer wl.mu.RUnlock()

	if len(wl.content) == 0 {
		return nil
	}

	var sections []PromptSection

	// Convention files.
	for _, cf := range conventionFiles {
		if content, ok := wl.content[cf.Section]; ok && content != "" {
			sections = append(sections, &WorkspaceSection{
				name:     cf.Section,
				priority: cf.Priority,
				heading:  cf.Heading,
				loader:   wl,
			})
		}
	}

	// Extra prompt files (sorted by name for deterministic ordering).
	var extraNames []string
	for name := range wl.content {
		isConvention := false
		for _, cf := range conventionFiles {
			if name == cf.Section {
				isConvention = true
				break
			}
		}
		if !isConvention {
			extraNames = append(extraNames, name)
		}
	}
	sort.Strings(extraNames)

	for i, name := range extraNames {
		sections = append(sections, &WorkspaceSection{
			name:     name,
			priority: extraSectionBasePriority + i,
			heading:  fmt.Sprintf("## %s", humanize(name)),
			loader:   wl,
		})
	}

	return sections
}

// GetContent returns the cached content for a section name.
// Thread-safe.
func (wl *WorkspaceLoader) GetContent(sectionName string) string {
	if wl == nil {
		return ""
	}
	wl.mu.RLock()
	defer wl.mu.RUnlock()
	return wl.content[sectionName]
}

// Close stops the file watcher and releases resources.
func (wl *WorkspaceLoader) Close() {
	if wl == nil {
		return
	}
	wl.mu.Lock()
	defer wl.mu.Unlock()

	if wl.closed {
		return
	}
	wl.closed = true
	close(wl.closeCh)

	if wl.watcher != nil {
		wl.watcher.Close()
	}
}

// reload scans the workspace directory and refreshes the content cache.
func (wl *WorkspaceLoader) reload() {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	newContent := make(map[string]string)

	// Load convention files.
	for _, cf := range conventionFiles {
		path := filepath.Join(wl.dir, cf.Filename)
		data, err := os.ReadFile(path)
		if err != nil {
			continue // File doesn't exist — skip silently.
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			newContent[cf.Section] = content
		}
	}

	// Load extra prompt files from prompts/ subdirectory.
	promptsDir := filepath.Join(wl.dir, "prompts")
	if info, err := os.Stat(promptsDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(promptsDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				path := filepath.Join(promptsDir, entry.Name())
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				content := strings.TrimSpace(string(data))
				if content == "" {
					continue
				}
				// Section name: filename without extension, prefixed with "extra:"
				name := "extra:" + strings.TrimSuffix(entry.Name(), ".md")
				newContent[name] = content
			}
		}
	}

	wl.content = newContent

	if len(newContent) > 0 {
		logger.Debug("[WorkspaceLoader] loaded %d files from %s", len(newContent), wl.dir)
	}
}

// startWatcher initializes fsnotify to watch the workspace directory.
func (wl *WorkspaceLoader) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	wl.watcher = watcher

	// Watch the root directory for convention files.
	if err := watcher.Add(wl.dir); err != nil {
		watcher.Close()
		return fmt.Errorf("watch %q: %w", wl.dir, err)
	}

	// Watch prompts/ subdirectory if it exists.
	promptsDir := filepath.Join(wl.dir, "prompts")
	if info, err := os.Stat(promptsDir); err == nil && info.IsDir() {
		_ = watcher.Add(promptsDir)
	}

	go wl.watchLoop()

	logger.Debug("[WorkspaceLoader] watcher started for %s", wl.dir)
	return nil
}

// watchLoop runs the fsnotify event loop with debounce.
func (wl *WorkspaceLoader) watchLoop() {
	// Debounce: wait 500ms after the last event before reloading.
	const debounceMs = 500

	var debounceTimer *debounceState
	debounceTimer = newDebounceState(debounceMs, func() {
		wl.reload()
	})
	defer debounceTimer.stop()

	for {
		select {
		case event, ok := <-wl.watcher.Events:
			if !ok {
				return
			}
			// Only react to write/create/remove of .md files.
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				if strings.HasSuffix(event.Name, ".md") {
					debounceTimer.trigger()
				}
			}
		case _, ok := <-wl.watcher.Errors:
			if !ok {
				return
			}
		case <-wl.closeCh:
			return
		}
	}
}

// --- WorkspaceSection ---

// WorkspaceSection is a dynamic PromptSection backed by a WorkspaceLoader.
// It reads content lazily from the loader's cache at render time.
type WorkspaceSection struct {
	name     string
	priority int
	heading  string
	loader   *WorkspaceLoader
}

func (s *WorkspaceSection) Name() string  { return "workspace:" + s.name }
func (s *WorkspaceSection) Priority() int { return s.priority }

func (s *WorkspaceSection) Enabled(_ context.Context, _ *PromptContext) bool {
	return s.loader.GetContent(s.name) != ""
}

func (s *WorkspaceSection) Render(_ context.Context, _ *PromptContext) (string, error) {
	content := s.loader.GetContent(s.name)
	if content == "" {
		return "", nil
	}
	return fmt.Sprintf("%s\n\n%s", s.heading, content), nil
}

// --- debounce helper ---

type debounceState struct {
	mu       sync.Mutex
	timer    *timerWrapper
	callback func()
	delayMs  int
}

type timerWrapper struct {
	ch      chan struct{}
	stopped bool
}

func newDebounceState(delayMs int, callback func()) *debounceState {
	return &debounceState{
		delayMs:  delayMs,
		callback: callback,
	}
}

func (d *debounceState) trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Cancel previous timer.
	if d.timer != nil && !d.timer.stopped {
		d.timer.stopped = true
		close(d.timer.ch)
	}

	tw := &timerWrapper{ch: make(chan struct{})}
	d.timer = tw

	go func() {
		select {
		case <-tw.ch:
			return // cancelled
		case <-timeAfter(d.delayMs):
			d.mu.Lock()
			if !tw.stopped {
				tw.stopped = true
				d.mu.Unlock()
				d.callback()
			} else {
				d.mu.Unlock()
			}
		}
	}()
}

func (d *debounceState) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil && !d.timer.stopped {
		d.timer.stopped = true
		close(d.timer.ch)
	}
}

// timeAfter wraps time.After for debounce.
func timeAfter(ms int) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		close(ch)
	}()
	return ch
}

// --- Helpers ---

// humanize converts a section name like "extra:my-rules" to "My Rules".
func humanize(name string) string {
	// Strip "extra:" prefix.
	name = strings.TrimPrefix(name, "extra:")
	// Replace dashes/underscores with spaces.
	name = strings.NewReplacer("-", " ", "_", " ").Replace(name)
	// Title case.
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}
