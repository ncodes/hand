package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentsession "github.com/wandxy/hand/pkg/agent/session"

	storage "github.com/wandxy/hand/internal/state/core"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

func TestNewSessionStoreImplementsAgentSessionInterfaces(t *testing.T) {
	var store any = NewSessionStore(&sessionManagerStub{})

	_, ok := store.(agentsession.Store)
	require.True(t, ok)

	_, ok = store.(agentsession.TraceRecorder)
	require.True(t, ok)
}

func TestSessionStoreResolveConvertsSessionShape(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	store := NewSessionStore(&sessionManagerStub{
		ResolveFunc: func(_ context.Context, id string) (storage.Session, error) {
			require.Equal(t, "ses_test", id)
			return storage.Session{
				CreatedAt:                  now,
				Compaction:                 storage.SessionCompaction{Status: storage.CompactionStatusSucceeded, TargetOffset: 7},
				ID:                         id,
				EpisodicCheckpointOffset:   3,
				LastPromptTokens:           99,
				ReflectionCheckpointOffset: 4,
				Title:                      "Title",
				TitleSource:                storage.SessionTitleSourceGenerated,
				UpdatedAt:                  now.Add(time.Minute),
			}, nil
		},
	})

	session, err := store.Resolve(context.Background(), "ses_test")
	require.NoError(t, err)
	require.Equal(t, agentsession.Session{
		CreatedAt:                  now,
		Compaction:                 agentsession.Compaction{Status: agentsession.CompactionStatusSucceeded, TargetOffset: 7},
		ID:                         "ses_test",
		EpisodicCheckpointOffset:   3,
		LastPromptTokens:           99,
		ReflectionCheckpointOffset: 4,
		Title:                      "Title",
		TitleSource:                storage.SessionTitleSourceGenerated,
		UpdatedAt:                  now.Add(time.Minute),
	}, session)
}

func TestSessionStoreGetMessagesConvertsQuery(t *testing.T) {
	store := NewSessionStore(&sessionManagerStub{
		GetMessagesFunc: func(_ context.Context, id string, query storage.MessageQueryOptions) ([]handmsg.Message, error) {
			require.Equal(t, "default", id)
			require.Equal(t, storage.MessageQueryOptions{
				Archived: true,
				Limit:    5,
				Name:     "tool",
				Order:    storage.MessageOrderDesc,
				Offset:   2,
				Role:     handmsg.RoleAssistant,
			}, query)

			return []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "hello"}}, nil
		},
	})

	messages, err := store.GetMessages(context.Background(), agentsession.DefaultID, agentsession.MessageQuery{
		Archived: true,
		Limit:    5,
		Name:     "tool",
		Order:    agentsession.MessageOrderDesc,
		Offset:   2,
		Role:     handmsg.RoleAssistant,
	})
	require.NoError(t, err)
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "hello"}}, messages)
}

func TestSessionStoreAppendMessagesDelegates(t *testing.T) {
	expected := []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}
	store := NewSessionStore(&sessionManagerStub{
		AppendMessagesFunc: func(_ context.Context, id string, messages []handmsg.Message) error {
			require.Equal(t, "default", id)
			require.Equal(t, expected, messages)
			return nil
		},
	})

	require.NoError(t, store.AppendMessages(context.Background(), agentsession.DefaultID, expected))
}

func TestSessionStoreUpdateLastPromptTokensDelegates(t *testing.T) {
	store := NewSessionStore(&sessionManagerStub{
		UpdateLastPromptTokensFunc: func(_ context.Context, id string, tokens int) error {
			require.Equal(t, "default", id)
			require.Equal(t, 123, tokens)
			return nil
		},
	})

	require.NoError(t, store.UpdateLastPromptTokens(context.Background(), agentsession.DefaultID, 123))
}

func TestSessionStoreAppendTraceEventConvertsTraceShape(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	store := NewSessionStore(&sessionManagerStub{
		AppendTraceEventFunc: func(_ context.Context, event storage.TraceEvent) (storage.TraceEvent, error) {
			require.Equal(t, storage.TraceEvent{
				SessionID: "default",
				Type:      "model.request",
				Timestamp: now,
				Payload:   map[string]any{"ok": true},
			}, event)

			event.ID = 9
			event.Sequence = 3
			return event, nil
		},
	})

	event, err := store.AppendTraceEvent(context.Background(), agentsession.TraceEvent{
		SessionID: "default",
		Type:      "model.request",
		Timestamp: now,
		Payload:   map[string]any{"ok": true},
	})
	require.NoError(t, err)
	require.Equal(t, agentsession.TraceEvent{
		ID:        9,
		SessionID: "default",
		Sequence:  3,
		Type:      "model.request",
		Timestamp: now,
		Payload:   map[string]any{"ok": true},
	}, event)
}

type sessionManagerStub struct {
	ResolveFunc                func(context.Context, string) (storage.Session, error)
	GetMessagesFunc            func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error)
	AppendMessagesFunc         func(context.Context, string, []handmsg.Message) error
	UpdateLastPromptTokensFunc func(context.Context, string, int) error
	AppendTraceEventFunc       func(context.Context, storage.TraceEvent) (storage.TraceEvent, error)
}

func (s *sessionManagerStub) Resolve(ctx context.Context, id string) (storage.Session, error) {
	return s.ResolveFunc(ctx, id)
}

func (s *sessionManagerStub) GetMessages(
	ctx context.Context,
	id string,
	query storage.MessageQueryOptions,
) ([]handmsg.Message, error) {
	return s.GetMessagesFunc(ctx, id, query)
}

func (s *sessionManagerStub) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	return s.AppendMessagesFunc(ctx, id, messages)
}

func (s *sessionManagerStub) UpdateLastPromptTokens(ctx context.Context, id string, tokens int) error {
	return s.UpdateLastPromptTokensFunc(ctx, id, tokens)
}

func (s *sessionManagerStub) AppendTraceEvent(
	ctx context.Context,
	event storage.TraceEvent,
) (storage.TraceEvent, error) {
	return s.AppendTraceEventFunc(ctx, event)
}
