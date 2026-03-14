package messagebus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// TranscriptConfig configures team conversation transcript persistence.
type TranscriptConfig struct {
	// Enabled controls whether transcript persistence is active.
	Enabled bool `json:"enabled"`

	// OutputDir is the directory where transcript files are stored.
	OutputDir string `json:"output_dir"`

	// Format is the output format (currently only "markdown" is supported).
	Format string `json:"format"`
}

// DefaultTranscriptConfig returns sensible defaults.
func DefaultTranscriptConfig() *TranscriptConfig {
	return &TranscriptConfig{
		Enabled:   true,
		OutputDir: "./logs/team-transcripts",
		Format:    "markdown",
	}
}

// TranscriptPlugin persists team messages as Markdown files.
// It implements the Hook Observer pattern — it listens to MessageBus hooks
// and writes messages to disk using write-through strategy (O_APPEND).
//
// Design decisions:
//   - Write-through (every message immediately flushed) to avoid crash data loss
//   - O_APPEND mode for safe concurrent writes
//   - Markdown format for readability and Git diff support
//   - File naming: {teamID}-{date}.md for daily rotation
type TranscriptPlugin struct {
	config *TranscriptConfig
	mu     sync.Mutex
	files  map[string]*os.File // teamID → open file handle
}

// NewTranscriptPlugin creates a transcript plugin with the given config.
func NewTranscriptPlugin(config TranscriptConfig) *TranscriptPlugin {
	return &TranscriptPlugin{
		config: &config,
		files:  make(map[string]*os.File),
	}
}

// RegisterHooks registers the transcript plugin's hooks on a MessageBus.
func (p *TranscriptPlugin) RegisterHooks(bus *defaultMessageBus) {
	if !p.config.Enabled {
		return
	}
	bus.RegisterHook(HookMessageSent, p.OnMessageSent)
	bus.RegisterHook(HookMessageBroadcast, p.OnMessageBroadcast)
	logger.Info("[TranscriptPlugin] hooks registered (output_dir=%s)", p.config.OutputDir)
}

// OnMessageSent handles point-to-point message events.
func (p *TranscriptPlugin) OnMessageSent(_ context.Context, msg *Message) error {
	return p.writeEntry(msg, false)
}

// OnMessageBroadcast handles broadcast message events.
func (p *TranscriptPlugin) OnMessageBroadcast(_ context.Context, msg *Message) error {
	return p.writeEntry(msg, true)
}

// writeEntry appends a message entry to the transcript file.
func (p *TranscriptPlugin) writeEntry(msg *Message, isBroadcast bool) error {
	if msg.TeamID == "" {
		return nil // Skip non-team messages.
	}

	f, err := p.getOrCreateFile(msg.TeamID)
	if err != nil {
		return fmt.Errorf("failed to open transcript file: %w", err)
	}

	// Format the entry.
	entry := p.formatEntry(msg, isBroadcast)

	// Write-through: immediately append and flush.
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write transcript entry: %w", err)
	}

	return nil
}

// formatEntry formats a message as a Markdown transcript entry.
func (p *TranscriptPlugin) formatEntry(msg *Message, isBroadcast bool) string {
	timestamp := msg.CreatedAt.Format("15:04:05")
	target := msg.To
	if isBroadcast {
		target = "all (broadcast)"
	}

	return fmt.Sprintf(
		"\n---\n\n## [%s] %s → %s (%s)\n\n%s\n",
		timestamp,
		msg.From,
		target,
		msg.Type,
		msg.Content,
	)
}

// getOrCreateFile returns or creates the transcript file for a team.
func (p *TranscriptPlugin) getOrCreateFile(teamID string) (*os.File, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for existing file handle.
	if f, ok := p.files[teamID]; ok {
		return f, nil
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(p.config.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	// Generate file path: {teamID}-{date}.md
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.md", teamID, date)
	filepath := filepath.Join(p.config.OutputDir, filename)

	// Open in append mode.
	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	// Write header if the file is new.
	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		header := fmt.Sprintf("# Team Transcript: %s\n> Team ID: %s | Created: %s\n",
			teamID, teamID, time.Now().Format("2006-01-02 15:04:05"))
		if _, err := f.WriteString(header); err != nil {
			f.Close()
			return nil, err
		}
	}

	p.files[teamID] = f
	return f, nil
}

// Close closes all open file handles.
func (p *TranscriptPlugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	for teamID, f := range p.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.files, teamID)
	}
	return firstErr
}
