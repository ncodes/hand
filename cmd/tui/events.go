package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/guardrails"
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
		return toolInvocationStartedMsg{
			ID:     strings.TrimSpace(value.ID),
			Name:   strings.TrimSpace(value.Name),
			Detail: getToolInputDisplayDetail(value.Name, value.Input),
		}, true
	case handmsg.ToolCall:
		return toolInvocationStartedMsg{
			ID:     strings.TrimSpace(value.ID),
			Name:   strings.TrimSpace(value.Name),
			Detail: getToolInputDisplayDetail(value.Name, value.Input),
		}, true
	default:
		name := getPayloadString(payload, "name", "tool")
		id := getPayloadString(payload, "id", "tool_call_id")
		if name == "" && id == "" {
			return nil, false
		}

		return toolInvocationStartedMsg{
			ID:     id,
			Name:   name,
			Detail: getPayloadString(payload, "detail"),
		}, true
	}
}

func toolMessagePayloadToTUIMessage(payload any) (any, bool) {
	switch value := payload.(type) {
	case handmsg.Message:
		return toolInvocationCompletedMsg{
			ID:   strings.TrimSpace(value.ToolCallID),
			Name: strings.TrimSpace(value.Name),
		}, true
	default:
		name := getPayloadString(payload, "name", "tool")
		id := getPayloadString(payload, "tool_call_id", "id")
		if name == "" && id == "" {
			return nil, false
		}

		return toolInvocationCompletedMsg{
			ID:     id,
			Name:   name,
			Detail: getPayloadString(payload, "detail"),
		}, true
	}
}

func getToolInputDisplayDetail(name string, input string) string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &fields); err != nil {
		return ""
	}

	switch getToolActionName(name) {
	case "Run":
		return getRunToolDisplayDetail(fields)
	case "Web Search", "Memory Search":
		return getSearchToolDisplayDetail(fields)
	default:
		if !isGenericToolParamDisplayEnabled(name) {
			return ""
		}
		return getGenericToolDisplayDetail(name, fields)
	}
}

func isGenericToolParamDisplayEnabled(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "list_files":
		return true
	default:
		return false
	}
}

func getGenericToolDisplayDetail(name string, fields map[string]any) string {
	name = strings.TrimSpace(name)
	if name == "" || len(fields) == 0 {
		return ""
	}

	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if strings.TrimSpace(key) == "" || isEmptyToolInputValue(value) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, strings.TrimSpace(key)+"="+formatToolInputValueForKey(key, fields[key]))
	}

	return name + "(" + strings.Join(parts, " ") + ")"
}

func getRunToolDisplayDetail(fields map[string]any) string {
	command := getMapString(fields, "command")
	if command == "" {
		return ""
	}

	args := getMapStringSlice(fields, "args")
	if len(args) == 0 {
		return appendToolTimeout(command, fields["timeout_seconds"])
	}

	parts := append([]string{command}, args...)
	for index, part := range parts {
		parts[index] = shellQuoteCommandPart(part)
	}

	return appendToolTimeout(strings.Join(parts, " "), fields["timeout_seconds"])
}

func getSearchToolDisplayDetail(fields map[string]any) string {
	query := getMapString(fields, "query", "q", "search_query")
	if query == "" {
		return ""
	}

	sanitized, _ := guardrails.NewRedactor().Sanitize(query).(string)
	sanitized = truncateToolDetail(sanitized, 80)
	if sanitized == "" {
		return ""
	}

	return `Search "` + strings.ReplaceAll(sanitized, `"`, `'`) + `"`
}

func getMapString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		value, _ := fields[key].(string)
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}

	return ""
}

func getMapStringSlice(fields map[string]any, key string) []string {
	raw, ok := fields[key].([]any)
	if !ok {
		return nil
	}

	values := make([]string, 0, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			values = append(values, text)
		}
	}

	return values
}

func isEmptyToolInputValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func formatToolInputValueForKey(key string, value any) string {
	switch typed := value.(type) {
	case string:
		sanitized, _ := guardrails.NewRedactor().Sanitize(typed).(string)
		if strings.EqualFold(strings.TrimSpace(key), "path") {
			return shortenToolPath(sanitized, 42)
		}
		return truncateToolDetail(sanitized, 60)
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", typed), "0"), ".")
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return truncateToolDetail(fmt.Sprintf("%v", typed), 60)
		}
		return truncateToolDetail(string(data), 60)
	}
}

func shortenToolPath(path string, limit int) string {
	path = strings.Join(strings.Fields(strings.TrimSpace(path)), " ")
	if limit <= 0 {
		return path
	}

	runes := []rune(path)
	if len(runes) <= limit {
		return path
	}
	if limit <= 5 {
		return string(runes[:limit])
	}

	separator := "/"
	if strings.Contains(path, "\\") && !strings.Contains(path, "/") {
		separator = "\\"
	}
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	tail := ""
	if len(parts) > 0 {
		tail = parts[len(parts)-1]
	}
	if tail == "" {
		return truncateToolDetail(path, limit)
	}

	tailRunes := []rune(tail)
	if len(tailRunes)+5 >= limit {
		return "..." + separator + string(tailRunes[max(len(tailRunes)-(limit-4), 0):])
	}

	prefixLimit := limit - len(tailRunes) - 4
	prefix := string(runes[:max(prefixLimit, 1)])

	return strings.TrimRight(prefix, `/\`) + separator + "..." + separator + tail
}

func shellQuoteCommandPart(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n\"'\\$&|;()<>*?![]{}") {
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}

	return value
}

func appendToolTimeout(command string, raw any) string {
	timeout, ok := raw.(float64)
	if !ok || timeout <= 0 {
		return command
	}

	return command + " (" + strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", timeout), "0"), ".") + "s)"
}

func truncateToolDetail(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 {
		return value
	}

	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}

	return string(runes[:limit-3]) + "..."
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
