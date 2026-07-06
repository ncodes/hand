package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"github.com/wandxy/morph/pkg/str"
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
		stringValue2 := str.String(ansi.Strip(m.transcript.GetContent()))
		return stringValue2.Trim()
	}

	parts := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell != nil && !cell.IsEmpty() {
			parts = append(parts, cell.PlainText())
		}
	}
	stringValue1 := str.String(strings.Join(parts, "\n\n"))
	return stringValue1.Trim()
}
