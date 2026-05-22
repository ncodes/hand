package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type slashCommandDefinition struct {
	Name        string
	Description string
}

var slashCommandDefinitions = []slashCommandDefinition{
	{Name: "clear", Description: "Clear the transcript"},
	{Name: "copy", Description: "Copy the transcript"},
	{Name: "help", Description: "Show supported commands"},
}

func (m *model) handleSlashCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	switch input.Name {
	case "clear":
		m.applyAction(clearTranscriptAction{})
		cmd = m.setStatus("transcript cleared")
	case "help":
		m.applyAction(appendTranscriptCellAction{Cell: systemTranscriptCell{text: getSlashCommandHelpText()}})
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

func getSlashCommandHelpText() string {
	commands := make([]string, 0, len(slashCommandDefinitions))
	for _, command := range slashCommandDefinitions {
		commands = append(commands, "/"+command.Name)
	}

	return "Commands: " + strings.Join(commands, ", ")
}

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
