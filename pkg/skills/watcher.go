package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Watcher monitors skill directories for changes and triggers reloads.
type Watcher struct {
	watcher  *fsnotify.Watcher
	registry *Registry
	dirs     []string
	debounce time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// WatcherOption configures the Watcher.
type WatcherOption func(watcher *Watcher)

// WithDebounce sets the debounce duration for batching rapid file changes.
// Default 100ms.
func WithDebounce(d time.Duration) WatcherOption {
	return func(w *Watcher) {
		w.debounce = d
	}
}

// NewWatcher creates a new file system watcher for skill directories.
func NewWatcher(registry *Registry, dirs []string, opts ...WatcherOption) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		watcher:  fsWatcher,
		registry: registry,
		dirs:     dirs,
		debounce: 100 * time.Millisecond,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}

	for _, opt := range opts {
		opt(w)
	}

	return w, nil
}

// Start begins watching the configured directories.
// It runs in a background goroutine and returns immediately.
func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("watcher already running")
	}
	w.running = true
	w.mu.Unlock()

	for _, dir := range w.dirs {
		expanded := expandPath(dir)
		if err := w.addDirRecursive(expanded); err != nil {
			logger.WarnX(logModule, "could not watch %s:%w", dir, err)
		}
	}

	go w.run(ctx)

	return nil
}

// Stop gracefully stops the watcher.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}

	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	<-w.doneCh //  wait for goroutine to finish

	return nil
}

// --- internal ---

func (w *Watcher) addDirRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := w.watcher.Add(path); err != nil {
				return fmt.Errorf("failed to watch %s: %w", path, err)
			}
		}
		return nil
	})
}

func (w *Watcher) run(ctx context.Context) {
	defer close(w.doneCh)

	var (
		timer   *time.Timer
		timerCh <-chan time.Time
		pending bool
	)

	for {
		select {
		case <-ctx.Done():
			w.cleanup(timer)
			return

		case <-w.stopCh:
			w.cleanup(timer)
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only react to SKILL.md changes.
			if filepath.Base(event.Name) != SkillFileName {
				// But watch for new directories.
				if event.Has(fsnotify.Create) {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						_ = w.watcher.Add(event.Name)
					}
				}
				continue
			}

			// Debounce: reset timer on each event.
			if timer == nil {
				timer = time.NewTimer(w.debounce)
				timerCh = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.debounce)
			}
			pending = true

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			logger.ErrorX(logModule, "watcher error: %v", err)

		case <-timerCh:
			if pending {
				w.triggerReload(ctx)
				pending = false
			}
			timer = nil
			timerCh = nil
		}
	}
}

func (w *Watcher) triggerReload(ctx context.Context) {
	logger.InfoX(logModule, "skills changed, reloading...")
	if err := w.registry.Reload(ctx); err != nil {
		logger.ErrorX(logModule, "failed to reload skills: %v", err)
	} else {
		logger.InfoX(logModule, "reloaded %d skill(s)", w.registry.Count())
	}
}

func (w *Watcher) cleanup(timer *time.Timer) {
	if timer != nil {
		timer.Stop()
	}
	w.watcher.Close()
}
