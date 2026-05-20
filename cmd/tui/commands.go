package tui

import tea "charm.land/bubbletea/v2"

func (m *model) handleSlashCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	switch input.Name {
	case "clear":
		m.messages = nil
		m.live = nil
		m.showIntro = false
		m.stream.Reset()
		m.clearReasoningTranscriptState()
		cmd = m.setStatus("transcript cleared")
	case "help":
		m.messages = append(m.messages, systemTranscriptCell{text: "Commands: /clear, /copy, /help"})
	case "copy":
		cmd = m.copyTranscript()
	case "":
		cmd = m.setStatus("empty command")
	default:
		cmd = m.setStatus("unknown command: /" + input.Name)
	}

	if m.responding {
		m.setTranscriptContentForResponseUpdate()
	} else {
		m.setTranscriptContent()
	}
	return cmd
}

func (m *model) handleLocalCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	if !m.allowShell {
		cmd = m.setStatus("local commands are disabled")
		m.messages = append(m.messages, systemTranscriptCell{text: "Local command blocked: !" + input.Args})
		if m.responding {
			m.setTranscriptContentForResponseUpdate()
		} else {
			m.setTranscriptContent()
		}
		return cmd
	}

	cmd = m.setStatus("local command execution is not connected yet")
	m.messages = append(m.messages, systemTranscriptCell{text: "Local command queued: !" + input.Args})
	if m.responding {
		m.setTranscriptContentForResponseUpdate()
	} else {
		m.setTranscriptContent()
	}
	return cmd
}
