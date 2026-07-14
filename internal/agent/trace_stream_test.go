package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/mocks"
	"github.com/wandxy/morph/internal/trace"
)

func TestIsStreamableTraceEvent_IncludesLiveToolOutputSafety(t *testing.T) {
	require.True(t, isStreamableTraceEvent(trace.EvtToolOutputSafetyApplied))
}

func TestIsStreamableTraceEvent_IncludesPermissionApprovalLifecycle(t *testing.T) {
	require.True(t, isStreamableTraceEvent(trace.EvtPermissionApprovalChanged))
}

func TestTraceFanoutSessionStreamsOnlyAllowedRedactedEvents(t *testing.T) {
	primary := &mocks.TraceSessionStub{SessionID: "primary"}
	var streamed []trace.Event
	session := newFanoutTraceSession(primary, "fallback", func(event trace.Event) {
		streamed = append(streamed, event)
	})

	require.Equal(t, "primary", session.ID())
	session.Record(trace.EvtToolInvocationStarted, map[string]any{"token": "secret"})
	session.Record(trace.EvtModelRequest, map[string]any{"ignored": true})
	session.Record(trace.EvtPermissionApprovalChanged, trace.PermissionApprovalPayload{
		RequestID: "approval_1",
		Status:    "pending",
	})
	session.Close()

	require.True(t, primary.Closed)
	require.Len(t, primary.Events, 3)
	require.Len(t, streamed, 2)
	require.Equal(t, trace.EvtToolInvocationStarted, streamed[0].Type)
	require.Equal(t, trace.EvtPermissionApprovalChanged, streamed[1].Type)
	require.Equal(t, "primary", streamed[0].SessionID)
	require.False(t, isStreamableTraceEvent(trace.EvtModelRequest))
	require.True(t, isStreamableTraceEvent(trace.EvtFinalAssistantResponse))
	require.Equal(t, trace.NoopSession().ID(), newFanoutTraceSession(nil, "", nil).ID())
	require.Equal(t, "", newFanoutTraceSession(&mocks.TraceSessionStub{}, "fallback", nil).ID())

	var streamedWithFallback []trace.Event
	fallbackSession := newFanoutTraceSession(nil, "fallback", func(event trace.Event) {
		streamedWithFallback = append(streamedWithFallback, event)
	})
	require.Equal(t, "fallback", fallbackSession.ID())
	fallbackSession.Record(trace.EvtSessionFailed, nil)
	require.Equal(t, "fallback", streamedWithFallback[0].SessionID)
}
