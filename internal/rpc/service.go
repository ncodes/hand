package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	handagent "github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/guardrails"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	"github.com/wandxy/hand/internal/trace"
	agent "github.com/wandxy/hand/pkg/agent"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service is the RPC service that wraps the agent-facing service interface.
type Service struct {
	handpb.UnimplementedHandServiceServer
	api handagent.ServiceAPI
}

// NewService creates a new RPC service that wraps the shared service interface.
func NewService(api handagent.ServiceAPI) *Service {
	return &Service{api: api}
}

// Respond handles a respond request and returns a response.
func (s *Service) Respond(req *handpb.RespondRequest, stream handpb.HandService_RespondServer) error {
	if s == nil {
		return status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return status.Error(codes.InvalidArgument, "respond request is required")
	}

	ctx := stream.Context()
	streamed := false
	var sendErr error
	opts := agent.RespondOptions{
		Instruct:    req.Instruct,
		SessionID:   req.GetId(),
		Stream:      req.Stream,
		TraceEvents: true,
		OnEvent: func(event agent.Event) {
			if sendErr != nil {
				return
			}
			protoEvent, ok := eventToProtoRespondEvent(event)
			if !ok {
				return
			}
			if protoEvent.GetType() == handpb.RespondEvent_TEXT_DELTA {
				streamed = true
			}
			sendErr = stream.Send(protoEvent)
		},
	}

	reply, err := s.api.Respond(ctx, req.Message, opts)
	if sendErr != nil {
		return sendErr
	}
	if err != nil {
		grpcErr := getGRPCError(err)
		if sendErr := stream.Send(&handpb.RespondEvent{
			Type:      handpb.RespondEvent_ERROR,
			Error:     status.Convert(grpcErr).Message(),
			Timestamp: timestamppb.New(time.Now().UTC()),
		}); sendErr != nil {
			return sendErr
		}
		return nil
	}

	if !streamed {
		if err := stream.Send(&handpb.RespondEvent{
			Type:    handpb.RespondEvent_TEXT_DELTA,
			Text:    reply,
			Channel: handpb.RespondEvent_ASSISTANT,
		}); err != nil {
			return err
		}
	}

	return stream.Send(&handpb.RespondEvent{
		Type:      handpb.RespondEvent_DONE,
		Timestamp: timestamppb.New(time.Now().UTC()),
	})
}

func eventToProtoRespondEvent(event agent.Event) (*handpb.RespondEvent, bool) {
	kind := strings.TrimSpace(event.Kind)
	if kind == agent.EventKindTrace {
		traceEvent, ok := traceEventFromAgentEvent(event)
		if !ok {
			return nil, false
		}
		return traceEventToProtoRespondEvent(traceEvent)
	}
	if kind != "" && kind != agent.EventKindTextDelta {
		return nil, false
	}

	return &handpb.RespondEvent{
		Type:    handpb.RespondEvent_TEXT_DELTA,
		Text:    event.Text,
		Channel: agentChannelToProtoStreamChannel(event.Channel),
	}, true
}

func traceEventFromAgentEvent(event agent.Event) (trace.Event, bool) {
	switch value := event.TraceEvent.(type) {
	case trace.Event:
		return value, true
	case *trace.Event:
		if value == nil {
			return trace.Event{}, false
		}
		return *value, true
	default:
		return trace.Event{}, false
	}
}

func agentChannelToProtoStreamChannel(channel string) handpb.RespondEvent_Channel {
	switch strings.TrimSpace(strings.ToLower(channel)) {
	case "reasoning":
		return handpb.RespondEvent_REASONING
	default:
		return handpb.RespondEvent_ASSISTANT
	}
}

func traceEventToProtoRespondEvent(event trace.Event) (*handpb.RespondEvent, bool) {
	event.Type = strings.TrimSpace(event.Type)
	if event.Type == "" {
		return nil, false
	}

	payload, ok := getRPCTracePayload(event.Type, event.Payload)
	if !ok {
		return nil, false
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}

	protoEvent := &handpb.RespondEvent{
		Type:             handpb.RespondEvent_TRACE_EVENT,
		TraceSessionId:   strings.TrimSpace(event.SessionID),
		TraceType:        event.Type,
		TracePayloadJson: string(payloadJSON),
	}
	if !event.Timestamp.IsZero() {
		protoEvent.Timestamp = timestamppb.New(event.Timestamp.UTC())
	} else {
		protoEvent.Timestamp = timestamppb.New(time.Now().UTC())
	}

	return protoEvent, true
}

func getRPCTracePayload(eventType string, payload any) (any, bool) {
	typedPayload, payloadOK := trace.DecodePayload(eventType, payload)

	switch eventType {
	case trace.EvtToolInvocationStarted:
		toolPayload, ok := typedPayload.(trace.ToolInvocationStartedPayload)
		if !payloadOK || !ok {
			return nil, false
		}
		result := rpcToolInvocationStartedPayload{
			ID:     strings.TrimSpace(toolPayload.ID),
			Name:   strings.TrimSpace(toolPayload.Name),
			Detail: getRPCTraceToolDetail(toolPayload.Name, toolPayload.Input),
		}
		result.PlanState = toolPayload.PlanState
		result.ProcessState = toolPayload.ProcessState
		return result, result.hasData()
	case trace.EvtToolInvocationCompleted:
		toolPayload, ok := typedPayload.(trace.ToolInvocationCompletedPayload)
		if !payloadOK || !ok {
			return nil, false
		}
		result := rpcToolInvocationCompletedPayload{
			ToolCallID: strings.TrimSpace(toolPayload.ToolCallID),
			Name:       strings.TrimSpace(toolPayload.Name),
		}
		result.PlanState = toolPayload.PlanState
		result.ProcessState = toolPayload.ProcessState
		return result, result.hasData()
	case trace.EvtInputSafetyBlocked,
		trace.EvtOutputSafetyApplied,
		trace.EvtToolOutputSafetyApplied,
		trace.EvtLoadedContentSafetyBlocked,
		trace.EvtMemorySafetyBlocked:
		safetyPayload, ok := typedPayload.(trace.SafetyEventPayload)
		if !payloadOK || !ok {
			return nil, false
		}
		result := rpcSafetyEventPayload{
			Action:   strings.TrimSpace(safetyPayload.Action),
			Blocked:  safetyPayload.Blocked,
			Redacted: safetyPayload.Redacted,
			Refusal:  strings.TrimSpace(safetyPayload.Refusal),
			Findings: getRPCSafetyFindingSummaries(safetyPayload.Findings),
		}
		return result, true
	case trace.EvtSessionFailed:
		sessionPayload, ok := typedPayload.(trace.SessionFailedPayload)
		if !payloadOK || !ok {
			return nil, false
		}
		result := trace.SessionFailedPayload{
			Error:   strings.TrimSpace(sessionPayload.Error),
			Message: strings.TrimSpace(sessionPayload.Message),
		}
		return result, result.Error != "" || result.Message != ""
	case trace.EvtPlanUpdated,
		trace.EvtPlanCleared,
		trace.EvtPlanHydrated:
		planPayload, ok := typedPayload.(trace.PlanEventPayload)
		if !payloadOK || !ok {
			return nil, false
		}
		result := rpcPlanPayload{
			SessionID:    strings.TrimSpace(planPayload.SessionID),
			Source:       strings.TrimSpace(planPayload.Source),
			ActiveStepID: strings.TrimSpace(planPayload.ActiveStepID),
			Explanation:  strings.TrimSpace(planPayload.Explanation),
			Steps:        getRPCPlanSteps(planPayload.Steps),
			Summary:      getRPCPlanSummary(planPayload.Summary),
			Changes:      append([]trace.PlanToolChange(nil), planPayload.Changes...),
		}
		return result, true
	case trace.EvtContextCompactionPending,
		trace.EvtContextCompactionRunning,
		trace.EvtContextCompactionSucceeded,
		trace.EvtContextCompactionFailed:
		compactionPayload, ok := typedPayload.(trace.CompactionEventPayload)
		if !payloadOK || !ok {
			return nil, false
		}
		return trace.CompactionEventPayload{
			SessionID:          strings.TrimSpace(compactionPayload.SessionID),
			Status:             strings.TrimSpace(compactionPayload.Status),
			Auto:               compactionPayload.Auto,
			TargetMessageCount: compactionPayload.TargetMessageCount,
			TargetOffset:       compactionPayload.TargetOffset,
			RequestedAt:        compactionPayload.RequestedAt,
			StartedAt:          compactionPayload.StartedAt,
			CompletedAt:        compactionPayload.CompletedAt,
			FailedAt:           compactionPayload.FailedAt,
			Error:              strings.TrimSpace(compactionPayload.Error),
		}, true
	case trace.EvtModelReasoningCompleted:
		reasoningPayload, ok := typedPayload.(trace.ModelReasoningCompletedPayload)
		if !payloadOK || !ok {
			return nil, false
		}
		result := trace.ModelReasoningCompletedPayload{DurationMS: reasoningPayload.DurationMS}
		return result, result.DurationMS != 0
	case trace.EvtFinalAssistantResponse:
		finalPayload, ok := typedPayload.(trace.FinalAssistantResponsePayload)
		if !payloadOK || !ok {
			return nil, false
		}
		result := trace.FinalAssistantResponsePayload{
			Message: strings.TrimSpace(finalPayload.Message),
			Text:    strings.TrimSpace(finalPayload.Text),
		}
		return result, result.Message != "" || result.Text != ""
	default:
		return nil, false
	}
}

type rpcToolInvocationStartedPayload struct {
	ID           string                  `json:"id,omitempty"`
	Name         string                  `json:"name,omitempty"`
	Detail       string                  `json:"detail,omitempty"`
	PlanState    *trace.PlanToolState    `json:"plan_state,omitempty"`
	ProcessState *trace.ProcessToolState `json:"process_state,omitempty"`
}

func (p rpcToolInvocationStartedPayload) hasData() bool {
	return p.ID != "" ||
		p.Name != "" ||
		p.Detail != "" ||
		p.PlanState != nil ||
		p.ProcessState != nil
}

type rpcToolInvocationCompletedPayload struct {
	ToolCallID   string                  `json:"tool_call_id,omitempty"`
	Name         string                  `json:"name,omitempty"`
	PlanState    *trace.PlanToolState    `json:"plan_state,omitempty"`
	ProcessState *trace.ProcessToolState `json:"process_state,omitempty"`
}

func (p rpcToolInvocationCompletedPayload) hasData() bool {
	return p.ToolCallID != "" ||
		p.Name != "" ||
		p.PlanState != nil ||
		p.ProcessState != nil
}

type rpcSafetyEventPayload struct {
	Action   string                    `json:"action,omitempty"`
	Blocked  bool                      `json:"blocked,omitempty"`
	Redacted bool                      `json:"redacted,omitempty"`
	Refusal  string                    `json:"refusal,omitempty"`
	Findings []rpcSafetyFindingSummary `json:"findings,omitempty"`
}

type rpcSafetyFindingSummary struct {
	ID       string `json:"id,omitempty"`
	Category string `json:"category,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type rpcPlanPayload struct {
	SessionID    string                    `json:"session_id,omitempty"`
	Source       string                    `json:"source,omitempty"`
	ActiveStepID string                    `json:"active_step_id,omitempty"`
	Explanation  string                    `json:"explanation,omitempty"`
	Steps        []trace.PlanStepPayload   `json:"steps,omitempty"`
	Summary      *trace.PlanSummaryPayload `json:"summary,omitempty"`
	Changes      []trace.PlanToolChange    `json:"changes,omitempty"`
}

func getRPCTraceToolDetail(name string, input string) string {
	toolName := strings.TrimSpace(name)
	action := getRPCToolActionName(toolName)
	if strings.TrimSpace(toolName) == "" || action == "" {
		return ""
	}

	inputFields := getRPCTraceToolInputFields(input)
	if inputFields == nil {
		return ""
	}

	switch action {
	case "Run":
		return getRPCRunToolDetail(inputFields)
	case "Web Search", "Memory Search":
		return getRPCSearchToolDetail(inputFields)
	case "Search Files":
		return getRPCSearchFilesToolDetail(inputFields)
	case "Read", "Write":
		return getRPCPathToolDetail(toolName, inputFields)
	case "Patch":
		return getRPCPatchToolDetail(toolName, inputFields)
	default:
		if isRPCGenericToolDetailEnabled(toolName) {
			return getRPCGenericToolDetail(toolName, inputFields)
		}
		return ""
	}
}

func getRPCTraceToolInputFields(input string) map[string]any {
	inputText := strings.TrimSpace(input)
	if inputText == "" {
		return nil
	}

	var inputFields map[string]any
	if err := json.Unmarshal([]byte(inputText), &inputFields); err != nil {
		return nil
	}

	return inputFields
}

func getRPCRunToolDetail(inputFields map[string]any) string {
	command := getRPCMapString(inputFields, "command")
	if command == "" {
		return ""
	}

	args := getRPCStringSlice(inputFields["args"])
	if len(args) > 0 {
		parts := append([]string{command}, args...)
		for index, part := range parts {
			parts[index] = shellQuoteRPCCommandPart(part)
		}
		command = strings.Join(parts, " ")
	}
	command = appendRPCToolTimeout(command, inputFields["timeout_seconds"])

	sanitized, _ := guardrails.NewRedactor().Sanitize(command).(string)
	return strings.TrimSpace(sanitized)
}

func getRPCSearchToolDetail(inputFields map[string]any) string {
	query := getRPCMapString(inputFields, "query", "q", "search_query")
	if query == "" {
		return ""
	}

	sanitized, _ := guardrails.NewRedactor().Sanitize(query).(string)
	sanitized = truncateRPCTraceToolDetail(sanitized, 80)
	if sanitized == "" {
		return ""
	}

	return `Search "` + strings.ReplaceAll(sanitized, `"`, `'`) + `"`
}

func getRPCSearchFilesToolDetail(inputFields map[string]any) string {
	pattern := getRPCMapString(inputFields, "pattern", "query", "q")
	if pattern == "" {
		return ""
	}

	sanitized, _ := guardrails.NewRedactor().Sanitize(pattern).(string)
	sanitized = truncateRPCTraceToolDetail(sanitized, 80)
	if sanitized == "" {
		return ""
	}

	detail := `Search "` + strings.ReplaceAll(sanitized, `"`, `'`) + `"`
	if path := getRPCDisplayPath(inputFields); path != "" {
		detail += " in " + path
	}
	if maxResults := formatOptionalRPCToolNumber(inputFields["max_results"]); maxResults != "" {
		detail += " max_results=" + maxResults
	}

	return detail
}

func getRPCPathToolDetail(name string, inputFields map[string]any) string {
	path := getRPCDisplayPath(inputFields)
	if path == "" {
		return ""
	}

	return strings.TrimSpace(name) + " " + path
}

func getRPCPatchToolDetail(name string, inputFields map[string]any) string {
	patch := getRPCMapString(inputFields, "patch", "diff", "unified_diff")
	path, added, removed := getRPCPatchToolSummary(patch)
	if path == "" {
		path = getRPCDisplayPath(inputFields)
	}

	parts := []string{strings.TrimSpace(name)}
	if path != "" {
		parts = append(parts, path)
	}
	if added > 0 || removed > 0 {
		parts = append(parts, fmt.Sprintf("+%d -%d", added, removed))
	}

	return strings.Join(parts, " ")
}

func getRPCDisplayPath(inputFields map[string]any) string {
	path := getRPCMapString(inputFields, "path", "file", "filepath", "filename")
	if path == "" {
		return ""
	}

	sanitized, _ := guardrails.NewRedactor().Sanitize(path).(string)
	return shortenRPCTraceToolPath(sanitized, 42)
}

func getRPCPatchToolSummary(patch string) (string, int, int) {
	var path string
	added := 0
	removed := 0

	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			candidate := normalizeRPCPatchToolPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
			if candidate != "" && candidate != "/dev/null" {
				path = candidate
			}
		case strings.HasPrefix(line, "--- "):
			if path == "" {
				candidate := normalizeRPCPatchToolPath(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))
				if candidate != "" && candidate != "/dev/null" {
					path = candidate
				}
			}
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}

	if path != "" {
		sanitized, _ := guardrails.NewRedactor().Sanitize(path).(string)
		path = shortenRPCTraceToolPath(sanitized, 42)
	}

	return path, added, removed
}

func normalizeRPCPatchToolPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return strings.Trim(path, `"`)
}

func isRPCGenericToolDetailEnabled(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "list_files":
		return true
	default:
		return false
	}
}

func getRPCGenericToolDetail(name string, inputFields map[string]any) string {
	name = strings.TrimSpace(name)
	if name == "" || len(inputFields) == 0 {
		return ""
	}

	keys := make([]string, 0, len(inputFields))
	for key, value := range inputFields {
		if strings.TrimSpace(key) == "" || isRPCEmptyToolInputValue(value) {
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
		parts = append(parts, strings.TrimSpace(key)+"="+formatRPCGenericToolInputValue(key, inputFields[key]))
	}

	return name + "(" + strings.Join(parts, " ") + ")"
}

func isRPCEmptyToolInputValue(value any) bool {
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

func formatOptionalRPCToolNumber(value any) string {
	switch typed := value.(type) {
	case float64:
		if typed <= 0 {
			return ""
		}
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", typed), "0"), ".")
	case int:
		if typed <= 0 {
			return ""
		}
		return fmt.Sprintf("%d", typed)
	default:
		return ""
	}
}

func formatRPCGenericToolInputValue(key string, value any) string {
	switch typed := value.(type) {
	case string:
		sanitized, _ := guardrails.NewRedactor().Sanitize(typed).(string)
		if strings.EqualFold(strings.TrimSpace(key), "path") {
			return shortenRPCTraceToolPath(sanitized, 42)
		}
		return truncateRPCTraceToolDetail(sanitized, 60)
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
			return truncateRPCTraceToolDetail(fmt.Sprintf("%v", typed), 60)
		}
		return truncateRPCTraceToolDetail(string(data), 60)
	}
}

func getRPCMapString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		value, _ := fields[key].(string)
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}

	return ""
}

func getRPCStringSlice(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		text, _ := value.(string)
		if text = strings.TrimSpace(text); text != "" {
			result = append(result, text)
		}
	}

	return result
}

func getRPCToolActionName(name string) string {
	normalized := strings.TrimSpace(strings.ToLower(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "read", "read_file", "view_file", "open_file", "cat":
		return "Read"
	case "write", "write_file", "edit_file", "create_file":
		return "Write"
	case "patch", "apply_patch":
		return "Patch"
	case "process":
		return "Process"
	case "exec", "exec_command", "run", "run_command", "shell", "bash":
		return "Run"
	case "web_search", "search_web", "search", "web":
		return "Web Search"
	case "search_files":
		return "Search Files"
	case "memory_search", "search_memory", "memory":
		return "Memory Search"
	case "memory_extract", "extract_memory":
		return "Memory Extract"
	case "memory_add", "add_memory":
		return "Memory Add"
	case "memory_update", "update_memory":
		return "Memory Update"
	case "memory_delete", "delete_memory":
		return "Memory Delete"
	case "plan", "plan_tool", "update_plan":
		return "Plan"
	default:
		return humanizeRPCToolActionName(name)
	}
}

func shellQuoteRPCCommandPart(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n\"'\\$&|;()<>*?![]{}") {
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}

	return value
}

func appendRPCToolTimeout(command string, raw any) string {
	timeout, ok := raw.(float64)
	if !ok || timeout <= 0 {
		return command
	}

	return command + " [timeout " + strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", timeout), "0"), ".") + "s]"
}

func shortenRPCTraceToolPath(path string, limit int) string {
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
		return truncateRPCTraceToolDetail(path, limit)
	}

	tailRunes := []rune(tail)
	if len(tailRunes)+5 >= limit {
		return "..." + separator + string(tailRunes[max(len(tailRunes)-(limit-4), 0):])
	}

	prefixLimit := limit - len(tailRunes) - 4
	prefix := string(runes[:max(prefixLimit, 1)])

	return strings.TrimRight(prefix, `/\`) + separator + "..." + separator + tail
}

func humanizeRPCToolActionName(name string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(name), func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[index] = string(runes)
	}

	return strings.Join(parts, " ")
}

func truncateRPCTraceToolDetail(value string, limit int) string {
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

func getRPCSafetyFindingSummaries(findings []map[string]string) []rpcSafetyFindingSummary {
	summaries := make([]rpcSafetyFindingSummary, 0, len(findings))
	for _, finding := range findings {
		summary := rpcSafetyFindingSummary{
			ID:       strings.TrimSpace(finding["id"]),
			Category: strings.TrimSpace(finding["category"]),
			Severity: strings.TrimSpace(finding["severity"]),
		}
		if summary.ID != "" || summary.Category != "" || summary.Severity != "" {
			summaries = append(summaries, summary)
		}
	}

	return summaries
}

func getRPCPlanSummary(summary trace.PlanSummaryPayload) *trace.PlanSummaryPayload {
	if summary.Total == 0 &&
		summary.Pending == 0 &&
		summary.InProgress == 0 &&
		summary.Completed == 0 &&
		summary.Cancelled == 0 {
		return nil
	}

	return &summary
}

func getRPCPlanSteps(steps []trace.PlanStepPayload) []trace.PlanStepPayload {
	if len(steps) == 0 {
		return nil
	}

	result := make([]trace.PlanStepPayload, 0, len(steps))
	for _, step := range steps {
		item := trace.PlanStepPayload{
			ID:      strings.TrimSpace(step.ID),
			Content: strings.TrimSpace(step.Content),
			Status:  strings.TrimSpace(step.Status),
		}
		if item.ID != "" || item.Content != "" || item.Status != "" {
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return nil
	}

	return result
}

func (s *Service) CreateSession(ctx context.Context, req *handpb.CreateSessionRequest) (*handpb.CreateSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "create session request is required")
	}

	session, err := s.api.CreateSession(ctx, req.GetId())
	if err != nil {
		return nil, getGRPCError(err)
	}

	return &handpb.CreateSessionResponse{Session: sessionToProtoSummary(session)}, nil
}

func (s *Service) ListSessions(ctx context.Context, req *handpb.ListSessionsRequest) (*handpb.ListSessionsResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "list sessions request is required")
	}

	sessions, err := s.api.ListSessions(ctx)
	if err != nil {
		return nil, getGRPCError(err)
	}

	items := make([]*handpb.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, sessionToProtoSummary(session))
	}

	return &handpb.ListSessionsResponse{Sessions: items}, nil
}

func (s *Service) UseSession(ctx context.Context, req *handpb.UseSessionRequest) (*handpb.UseSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "use session request is required")
	}

	if err := s.api.UseSession(ctx, req.GetId()); err != nil {
		return nil, getGRPCError(err)
	}

	return &handpb.UseSessionResponse{Id: req.GetId()}, nil
}

func (s *Service) CurrentSession(ctx context.Context, req *handpb.CurrentSessionRequest) (*handpb.CurrentSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "current session request is required")
	}

	session, err := s.api.CurrentSession(ctx)
	if err != nil {
		return nil, getGRPCError(err)
	}

	response := &handpb.CurrentSessionResponse{
		Id:          session.ID,
		Title:       session.Title,
		TitleSource: session.TitleSource,
	}

	return response, nil
}

func (s *Service) CompactSession(
	ctx context.Context,
	req *handpb.CompactSessionRequest,
) (*handpb.CompactSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "compact session request is required")
	}

	result, err := s.api.CompactSession(ctx, req.GetId())
	if err != nil {
		return nil, getGRPCError(err)
	}

	return &handpb.CompactSessionResponse{
		Id:                   result.SessionID,
		SourceEndOffset:      int32(result.SourceEndOffset),
		SourceMessageCount:   int32(result.SourceMessageCount),
		UpdatedAt:            timestamppb.New(result.UpdatedAt),
		CurrentContextLength: int32(result.CurrentContextLength),
		TotalContextLength:   int32(result.TotalContextLength),
	}, nil
}

func (s *Service) RepairSession(
	ctx context.Context,
	req *handpb.RepairSessionRequest,
) (*handpb.RepairSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "repair session request is required")
	}
	if req.GetType() != handpb.RepairSessionRequest_VECTOR {
		return nil, status.Error(codes.InvalidArgument, "repair session type must be vector")
	}
	if req.GetVector() == nil {
		return nil, status.Error(codes.InvalidArgument, "repair session vector options are required")
	}

	result, err := s.api.RepairSession(ctx, search.VectorRepairOptions{
		SessionID: req.GetVector().GetId(),
		Full:      req.GetVector().GetFull(),
	})
	if err != nil {
		return nil, getGRPCError(err)
	}

	return &handpb.RepairSessionResponse{
		Type: handpb.RepairSessionRequest_VECTOR,
		Vector: &handpb.VectorRepairResponse{
			SessionsScanned: int32(result.SessionsScanned),
			MessagesScanned: int32(result.MessagesScanned),
			RowsScanned:     int32(result.RowsScanned),
			MissingRows:     int32(result.MissingRows),
			StaleRows:       int32(result.StaleRows),
			UnchangedRows:   int32(result.UnchangedRows),
			RebuiltRows:     int32(result.RebuiltRows),
			DeletedSources:  int32(result.DeletedSources),
			Batches:         int32(result.Batches),
		},
	}, nil
}

func (s *Service) GetSession(ctx context.Context, req *handpb.GetSessionRequest) (*handpb.GetSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "get session request is required")
	}
	if req.GetContext() == nil {
		return nil, status.Error(codes.InvalidArgument, "get session request context is required")
	}

	result, err := s.api.ContextStatus(ctx, req.GetContext().GetId())
	if err != nil {
		return nil, getGRPCError(err)
	}

	return &handpb.GetSessionResponse{
		Id:               result.SessionID,
		Size:             int32(result.Size),
		CreatedAt:        timestamppb.New(result.CreatedAt),
		UpdatedAt:        timestamppb.New(result.UpdatedAt),
		CompactionStatus: result.CompactionStatus,
		Context: &handpb.GetSessionResponse_Context{
			Offset:       int32(result.Offset),
			Length:       int32(result.Length),
			Used:         int32(result.Used),
			Remaining:    int32(result.Remaining),
			UsedPct:      result.UsedPct,
			RemainingPct: result.RemainingPct,
		},
	}, nil
}

func (s *Service) GetSessionTimeline(
	ctx context.Context,
	req *handpb.GetSessionTimelineRequest,
) (*handpb.GetSessionTimelineResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.api == nil {
		return nil, status.Error(codes.Internal, "agent handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "get session timeline request is required")
	}

	result, err := s.api.GetSessionTimeline(ctx, agent.SessionTimelineOptions{
		SessionID:     req.GetId(),
		MessageOffset: int(req.GetMessageOffset()),
		MessageLimit:  int(req.GetMessageLimit()),
		TraceOffset:   int(req.GetTraceOffset()),
		TraceLimit:    int(req.GetTraceLimit()),
	})
	if err != nil {
		return nil, getGRPCError(err)
	}

	return sessionTimelineToProtoResponse(result), nil
}

func getGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if _, ok := status.FromError(err); ok {
		return err
	}

	message := err.Error()
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, message)
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, message)
	case strings.HasSuffix(message, "is required"),
		strings.Contains(message, "must be a valid"),
		strings.Contains(message, "must be greater than or equal to"),
		strings.Contains(message, "cannot be deleted"):
		return status.Error(codes.InvalidArgument, message)
	case strings.HasSuffix(message, "not found"):
		return status.Error(codes.NotFound, message)
	case strings.HasSuffix(message, "already exists"):
		return status.Error(codes.AlreadyExists, message)
	default:
		return status.Error(codes.Internal, message)
	}
}

func sessionToProtoSummary(session storage.Session) *handpb.SessionSummary {
	return &handpb.SessionSummary{
		Id:            session.ID,
		Title:         session.Title,
		TitleSource:   session.TitleSource,
		UpdatedAtUnix: session.UpdatedAt.Unix(),
	}
}

func sessionTimelineToProtoResponse(timeline agent.SessionTimeline) *handpb.GetSessionTimelineResponse {
	response := &handpb.GetSessionTimelineResponse{
		Id:                    timeline.SessionID,
		Title:                 timeline.Title,
		TitleSource:           timeline.TitleSource,
		MessagesHasMore:       timeline.MessagesHasMore,
		TracesHasMore:         timeline.TracesHasMore,
		TracesTruncatedBefore: timeline.TracesTruncatedBefore,
		Messages:              make([]*handpb.SessionTimelineMessage, 0, len(timeline.Messages)),
		TraceEvents:           make([]*handpb.SessionTimelineTraceEvent, 0, len(timeline.TraceEvents)),
	}
	for _, message := range timeline.Messages {
		response.Messages = append(response.Messages, timelineMessageToProto(message))
	}
	for _, event := range timeline.TraceEvents {
		if protoEvent, ok := timelineTraceEventToProto(event.Event); ok {
			response.TraceEvents = append(response.TraceEvents, protoEvent)
		}
	}
	if len(response.TraceEvents) > 0 {
		response.FirstTraceSequence = response.TraceEvents[0].GetSequence()
		response.LastTraceSequence = response.TraceEvents[len(response.TraceEvents)-1].GetSequence()
	}

	return response
}

func timelineMessageToProto(record agent.SessionTimelineMessage) *handpb.SessionTimelineMessage {
	message := record.Message
	protoMessage := &handpb.SessionTimelineMessage{
		Offset:     int32(record.Offset),
		Id:         uint64(message.ID),
		Role:       string(message.Role),
		Name:       message.Name,
		ToolCallId: message.ToolCallID,
		Content:    message.Content,
		CreatedAt:  timestamppb.New(message.CreatedAt),
		ToolCalls:  make([]*handpb.SessionTimelineToolCall, 0, len(message.ToolCalls)),
	}
	for _, toolCall := range message.ToolCalls {
		protoMessage.ToolCalls = append(protoMessage.ToolCalls, &handpb.SessionTimelineToolCall{
			Id:    strings.TrimSpace(toolCall.ID),
			Name:  strings.TrimSpace(toolCall.Name),
			Input: strings.TrimSpace(toolCall.Input),
		})
	}

	return protoMessage
}

func timelineTraceEventToProto(event agentsession.TraceEvent) (*handpb.SessionTimelineTraceEvent, bool) {
	payload, ok := getRPCTracePayload(event.Type, event.Payload)
	if !ok {
		return nil, false
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}

	return &handpb.SessionTimelineTraceEvent{
		Id:          uint64(event.ID),
		Sequence:    int32(event.Sequence),
		Type:        strings.TrimSpace(event.Type),
		Timestamp:   timestamppb.New(event.Timestamp),
		PayloadJson: string(payloadJSON),
	}, true
}
