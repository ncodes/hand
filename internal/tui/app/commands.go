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
	{Name: "changelog", Description: "Show the latest changelog entry"},
	{Name: "clear", Description: "Clear the transcript"},
	{Name: "compact", Description: "Compact the current session"},
	{Name: "copy", Description: "Copy the transcript"},
	{Name: "help", Description: "Show supported commands"},
	{Name: "new-chat", Description: "Start a new chat session"},
}

func (m *model) handleSlashCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	switch input.Name {
	case "changelog":
		cmd = m.showChangelogCommand()
	case "clear":
		m.applyAction(clearTranscriptAction{})
		cmd = m.setStatus("transcript cleared")
	case "compact":
		cmd = m.startCompactSession()
	case "help":
		m.applyAction(appendTranscriptCellAction{Cell: systemTranscriptCell{text: getSlashCommandHelpText()}})
	case "copy":
		cmd = m.copyTranscript()
	case "new-chat":
		cmd = m.startNewChat()
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
