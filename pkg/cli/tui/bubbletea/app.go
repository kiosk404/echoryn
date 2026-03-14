package bubbletea

import (
	tea "github.com/charmbracelet/bubbletea"
)

var _ tea.Model = (*AppModel)(nil)

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tea.SetWindowTitle("Echoryn"))
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return handleUpdate(m, msg)
}

func (m AppModel) View() string {
	return View(m)
}
