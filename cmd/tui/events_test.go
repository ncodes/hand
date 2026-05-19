package tui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/guardrails"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/trace"
)

func TestAgentEventToTUIMessage_ConvertsAssistantDelta(t *testing.T) {
	msg, ok := agentEventToTUIMessage(agent.Event{
		Kind:    agent.EventKindTextDelta,
		Channel: "assistant",
		Text:    "hello",
	})

	require.True(t, ok)
	require.Equal(t, assistantTextDeltaMsg{Channel: "assistant", Text: "hello"}, msg)
}

func TestAgentEventToTUIMessage_DefaultsMissingChannel(t *testing.T) {
	msg, ok := agentEventToTUIMessage(agent.Event{Kind: agent.EventKindTextDelta, Text: "hello"})

	require.True(t, ok)
	require.Equal(t, assistantTextDeltaMsg{Channel: "assistant", Text: "hello"}, msg)
}

func TestAgentEventToTUIMessage_ConvertsRPCClientEvent(t *testing.T) {
	msg, ok := agentEventToTUIMessage(rpcclient.Event{
		Kind:    agent.EventKindTextDelta,
		Channel: "assistant",
		Text:    "daemon delta",
	})

	require.True(t, ok)
	require.Equal(t, assistantTextDeltaMsg{Channel: "assistant", Text: "daemon delta"}, msg)
}

func TestAgentEventToTUIMessage_ConvertsRPCTraceEvent(t *testing.T) {
	traceEvent := trace.Event{
		Type:    trace.EvtSessionFailed,
		Payload: map[string]any{"error": "model unavailable"},
	}

	msg, ok := agentEventToTUIMessage(rpcclient.Event{
		Kind:       agent.EventKindTrace,
		TraceEvent: &traceEvent,
	})

	require.True(t, ok)
	require.Equal(t, sessionErrorMsg{Message: "model unavailable"}, msg)
}

func TestAgentEventToTUIMessage_PreservesWhitespaceDelta(t *testing.T) {
	msg, ok := agentEventToTUIMessage(agent.Event{
		Kind:    agent.EventKindTextDelta,
		Channel: "assistant",
		Text:    " ",
	})

	require.True(t, ok)
	require.Equal(t, assistantTextDeltaMsg{Channel: "assistant", Text: " "}, msg)
}

func TestAgentEventToTUIMessage_IgnoresZeroLengthDelta(t *testing.T) {
	msg, ok := agentEventToTUIMessage(agent.Event{
		Kind:    agent.EventKindTextDelta,
		Channel: "assistant",
	})

	require.False(t, ok)
	require.Nil(t, msg)
}

func TestTraceEventToTUIMessage_ConvertsUserMessageAccepted(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type:    trace.EvtUserMessageAccepted,
		Payload: map[string]any{"message": "hello"},
	})

	require.True(t, ok)
	require.Equal(t, userMessageAcceptedMsg{Text: "hello"}, msg)
}

func TestTraceEventToTUIMessage_ConvertsAssistantResponseCompleted(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type:    trace.EvtFinalAssistantResponse,
		Payload: map[string]any{"message": "done"},
	})

	require.True(t, ok)
	require.Equal(t, assistantResponseCompletedMsg{Text: "done"}, msg)
}

func TestTraceEventToTUIMessage_ConvertsToolInvocationStarted(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type: trace.EvtToolInvocationStarted,
		Payload: models.ToolCall{
			ID:    "call_1",
			Name:  "read_file",
			Input: `{"path":"secret.txt"}`,
		},
	})

	require.True(t, ok)
	require.Equal(t, toolInvocationStartedMsg{ID: "call_1", Name: "read_file"}, msg)
}

func TestToolCallPayloadToTUIMessage_ConvertsMessageToolCall(t *testing.T) {
	msg, ok := toolCallPayloadToTUIMessage(handmsg.ToolCall{
		ID:    " call_1 ",
		Name:  " read_file ",
		Input: `{"path":"secret.txt"}`,
	})

	require.True(t, ok)
	require.Equal(t, toolInvocationStartedMsg{ID: "call_1", Name: "read_file"}, msg)
	require.NotContains(t, fmt.Sprintf("%#v", msg), "secret.txt")
}

func TestToolCallPayloadToTUIMessage_ExtractsRunCommandDetail(t *testing.T) {
	msg, ok := toolCallPayloadToTUIMessage(models.ToolCall{
		ID:    "call_1",
		Name:  "run_command",
		Input: `{"command":"sleep 10 && echo \"Done\"","timeout_seconds":8}`,
	})

	require.True(t, ok)
	require.Equal(t, toolInvocationStartedMsg{
		ID:     "call_1",
		Name:   "run_command",
		Detail: `sleep 10 && echo "Done" (8s)`,
	}, msg)
}

func TestToolCallPayloadToTUIMessage_ExtractsWebSearchDetail(t *testing.T) {
	msg, ok := toolCallPayloadToTUIMessage(models.ToolCall{
		ID:    "call_1",
		Name:  "web_search",
		Input: `{"query":"what is todays news about open source ai releases and model updates happening around the world"}`,
	})

	require.True(t, ok)
	require.Equal(t, toolInvocationStartedMsg{
		ID:     "call_1",
		Name:   "web_search",
		Detail: `Search "what is todays news about open source ai releases and model updates happening..."`,
	}, msg)
}

func TestToolCallPayloadToTUIMessage_IgnoresPayloadWithoutIdentity(t *testing.T) {
	msg, ok := toolCallPayloadToTUIMessage(map[string]any{"input": `{"path":"secret.txt"}`})

	require.False(t, ok)
	require.Nil(t, msg)
}

func TestTraceEventToTUIMessage_ConvertsToolInvocationCompleted(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type: trace.EvtToolInvocationCompleted,
		Payload: handmsg.Message{
			Name:       "read_file",
			ToolCallID: "call_1",
			Content:    "SECRET=example",
		},
	})

	require.True(t, ok)
	require.Equal(t, toolInvocationCompletedMsg{ID: "call_1", Name: "read_file"}, msg)
}

func TestToolMessagePayloadToTUIMessage_IgnoresPayloadWithoutIdentity(t *testing.T) {
	msg, ok := toolMessagePayloadToTUIMessage(map[string]any{"content": "SECRET=example"})

	require.False(t, ok)
	require.Nil(t, msg)
}

func TestTraceEventToTUIMessage_ConvertsMapToolEventsWithoutRawPayload(t *testing.T) {
	started, ok := traceEventToTUIMessage(trace.Event{
		Type: trace.EvtToolInvocationStarted,
		Payload: map[string]any{
			"id":    "call_1",
			"name":  "read_file",
			"input": `{"path":"secret.txt"}`,
		},
	})
	require.True(t, ok)
	require.Equal(t, toolInvocationStartedMsg{ID: "call_1", Name: "read_file"}, started)
	require.NotContains(t, fmt.Sprintf("%#v", started), "secret.txt")

	completed, ok := traceEventToTUIMessage(trace.Event{
		Type: trace.EvtToolInvocationCompleted,
		Payload: map[string]any{
			"tool_call_id": "call_1",
			"name":         "read_file",
			"content":      "SECRET=example",
		},
	})
	require.True(t, ok)
	require.Equal(t, toolInvocationCompletedMsg{ID: "call_1", Name: "read_file"}, completed)
	require.NotContains(t, fmt.Sprintf("%#v", completed), "SECRET=example")
}

func TestTraceEventToTUIMessage_ConvertsSafetyEventsWithoutRawPayload(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type: trace.EvtOutputSafetyApplied,
		Payload: map[string]any{
			"action":         "redacted",
			"blocked":        false,
			"redacted":       true,
			"refusal":        "safe refusal",
			"raw_content":    "SECRET=example",
			"content":        "developer instructions",
			"authorization":  "Bearer secret",
			"content_length": 42,
			"findings": []any{
				map[string]any{"id": "secret_exfiltration", "sample": "SECRET=example"},
			},
		},
	})

	require.True(t, ok)
	require.Equal(t, safetyEventMsg{
		Kind:       trace.EvtOutputSafetyApplied,
		Action:     "redacted",
		Refusal:    "safe refusal",
		Redacted:   true,
		FindingIDs: []string{"secret_exfiltration"},
	}, msg)
	displayPayload := fmt.Sprintf("%#v", msg)
	require.NotContains(t, displayPayload, "SECRET=example")
	require.NotContains(t, displayPayload, "Bearer secret")
}

func TestTraceEventToTUIMessage_ConvertsGuardrailSafetyFindingIDs(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type: trace.EvtInputSafetyBlocked,
		Payload: guardrails.SafetyTracePayload(guardrails.SafetyTracePayloadOptions{
			Action:  "blocked",
			Blocked: true,
			Findings: []guardrails.SafetyFinding{{
				ID:       guardrails.SafetyFindingPromptExfiltration,
				Category: guardrails.SafetyCategoryPromptExfiltration,
			}},
		}),
	})

	require.True(t, ok)
	require.Equal(t, safetyEventMsg{
		Kind:       trace.EvtInputSafetyBlocked,
		Action:     "blocked",
		Blocked:    true,
		FindingIDs: []string{string(guardrails.SafetyFindingPromptExfiltration)},
	}, msg)
}

func TestTraceEventToTUIMessage_ConvertsSessionError(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type:    trace.EvtSessionFailed,
		Payload: map[string]any{"error": "model unavailable"},
	})

	require.True(t, ok)
	require.Equal(t, sessionErrorMsg{Message: "model unavailable"}, msg)
}

func TestTraceEventToTUIMessage_UsesFallbackPayloadKeys(t *testing.T) {
	userMsg, ok := traceEventToTUIMessage(trace.Event{
		Type:    trace.EvtUserMessageAccepted,
		Payload: map[string]any{"text": "hello"},
	})
	require.True(t, ok)
	require.Equal(t, userMessageAcceptedMsg{Text: "hello"}, userMsg)

	assistantMsg, ok := traceEventToTUIMessage(trace.Event{
		Type:    trace.EvtFinalAssistantResponse,
		Payload: map[string]any{"text": "done"},
	})
	require.True(t, ok)
	require.Equal(t, assistantResponseCompletedMsg{Text: "done"}, assistantMsg)

	errMsg, ok := traceEventToTUIMessage(trace.Event{
		Type:    trace.EvtSessionFailed,
		Payload: map[string]any{"message": "model unavailable"},
	})
	require.True(t, ok)
	require.Equal(t, sessionErrorMsg{Message: "model unavailable"}, errMsg)
}

func TestTraceEventToTUIMessage_IgnoresEmptyPayloadFields(t *testing.T) {
	for _, eventType := range []string{
		trace.EvtUserMessageAccepted,
		trace.EvtFinalAssistantResponse,
		trace.EvtSessionFailed,
	} {
		msg, ok := traceEventToTUIMessage(trace.Event{
			Type:    eventType,
			Payload: map[string]any{"message": " "},
		})

		require.False(t, ok)
		require.Nil(t, msg)
	}
}

func TestTraceEventToTUIMessage_IgnoresUnknownTraceEvent(t *testing.T) {
	msg, ok := traceEventToTUIMessage(trace.Event{
		Type:    trace.EvtModelRequest,
		Payload: map[string]any{"authorization": "Bearer secret"},
	})

	require.False(t, ok)
	require.Nil(t, msg)
}

func TestSafetyPayloadToTUIMessage_IgnoresEmptyKind(t *testing.T) {
	msg, ok := safetyPayloadToTUIMessage(" ", map[string]any{"blocked": true})

	require.False(t, ok)
	require.Nil(t, msg)
}

func TestPayloadHelpers_HandleMalformedAndConvertedPayloads(t *testing.T) {
	require.Nil(t, getPayloadFields(nil))
	require.Nil(t, getPayloadFields(make(chan int)))
	require.Nil(t, getPayloadFields("not an object"))
	require.Equal(t, map[string]any{"name": "read_file"}, getPayloadFields(map[string]any{"name": "read_file"}))

	fields := getPayloadFields(struct {
		Name string `json:"name"`
	}{
		Name: "read_file",
	})
	require.Equal(t, map[string]any{"name": "read_file"}, fields)

	require.False(t, getPayloadBool(map[string]any{"blocked": "true"}, "blocked"))
	require.False(t, getPayloadBool(map[string]any{"redacted": true}, "blocked"))
}

func TestGetSafetyFindingIDs_HandlesSupportedShapes(t *testing.T) {
	require.Nil(t, getSafetyFindingIDs(nil))
	require.Nil(t, getSafetyFindingIDs(map[string]any{"findings": "secret_exfiltration"}))

	ids := getSafetyFindingIDs(map[string]any{
		"findings": []map[string]string{
			{"id": "secret_exfiltration"},
			{"id": " "},
		},
	})
	require.Equal(t, []string{"secret_exfiltration"}, ids)

	ids = getSafetyFindingIDs(map[string]any{
		"findings": []any{
			map[string]any{"id": "prompt_exfiltration"},
			map[string]any{"id": " "},
			"not a finding",
		},
	})
	require.Equal(t, []string{"prompt_exfiltration"}, ids)
}
