package bubbletea

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleUpdate is the main update dispatcher for the input model.
func handleUpdate(m InputModel, msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true
		// Set viewport for text buffer wrapping
		if m.Width > 4 {
			m.InputBuffer.SetViewport(5, m.Width-4)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	}

	return m, nil
}

// handleKeyPress processes keyboard input.
func (m InputModel) handleKeyPress(msg tea.KeyMsg) (InputModel, tea.Cmd) {
	// If showing completions, handle completion navigation
	if m.ShowCompletion && len(m.Completions) > 0 {
		switch msg.String() {
		case "up":
			if m.CompletionIndex > 0 {
				m.CompletionIndex--
			}
			return m, nil
		case "down":
			if m.CompletionIndex < len(m.Completions)-1 {
				m.CompletionIndex++
			}
			return m, nil
		case "tab", "enter", "right":
			// Accept completion on Tab/Enter/Right when completion menu is shown
			return m.acceptCompletion()
		case "esc":
			m.ShowCompletion = false
			m.Completions = nil
			m.GhostText = ""
			return m, nil
		}
	}

	switch msg.String() {
	case "ctrl+c":
		m.Result = &InputResult{Quit: true}
		return m, tea.Quit

	case "ctrl+d":
		if m.InputBuffer.Text() == "" {
			m.Result = &InputResult{Quit: true}
			return m, tea.Quit
		}
		return m, nil

	case "ctrl+z":
		m.InputBuffer.Undo()
		return m, nil

	case "ctrl+y":
		m.InputBuffer.Redo()
		return m, nil

	case "enter":
		// Submit input
		content := m.InputBuffer.Text()
		if content != "" {
			m.InputBuffer.AddToHistory()
			m.InputBuffer.Clear()
			m.Result = &InputResult{Content: content}
			return m, tea.Quit
		}
		return m, nil

	case "alt+enter", "ctrl+j":
		// Newline in input (multiline editing)
		m.InputBuffer.NewLine()
		return m, nil

	case "up":
		// History navigation when input is empty
		if m.InputBuffer.Text() == "" {
			m.InputBuffer.HistoryUp()
		} else {
			// Move cursor up in multiline
			_, row := m.InputBuffer.Cursor()
			if row > 0 {
				m.InputBuffer.MoveUp()
			} else {
				m.InputBuffer.HistoryUp()
			}
		}
		return m, nil

	case "down":
		m.InputBuffer.HistoryDown()
		return m, nil

	case "left":
		m.InputBuffer.MoveLeft()
		return m, nil

	case "right":
		// Accept ghost text on right-arrow at end of line
		if m.GhostText != "" {
			_, col := m.InputBuffer.Cursor()
			runes := []rune(m.InputBuffer.Text())
			if col >= len(runes) {
				m.InputBuffer.Insert(m.GhostText)
				m.GhostText = ""
				m.ShowCompletion = false
				m.Completions = nil
				return m, nil
			}
		}
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
		m.InputBuffer.DeleteWordLeft()
		m.updateCompletions()
		return m, nil

	case "ctrl+u":
		m.InputBuffer.SetText("")
		m.updateCompletions()
		return m, nil

	case "ctrl+k":
		// Kill from cursor to end of line
		m.InputBuffer.SetText("")
		return m, nil

	case "tab":
		if m.GhostText != "" {
			m.InputBuffer.Insert(m.GhostText)
			m.GhostText = ""
			m.ShowCompletion = false
			m.Completions = nil
			return m, nil
		}
		if len(m.Completions) > 0 {
			return m.acceptCompletion()
		}
		return m, nil

	case "esc":
		m.ShowCompletion = false
		m.Completions = nil
		m.GhostText = ""
		return m, nil
	}

	// Regular character input
	if len(msg.Runes) > 0 {
		for _, r := range msg.Runes {
			m.InputBuffer.Insert(string(r))
		}
		m.updateCompletions()
	}

	return m, nil
}

// updateCompletions updates the completion suggestions based on current input.
func (m *InputModel) updateCompletions() {
	input := m.InputBuffer.Text()
	_, col := m.InputBuffer.Cursor()

	completions := m.CompletionMgr.Complete(input, col)
	if len(completions) > 0 {
		m.Completions = completions
		m.CompletionIndex = 0
		m.ShowCompletion = true

		// Set ghost text from first completion
		if len(input) > 0 {
			c := completions[0]
			if len(c.Value) > col {
				m.GhostText = c.Value[col:]
			} else {
				m.GhostText = ""
			}
		}
	} else {
		m.Completions = nil
		m.ShowCompletion = false
		m.GhostText = ""
	}
}

// acceptCompletion accepts the currently selected completion.
func (m InputModel) acceptCompletion() (InputModel, tea.Cmd) {
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
