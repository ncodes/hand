package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

var thinkingComposerInterval = 140 * time.Millisecond

var thinkingComposerBorderColors = []string{
	"36",
	"37",
	"38",
	"39",
	"75",
	"81",
}

type thinkingComposerTickMsg struct{}

func (m *model) startThinkingComposer() tea.Cmd {
	if !m.isThinkingComposerVisible() {
		m.thinkingComposerActive = false
		return nil
	}
	if m.thinkingComposerActive {
		return nil
	}

	m.thinkingComposerActive = true
	return thinkingComposerTickCmd()
}

func (m *model) updateThinkingComposer() (tea.Model, tea.Cmd) {
	if !m.isThinkingComposerVisible() {
		m.thinkingComposerActive = false
		return *m, nil
	}

	m.thinkingComposerFrame++
	return *m, thinkingComposerTickCmd()
}

func (m model) isThinkingComposerVisible() bool {
	return m.thinkingComposerEnabled && m.isModelThinking()
}

func (m model) isModelThinking() bool {
	return m.responding &&
		strings.TrimSpace(m.live) == "" &&
		!m.hasRunningToolTranscriptCells()
}

func getThinkingComposerBorderColor(frame int) string {
	if len(thinkingComposerBorderColors) == 0 {
		return "8"
	}

	index := frame % len(thinkingComposerBorderColors)
	if index < 0 {
		index += len(thinkingComposerBorderColors)
	}

	return thinkingComposerBorderColors[index]
}

func thinkingComposerTickCmd() tea.Cmd {
	return tea.Tick(thinkingComposerInterval, func(time.Time) tea.Msg {
		return thinkingComposerTickMsg{}
	})
}
