package tui

import (
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// newTranscript creates the scrollable conversation viewport.
func newTranscript() viewport.Model {
	transcript := viewport.New()
	transcript.SoftWrap = true
	transcript.SetContent("Welcome to Hand TUI.\n\nThe interactive shell is ready.")

	return transcript
}

// renderTranscript draws the conversation viewport.
func (m model) renderTranscript() string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(max(m.transcript.Height(), 1)).
		Render(m.transcript.View())
}
