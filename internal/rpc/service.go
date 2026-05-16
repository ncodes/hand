package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/agent"
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
