package bubbletea

import (
	"context"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
	"github.com/kiosk404/echoryn/pkg/cli/tui/render"
)

// ─────────────────────────────────────────────────────────────────────────────
// Update dispatch — Strategy Pattern.
//
// Each phase has a dedicated handler. The top-level chatUpdate dispatches
// to the correct handler based on the current Phase.
// ─────────────────────────────────────────────────────────────────────────────

// chatUpdate is the main Update dispatcher for ChatModel.
// It takes a pointer to avoid copying strings.Builder and other non-copyable state.
func chatUpdate(m *ChatModel, msg tea.Msg) tea.Cmd {
	// Global messages handled in any phase.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		if m.width > 4 {
			m.inputBuffer.SetViewport(5, m.width-4)
		}
		return nil
	default:
		_ = msg
	}

	// Phase-specific dispatch.
	switch m.phase {
	case PhaseInput:
		return updateInput(m, msg)
	case PhaseThinking:
		return updateThinking(m, msg)
	case PhaseStreaming:
		return updateStreaming(m, msg)
	case PhaseRendering:
		return updateRendering(m, msg)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Input
// ─────────────────────────────────────────────────────────────────────────────

// updateInput handles key events during the input phase.
func updateInput(m *ChatModel, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleChatKeyPress(m, msg)
	}
	return nil
}

// handleChatKeyPress processes key events in the input phase.
// It delegates most keys to the existing textbuffer/completion logic,
// but intercepts Enter (submit) and Ctrl-C (quit).
func handleChatKeyPress(m *ChatModel, msg tea.KeyMsg) tea.Cmd {
	// Completion navigation.
	if m.showCompletion && len(m.completions) > 0 {
		switch msg.String() {
		case "up":
			if m.completionIndex > 0 {
				m.completionIndex--
			}
			return nil
		case "down":
			if m.completionIndex < len(m.completions)-1 {
				m.completionIndex++
			}
			return nil
		case "tab", "enter", "right":
			return acceptChatCompletion(m)
		case "esc":
			m.showCompletion = false
			m.completions = nil
			m.ghostText = ""
			return nil
		}
	}

	switch msg.String() {
	case "ctrl+c":
		return tea.Quit

	case "ctrl+d":
		if m.inputBuffer.Text() == "" {
			return tea.Quit
		}
		return nil

	case "ctrl+z":
		m.inputBuffer.Undo()
		return nil

	case "ctrl+y":
		m.inputBuffer.Redo()
		return nil

	case "enter":
		content := m.inputBuffer.Text()
		if content == "" {
			return nil
		}
		m.inputBuffer.AddToHistory()
		m.inputBuffer.Clear()
		m.showCompletion = false
		m.completions = nil
		m.ghostText = ""

		line := strings.TrimSpace(content)

		// Slash command handling.
		if strings.HasPrefix(line, "/") {
			return handleSlashCommand(m, line)
		}

		// Chat message — transition to PhaseThinking.
		return startChat(m, line)

	case "alt+enter", "ctrl+j":
		m.inputBuffer.NewLine()
		return nil

	case "up":
		if m.inputBuffer.Text() == "" {
			m.inputBuffer.HistoryUp()
		} else {
			_, row := m.inputBuffer.Cursor()
			if row > 0 {
				m.inputBuffer.MoveUp()
			} else {
				m.inputBuffer.HistoryUp()
			}
		}
		return nil

	case "down":
		m.inputBuffer.HistoryDown()
		return nil

	case "left":
		m.inputBuffer.MoveLeft()
		return nil

	case "right":
		if m.ghostText != "" {
			_, col := m.inputBuffer.Cursor()
			runes := []rune(m.inputBuffer.Text())
			if col >= len(runes) {
				m.inputBuffer.Insert(m.ghostText)
				m.ghostText = ""
				m.showCompletion = false
				m.completions = nil
				return nil
			}
		}
		m.inputBuffer.MoveRight()
		return nil

	case "home", "ctrl+a":
		m.inputBuffer.MoveToStart()
		return nil

	case "end", "ctrl+e":
		m.inputBuffer.MoveToEnd()
		return nil

	case "backspace":
		m.inputBuffer.DeleteChar()
		updateChatCompletions(m)
		return nil

	case "delete":
		m.inputBuffer.DeleteCharForward()
		updateChatCompletions(m)
		return nil

	case "ctrl+w":
		m.inputBuffer.DeleteWordLeft()
		updateChatCompletions(m)
		return nil

	case "ctrl+u":
		m.inputBuffer.SetText("")
		updateChatCompletions(m)
		return nil

	case "ctrl+k":
		m.inputBuffer.SetText("")
		return nil

	case "tab":
		if m.ghostText != "" {
			m.inputBuffer.Insert(m.ghostText)
			m.ghostText = ""
			m.showCompletion = false
			m.completions = nil
			return nil
		}
		if len(m.completions) > 0 {
			return acceptChatCompletion(m)
		}
		return nil

	case "esc":
		m.showCompletion = false
		m.completions = nil
		m.ghostText = ""
		return nil
	}

	// Regular character input.
	if len(msg.Runes) > 0 {
		for _, r := range msg.Runes {
			m.inputBuffer.Insert(string(r))
		}
		updateChatCompletions(m)
	}

	return nil
}

// startChat transitions from PhaseInput to PhaseThinking.
func startChat(m *ChatModel, message string) tea.Cmd {
	m.messages = append(m.messages, ChatMessage{Role: "user", Content: message})
	m.phase = PhaseThinking
	m.streamContent.Reset()
	m.fullContent.Reset()
	m.toolCalls = nil
	m.lastAborted = false
	m.spinnerFrame = 0
	m.spinnerStart = time.Now().UnixNano()
	m.lastStartTime = m.spinnerStart

	// Flush user message to scroll-back, then start spinner + streaming.
	return tea.Batch(
		tea.Println(render.FormatUserMessage(message)),
		spinnerTickCmd(),
		func() tea.Msg { return StreamStartMsg{} },
	)
}

// handleSlashCommand executes a slash command and returns to input.
func handleSlashCommand(m *ChatModel, rawInput string) tea.Cmd {
	cmd, args, ok := m.commands.Lookup(rawInput)
	if !ok {
		return tea.Println("Unknown command: " + rawInput + " (type /help for available commands)")
	}

	// Execute the command synchronously (commands are fast).
	env := &command.Env{
		Out:          os.Stdout,
		ClearHistory: func() { m.messages = nil },
		Model:        m.client.Model,
		SessionKey:   m.client.SessionKey,
		TeamState:    m.teamState,
		SetTeamState: func(state *command.TeamState) { m.teamState = state },
		TeamAPI:      m.teamAPI,
	}

	if err := cmd.Execute(context.Background(), env, args); err != nil {
		if err.Error() == "quit" {
			return tea.Quit
		}
		return tea.Println("Error: " + err.Error())
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Thinking
// ─────────────────────────────────────────────────────────────────────────────

func updateThinking(m *ChatModel, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case SpinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % 10
		return spinnerTickCmd()

	case StreamStartMsg:
		// Handled in chat_app.go Update() — nothing to do here.
		return nil

	case StreamDeltaMsg:
		// First delta — transition to streaming.
		m.phase = PhaseStreaming
		m.streamContent.WriteString(msg.Delta)
		return nil

	case StreamToolCallMsg:
		m.phase = PhaseStreaming
		return handleToolCall(m, msg.Name)

	case StreamDoneMsg:
		return finishStreaming(m, msg)

	case tea.KeyMsg:
		if msg.String() == "esc" {
			return abortStreaming(m)
		}
		return nil
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Streaming
// ─────────────────────────────────────────────────────────────────────────────

func updateStreaming(m *ChatModel, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case StreamDeltaMsg:
		m.streamContent.WriteString(msg.Delta)
		return nil

	case StreamToolCallMsg:
		return handleToolCall(m, msg.Name)

	case StreamDoneMsg:
		return finishStreaming(m, msg)

	case tea.KeyMsg:
		if msg.String() == "esc" {
			return abortStreaming(m)
		}
		return nil
	}
	return nil
}

// handleToolCall flushes any pending streaming content to scroll-back,
// then renders a styled tool call indicator inline in the conversation flow.
// This keeps tool calls visually embedded in the output rather than
// stacked at the bottom.
func handleToolCall(m *ChatModel, name string) tea.Cmd {
	m.toolCalls = append(m.toolCalls, name)

	var cmds []tea.Cmd

	// Flush any pending streaming content to scroll-back first.
	pending := m.streamContent.String()
	if pending != "" {
		marker := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("183")).
			Render("✦ ")
		cmds = append(cmds, tea.Println(marker+pending))
		// Accumulate into fullContent for history, but reset the live view.
		m.fullContent.WriteString(pending)
		m.streamContent.Reset()
	}

	// Render a styled tool call panel and flush to scroll-back.
	panel := formatToolCallPanel(name, m.width)
	cmds = append(cmds, tea.Println(panel))

	return tea.Batch(cmds...)
}

// finishStreaming transitions to PhaseRendering and kicks off markdown rendering.
func finishStreaming(m *ChatModel, done StreamDoneMsg) tea.Cmd {
	m.lastUsage = done.Usage

	// Merge fullContent (flushed before tool calls) + streamContent (remaining live text).
	m.fullContent.WriteString(m.streamContent.String())
	content := m.fullContent.String()
	if done.Content != "" && content == "" {
		content = done.Content
	}

	if done.Err != nil {
		m.phase = PhaseInput
		if content != "" {
			m.messages = append(m.messages, ChatMessage{Role: "assistant", Content: content})
		}
		return tea.Println("Error: " + done.Err.Error())
	}

	if content == "" {
		m.phase = PhaseInput
		return nil
	}

	// Transition to rendering phase.
	m.phase = PhaseRendering
	m.messages = append(m.messages, ChatMessage{Role: "assistant", Content: content})
	return renderMarkdownCmd(content, m.width)
}

// abortStreaming cancels the stream and returns to input.
func abortStreaming(m *ChatModel) tea.Cmd {
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
	}
	// Best-effort server-side abort.
	go m.client.Abort(context.Background()) //nolint:errcheck

	m.lastAborted = true
	m.fullContent.WriteString(m.streamContent.String())
	content := m.fullContent.String()
	if content != "" {
		m.messages = append(m.messages, ChatMessage{Role: "assistant", Content: content})
	}

	m.phase = PhaseInput
	return tea.Println("── aborted ──")
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase: Rendering
// ─────────────────────────────────────────────────────────────────────────────

func updateRendering(m *ChatModel, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case RenderDoneMsg:
		m.renderedMarkdown = msg.Rendered
		m.phase = PhaseInput

		// Flush rendered content + status bar to scroll-back.
		var cmds []tea.Cmd
		cmds = append(cmds, tea.Println(msg.Rendered))

		// Status bar.
		bar := render.NewStatusBar(m.width)
		elapsed := time.Duration(time.Now().UnixNano()-m.lastStartTime) * time.Nanosecond
		bar.Model = m.client.Model()
		bar.Duration = elapsed
		bar.Aborted = m.lastAborted
		if m.lastUsage != nil {
			bar.PromptTokens = m.lastUsage.PromptTokens
			bar.CompletionTokens = m.lastUsage.CompletionTokens
			bar.TotalTokens = m.lastUsage.TotalTokens
		}
		cmds = append(cmds, tea.Println(bar.Render()))

		// Awaiting message.
		cmds = append(cmds, tea.Println(""))
		cmds = append(cmds, tea.Println("✦ Awaiting your next command or request."))
		cmds = append(cmds, tea.Println(""))

		return tea.Batch(cmds...)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Completion helpers
// ─────────────────────────────────────────────────────────────────────────────

func updateChatCompletions(m *ChatModel) {
	input := m.inputBuffer.Text()
	_, col := m.inputBuffer.Cursor()

	completions := m.completionMgr.Complete(input, col)
	if len(completions) > 0 {
		m.completions = completions
		m.completionIndex = 0
		m.showCompletion = true
		if len(input) > 0 {
			c := completions[0]
			if len(c.Value) > col {
				m.ghostText = c.Value[col:]
			} else {
				m.ghostText = ""
			}
		}
	} else {
		m.completions = nil
		m.showCompletion = false
		m.ghostText = ""
	}
}

func acceptChatCompletion(m *ChatModel) tea.Cmd {
	if len(m.completions) == 0 || m.completionIndex >= len(m.completions) {
		return nil
	}
	c := m.completions[m.completionIndex]
	m.inputBuffer.SetText(c.Value)
	m.inputBuffer.MoveToEnd()
	m.showCompletion = false
	m.completions = nil
	m.ghostText = ""
	return nil
}
