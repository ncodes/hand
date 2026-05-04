package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type invalidTraceJSON struct{}

func (invalidTraceJSON) MarshalJSON() ([]byte, error) {
	return []byte("{"), nil
}

func TestTrace_NormalizeTraceTypes(t *testing.T) {
	require.Equal(t, []string{"model.request", "tool.done"}, NormalizeTraceTypes([]string{
		" model.request ",
		"",
		"tool.done",
		"model.request",
		"   ",
	}))
}

func TestTrace_EventMatchesQuery(t *testing.T) {
	event := TraceEvent{SessionID: DefaultSessionID, Type: "model.request"}

	require.True(t, TraceEventMatchesQuery(event, TraceQuery{}))
	require.True(t, TraceEventMatchesQuery(event, TraceQuery{SessionID: DefaultSessionID}))
	require.True(t, TraceEventMatchesQuery(event, TraceQuery{Types: []string{" model.request "}}))
	require.False(t, TraceEventMatchesQuery(event, TraceQuery{SessionID: "ses_other"}))
	require.False(t, TraceEventMatchesQuery(event, TraceQuery{Types: []string{"model.response"}}))
}

func TestTrace_CloneTraceEventClonesPayload(t *testing.T) {
	original := TraceEvent{
		ID:        1,
		SessionID: DefaultSessionID,
		Sequence:  2,
		Type:      "model.request",
		Timestamp: time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"nested": map[string]any{"value": "original"},
		},
	}

	cloned := CloneTraceEvent(original)
	original.Payload.(map[string]any)["nested"].(map[string]any)["value"] = "mutated"

	require.Equal(t, "original", cloned.Payload.(map[string]any)["nested"].(map[string]any)["value"])
	require.Equal(t, original.ID, cloned.ID)
	require.Equal(t, original.SessionID, cloned.SessionID)
	require.Equal(t, original.Sequence, cloned.Sequence)
	require.Equal(t, original.Type, cloned.Type)
}

func TestTrace_CloneTraceEventDropsUncloneablePayload(t *testing.T) {
	event := CloneTraceEvent(TraceEvent{
		SessionID: DefaultSessionID,
		Type:      "model.request",
		Payload:   map[string]any{"bad": func() {}},
	})

	require.Nil(t, event.Payload)

	event = CloneTraceEvent(TraceEvent{SessionID: DefaultSessionID, Type: "model.request"})
	require.Nil(t, event.Payload)
}

func TestTrace_CloneTraceEventDropsInvalidJSONPayload(t *testing.T) {
	event := CloneTraceEvent(TraceEvent{
		SessionID: DefaultSessionID,
		Type:      "model.request",
		Payload:   invalidTraceJSON{},
	})

	require.Nil(t, event.Payload)
}
