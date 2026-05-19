package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const jumpToBottomLabel = "Jump to bottom (ctrl+End) ↓"

// View composes the scrollable transcript and fixed input composer.
func (m model) View() tea.View {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderTranscript(),
		m.renderTranscriptComposerGap(),
		m.renderInput(),
	)
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}

func (m model) renderTranscriptComposerGap() string {
	if m.transcript.AtBottom() {
		return ""
	}

	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("238")).
		Padding(0, 1).
		Render(jumpToBottomLabel)

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(label)
}

func (m model) clicksJumpToBottomIndicator(msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft || m.transcript.AtBottom() {
		return false
	}

	return msg.Y == m.getJumpToBottomIndicatorRow()
}

func (m model) getJumpToBottomIndicatorRow() int {
	return m.getTranscriptTop() + max(m.transcript.Height(), 1)
}

func (m *model) jumpTranscriptToBottom() {
	if m.selection.active {
		m.restoreTranscriptContentAfterSelection()
	}

	m.transcript.GotoBottom()
	if m.responding {
		m.responseTranscriptFollow = true
		m.responseTranscriptScrolled = false
	}
}
