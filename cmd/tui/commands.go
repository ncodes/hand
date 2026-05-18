package tui

import tea "charm.land/bubbletea/v2"

func (m *model) handleSlashCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	switch input.Name {
	case "clear":
		m.messages = nil
		m.live = ""
		m.showIntro = false
		m.stream.Reset()
		cmd = m.setStatus("transcript cleared")
	case "help":
		m.messages = append(m.messages, "Commands: /clear, /copy, /help")
	case "copy":
		cmd = m.copyTranscript()
	case "":
		cmd = m.setStatus("empty command")
	default:
		cmd = m.setStatus("unknown command: /" + input.Name)
	}

	if m.responding {
		m.setTranscriptContentForActiveTurn()
	} else {
		m.setTranscriptContent()
	}
	return cmd
}

func (m *model) handleLocalCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	if !m.allowShell {
		cmd = m.setStatus("local commands are disabled")
		m.messages = append(m.messages, "Local command blocked: !"+input.Args)
		if m.responding {
			m.setTranscriptContentForActiveTurn()
		} else {
			m.setTranscriptContent()
		}
		return cmd
	}

	cmd = m.setStatus("local command execution is not connected yet")
	m.messages = append(m.messages, "Local command queued: !"+input.Args)
	if m.responding {
		m.setTranscriptContentForActiveTurn()
	} else {
		m.setTranscriptContent()
	}
	return cmd
}
