package rpc

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/agent"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	"github.com/wandxy/hand/internal/storage"
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
			streamed = true
			sendErr = stream.Send(&handpb.RespondEvent{
				Type:    handpb.RespondEvent_TEXT_DELTA,
				Text:    event.Text,
				Channel: streamChannelFromAgent(event.Channel),
			})
		},
	}

	reply, err := s.api.Respond(ctx, req.Message, opts)
	if sendErr != nil {
		return sendErr
	}
	if err != nil {
		grpcErr := grpcError(err)
		if sendErr := stream.Send(&handpb.RespondEvent{
			Type:  handpb.RespondEvent_ERROR,
			Error: status.Convert(grpcErr).Message(),
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

	return stream.Send(&handpb.RespondEvent{Type: handpb.RespondEvent_DONE})
}

func streamChannelFromAgent(channel string) handpb.RespondEvent_Channel {
	switch strings.TrimSpace(strings.ToLower(channel)) {
	case "reasoning":
		return handpb.RespondEvent_REASONING
	default:
		return handpb.RespondEvent_ASSISTANT
	}
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
		return nil, grpcError(err)
	}

	return &handpb.CreateSessionResponse{Session: sessionSummary(session)}, nil
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
		return nil, grpcError(err)
	}

	items := make([]*handpb.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, sessionSummary(session))
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
		return nil, grpcError(err)
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
		return nil, grpcError(err)
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
		return nil, grpcError(err)
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

	result, err := s.api.SessionContextStatus(ctx, req.GetContext().GetId())
	if err != nil {
		return nil, grpcError(err)
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

func grpcError(err error) error {
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

func sessionSummary(session storage.Session) *handpb.SessionSummary {
	return &handpb.SessionSummary{
		Id:            session.ID,
		UpdatedAtUnix: session.UpdatedAt.Unix(),
	}
}
