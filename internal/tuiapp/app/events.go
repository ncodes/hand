package tui

import (
	"encoding/json"
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

type toolInvocationStartedMsg struct {
	ID        string
	Name      string
	Detail    string
	StartedAt time.Time
}

type toolInvocationCompletedMsg struct {
	ID          string
	Name        string
	Detail      string
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
	switch strings.TrimSpace(event.Type) {
	case trace.EvtUserMessageAccepted:
		if text := getPayloadString(event.Payload, "message", "text"); text != "" {
			return userMessageAcceptedMsg{Text: text}, true
		}
	case trace.EvtFinalAssistantResponse:
		if text := getPayloadString(event.Payload, "message", "text"); text != "" {
			return assistantResponseCompletedMsg{Text: text}, true
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
		if message := getPayloadString(event.Payload, "error", "message"); message != "" {
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
		name := getPayloadString(payload, "name", "tool")
		id := getPayloadString(payload, "id", "tool_call_id")
		msg, ok := newToolInvocationStartedMsg(
			id,
			name,
			getPayloadString(payload, "detail"),
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
		name := getPayloadString(payload, "name", "tool")
		id := getPayloadString(payload, "tool_call_id", "id")
		msg, ok := newToolInvocationCompletedMsg(
			id,
			name,
			getPayloadString(payload, "detail"),
			time.Time{},
		)
		if !ok {
			return nil, false
		}

		return msg, true
	}
}

func safetyPayloadToTUIMessage(kind string, payload any) (any, bool) {
	msg := safetyEventMsg{
		Kind:       strings.TrimSpace(kind),
		Action:     getPayloadString(payload, "action"),
		Refusal:    getPayloadString(payload, "refusal"),
		Blocked:    getPayloadBool(payload, "blocked"),
		Redacted:   getPayloadBool(payload, "redacted"),
		FindingIDs: getSafetyFindingIDs(payload),
	}
	if msg.Kind == "" {
		return nil, false
	}

	return msg, true
}

func getPayloadString(payload any, keys ...string) string {
	fields := getPayloadFields(payload)
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			if text, ok := value.(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}

	return ""
}

func getPayloadBool(payload any, key string) bool {
	fields := getPayloadFields(payload)
	if value, ok := fields[key]; ok {
		result, _ := value.(bool)
		return result
	}

	return false
}

func getPayloadFields(payload any) map[string]any {
	if payload == nil {
		return nil
	}
	if fields, ok := payload.(map[string]any); ok {
		return fields
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil
	}

	return fields
}

func getSafetyFindingIDs(payload any) []string {
	fields := getPayloadFields(payload)
	rawFindings, ok := fields["findings"]
	if !ok {
		return nil
	}

	switch findings := rawFindings.(type) {
	case []map[string]string:
		return getSafetyFindingIDsFromStringMaps(findings)
	case []any:
		return getSafetyFindingIDsFromValues(findings)
	default:
		return nil
	}
}

func getSafetyFindingIDsFromStringMaps(findings []map[string]string) []string {
	ids := make([]string, 0, len(findings))
	for _, finding := range findings {
		id := strings.TrimSpace(finding["id"])
		if id != "" {
			ids = append(ids, id)
		}
	}

	return ids
}

func getSafetyFindingIDsFromValues(findings []any) []string {
	ids := make([]string, 0, len(findings))
	for _, rawFinding := range findings {
		finding, ok := rawFinding.(map[string]any)
		if !ok {
			continue
		}
		id, _ := finding["id"].(string)
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}

	return ids
}
