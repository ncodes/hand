package rpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wandxy/hand/internal/agent"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	sessionstore "github.com/wandxy/hand/internal/session"
)

type chatterStub struct {
	message   string
	instruct  string
	sessionID string
	reply     string
	err       error
	created   sessionstore.Session
	listed    []sessionstore.Session
	current   string
}

func (s *chatterStub) Respond(_ context.Context, message string, opts agent.RespondOptions) (string, error) {
	s.message = message
	s.instruct = opts.Instruct
	s.sessionID = opts.SessionID
	return s.reply, s.err
}

func (s *chatterStub) CreateSession(context.Context, string) (sessionstore.Session, error) {
	return s.created, s.err
}

func (s *chatterStub) ListSessions(context.Context) ([]sessionstore.Session, error) {
	return s.listed, s.err
}

func (s *chatterStub) UseSession(context.Context, string) error {
	return s.err
}

func (s *chatterStub) CurrentSession(context.Context) (string, error) {
	return s.current, s.err
}

func TestNewService_ReturnsService(t *testing.T) {
	require.NotNil(t, NewService(nil))
}

func TestService_ChatReturnsMessage(t *testing.T) {
	stub := &chatterStub{reply: "hello back"}
	svc := NewService(stub)

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello", Instruct: "be terse"})

	require.NoError(t, err)
	require.Equal(t, "hello", stub.message)
	require.Equal(t, "be terse", stub.instruct)
	require.Empty(t, stub.sessionID)
	require.Equal(t, "hello back", resp.Message)
}

func TestService_ChatReturnsHandlerError(t *testing.T) {
	stub := &chatterStub{err: errors.New("boom")}
	svc := NewService(stub)

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello"})

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "boom", status.Convert(err).Message())
	require.Nil(t, resp)
}

func TestService_ChatRejectsNilRequest(t *testing.T) {
	svc := NewService(&chatterStub{})

	resp, err := svc.Chat(context.Background(), nil)

	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "chat request is required", status.Convert(err).Message())
	require.Nil(t, resp)
}

func TestService_ChatRejectsMissingHandler(t *testing.T) {
	svc := NewService(nil)

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello"})

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "chat handler is required", status.Convert(err).Message())
	require.Nil(t, resp)
}

func TestService_ChatRejectsNilReceiver(t *testing.T) {
	var svc *Service

	resp, err := svc.Chat(context.Background(), &handpb.ChatRequest{Message: "hello"})

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "service is required", status.Convert(err).Message())
	require.Nil(t, resp)
}

func TestService_CreateSessionReturnsSummary(t *testing.T) {
	stub := &chatterStub{created: sessionstore.Session{ID: "project-a"}}
	svc := NewService(stub)

	resp, err := svc.CreateSession(context.Background(), &handpb.CreateSessionRequest{SessionId: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", resp.GetSession().GetSessionId())
}

func TestService_CreateSessionRejectsInvalidState(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var svc *Service

		resp, err := svc.CreateSession(context.Background(), &handpb.CreateSessionRequest{})

		requireStatusError(t, err, codes.Internal, "service is required")
		require.Nil(t, resp)
	})

	t.Run("missing handler", func(t *testing.T) {
		svc := NewService(nil)

		resp, err := svc.CreateSession(context.Background(), &handpb.CreateSessionRequest{})

		requireStatusError(t, err, codes.Internal, "chat handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&chatterStub{})

		resp, err := svc.CreateSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "create session request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&chatterStub{err: errors.New("session already exists")})

		resp, err := svc.CreateSession(context.Background(), &handpb.CreateSessionRequest{SessionId: "project-a"})

		requireStatusError(t, err, codes.AlreadyExists, "session already exists")
		require.Nil(t, resp)
	})
}

func TestService_ListSessionsReturnsItems(t *testing.T) {
	stub := &chatterStub{listed: []sessionstore.Session{{ID: "default"}, {ID: "project-a"}}}
	svc := NewService(stub)

	resp, err := svc.ListSessions(context.Background(), &handpb.ListSessionsRequest{})

	require.NoError(t, err)
	require.Len(t, resp.GetSessions(), 2)
	require.Equal(t, "default", resp.GetSessions()[0].GetSessionId())
	require.Equal(t, "project-a", resp.GetSessions()[1].GetSessionId())
}

func TestService_ListSessionsRejectsInvalidState(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var svc *Service

		resp, err := svc.ListSessions(context.Background(), &handpb.ListSessionsRequest{})

		requireStatusError(t, err, codes.Internal, "service is required")
		require.Nil(t, resp)
	})

	t.Run("missing handler", func(t *testing.T) {
		svc := NewService(nil)

		resp, err := svc.ListSessions(context.Background(), &handpb.ListSessionsRequest{})

		requireStatusError(t, err, codes.Internal, "chat handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&chatterStub{})

		resp, err := svc.ListSessions(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "list sessions request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&chatterStub{err: errors.New("boom")})

		resp, err := svc.ListSessions(context.Background(), &handpb.ListSessionsRequest{})

		requireStatusError(t, err, codes.Internal, "boom")
		require.Nil(t, resp)
	})
}

func TestService_UseSessionReturnsSessionID(t *testing.T) {
	svc := NewService(&chatterStub{})

	resp, err := svc.UseSession(context.Background(), &handpb.UseSessionRequest{SessionId: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", resp.GetSessionId())
}

func TestService_UseSessionRejectsInvalidState(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var svc *Service

		resp, err := svc.UseSession(context.Background(), &handpb.UseSessionRequest{})

		requireStatusError(t, err, codes.Internal, "service is required")
		require.Nil(t, resp)
	})

	t.Run("missing handler", func(t *testing.T) {
		svc := NewService(nil)

		resp, err := svc.UseSession(context.Background(), &handpb.UseSessionRequest{})

		requireStatusError(t, err, codes.Internal, "chat handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&chatterStub{})

		resp, err := svc.UseSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "use session request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&chatterStub{err: errors.New("session not found")})

		resp, err := svc.UseSession(context.Background(), &handpb.UseSessionRequest{SessionId: "project-a"})

		requireStatusError(t, err, codes.NotFound, "session not found")
		require.Nil(t, resp)
	})
}

func TestService_CurrentSessionReturnsValue(t *testing.T) {
	svc := NewService(&chatterStub{current: sessionstore.DefaultSessionID})

	resp, err := svc.CurrentSession(context.Background(), &handpb.CurrentSessionRequest{})

	require.NoError(t, err)
	require.Equal(t, sessionstore.DefaultSessionID, resp.GetSessionId())
}

func TestService_CurrentSessionRejectsInvalidState(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var svc *Service

		resp, err := svc.CurrentSession(context.Background(), &handpb.CurrentSessionRequest{})

		requireStatusError(t, err, codes.Internal, "service is required")
		require.Nil(t, resp)
	})

	t.Run("missing handler", func(t *testing.T) {
		svc := NewService(nil)

		resp, err := svc.CurrentSession(context.Background(), &handpb.CurrentSessionRequest{})

		requireStatusError(t, err, codes.Internal, "chat handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&chatterStub{})

		resp, err := svc.CurrentSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "current session request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&chatterStub{err: errors.New("boom")})

		resp, err := svc.CurrentSession(context.Background(), &handpb.CurrentSessionRequest{})

		requireStatusError(t, err, codes.Internal, "boom")
		require.Nil(t, resp)
	})
}

func TestService_MapsDomainErrorsToGRPCCodes(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "required", err: errors.New("session id is required"), code: codes.InvalidArgument},
		{name: "invalid", err: errors.New("session id must be a valid ses_ nanoid"), code: codes.InvalidArgument},
		{name: "not found", err: errors.New("session not found"), code: codes.NotFound},
		{name: "already exists", err: errors.New("session already exists"), code: codes.AlreadyExists},
		{name: "cannot be deleted", err: errors.New("default session cannot be deleted"), code: codes.InvalidArgument},
		{name: "canceled", err: context.Canceled, code: codes.Canceled},
		{name: "deadline", err: context.DeadlineExceeded, code: codes.DeadlineExceeded},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := grpcError(tc.err)
			require.Equal(t, tc.code, status.Code(err))
			require.Equal(t, tc.err.Error(), status.Convert(err).Message())
		})
	}
}

func TestService_PreservesExistingGRPCStatus(t *testing.T) {
	original := status.Error(codes.PermissionDenied, "nope")

	err := grpcError(original)

	require.Same(t, original, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestService_GrpcErrorNil(t *testing.T) {
	require.NoError(t, grpcError(nil))
}

func requireStatusError(t *testing.T, err error, code codes.Code, message string) {
	t.Helper()
	require.Equal(t, code, status.Code(err))
	require.Equal(t, message, status.Convert(err).Message())
}
