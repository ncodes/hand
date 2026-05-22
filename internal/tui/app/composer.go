package tui

import (
	tea "charm.land/bubbletea/v2"

	tuicomposer "github.com/wandxy/hand/internal/tui/composer"
)

const (
	composerInputEmpty        = tuicomposer.InputEmpty
	composerInputPrompt       = tuicomposer.InputPrompt
	composerInputCommand      = tuicomposer.InputCommand
	composerInputLocalCommand = tuicomposer.InputLocalCommand
)

type composerInput = tuicomposer.Input

func parseComposerInput(value string) composerInput {
	return tuicomposer.ParseInput(value)
}

func (m model) parseComposerInputForSubmit() composerInput {
	input := parseComposerInput(m.input.Value())
	if input.Kind != composerInputCommand {
		return input
	}
	if input.Name == "" && m.commandMenuSelected == 0 {
		return input
	}

	command, ok := m.getSelectedSlashCommand()
	if !ok {
		return input
	}

	return composerInput{
		Kind: composerInputCommand,
		Text: "/" + command.Name,
		Name: command.Name,
	}
}

func normalizeComposerPaste(value string) string {
	return tuicomposer.NormalizePaste(value)
}

// submitPrompt routes a non-empty composer value to prompt or command handling.
func (m *model) submitPrompt() tea.Cmd {
	input := m.parseComposerInputForSubmit()
	if input.Kind == composerInputEmpty {
		return nil
	}
	if input.Kind == composerInputPrompt && m.responding {
		return m.setStatus("response already in progress")
	}

	cmd := m.addPromptHistory(input.Text)
	promptSubmitted := false
	switch input.Kind {
	case composerInputPrompt:
		m.applyAction(appendTranscriptCellAction{Cell: userTranscriptCell{text: input.Text}})
		m.clearComposer()
		m.resize()
		m.setTranscriptContent()
		cmd = tea.Batch(cmd, m.runEffect(sendPromptEffect{Text: input.Text}))
		promptSubmitted = true
	case composerInputCommand:
		cmd = tea.Batch(cmd, m.handleSlashCommand(input))
	case composerInputLocalCommand:
		cmd = tea.Batch(cmd, m.handleLocalCommand(input))
	}
	if promptSubmitted {
		return cmd
	} else if m.responding {
		m.setTranscriptContentForResponseUpdate()
	} else {
		m.setTranscriptContent()
	}
	m.clearComposer()
	m.resize()

	return cmd
}

func (m *model) clearComposer() {
	m.input.SetValue("")
	m.commandMenuOffset = 0
	m.historyAt = len(m.history)
	m.draft = ""
}
