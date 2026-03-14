// Package bubbletea provides a BubbleTea-based TUI for Echoryn.
// It implements the Elm MVU architecture for terminal user interaction.
package bubbletea

import (
	"time"

	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/client"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/completion"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/list"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/spinner"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/textbuffer"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
)

// =============================================================================
// Core Model
// =============================================================================

// AppModel is the central state for the BubbleTea application.
// It follows the Elm architecture's single source of truth principle.
type AppModel struct {
	// === Session State ===
	SessionID string
	AgentID   string
	ModelName string
	AuthState AuthState

	// === Conversation State ===
	Messages  []HistoryItem
	MsgList   *MsgListState
	Streaming StreamingState

	// === Input State ===
	InputBuffer  *textbuffer.Buffer
	InputHistory []string
	HistoryIndex int

	// === Completion State ===
	CompletionMgr   *completion.Manager
	Completions     []completion.Completion
	CompletionIndex int
	ShowCompletion  bool
	GhostText       string

	// === Team State ===
	Team *TeamState

	// === UI State ===
	Width         int
	Height        int
	Focus         FocusArea
	ShowTeamPanel bool
	Ready         bool
	Spinner       spinner.SpinnerModel

	// === Layout ===
	Layout *Layout

	// === Theme ===
	Theme  *theme.SemanticColors
	Styles *theme.Styles

	// === Error State ===
	LastError string

	// === Dependencies ===
	Client     client.StreamClient
	TeamClient client.TeamClient
	Config     *Config
}

// MsgListState manages the virtual list for messages.
type MsgListState struct {
	items       []list.ListItem
	scrollY     int
	height      int
	width       int
	itemHeights map[string]int
}

// NewMsgListState creates a new message list state.
func NewMsgListState() *MsgListState {
	return &MsgListState{
		items:       []list.ListItem{},
		itemHeights: make(map[string]int),
	}
}

// VisibleItems returns visible items with their positions.
func (l *MsgListState) VisibleItems() []VisibleMsg {
	if len(l.items) == 0 || l.height == 0 {
		return nil
	}

	var result []VisibleMsg
	y := 0

	for i, item := range l.items {
		if y >= l.scrollY && y < l.scrollY+l.height {
			result = append(result, VisibleMsg{
				Index: i,
				Item:  item,
				Y:     y - l.scrollY,
			})
		}
		y += l.itemHeights[item.ID()]
	}

	return result
}

// SetItemHeight sets the height for an item.
func (l *MsgListState) SetItemHeight(id string, height int) {
	l.itemHeights[id] = height
}

// ItemCount returns the number of items.
func (l *MsgListState) ItemCount() int {
	return len(l.items)
}

// ScrollToBottom scrolls to the bottom.
func (l *MsgListState) ScrollToBottom() {
	total := 0
	for _, item := range l.items {
		total += l.itemHeights[item.ID()]
	}
	l.scrollY = max(0, total-l.height)
}

// SetViewport sets the viewport dimensions.
func (l *MsgListState) SetViewport(height, width int) {
	l.height = height
	l.width = width
}

// VisibleMsg represents a visible message in the list.
type VisibleMsg struct {
	Index int
	Item  list.ListItem
	Y     int
}

// =============================================================================
// Supporting Types
// =============================================================================

// AuthState represents the authentication status.
type AuthState int

const (
	AuthUnauthenticated AuthState = iota
	AuthAuthenticated
)

// StreamingState represents the current streaming response status.
type StreamingState int

const (
	StreamingIdle StreamingState = iota
	StreamingResponding
	StreamingWaitingConfirmation // waiting for user to confirm tool execution
)

// FocusArea indicates which UI element has focus.
type FocusArea int

const (
	FocusInput FocusArea = iota
	FocusHistory
	FocusTeamPanel
	FocusCompletion
)

// TeamState holds the current team collaboration state.
type TeamState struct {
	Enabled    bool
	ID         string
	Name       string
	TemplateID string
	Strategy   string
	Members    []MemberState
	Messages   []TeamMessage
	FocusIndex int
}

// MemberState represents a team member's current status.
type MemberState struct {
	SessionID string
	Label     string
	Role      TeamRole
	Status    MemberStatus
	Progress  string
	LastMsg   string
	NodeID    string
}

// TeamRole is the member's role in the team.
type TeamRole string

const (
	RoleLead     TeamRole = "lead"
	RoleWorker   TeamRole = "worker"
	RoleReviewer TeamRole = "reviewer"
)

// MemberStatus is the current status of a team member.
type MemberStatus string

const (
	StatusIdle      MemberStatus = "idle"
	StatusRunning   MemberStatus = "running"
	StatusCompleted MemberStatus = "completed"
	StatusFailed    MemberStatus = "failed"
)

// TeamMessage is a message from a team member.
type TeamMessage struct {
	From      string
	Content   string
	Timestamp time.Time
}

// Layout holds layout calculations.
type Layout struct {
	HeaderHeight   int
	FooterHeight   int
	InputHeight    int
	ContentHeight  int
	TeamPanelWidth int
	MainWidth      int
}

// Config holds application configuration.
type Config struct {
	Prompt          string
	MultilinePrompt string
	RequestTimeout  time.Duration
	ShowLineNumbers bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Prompt:          "> ",
		MultilinePrompt: "· ",
		RequestTimeout:  120 * time.Second,
		ShowLineNumbers: false,
	}
}

// =============================================================================
// Constructor
// =============================================================================

// NewAppModel creates a new AppModel with the given clients.
func NewAppModel(streamClient client.StreamClient, teamClient client.TeamClient, cfg *Config) AppModel {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	t := theme.GetTheme()
	s := theme.GetStyles()

	// Create text buffer
	buf := textbuffer.NewBuffer()

	// Create completion manager
	compMgr := completion.NewManager()
	for _, cmd := range completion.DefaultCommands() {
		compMgr.RegisterCommand(cmd)
	}

	return AppModel{
		Client:        streamClient,
		TeamClient:    teamClient,
		Config:        cfg,
		Messages:      make([]HistoryItem, 0),
		MsgList:       NewMsgListState(),
		Streaming:     StreamingIdle,
		Focus:         FocusInput,
		InputBuffer:   buf,
		CompletionMgr: compMgr,
		Spinner:       spinner.NewSpinner(),
		Theme:         t,
		Styles:        s,
		Layout:        &Layout{},
	}
}

// =============================================================================
// Computed Properties
// =============================================================================

// IsStreaming returns true if currently receiving a stream.
func (m AppModel) IsStreaming() bool {
	return m.Streaming == StreamingResponding || m.Streaming == StreamingWaitingConfirmation
}

// CanSendInput returns true if user input can be processed.
func (m AppModel) CanSendInput() bool {
	return m.Streaming == StreamingIdle
}

// ShortSessionID returns a truncated session ID for display.
func (m AppModel) ShortSessionID() string {
	if len(m.SessionID) > 12 {
		return m.SessionID[:12] + "..."
	}
	return m.SessionID
}

// InputText returns the current input text.
func (m AppModel) InputText() string {
	return m.InputBuffer.Text()
}

// CursorPosition returns the current cursor position.
func (m AppModel) CursorPosition() (int, int) {
	return m.InputBuffer.Cursor()
}

// =============================================================================
// Layout Calculations
// =============================================================================

// CalculateLayout updates layout dimensions.
func (m *AppModel) CalculateLayout() {
	l := m.Layout

	l.HeaderHeight = 2
	l.FooterHeight = 1
	l.InputHeight = m.calculateInputHeight()
	l.ContentHeight = m.Height - l.HeaderHeight - l.FooterHeight - l.InputHeight

	if m.ShowTeamPanel && m.Team != nil {
		l.TeamPanelWidth = 30
		l.MainWidth = m.Width - l.TeamPanelWidth - 1
	} else {
		l.TeamPanelWidth = 0
		l.MainWidth = m.Width
	}
}

// calculateInputHeight calculates the height needed for the input area.
func (m AppModel) calculateInputHeight() int {
	lines := m.InputBuffer.LineCount()
	if lines < 1 {
		lines = 1
	}
	// 2 for border, plus content lines
	return 2 + min(lines, 5) // Cap at 5 visible lines
}

// =============================================================================
// Status Icons
// =============================================================================

// MemberStatusIcon returns an icon for the member status.
func MemberStatusIcon(status MemberStatus) string {
	return spinner.StatusIcon(string(status))
}

// MemberRoleIcon returns an icon for the member role.
func MemberRoleIcon(role TeamRole, isLeader bool) string {
	if isLeader || role == RoleLead {
		return "👑"
	}
	if role == RoleReviewer {
		return "🔍"
	}
	return "🔧"
}
