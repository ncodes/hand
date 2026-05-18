package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// newTranscript creates the scrollable conversation viewport.
func newTranscript() viewport.Model {
	transcript := viewport.New()
	transcript.SoftWrap = true

	return transcript
}

// renderTranscript draws the conversation viewport.
func (m model) renderTranscript() string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(max(m.transcript.Height(), 1)).
		Render(m.transcript.View())
}

func (m *model) setTranscriptContent() {
	m.clearTranscriptSelection()
	m.transcript.SetContent(m.renderTranscriptContent())
	m.transcript.GotoBottom()
}

func (m *model) setTranscriptContentForActiveTurn() {
	offset := m.transcript.YOffset()
	m.clearTranscriptSelection()
	m.transcript.SetContent(m.renderTranscriptContent())
	m.transcript.SetYOffset(offset)
}

func (m *model) renderTranscriptContent() string {
	cells := make([]string, 0, len(m.messages)+1)
	cells = append(cells, m.messages...)
	if strings.TrimSpace(m.live) != "" {
		cells = append(cells, m.live)
	}
	if len(cells) == 0 && m.showIntro {
		cells = append(cells, "Welcome to Hand TUI.\n\nThe interactive shell is ready.")
	}
	if len(cells) > 0 {
		m.showIntro = false
	}

	content := strings.TrimSpace(m.renderHeader())
	if cellsText := strings.TrimSpace(renderTranscriptCells(cells)); cellsText != "" {
		content = strings.Join([]string{content, cellsText}, "\n\n")
	}

	return content
}

func (m *model) updateTranscript(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)

	return *m, cmd
}

func (m *model) scrollTranscriptWithKey(msg tea.KeyPressMsg) bool {
	switch msg.Key().Code {
	case tea.KeyPgUp:
		m.transcript.PageUp()
	case tea.KeyPgDown:
		m.transcript.PageDown()
	case tea.KeyHome:
		m.transcript.GotoTop()
	case tea.KeyEnd:
		m.transcript.GotoBottom()
	default:
		return false
	}

	return true
}
