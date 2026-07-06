package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(m.sessionID)
	sessionID := stringValue1.Trim()
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
		stringValue2 := str.String(sessionID)
		result, err := client.Compact(ctx, stringValue2.Trim())
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
