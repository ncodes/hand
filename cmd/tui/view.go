package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View composes the title bar, transcript, and input composer.
func (m model) View() tea.View {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.renderTranscript(),
		m.renderInput(),
	)
	view := tea.NewView(content)
	view.AltScreen = true

	return view
}
