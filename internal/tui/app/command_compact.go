package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/pkg/stringx"
)

type sessionCompactor interface {
	Compact(context.Context, string) (rpcclient.CompactSessionResult, error)
}

type compactSessionCompletedMsg struct {
	Result rpcclient.CompactSessionResult
	Err    error
}

func (m *model) startCompactSession() tea.Cmd {
	client, ok := m.sessionClient.(sessionCompactor)
	if m.sessionClient == nil || !ok {
		return m.setStatus("compaction unavailable")
	}

	return tea.Batch(
		m.startManualCompactionStatus(),
		m.setStatus("compaction started"),
		compactSessionCmd(m.chatCtx, client, m.getCurrentSessionID()),
	)
}

func (m model) getCurrentSessionID() string {
	sessionID := stringx.String(m.sessionID).Trim()
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

		result, err := client.Compact(ctx, stringx.String(sessionID).Trim())
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
