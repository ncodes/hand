package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type slashCommandDefinition struct {
	Name        string
	Description string
}

var slashCommandDefinitions = []slashCommandDefinition{
	{Name: "clear", Description: "Clear the transcript"},
	{Name: "compact", Description: "Compact the current session"},
	{Name: "copy", Description: "Copy the transcript"},
	{Name: "help", Description: "Show supported commands"},
}

type sessionCompactor interface {
	CompactSession(context.Context, string) (rpcclient.CompactSessionResult, error)
}

type compactSessionCompletedMsg struct {
	Result rpcclient.CompactSessionResult
	Err    error
}

func (m *model) handleSlashCommand(input composerInput) tea.Cmd {
	var cmd tea.Cmd
	switch input.Name {
	case "clear":
		m.applyAction(clearTranscriptAction{})
		cmd = m.setStatus("transcript cleared")
	case "compact":
		cmd = m.startCompactSession()
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

func (m *model) startCompactSession() tea.Cmd {
	client, ok := m.chatClient.(sessionCompactor)
	if m.chatClient == nil || !ok {
		return m.setStatus("compaction unavailable")
	}

	return tea.Batch(
		m.startManualCompactionStatus(),
		m.setStatus("compaction started"),
		compactSessionCmd(m.chatCtx, client, m.getCurrentSessionID()),
	)
}

func (m model) getCurrentSessionID() string {
	sessionID := strings.TrimSpace(m.sessionID)
	if sessionID != "" {
		return sessionID
	}

	return defaultSessionID
}

func compactSessionCmd(ctx context.Context, client sessionCompactor, sessionID string) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		result, err := client.CompactSession(ctx, strings.TrimSpace(sessionID))
		return compactSessionCompletedMsg{Result: result, Err: err}
	}
}

func (m *model) completeCompactSession(msg compactSessionCompletedMsg) tea.Cmd {
	m.completeManualCompactionStatus(msg.Err)
	if msg.Err != nil {
		return m.setStatus("compaction failed")
	}

	return tea.Batch(
		m.setStatus("session compacted"),
		loadSessionContextCmd(m.chatCtx, m.contextLoader, m.getCurrentSessionID()),
	)
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
