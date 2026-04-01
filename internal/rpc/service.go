package rpc

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/agent"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	sessionstore "github.com/wandxy/hand/internal/session"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type chatter interface {
	Respond(context.Context, string, agent.RespondOptions) (string, error)
	CreateSession(context.Context, string) (sessionstore.Session, error)
	ListSessions(context.Context) ([]sessionstore.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (string, error)
}

// Service is the RPC service that wraps the chatter interface.
type Service struct {
	handpb.UnimplementedHandServiceServer
	chatter chatter
}

// NewService creates a new RPC service that wraps the chatter interface.
func NewService(chatter chatter) *Service {
	return &Service{chatter: chatter}
}

// Chat handles a chat request and returns a chat response.
func (s *Service) Chat(ctx context.Context, req *handpb.ChatRequest) (*handpb.ChatResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.chatter == nil {
		return nil, status.Error(codes.Internal, "chat handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "chat request is required")
	}

	reply, err := s.chatter.Respond(ctx, req.Message, agent.RespondOptions{Instruct: req.Instruct, SessionID: req.SessionId})
	if err != nil {
		return nil, grpcError(err)
	}

	return &handpb.ChatResponse{Message: reply}, nil
}

func (s *Service) CreateSession(ctx context.Context, req *handpb.CreateSessionRequest) (*handpb.CreateSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.chatter == nil {
		return nil, status.Error(codes.Internal, "chat handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "create session request is required")
	}

	session, err := s.chatter.CreateSession(ctx, req.SessionId)
	if err != nil {
		return nil, grpcError(err)
	}

	return &handpb.CreateSessionResponse{Session: sessionSummary(session)}, nil
}

func (s *Service) ListSessions(ctx context.Context, req *handpb.ListSessionsRequest) (*handpb.ListSessionsResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.chatter == nil {
		return nil, status.Error(codes.Internal, "chat handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "list sessions request is required")
	}

	sessions, err := s.chatter.ListSessions(ctx)
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
	if s.chatter == nil {
		return nil, status.Error(codes.Internal, "chat handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "use session request is required")
	}

	if err := s.chatter.UseSession(ctx, req.SessionId); err != nil {
		return nil, grpcError(err)
	}

	return &handpb.UseSessionResponse{SessionId: req.SessionId}, nil
}

func (s *Service) CurrentSession(ctx context.Context, req *handpb.CurrentSessionRequest) (*handpb.CurrentSessionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "service is required")
	}
	if s.chatter == nil {
		return nil, status.Error(codes.Internal, "chat handler is required")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "current session request is required")
	}

	id, err := s.chatter.CurrentSession(ctx)
	if err != nil {
		return nil, grpcError(err)
	}

	return &handpb.CurrentSessionResponse{SessionId: id}, nil
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

func sessionSummary(session sessionstore.Session) *handpb.SessionSummary {
	return &handpb.SessionSummary{
		SessionId:     session.ID,
		UpdatedAtUnix: session.UpdatedAt.Unix(),
	}
}
