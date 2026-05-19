package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/guardrails"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service is the RPC service that wraps the agent-facing service interface.
type Service struct {
	handpb.UnimplementedHandServiceServer
	api agent.ServiceAPI
}

// NewService creates a new RPC service that wraps the shared service interface.
func NewService(api agent.ServiceAPI) *Service {
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
		Instruct:  req.Instruct,
		SessionID: req.GetId(),
		Stream:    req.Stream,
		OnEvent: func(event agent.Event) {
			if sendErr != nil {
				return
			}
			protoEvent, ok := agentEventToProtoRespondEvent(event)
			if !ok {
				return
			}
			streamed = true
			sendErr = stream.Send(protoEvent)
		},
	}
	opts.OnTraceEvent = func(event trace.Event) {
		if sendErr != nil {
			return
		}
		protoEvent, ok := traceEventToProtoRespondEvent(event)
		if !ok {
			return
		}
		sendErr = stream.Send(protoEvent)
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

func agentEventToProtoRespondEvent(event agent.Event) (*handpb.RespondEvent, bool) {
	kind := strings.TrimSpace(event.Kind)
	if kind != "" && kind != agent.EventKindTextDelta {
		return nil, false
	}

	return &handpb.RespondEvent{
		Type:    handpb.RespondEvent_TEXT_DELTA,
		Text:    event.Text,
		Channel: agentChannelToProtoStreamChannel(event.Channel),
	}, true
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

func getRPCTracePayload(eventType string, payload any) (map[string]any, bool) {
	fields := getPayloadFields(payload)

	switch eventType {
	case trace.EvtToolInvocationStarted:
		result := map[string]any{}
		if id, ok := getRPCTraceValue(fields, "id", "ID", "tool_call_id", "ToolCallID"); ok {
			result["id"] = id
		}
		if name, ok := getRPCTraceValue(fields, "name", "Name", "tool"); ok {
			result["name"] = name
		}
		if detail := getRPCTraceToolDetail(result["name"], fields); detail != "" {
			result["detail"] = detail
		}
		return result, len(result) > 0
	case trace.EvtToolInvocationCompleted:
		result := map[string]any{}
		if id, ok := getRPCTraceValue(fields, "tool_call_id", "ToolCallID", "id", "ID"); ok {
			result["tool_call_id"] = id
		}
		if name, ok := getRPCTraceValue(fields, "name", "Name", "tool"); ok {
			result["name"] = name
		}
		return result, len(result) > 0
	case trace.EvtInputSafetyBlocked,
		trace.EvtOutputSafetyApplied:
		result := getRPCTraceFields(fields, "action", "refusal", "blocked", "redacted")
		if findings := getRPCSafetyFindings(fields["findings"]); len(findings) > 0 {
			result["findings"] = findings
		}
		return result, true
	case trace.EvtSessionFailed:
		result := getRPCTraceFields(fields, "error", "message")
		return result, len(result) > 0
	case trace.EvtPlanHydrated:
		result := getRPCTraceFields(fields, "session_id", "summary", "active_step_id", "source")
		if steps, ok := fields["steps"].([]any); ok {
			result["step_count"] = len(steps)
		}
		return result, true
	case trace.EvtFinalAssistantResponse:
		result := getRPCTraceFields(fields, "message", "text")
		return result, len(result) > 0
	default:
		return nil, false
	}
}

func getRPCTraceFields(fields map[string]any, keys ...string) map[string]any {
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			result[key] = value
		}
	}

	return result
}

func getRPCTraceValue(fields map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			return value, true
		}
	}

	return nil, false
}

func getRPCTraceToolDetail(name any, fields map[string]any) string {
	toolName, _ := name.(string)
	action := getRPCToolActionName(toolName)
	if strings.TrimSpace(toolName) == "" || action == "" {
		return ""
	}

	input, ok := getRPCTraceValue(fields, "input", "Input")
	if !ok {
		return ""
	}

	inputText, _ := input.(string)
	inputText = strings.TrimSpace(inputText)
	if inputText == "" {
		return ""
	}

	var inputFields map[string]any
	if err := json.Unmarshal([]byte(inputText), &inputFields); err != nil {
		return ""
	}

	switch action {
	case "Run":
		return getRPCRunToolDetail(inputFields)
	case "Web Search", "Memory Search":
		return getRPCSearchToolDetail(inputFields)
	default:
		return ""
	}
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
	case "exec", "exec_command", "run", "run_command", "shell", "bash", "process":
		return "Run"
	case "web_search", "search_web", "search", "web":
		return "Web Search"
	case "memory_search", "search_memory", "memory":
		return "Memory Search"
	default:
		return ""
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

	return command + " (" + strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", timeout), "0"), ".") + "s)"
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

func getRPCSafetyFindings(raw any) []map[string]any {
	values, ok := raw.([]any)
	if !ok {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(data, &values); err != nil {
			return nil
		}
	}

	findings := make([]map[string]any, 0, len(values))
	for _, value := range values {
		fields, ok := value.(map[string]any)
		if !ok {
			continue
		}

		finding := getRPCTraceFields(fields, "id", "category", "severity")
		if len(finding) > 0 {
			findings = append(findings, finding)
		}
	}

	return findings
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

	id, err := s.api.CurrentSession(ctx)
	if err != nil {
		return nil, getGRPCError(err)
	}

	return &handpb.CurrentSessionResponse{Id: id}, nil
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

	result, err := s.api.RepairSession(ctx, agent.RepairSessionOptions{
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
		UpdatedAtUnix: session.UpdatedAt.Unix(),
	}
}

func sessionTimelineToProtoResponse(timeline agent.SessionTimeline) *handpb.GetSessionTimelineResponse {
	response := &handpb.GetSessionTimelineResponse{
		Id:                    timeline.SessionID,
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

func timelineTraceEventToProto(event storage.TraceEvent) (*handpb.SessionTimelineTraceEvent, bool) {
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
