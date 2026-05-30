package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type sessionCompactor interface {
	CompactSession(context.Context, string) (rpcclient.CompactSessionResult, error)
}

type compactSessionCompletedMsg struct {
	Result rpcclient.CompactSessionResult
	Err    error
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
