package tui

import tea "charm.land/bubbletea/v2"

func (m *model) handleLocalCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	if !m.allowShell {
		cmd = m.setStatus("local commands are disabled")
		m.applyAction(appendTranscriptCellAction{Cell: systemTranscriptCell{text: "Local command blocked: !" + input.Args}})
		if m.responding {
			m.setTranscriptContentForResponseUpdate()
		} else {
			m.setTranscriptContent()
		}
		return cmd
	}

	cmd = m.setStatus("local command execution is not connected yet")
	m.applyAction(appendTranscriptCellAction{Cell: systemTranscriptCell{text: "Local command queued: !" + input.Args}})
	if m.responding {
		m.setTranscriptContentForResponseUpdate()
	} else {
		m.setTranscriptContent()
	}
	return cmd
}
