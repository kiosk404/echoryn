// Package spinner provides a loading spinner component.
package spinner

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
)

// Spinner frames (dots style)
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// TickMsg is sent periodically to animate the spinner.
type TickMsg struct{}

// SpinnerModel represents a loading spinner.
type SpinnerModel struct {
	frame     int
	startTime time.Time
	running   bool
	text      string
	thought   string
	mu        sync.Mutex
}

// NewSpinner creates a new spinner.
func NewSpinner() SpinnerModel {
	return SpinnerModel{
		startTime: time.Now(),
		running:   false,
	}
}

// Init initializes the spinner.
func (s SpinnerModel) Init() tea.Cmd {
	return nil
}

// Update handles spinner animation.
func (s SpinnerModel) Update(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	switch msg.(type) {
	case TickMsg:
		if s.running {
			s.frame = (s.frame + 1) % len(spinnerFrames)
			return s, tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
				return TickMsg{}
			})
		}
	}
	return s, nil
}

// View renders the spinner.
func (s SpinnerModel) View() string {
	if !s.running {
		return ""
	}

	t := theme.GetTheme()
	styles := theme.GetStyles()

	// Get gradient color based on time
	elapsed := time.Since(s.startTime).Milliseconds()
	colorIndex := int(elapsed/100) % len(t.Accent)
	color := t.Accent[colorIndex]

	// Build spinner
	frame := lipgloss.NewStyle().Foreground(color).Render(spinnerFrames[s.frame])

	var parts []string
	parts = append(parts, frame)

	// Add text
	if s.thought != "" {
		parts = append(parts, styles.WarningMessage.Render(s.thought))
	} else if s.text != "" {
		parts = append(parts, s.text)
	} else {
		parts = append(parts, "Thinking...")
	}

	// Add elapsed time
	elapsedSec := int(time.Since(s.startTime).Seconds())
	if elapsedSec > 0 {
		parts = append(parts, fmt.Sprintf("(%ds)", elapsedSec))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// Start starts the spinner animation.
func (s *SpinnerModel) Start() tea.Cmd {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	s.startTime = time.Now()
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// Stop stops the spinner.
func (s *SpinnerModel) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

// SetText sets the spinner text.
func (s *SpinnerModel) SetText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
}

// SetThought sets the thought text (shown in warning color).
func (s *SpinnerModel) SetThought(thought string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.thought = thought
}

// IsRunning returns whether the spinner is running.
func (s SpinnerModel) IsRunning() bool {
	return s.running
}

// =============================================================================
// Progress Bar
// =============================================================================

// ProgressBar renders a progress bar.
func ProgressBar(width int, progress float64, label string) string {
	t := theme.GetTheme()

	if width < 10 {
		width = 10
	}

	// Calculate filled width
	barWidth := width - 4 // Account for brackets and percentage
	filled := int(float64(barWidth) * progress)
	if filled > barWidth {
		filled = barWidth
	}

	// Build bar
	empty := barWidth - filled
	bar := lipgloss.NewStyle().Foreground(t.Status.Success).Render(strings.Repeat("█", filled))
	bar += lipgloss.NewStyle().Foreground(t.Text.Secondary).Render(strings.Repeat("░", empty))

	// Percentage
	percent := fmt.Sprintf("%3.0f%%", progress*100)

	result := fmt.Sprintf("[%s] %s", bar, percent)
	if label != "" {
		result += " " + label
	}

	return result
}

// =============================================================================
// Loading Indicator
// =============================================================================

// LoadingIndicator shows a loading message with optional spinner.
type LoadingIndicator struct {
	Spinner   SpinnerModel
	Text      string
	Thought   string
	Elapsed   time.Duration
	CancelKey string
}

// NewLoadingIndicator creates a new loading indicator.
func NewLoadingIndicator(text string) LoadingIndicator {
	s := NewSpinner()
	return LoadingIndicator{
		Spinner:   s,
		Text:      text,
		CancelKey: "esc",
	}
}

// View renders the loading indicator.
func (l LoadingIndicator) View() string {
	styles := theme.GetStyles()

	var parts []string

	// Spinner
	parts = append(parts, l.Spinner.View())

	// Text
	if l.Thought != "" {
		parts = append(parts, styles.WarningMessage.Render(l.Thought))
	} else if l.Text != "" {
		parts = append(parts, styles.SystemMessage.Render(l.Text))
	}

	// Elapsed time
	if l.Elapsed > 0 {
		parts = append(parts, styles.HeaderInfo.Render(fmt.Sprintf("(esc to cancel, %ds)", int(l.Elapsed.Seconds()))))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// =============================================================================
// Status Icons
// =============================================================================

// StatusIcon returns an icon for the given status.
func StatusIcon(status string) string {
	t := theme.GetTheme()

	var icon string
	var color lipgloss.Color

	switch status {
	case "success", "completed", "done":
		icon = "✓"
		color = t.Status.Success
	case "error", "failed":
		icon = "✗"
		color = t.Status.Error
	case "warning", "pending":
		icon = "⚠"
		color = t.Status.Warning
	case "running", "loading", "thinking":
		icon = "●"
		color = t.Status.Info
	case "idle":
		icon = "○"
		color = t.Text.Secondary
	default:
		icon = "•"
		color = t.Text.Secondary
	}

	return lipgloss.NewStyle().Foreground(color).Render(icon)
}
