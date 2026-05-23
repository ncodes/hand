package agent

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/trace"
)

func TestFanoutTraceSession_RecordFansOutStreamableEvents(t *testing.T) {
	primary := &recordingTraceSession{id: "default"}
	var streamed []trace.Event
	session := newFanoutTraceSession(primary, "fallback", func(event trace.Event) {
		streamed = append(streamed, event)
	})

	session.Record(trace.EvtInputSafetyBlocked, map[string]any{
		"blocked": true,
		"secret":  "SECRET=example",
	})

	require.Len(t, primary.events, 1)
	require.Equal(t, trace.EvtInputSafetyBlocked, primary.events[0].Type)
	require.Len(t, streamed, 1)
	require.Equal(t, "default", streamed[0].SessionID)
	require.Equal(t, trace.EvtInputSafetyBlocked, streamed[0].Type)
	require.NotContains(t, fmt.Sprintf("%#v", streamed[0].Payload), "SECRET=example")
}

func TestFanoutTraceSession_RecordFansOutCompactionEvents(t *testing.T) {
	primary := &recordingTraceSession{id: "default"}
	var streamed []trace.Event
	session := newFanoutTraceSession(primary, "fallback", func(event trace.Event) {
		streamed = append(streamed, event)
	})

	session.Record(trace.EvtContextCompactionRunning, trace.CompactionEventPayload{
		SessionID: "default",
		Status:    "running",
		Auto:      true,
	})

	require.Len(t, primary.events, 1)
	require.Equal(t, trace.EvtContextCompactionRunning, primary.events[0].Type)
	require.Len(t, streamed, 1)
	require.Equal(t, trace.EvtContextCompactionRunning, streamed[0].Type)
	require.Equal(t, "default", streamed[0].SessionID)
}

func TestFanoutTraceSession_RecordSkipsNonStreamableEvents(t *testing.T) {
	primary := &recordingTraceSession{id: "default"}
	var streamed []trace.Event
	session := newFanoutTraceSession(primary, "fallback", func(event trace.Event) {
		streamed = append(streamed, event)
	})

	session.Record(trace.EvtModelRequest, map[string]any{"authorization": "Bearer secret"})

	require.Len(t, primary.events, 1)
	require.Empty(t, streamed)
}

func TestFanoutTraceSession_ReturnsPrimaryWhenNoCallbackIsProvided(t *testing.T) {
	primary := &recordingTraceSession{id: "default"}

	session := newFanoutTraceSession(primary, "fallback", nil)

	require.Same(t, primary, session)
}

func TestFanoutTraceSession_ReturnsNoopWhenNoPrimaryOrCallbackIsProvided(t *testing.T) {
	session := newFanoutTraceSession(nil, "fallback", nil)

	require.NotNil(t, session)
	require.Empty(t, session.ID())
	require.NotPanics(t, func() {
		session.Record(trace.EvtSessionFailed, map[string]any{"error": "boom"})
		session.Close()
	})
}

func TestFanoutTraceSession_UsesNoopPrimaryWhenCallbackIsProvidedWithoutPrimary(t *testing.T) {
	var streamed []trace.Event
	session := newFanoutTraceSession(nil, "fallback", func(event trace.Event) {
		streamed = append(streamed, event)
	})

	session.Record(trace.EvtSessionFailed, map[string]any{"error": "boom"})

	require.Equal(t, "fallback", session.ID())
	require.Len(t, streamed, 1)
	require.Equal(t, "fallback", streamed[0].SessionID)
}

func TestFanoutTraceSession_IDFallsBackToProvidedSessionID(t *testing.T) {
	session := newFanoutTraceSession(trace.NoopSession(), "default", func(trace.Event) {})

	require.Equal(t, "default", session.ID())
}

func TestFanoutTraceSession_CloseClosesPrimary(t *testing.T) {
	primary := &recordingTraceSession{id: "default"}
	session := newFanoutTraceSession(primary, "fallback", func(trace.Event) {})

	session.Close()

	require.True(t, primary.closed)
}

type recordingTraceSession struct {
	id     string
	closed bool
	events []trace.Event
}

func (s *recordingTraceSession) ID() string {
	return s.id
}

func (s *recordingTraceSession) Record(eventType string, payload any) {
	s.events = append(s.events, trace.Event{Type: eventType, Payload: payload})
}

func (s *recordingTraceSession) Close() {
	s.closed = true
}
