package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const jumpToBottomLabel = "Jump to bottom (ctrl+End) ↓"

// View composes the scrollable transcript and fixed input composer.
func (m model) View() tea.View {
	mainContent := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderTranscript(),
		m.renderTranscriptComposerGap(),
		m.renderInput(),
	)
	view := tea.NewView(mainContent)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}

func (m model) renderTranscriptComposerGap() string {
	if m.transcript.AtBottom() {
		return ""
	}

	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color(defaultTUITheme.JumpToBottomForeground)).
		Background(lipgloss.Color(defaultTUITheme.JumpToBottomBackground)).
		Padding(0, 1).
		Render(jumpToBottomLabel)

	return lipgloss.NewStyle().
		Width(m.getMainPaneWidth()).
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
	return m.getTUILayout(m.input.Height()).JumpToBottom.Y
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

func (m model) getMainPaneWidth() int {
	return getMainPaneWidth(m.width)
}

func (m model) getTUILayout(inputHeight int) tuiLayout {
	return getTUILayoutWithInputChromeHeight(
		m.width,
		m.height,
		inputHeight,
		m.getInputChromeHeightForValue(m.input.Value()),
	)
}
