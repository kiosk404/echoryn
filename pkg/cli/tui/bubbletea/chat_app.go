package bubbletea

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Compile-time check that *ChatModel implements tea.Model.
var _ tea.Model = (*ChatModel)(nil)

// chatProgram holds a reference to the tea.Program for p.Send() in streaming.
// This is set by SetChatProgram() after tea.NewProgram() is created.
var chatProgram *tea.Program

// SetChatProgram stores the tea.Program reference for async message delivery.
// Must be called after tea.NewProgram() but before p.Run().
func SetChatProgram(p *tea.Program) {
	chatProgram = p
}

// Init implements tea.Model. It fires the welcome banner and window title.
func (m *ChatModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, tea.SetWindowTitle("Echoryn"))

	// Output the welcome banner through BubbleTea's tea.Println
	// so it goes into scroll-back without conflicting with inline rendering.
	if m.bannerText != "" {
		cmds = append(cmds, tea.Println(m.bannerText))
		m.bannerText = "" // only print once
	}

	return tea.Batch(cmds...)
}

// Update implements tea.Model. It dispatches to the phase-specific handler.
//
// IMPORTANT: ChatModel uses pointer receiver because it contains
// strings.Builder (not safe to copy) and sync state. BubbleTea fully
// supports *Model as tea.Model — this avoids all value-copy issues.
func (m *ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle StreamStartMsg globally — this initiates the streaming goroutine.
	if _, ok := msg.(StreamStartMsg); ok && chatProgram != nil {
		cancel := startRealtimeStreamCmd(chatProgram, m.client, m.messages, 120*time.Second)
		m.streamCancel = cancel
		return m, nil
	}

	cmd := chatUpdate(m, msg)
	return m, cmd
}

// View implements tea.Model. It dispatches to the phase-specific renderer.
func (m *ChatModel) View() string {
	return chatView(m)
}
