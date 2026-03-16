package bubbletea

import (
	tea "github.com/charmbracelet/bubbletea"
)

var _ tea.Model = (*InputModel)(nil)

func (m InputModel) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("Echoryn"))
}

func (m InputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return handleUpdate(m, msg)
}

func (m InputModel) View() string {
	return renderInputView(m)
}
