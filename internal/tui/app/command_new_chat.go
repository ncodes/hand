package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	storage "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/pkg/stringx"
)

type sessionCreator interface {
	Create(context.Context, string) (storage.Session, error)
}

type newChatCompletedMsg struct {
	Session storage.Session
	Err     error
}

func (m *model) startNewChat() tea.Cmd {
	client, ok := m.sessionClient.(sessionCreator)
	if m.sessionClient == nil || !ok {
		return m.setStatus("new chat unavailable")
	}

	return tea.Batch(
		m.setStatus("creating new chat"),
		createNewChatCmd(m.chatCtx, client),
	)
}

func createNewChatCmd(ctx context.Context, client sessionCreator) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}

		session, err := client.Create(ctx, "")
		return newChatCompletedMsg{Session: session, Err: err}
	}
}

func (m *model) completeNewChat(msg newChatCompletedMsg) tea.Cmd {
	if msg.Err != nil {
		return m.setStatus("new chat failed")
	}
	if stringx.String(msg.Session.ID).Trim() == "" {
		return m.setStatus("new chat failed")
	}

	if m.responseCancel != nil {
		m.responseCancel()
	}
	m.applyAction(setSessionAction{
		ID:    msg.Session.ID,
		Title: getSessionDisplayName(msg.Session),
	})
	m.applyAction(clearTranscriptAction{})
	m.resetResponseState()
	m.setTranscriptContent()
	m.resize()

	cmds := []tea.Cmd{
		m.setStatus("new chat created"),
		loadSessionContextCmd(m.chatCtx, m.contextLoader, m.getCurrentSessionID()),
	}
	if err := saveLastSessionID(m.getCurrentSessionID()); err != nil {
		cmds = append(cmds, m.setStatus("last session unavailable"))
	}

	return tea.Batch(cmds...)
}
