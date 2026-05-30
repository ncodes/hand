package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	storage "github.com/wandxy/hand/internal/state/core"
)

type sessionCreator interface {
	CreateSession(context.Context, string) (storage.Session, error)
}

type newChatCompletedMsg struct {
	Session storage.Session
	Err     error
}

func (m *model) startNewChat() tea.Cmd {
	client, ok := m.chatClient.(sessionCreator)
	if m.chatClient == nil || !ok {
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

		session, err := client.CreateSession(ctx, "")
		return newChatCompletedMsg{Session: session, Err: err}
	}
}

func (m *model) completeNewChat(msg newChatCompletedMsg) tea.Cmd {
	if msg.Err != nil {
		return m.setStatus("new chat failed")
	}
	if strings.TrimSpace(msg.Session.ID) == "" {
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

	return tea.Batch(
		m.setStatus("new chat created"),
		loadSessionContextCmd(m.chatCtx, m.contextLoader, m.getCurrentSessionID()),
	)
}
