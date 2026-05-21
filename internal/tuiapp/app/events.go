package tui

import (
	"strings"
	"time"

	"github.com/wandxy/hand/internal/agent"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/trace"
)

type userMessageAcceptedMsg struct {
	Text string
}

type assistantTextDeltaMsg struct {
	Channel string
	Text    string
}

type assistantResponseCompletedMsg struct {
	Text string
}

type reasoningCompletedMsg struct {
	Duration time.Duration
}

type toolInvocationStartedMsg struct {
	ID        string
	Name      string
	Detail    string
	PlanState *trace.PlanToolState
	StartedAt time.Time
}

type toolInvocationCompletedMsg struct {
	ID          string
	Name        string
	Detail      string
	PlanState   *trace.PlanToolState
	CompletedAt time.Time
}

type safetyEventMsg struct {
	Kind       string
	Action     string
	Refusal    string
	Blocked    bool
	Redacted   bool
	FindingIDs []string
}

type sessionErrorMsg struct {
	Message string
}

func agentEventToTUIMessage(event agent.Event) (any, bool) {
	if event.TraceEvent != nil {
		return traceEventToTUIMessage(*event.TraceEvent)
	}
	if event.Text == "" {
		return nil, false
	}

	channel := strings.TrimSpace(event.Channel)
	if channel == "" {
		channel = "assistant"
	}

	return assistantTextDeltaMsg{Channel: channel, Text: event.Text}, true
}

func traceEventToTUIMessage(event trace.Event) (any, bool) {
	typedPayload, payloadOK := trace.DecodePayload(event.Type, event.Payload)

	switch strings.TrimSpace(event.Type) {
	case trace.EvtUserMessageAccepted:
		payload, ok := typedPayload.(trace.UserMessageAcceptedPayload)
		if text := firstNonEmptyTUI(payload.Message, payload.Text); payloadOK && ok && text != "" {
			return userMessageAcceptedMsg{Text: text}, true
		}
	case trace.EvtFinalAssistantResponse:
		payload, ok := typedPayload.(trace.FinalAssistantResponsePayload)
		if text := firstNonEmptyTUI(payload.Message, payload.Text); payloadOK && ok && text != "" {
			return assistantResponseCompletedMsg{Text: text}, true
		}
	case trace.EvtModelReasoningCompleted:
		payload, ok := typedPayload.(trace.ModelReasoningCompletedPayload)
		if payloadOK && ok && payload.DurationMS > 0 {
			return reasoningCompletedMsg{Duration: time.Duration(payload.DurationMS) * time.Millisecond}, true
		}
	case trace.EvtToolInvocationStarted:
		msg, ok := toolCallPayloadToTUIMessage(event.Payload)
		if !ok {
			return nil, false
		}
		if toolMsg, ok := msg.(toolInvocationStartedMsg); ok {
			toolMsg.StartedAt = getTraceEventTimestamp(event)
			return toolMsg, true
		}
		return msg, true
	case trace.EvtToolInvocationCompleted:
		msg, ok := toolMessagePayloadToTUIMessage(event.Payload)
		if !ok {
			return nil, false
		}
		if toolMsg, ok := msg.(toolInvocationCompletedMsg); ok {
			toolMsg.CompletedAt = getTraceEventTimestamp(event)
			return toolMsg, true
		}
		return msg, true
	case trace.EvtInputSafetyBlocked,
		trace.EvtOutputSafetyApplied,
		trace.EvtToolOutputSafetyApplied,
		trace.EvtLoadedContentSafetyBlocked:
		return safetyPayloadToTUIMessage(event.Type, event.Payload)
	case trace.EvtSessionFailed:
		payload, ok := typedPayload.(trace.SessionFailedPayload)
		if message := firstNonEmptyTUI(payload.Error, payload.Message); payloadOK && ok && message != "" {
			return sessionErrorMsg{Message: message}, true
		}
	}

	return nil, false
}

func getTraceEventTimestamp(event trace.Event) time.Time {
	return event.Timestamp
}

func toolCallPayloadToTUIMessage(payload any) (any, bool) {
	switch value := payload.(type) {
	case models.ToolCall:
		msg, ok := toolInvocationStartedMsgFromModelToolCall(value, time.Time{})
		if !ok {
			return nil, false
		}
		return msg, true
	case handmsg.ToolCall:
		msg, ok := toolInvocationStartedMsgFromMessageToolCall(value, time.Time{})
		if !ok {
			return nil, false
		}
		return msg, true
	default:
		toolPayload, ok := trace.ToolInvocationStartedPayloadFrom(payload)
		if !ok {
			return nil, false
		}
		msg, ok := newToolInvocationStartedMsgWithState(
			toolPayload.ID,
			toolPayload.Name,
			toolPayload.Detail,
			toolPayload.PlanState,
			time.Time{},
		)
		if !ok {
			return nil, false
		}

		return msg, true
	}
}

func toolMessagePayloadToTUIMessage(payload any) (any, bool) {
	switch value := payload.(type) {
	case handmsg.Message:
		msg, ok := toolInvocationCompletedMsgFromMessage(value, time.Time{})
		if !ok {
			return nil, false
		}
		return msg, true
	default:
		toolPayload, ok := trace.ToolInvocationCompletedPayloadFrom(payload)
		if !ok {
			return nil, false
		}
		msg, ok := newToolInvocationCompletedMsgWithState(
			toolPayload.ToolCallID,
			toolPayload.Name,
			toolPayload.Detail,
			toolPayload.PlanState,
			time.Time{},
		)
		if !ok {
			return nil, false
		}

		return msg, true
	}
}

func safetyPayloadToTUIMessage(kind string, payload any) (any, bool) {
	typedPayload, ok := payload.(trace.SafetyEventPayload)
	if !ok {
		decoded, decodedOK := trace.DecodePayload(kind, payload)
		typedPayload, ok = decoded.(trace.SafetyEventPayload)
		if !decodedOK || !ok {
			return nil, false
		}
	}

	msg := safetyEventMsg{
		Kind:       strings.TrimSpace(kind),
		Action:     strings.TrimSpace(typedPayload.Action),
		Refusal:    strings.TrimSpace(typedPayload.Refusal),
		Blocked:    typedPayload.Blocked,
		Redacted:   typedPayload.Redacted,
		FindingIDs: getSafetyFindingIDsFromTypedPayload(typedPayload),
	}
	if msg.Kind == "" {
		return nil, false
	}

	return msg, true
}

func firstNonEmptyTUI(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

func getSafetyFindingIDsFromTypedPayload(payload trace.SafetyEventPayload) []string {
	if len(payload.Findings) == 0 {
		return nil
	}

	ids := make([]string, 0, len(payload.Findings))
	for _, finding := range payload.Findings {
		id := strings.TrimSpace(finding["id"])
		if id != "" {
			ids = append(ids, id)
		}
	}

	return ids
}
