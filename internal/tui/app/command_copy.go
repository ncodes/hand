package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"github.com/wandxy/morph/pkg/stringx"
)

var writeClipboard = clipboard.WriteAll

func (m *model) copyTranscript() tea.Cmd {
	text := m.transcriptText()
	if text == "" {
		return m.setStatus("transcript is empty")
	}

	return m.runEffect(copyTranscriptEffect{Text: text})
}

func (m model) transcriptText() string {
	cells := make([]transcriptCell, 0, len(m.messages)+1)
	cells = append(cells, m.messages...)
	if m.live != nil && !m.live.IsEmpty() {
		cells = append(cells, m.live)
	}
	if len(cells) == 0 {
		return stringx.String(ansi.Strip(m.transcript.GetContent())).Trim()
	}

	parts := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell != nil && !cell.IsEmpty() {
			parts = append(parts, cell.PlainText())
		}
	}

	return stringx.String(strings.Join(parts, "\n\n")).Trim()
}
