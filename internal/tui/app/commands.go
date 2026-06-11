package tui

import tea "charm.land/bubbletea/v2"

type slashCommandDefinition struct {
	Name        string
	Description string
}

var slashCommandDefinitions = []slashCommandDefinition{
	{Name: "changelog", Description: "Show the latest changelog entry"},
	{Name: "chats", Description: "Show recent chat sessions"},
	{Name: "clear", Description: "Clear the transcript"},
	{Name: "compact", Description: "Compact the current session"},
	{Name: "copy", Description: "Copy the transcript"},
	{Name: "models", Description: "Show supported models"},
	{Name: "new-chat", Description: "Start a new chat session"},
	{Name: "archive", Description: "Show archived chat sessions"},
	{Name: "providers", Description: "Show supported model providers"},
	{Name: "setup", Description: "Open setup"},
}

func (m *model) handleSlashCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	switch input.Name {
	case "archive":
		cmd = m.startArchiveCommand()
	case "changelog":
		cmd = m.showChangelogCommand()
	case "chats":
		cmd = m.startChatsCommand()
	case "clear":
		m.applyAction(clearTranscriptAction{})
		cmd = m.setStatus("transcript cleared")
	case "compact":
		cmd = m.startCompactSession()
	case "models":
		cmd = m.startModelsCommand()
	case "providers":
		cmd = m.startProvidersCommand()
	case "setup":
		cmd = m.startProfileSetup(true)
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
