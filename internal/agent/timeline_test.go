package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	storagemock "github.com/wandxy/hand/internal/state/mock"
)

func TestAgent_GetSessionTimelineReturnsPagedMessagesAndTraceEvents(t *testing.T) {
	ctx := context.Background()
	manager := mustNewStateManager(t)
	session, err := manager.Resolve(ctx, "")
	require.NoError(t, err)
	firstMessageAt := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	require.NoError(t, manager.AppendMessages(ctx, session.ID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "one", CreatedAt: firstMessageAt},
		{Role: handmsg.RoleAssistant, Content: "two", CreatedAt: firstMessageAt.Add(time.Minute)},
		{Role: handmsg.RoleTool, Name: "read_file", ToolCallID: "call_1", Content: "three", CreatedAt: firstMessageAt.Add(2 * time.Minute)},
	}))
	traceAt := time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC)
	for _, event := range []storage.TraceEvent{
		{SessionID: session.ID, Type: "first", Timestamp: traceAt, Payload: map[string]any{"n": 1}},
		{SessionID: session.ID, Type: "second", Timestamp: traceAt.Add(time.Second), Payload: map[string]any{"n": 2}},
		{SessionID: session.ID, Type: "third", Timestamp: traceAt.Add(2 * time.Second), Payload: map[string]any{"n": 3}},
	} {
		_, err := manager.AppendTraceEvent(ctx, event)
		require.NoError(t, err)
	}
	agent := &Agent{stateMgr: manager}

	timeline, err := agent.GetSessionTimeline(ctx, SessionTimelineOptions{
		MessageOffset: 1,
		MessageLimit:  1,
		TraceOffset:   2,
		TraceLimit:    1,
	})

	require.NoError(t, err)
	require.Equal(t, session.ID, timeline.SessionID)
	require.True(t, timeline.MessagesHasMore)
	require.Len(t, timeline.Messages, 1)
	require.Equal(t, 1, timeline.Messages[0].Offset)
	require.Equal(t, "two", timeline.Messages[0].Message.Content)
	require.True(t, timeline.TracesHasMore)
	require.False(t, timeline.TracesTruncatedBefore)
	require.Equal(t, 2, timeline.FirstTraceSequence)
	require.Equal(t, 2, timeline.LastTraceSequence)
	require.Len(t, timeline.TraceEvents, 1)
	require.Equal(t, 2, timeline.TraceEvents[0].Event.Sequence)
	require.Equal(t, "second", timeline.TraceEvents[0].Event.Type)
}

func TestAgent_GetSessionTimelineDefaultsToRecentMessageTail(t *testing.T) {
	ctx := context.Background()
	manager := mustNewStateManager(t)
	session, err := manager.Resolve(ctx, "")
	require.NoError(t, err)

	messages := make([]handmsg.Message, 0, defaultSessionTimelineLimit+2)
	for index := 0; index < defaultSessionTimelineLimit+2; index++ {
		messages = append(messages, handmsg.Message{
			Role:    handmsg.RoleUser,
			Content: "message " + string(rune('A'+index%26)),
		})
	}
	messages[len(messages)-2].Content = "latest user"
	messages[len(messages)-1].Role = handmsg.RoleAssistant
	messages[len(messages)-1].Content = "latest assistant"
	require.NoError(t, manager.AppendMessages(ctx, session.ID, messages))

	timeline, err := (&Agent{stateMgr: manager}).GetSessionTimeline(ctx, SessionTimelineOptions{})

	require.NoError(t, err)
	require.True(t, timeline.MessagesHasMore)
	require.Len(t, timeline.Messages, defaultSessionTimelineLimit)
	require.Equal(t, 2, timeline.Messages[0].Offset)
	require.Equal(t, "latest user", timeline.Messages[len(timeline.Messages)-2].Message.Content)
	require.Equal(t, "latest assistant", timeline.Messages[len(timeline.Messages)-1].Message.Content)
}

func TestAgent_GetSessionTimelineReportsRetainedTraceGap(t *testing.T) {
	ctx := context.Background()
	manager := mustNewStateManager(t)
	session, err := manager.Resolve(ctx, "")
	require.NoError(t, err)
	traceAt := time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC)
	for _, eventType := range []string{"first", "second", "third"} {
		_, err := manager.AppendTraceEvent(ctx, storage.TraceEvent{
			SessionID: session.ID,
			Type:      eventType,
			Timestamp: traceAt,
			Payload:   map[string]any{"type": eventType},
		})
		require.NoError(t, err)
	}
	require.NoError(t, manager.PruneTraceEvents(ctx, session.ID, 2))

	timeline, err := (&Agent{stateMgr: manager}).GetSessionTimeline(ctx, SessionTimelineOptions{
		TraceLimit: 1,
	})

	require.NoError(t, err)
	require.True(t, timeline.TracesTruncatedBefore)
	require.Equal(t, 2, timeline.FirstTraceSequence)
	require.Equal(t, 2, timeline.LastTraceSequence)
	require.Len(t, timeline.TraceEvents, 1)
}

func TestAgent_GetSessionTimelineReportsRetainedTraceGapForEmptyPage(t *testing.T) {
	ctx := context.Background()
	manager := mustNewStateManager(t)
	session, err := manager.Resolve(ctx, "")
	require.NoError(t, err)
	for _, eventType := range []string{"first", "second", "third"} {
		_, err := manager.AppendTraceEvent(ctx, storage.TraceEvent{
			SessionID: session.ID,
			Type:      eventType,
			Payload:   map[string]any{"type": eventType},
		})
		require.NoError(t, err)
	}
	require.NoError(t, manager.PruneTraceEvents(ctx, session.ID, 2))

	timeline, err := (&Agent{stateMgr: manager}).GetSessionTimeline(ctx, SessionTimelineOptions{
		TraceOffset: 10,
		TraceLimit:  1,
	})

	require.NoError(t, err)
	require.True(t, timeline.TracesTruncatedBefore)
	require.Empty(t, timeline.TraceEvents)
	require.Zero(t, timeline.FirstTraceSequence)
	require.Zero(t, timeline.LastTraceSequence)
}

func TestAgent_GetSessionTimelineReturnsMessagesWhenTraceStoreIsUnsupported(t *testing.T) {
	ctx := context.Background()
	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	agent := &Agent{stateMgr: manager}

	timeline, err := agent.GetSessionTimeline(ctx, SessionTimelineOptions{})

	require.NoError(t, err)
	require.Len(t, timeline.Messages, 1)
	require.Equal(t, "hello", timeline.Messages[0].Message.Content)
	require.Empty(t, timeline.TraceEvents)
	require.False(t, timeline.TracesHasMore)
}

func TestAgent_GetSessionTimelineReturnsEmptyStreams(t *testing.T) {
	ctx := context.Background()
	agent := &Agent{stateMgr: mustNewStateManager(t)}

	timeline, err := agent.GetSessionTimeline(ctx, SessionTimelineOptions{})

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, timeline.SessionID)
	require.Empty(t, timeline.Messages)
	require.Empty(t, timeline.TraceEvents)
	require.False(t, timeline.MessagesHasMore)
	require.False(t, timeline.TracesHasMore)
	require.False(t, timeline.TracesTruncatedBefore)
	require.Zero(t, timeline.FirstTraceSequence)
	require.Zero(t, timeline.LastTraceSequence)
}

func TestAgent_GetSessionTimelineReturnsUntruncatedFirstTrace(t *testing.T) {
	ctx := context.Background()
	manager := mustNewStateManager(t)
	session, err := manager.Resolve(ctx, "")
	require.NoError(t, err)
	_, err = manager.AppendTraceEvent(ctx, storage.TraceEvent{
		SessionID: session.ID,
		Type:      "first",
	})
	require.NoError(t, err)

	timeline, err := (&Agent{stateMgr: manager}).GetSessionTimeline(ctx, SessionTimelineOptions{
		TraceLimit: 1,
	})

	require.NoError(t, err)
	require.False(t, timeline.TracesTruncatedBefore)
	require.Equal(t, 1, timeline.FirstTraceSequence)
	require.Equal(t, 1, timeline.LastTraceSequence)
	require.Len(t, timeline.TraceEvents, 1)
}

func TestAgent_GetSessionTimelineValidatesPagingOptions(t *testing.T) {
	agent := &Agent{stateMgr: mustNewStateManager(t)}

	cases := []struct {
		name string
		opts SessionTimelineOptions
		err  string
	}{
		{
			name: "message offset",
			opts: SessionTimelineOptions{MessageOffset: -1},
			err:  "message offset must be greater than or equal to zero",
		},
		{
			name: "message limit",
			opts: SessionTimelineOptions{MessageLimit: -1},
			err:  "message limit must be greater than or equal to zero",
		},
		{
			name: "trace offset",
			opts: SessionTimelineOptions{TraceOffset: -1},
			err:  "trace offset must be greater than or equal to zero",
		},
		{
			name: "trace limit",
			opts: SessionTimelineOptions{TraceLimit: -1},
			err:  "trace limit must be greater than or equal to zero",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := agent.GetSessionTimeline(context.Background(), tt.opts)

			require.EqualError(t, err, tt.err)
		})
	}
}

func TestAgent_GetSessionTimelineReturnsStateErrors(t *testing.T) {
	_, err := (*Agent)(nil).GetSessionTimeline(context.Background(), SessionTimelineOptions{})
	require.EqualError(t, err, "agent is required")

	_, err = (&Agent{}).GetSessionTimeline(context.Background(), SessionTimelineOptions{})
	require.EqualError(t, err, "state manager is required")

	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, errors.New("resolve failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	_, err = (&Agent{stateMgr: manager}).GetSessionTimeline(context.Background(), SessionTimelineOptions{})
	require.EqualError(t, err, "resolve failed")
}

func TestAgent_GetSessionTimelineReturnsMessageAndTraceErrors(t *testing.T) {
	manager, err := statemanager.NewManager(&timelineErrorStore{
		messageErr: errors.New("messages failed"),
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	_, err = (&Agent{stateMgr: manager}).GetSessionTimeline(context.Background(), SessionTimelineOptions{})
	require.EqualError(t, err, "messages failed")

	manager, err = statemanager.NewManager(&timelineErrorStore{
		traceErr: errors.New("traces failed"),
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	_, err = (&Agent{stateMgr: manager}).GetSessionTimeline(context.Background(), SessionTimelineOptions{})
	require.EqualError(t, err, "traces failed")

	manager, err = statemanager.NewManager(&timelineTraceGapErrorStore{}, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	_, err = (&Agent{stateMgr: manager}).GetSessionTimeline(context.Background(), SessionTimelineOptions{
		TraceOffset: 10,
	})
	require.EqualError(t, err, "trace gap check failed")

	manager, err = statemanager.NewManager(&timelineTraceGapUnsupportedStore{}, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	timeline, err := (&Agent{stateMgr: manager}).GetSessionTimeline(context.Background(), SessionTimelineOptions{
		TraceOffset: 10,
	})
	require.NoError(t, err)
	require.False(t, timeline.TracesTruncatedBefore)
}

func TestGetTimelineLimit_BoundsValues(t *testing.T) {
	require.Equal(t, defaultSessionTimelineLimit, getTimelineLimit(0))
	require.Equal(t, maxSessionTimelineLimit, getTimelineLimit(maxSessionTimelineLimit+1))
}
