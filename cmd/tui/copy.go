package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

var writeClipboard = clipboard.WriteAll

func (m *model) copyTranscript() tea.Cmd {
	text := m.transcriptText()
	if text == "" {
		return m.setStatus("transcript is empty")
	}
	if err := writeClipboard(text); err != nil {
		return m.setStatus("copy failed")
	}

	return m.setStatus("transcript copied")
}

func (m model) transcriptText() string {
	cells := make([]transcriptCell, 0, len(m.messages)+1)
	cells = append(cells, m.messages...)
	if m.live != nil && !m.live.IsEmpty() {
		cells = append(cells, m.live)
	}
	if len(cells) == 0 {
		return strings.TrimSpace(ansi.Strip(m.transcript.GetContent()))
	}

	parts := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell != nil && !cell.IsEmpty() {
			parts = append(parts, cell.PlainText())
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
