package bubbletea

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/components/spinner"
)

// handleUpdate is the main update dispatcher.
func handleUpdate(m AppModel, msg tea.Msg) (AppModel, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle spinner updates
	if m.Streaming == StreamingResponding {
		newSpinner, cmd := m.Spinner.Update(msg)
		m.Spinner = newSpinner
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	// === Window Resize ===
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true
		m.CalculateLayout()
		m.MsgList.SetViewport(m.Layout.ContentHeight, m.Layout.MainWidth)
		m.InputBuffer.SetViewport(5, m.Layout.MainWidth-4)
		return m, nil

	// === Spinner Tick ===
	case spinner.TickMsg:
		if m.Streaming == StreamingResponding {
			newSpinner, cmd := m.Spinner.Update(msg)
			m.Spinner = newSpinner
			return m, cmd
		}
		return m, nil

	// === Keyboard Input ===
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	// === User Input Submitted ===
	case InputMsg:
		return m.handleSubmitInput(msg)

	// === Streaming Response ===
	case StreamStartMsg:
		m.Streaming = StreamingResponding
		m.Spinner.Start()
		m.Messages = append(m.Messages, &HistoryItemAssistant{
			Content:   "",
			Time:      msg.RunID,
			Streaming: true,
		})
		return m, nil

	case StreamChunkMsg:
		return m.handleStreamChunk(msg)

	case StreamEndMsg:
		return m.handleStreamEnd(msg)

	case StreamErrorMsg:
		return m.handleStreamError(msg)

	// === Tool Execution ===
	case ToolCallRequestMsg:
		return m.handleToolRequest(msg)

	case ToolConfirmationMsg:
		return m.handleToolConfirmation(msg)

	case ToolResultMsg:
		return m.handleToolResult(msg)

	// === Team Events ===
	case TeamCreatedMsg:
		return m.handleTeamCreated(msg)

	case TeamDissolvedMsg:
		return m.handleTeamDissolved(msg)

	case MemberStatusMsg:
		return m.handleMemberStatus(msg)

	case TeamMessageMsg:
		return m.handleTeamMessage(msg)

	// === Info/Error Messages ===
	case InfoMsg:
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: msg.Content,
			Level:   InfoInfo,
			Time:    msg.Time,
		})
		return m, nil

	case ErrorMsg:
		m.LastError = msg.Error.Error()
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: msg.Error.Error(),
			Level:   InfoError,
			Time:    msg.Time,
		})
		return m, nil

	// === Tick for periodic updates ===
	case TickMsg:
		return m, tickCmd()

	// === Quit ===
	case QuitMsg:
		return m, tea.Quit
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// =============================================================================
// Keyboard Handling
// =============================================================================

// handleKeyPress processes keyboard input.
func (m AppModel) handleKeyPress(msg tea.KeyMsg) (AppModel, tea.Cmd) {
	// If showing completions, handle completion navigation
	if m.ShowCompletion && len(m.Completions) > 0 {
		switch msg.String() {
		case "up":
			if m.CompletionIndex > 0 {
				m.CompletionIndex--
			}
			return m, nil
		case "down", "tab":
			if m.CompletionIndex < len(m.Completions)-1 {
				m.CompletionIndex++
			}
			return m, nil
		case "enter", "right":
			// Accept completion
			return m.acceptCompletion()
		case "esc":
			m.ShowCompletion = false
			m.Completions = nil
			return m, nil
		}
	}

	// Handle special keys
	switch msg.String() {
	case "ctrl+c":
		if m.Streaming != StreamingIdle {
			// Interrupt current stream
			m.Streaming = StreamingIdle
			m.Spinner.Stop()
			return m, nil
		}
		return m, tea.Quit

	case "ctrl+d":
		// Exit
		return m, tea.Quit

	case "ctrl+t":
		// Toggle team panel
		if m.Team != nil {
			m.ShowTeamPanel = !m.ShowTeamPanel
			m.CalculateLayout()
		}
		return m, nil

	case "ctrl+n":
		// Focus next team member
		if m.Team != nil && len(m.Team.Members) > 0 {
			m.Team.FocusIndex = (m.Team.FocusIndex + 1) % len(m.Team.Members)
		}
		return m, nil

	case "ctrl+p":
		// Focus previous team member
		if m.Team != nil && len(m.Team.Members) > 0 {
			m.Team.FocusIndex = (m.Team.FocusIndex - 1 + len(m.Team.Members)) % len(m.Team.Members)
		}
		return m, nil

	case "ctrl+l":
		// Clear screen
		m.Messages = nil
		return m, nil

	case "ctrl+z":
		// Undo
		m.InputBuffer.Undo()
		return m, nil

	case "ctrl+y":
		// Redo
		m.InputBuffer.Redo()
		return m, nil

	case "enter":
		// Handle tool confirmation
		if m.Streaming == StreamingWaitingConfirmation {
			return m, func() tea.Msg {
				return ToolConfirmationMsg{Approved: true}
			}
		}

		// Check for shift+enter (multi-line)
		// Note: most terminals don't distinguish shift+enter
		// Use alt+enter or ctrl+enter for newline instead

		// Submit input
		if m.CanSendInput() && m.InputBuffer.Text() != "" {
			content := m.InputBuffer.Text()
			m.InputBuffer.AddToHistory()
			m.InputBuffer.Clear()
			return m, func() tea.Msg {
				return InputMsg{Content: content}
			}
		}
		return m, nil

	case "alt+enter", "ctrl+enter":
		// Newline in input
		m.InputBuffer.NewLine()
		return m, nil

	case "up":
		// History navigation
		if m.InputBuffer.Text() == "" || m.HistoryIndex > 0 {
			m.InputBuffer.HistoryUp()
		}
		return m, nil

	case "down":
		// History navigation
		m.InputBuffer.HistoryDown()
		return m, nil

	case "left":
		m.InputBuffer.MoveLeft()
		return m, nil

	case "right":
		m.InputBuffer.MoveRight()
		return m, nil

	case "home", "ctrl+a":
		m.InputBuffer.MoveToStart()
		return m, nil

	case "end", "ctrl+e":
		m.InputBuffer.MoveToEnd()
		return m, nil

	case "backspace":
		m.InputBuffer.DeleteChar()
		m.updateCompletions()
		return m, nil

	case "delete":
		m.InputBuffer.DeleteCharForward()
		m.updateCompletions()
		return m, nil

	case "ctrl+w":
		// Delete word backward
		m.InputBuffer.DeleteWordLeft()
		return m, nil

	case "ctrl+u":
		// Clear line
		m.InputBuffer.SetText("")
		return m, nil

	case "ctrl+k":
		// Clear from cursor to end of line
		m.InputBuffer.SetText("")
		return m, nil

	case "tab":
		// Trigger completion or accept ghost text
		if m.GhostText != "" {
			m.InputBuffer.Insert(m.GhostText)
			m.GhostText = ""
			return m, nil
		}
		if len(m.Completions) > 0 {
			return m.acceptCompletion()
		}
		return m, nil

	case "esc":
		// Cancel streaming or hide completion
		if m.Streaming != StreamingIdle {
			m.Streaming = StreamingIdle
			m.Spinner.Stop()
			return m, nil
		}
		m.ShowCompletion = false
		m.Completions = nil
		return m, nil

	case "/":
		// Start slash command
		if m.Streaming == StreamingIdle && m.InputBuffer.Text() == "" {
			m.InputBuffer.Insert("/")
			m.updateCompletions()
			return m, nil
		}
	}

	// Regular character input
	if m.Streaming == StreamingIdle && len(msg.Runes) > 0 {
		for _, r := range msg.Runes {
			m.InputBuffer.Insert(string(r))
		}
		m.updateCompletions()
	}

	return m, nil
}

// =============================================================================
// Input Handling
// =============================================================================

// handleSubmitInput processes user input.
func (m AppModel) handleSubmitInput(msg InputMsg) (AppModel, tea.Cmd) {
	input := strings.TrimSpace(msg.Content)
	if input == "" {
		return m, nil
	}

	// Handle slash commands
	if strings.HasPrefix(input, "/") {
		return m.handleCommand(input)
	}

	// Add user message to history
	m.Messages = append(m.Messages, &HistoryItemUser{
		Content: input,
		Time:    time.Now(),
	})

	// Start streaming response
	m.Streaming = StreamingResponding
	m.Spinner.Start()

	return m, nil
}

// handleCommand processes slash commands.
func (m AppModel) handleCommand(input string) (AppModel, tea.Cmd) {
	// Remove leading slash
	cmd := strings.TrimPrefix(input, "/")

	// Parse command and args
	parts := strings.SplitN(cmd, " ", 2)
	command := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	switch command {
	case "quit", "exit", "q":
		return m, tea.Quit

	case "clear", "c":
		m.Messages = nil
		return m, nil

	case "help", "h", "?":
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: `Available commands:
/team create <template> <task> - Create a team
/team status                   - Show team status
/team dissolve                 - Dissolve team
/agents                        - List team members
/msg <member> <message>        - Send message to member
/broadcast <message>           - Broadcast to all members
/focus [next|prev|member]      - Focus on member
/clear                         - Clear chat history
/help                          - Show this help
/quit                          - Exit`,
			Level: InfoInfo,
			Time:  time.Now(),
		})
		return m, nil

	case "team":
		return m.handleTeamCommand(args)

	case "agents":
		return m.handleAgentsCommand(args)

	case "msg", "message":
		return m.handleMsgCommand(args)

	case "broadcast", "bc":
		return m.handleBroadcastCommand(args)

	case "focus":
		return m.handleFocusCommand(args)

	default:
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "Unknown command: /" + command + " (type /help for available commands)",
			Level:   InfoError,
			Time:    time.Now(),
		})
		return m, nil
	}
}

// =============================================================================
// Completion Handling
// =============================================================================

// updateCompletions updates the completion suggestions based on current input.
func (m *AppModel) updateCompletions() {
	input := m.InputBuffer.Text()
	_, col := m.InputBuffer.Cursor()

	completions := m.CompletionMgr.Complete(input, col)
	if len(completions) > 0 {
		m.Completions = completions
		m.CompletionIndex = 0
		m.ShowCompletion = true

		// Set ghost text from first completion
		if len(completions) > 0 && len(input) > 0 {
			c := completions[0]
			if len(c.Value) > col {
				m.GhostText = c.Value[col:]
			}
		}
	} else {
		m.Completions = nil
		m.ShowCompletion = false
		m.GhostText = ""
	}
}

// acceptCompletion accepts the currently selected completion.
func (m AppModel) acceptCompletion() (AppModel, tea.Cmd) {
	if len(m.Completions) == 0 || m.CompletionIndex >= len(m.Completions) {
		return m, nil
	}

	c := m.Completions[m.CompletionIndex]
	m.InputBuffer.SetText(c.Value)
	m.InputBuffer.MoveToEnd()
	m.ShowCompletion = false
	m.Completions = nil
	m.GhostText = ""

	return m, nil
}

// =============================================================================
// Stream Handling
// =============================================================================

// handleStreamChunk handles a streaming content chunk.
func (m AppModel) handleStreamChunk(msg StreamChunkMsg) (AppModel, tea.Cmd) {
	// Find the last assistant message and append content
	for i := len(m.Messages) - 1; i >= 0; i-- {
		if item, ok := m.Messages[i].(*HistoryItemAssistant); ok && item.Streaming {
			item.Content += msg.Content
			break
		}
	}
	return m, nil
}

// handleStreamEnd handles end of stream.
func (m AppModel) handleStreamEnd(msg StreamEndMsg) (AppModel, tea.Cmd) {
	m.Streaming = StreamingIdle
	m.Spinner.Stop()

	// Mark streaming complete
	for i := len(m.Messages) - 1; i >= 0; i-- {
		if item, ok := m.Messages[i].(*HistoryItemAssistant); ok {
			item.Streaming = false
			break
		}
	}

	return m, nil
}

// handleStreamError handles streaming errors.
func (m AppModel) handleStreamError(msg StreamErrorMsg) (AppModel, tea.Cmd) {
	m.Streaming = StreamingIdle
	m.Spinner.Stop()
	m.LastError = msg.Error.Error()

	// Mark streaming complete with error
	for i := len(m.Messages) - 1; i >= 0; i-- {
		if item, ok := m.Messages[i].(*HistoryItemAssistant); ok {
			item.Streaming = false
			if item.Content == "" {
				item.Content = "Error: " + msg.Error.Error()
			}
			break
		}
	}

	return m, nil
}

// =============================================================================
// Tool Handling
// =============================================================================

// handleToolRequest handles pending tool calls.
func (m AppModel) handleToolRequest(msg ToolCallRequestMsg) (AppModel, tea.Cmd) {
	m.Streaming = StreamingWaitingConfirmation

	m.Messages = append(m.Messages, &HistoryItemToolGroup{
		Calls:  msg.Calls,
		Status: ToolGroupPending,
		Time:   time.Now(),
	})

	return m, nil
}

// handleToolConfirmation handles user's tool confirmation.
func (m AppModel) handleToolConfirmation(msg ToolConfirmationMsg) (AppModel, tea.Cmd) {
	// Find pending tool group
	for i := len(m.Messages) - 1; i >= 0; i-- {
		if item, ok := m.Messages[i].(*HistoryItemToolGroup); ok && item.Status == ToolGroupPending {
			if msg.Approved {
				item.Status = ToolGroupRunning
				m.Streaming = StreamingResponding
				m.Spinner.Start()
				return m, nil
			}
			item.Status = ToolGroupRejected
			m.Streaming = StreamingIdle
			return m, nil
		}
	}

	m.Streaming = StreamingIdle
	return m, nil
}

// handleToolResult handles tool execution results.
func (m AppModel) handleToolResult(msg ToolResultMsg) (AppModel, tea.Cmd) {
	for i := len(m.Messages) - 1; i >= 0; i-- {
		if item, ok := m.Messages[i].(*HistoryItemToolGroup); ok && item.Status == ToolGroupRunning {
			item.Results = append(item.Results, ToolResultInfo{
				CallID: msg.CallID,
				Result: msg.Result,
				Error:  msg.Error,
			})

			// Check if all tools complete
			if len(item.Results) == len(item.Calls) {
				item.Status = ToolGroupCompleted
			}
			break
		}
	}

	return m, nil
}

// =============================================================================
// Team Command Handling
// =============================================================================

// handleTeamCommand processes /team commands.
func (m AppModel) handleTeamCommand(args string) (AppModel, tea.Cmd) {
	parts := strings.SplitN(args, " ", 2)
	subCmd := ""
	if len(parts) > 0 {
		subCmd = parts[0]
	}
	subArgs := ""
	if len(parts) > 1 {
		subArgs = parts[1]
	}

	switch subCmd {
	case "create":
		return m.handleTeamCreate(subArgs)
	case "status":
		return m.handleTeamStatus()
	case "dissolve":
		return m.handleTeamDissolve()
	default:
		if m.Team == nil || !m.Team.Enabled {
			m.Messages = append(m.Messages, &HistoryItemInfo{
				Content: "No active team. Use /team create <template> <task> to start one.",
				Level:   InfoInfo,
				Time:    time.Now(),
			})
		} else {
			return m.handleTeamStatus()
		}
	}
	return m, nil
}

// handleTeamCreate handles team creation.
func (m AppModel) handleTeamCreate(args string) (AppModel, tea.Cmd) {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "Usage: /team create <template_id> <task_description>",
			Level:   InfoError,
			Time:    time.Now(),
		})
		return m, nil
	}

	templateID := parts[0]
	task := parts[1]

	m.Messages = append(m.Messages, &HistoryItemInfo{
		Content: "Creating team from template '" + templateID + "'...",
		Level:   InfoInfo,
		Time:    time.Now(),
	})

	// TODO: Call TeamClient.CreateTeam
	_ = templateID
	_ = task

	return m, nil
}

// handleTeamStatus shows team status.
func (m AppModel) handleTeamStatus() (AppModel, tea.Cmd) {
	if m.Team == nil || !m.Team.Enabled {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "No active team.",
			Level:   InfoInfo,
			Time:    time.Now(),
		})
		return m, nil
	}

	var content strings.Builder
	content.WriteString("Team: " + m.Team.Name + "\n")
	content.WriteString("Strategy: " + m.Team.Strategy + "\n")
	content.WriteString("Members:\n")
	for i, member := range m.Team.Members {
		focus := "  "
		if i == m.Team.FocusIndex {
			focus = "▸ "
		}
		icon := spinner.StatusIcon(string(member.Status))
		content.WriteString(focus + icon + " " + member.Label + " [" + string(member.Status) + "]")
		if member.Progress != "" {
			content.WriteString(" — " + member.Progress)
		}
		content.WriteString("\n")
	}

	m.Messages = append(m.Messages, &HistoryItemInfo{
		Content: content.String(),
		Level:   InfoInfo,
		Time:    time.Now(),
	})

	return m, nil
}

// handleTeamDissolve handles team dissolution.
func (m AppModel) handleTeamDissolve() (AppModel, tea.Cmd) {
	if m.Team == nil || !m.Team.Enabled {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "No active team to dissolve.",
			Level:   InfoInfo,
			Time:    time.Now(),
		})
		return m, nil
	}

	m.Messages = append(m.Messages, &HistoryItemInfo{
		Content: "Team dissolved.",
		Level:   InfoSuccess,
		Time:    time.Now(),
	})

	m.Team = nil
	m.ShowTeamPanel = false
	return m, nil
}

// handleAgentsCommand lists team members.
func (m AppModel) handleAgentsCommand(args string) (AppModel, tea.Cmd) {
	return m.handleTeamStatus()
}

// handleMsgCommand sends a message to a team member.
func (m AppModel) handleMsgCommand(args string) (AppModel, tea.Cmd) {
	if m.Team == nil || !m.Team.Enabled {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "No active team.",
			Level:   InfoError,
			Time:    time.Now(),
		})
		return m, nil
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "Usage: /msg <member_label> <message>",
			Level:   InfoError,
			Time:    time.Now(),
		})
		return m, nil
	}

	// TODO: Send via TeamClient
	m.Messages = append(m.Messages, &HistoryItemInfo{
		Content: "Message sent to " + parts[0],
		Level:   InfoSuccess,
		Time:    time.Now(),
	})

	return m, nil
}

// handleBroadcastCommand broadcasts to all members.
func (m AppModel) handleBroadcastCommand(args string) (AppModel, tea.Cmd) {
	if m.Team == nil || !m.Team.Enabled {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "No active team.",
			Level:   InfoError,
			Time:    time.Now(),
		})
		return m, nil
	}

	if args == "" {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "Usage: /broadcast <message>",
			Level:   InfoError,
			Time:    time.Now(),
		})
		return m, nil
	}

	// TODO: Broadcast via TeamClient
	m.Messages = append(m.Messages, &HistoryItemInfo{
		Content: "Broadcast sent to all members.",
		Level:   InfoSuccess,
		Time:    time.Now(),
	})

	return m, nil
}

// handleFocusCommand changes focused member.
func (m AppModel) handleFocusCommand(args string) (AppModel, tea.Cmd) {
	if m.Team == nil || !m.Team.Enabled {
		m.Messages = append(m.Messages, &HistoryItemInfo{
			Content: "No active team.",
			Level:   InfoError,
			Time:    time.Now(),
		})
		return m, nil
	}

	if len(m.Team.Members) == 0 {
		return m, nil
	}

	switch args {
	case "next", "n":
		m.Team.FocusIndex = (m.Team.FocusIndex + 1) % len(m.Team.Members)
	case "prev", "p":
		m.Team.FocusIndex = (m.Team.FocusIndex - 1 + len(m.Team.Members)) % len(m.Team.Members)
	case "":
		if m.Team.FocusIndex >= 0 && m.Team.FocusIndex < len(m.Team.Members) {
			member := m.Team.Members[m.Team.FocusIndex]
			m.Messages = append(m.Messages, &HistoryItemInfo{
				Content: "Focused on: " + member.Label + " [" + string(member.Status) + "]",
				Level:   InfoInfo,
				Time:    time.Now(),
			})
		}
	default:
		// Find by label
		for i, member := range m.Team.Members {
			if strings.EqualFold(member.Label, args) {
				m.Team.FocusIndex = i
				break
			}
		}
	}

	return m, nil
}

// =============================================================================
// Team Event Handling
// =============================================================================

// handleTeamCreated handles team creation event.
func (m AppModel) handleTeamCreated(msg TeamCreatedMsg) (AppModel, tea.Cmd) {
	m.Team = &TeamState{
		Enabled:    true,
		ID:         msg.TeamID,
		Name:       msg.Name,
		TemplateID: msg.Template,
		Members:    msg.Members,
		Messages:   make([]TeamMessage, 0),
		FocusIndex: 0,
	}
	m.ShowTeamPanel = true
	m.CalculateLayout()

	m.Messages = append(m.Messages, &HistoryItemInfo{
		Content: "Team '" + msg.Name + "' created with " + string(rune(len(msg.Members))) + " members.",
		Level:   InfoSuccess,
		Time:    time.Now(),
	})

	return m, nil
}

// handleTeamDissolved handles team dissolution event.
func (m AppModel) handleTeamDissolved(msg TeamDissolvedMsg) (AppModel, tea.Cmd) {
	m.Team = nil
	m.ShowTeamPanel = false
	m.CalculateLayout()

	m.Messages = append(m.Messages, &HistoryItemInfo{
		Content: "Team dissolved.",
		Level:   InfoInfo,
		Time:    time.Now(),
	})

	return m, nil
}

// handleMemberStatus handles member status updates.
func (m AppModel) handleMemberStatus(msg MemberStatusMsg) (AppModel, tea.Cmd) {
	if m.Team == nil {
		return m, nil
	}

	for i, member := range m.Team.Members {
		if member.SessionID == msg.SessionID {
			m.Team.Members[i].Status = msg.Status
			m.Team.Members[i].Progress = msg.Progress
			break
		}
	}

	return m, nil
}

// handleTeamMessage handles messages from team members.
func (m AppModel) handleTeamMessage(msg TeamMessageMsg) (AppModel, tea.Cmd) {
	if m.Team == nil {
		return m, nil
	}

	m.Team.Messages = append(m.Team.Messages, TeamMessage{
		From:      msg.From,
		Content:   msg.Content,
		Timestamp: msg.Timestamp,
	})

	// Keep only last 50 messages
	if len(m.Team.Messages) > 50 {
		m.Team.Messages = m.Team.Messages[len(m.Team.Messages)-50:]
	}

	return m, nil
}
