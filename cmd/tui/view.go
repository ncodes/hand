package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View composes the scrollable transcript and fixed input composer.
func (m model) View() tea.View {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderTranscript(),
		m.renderInput(),
	)
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}
