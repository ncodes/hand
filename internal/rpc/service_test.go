package rpc

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/wandxy/hand/internal/agent"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	"github.com/wandxy/hand/internal/storage"
)

type respondStreamServerStub struct {
	ctx       context.Context
	events    []*handpb.RespondEvent
	sendErrAt int
}

func (s *respondStreamServerStub) Send(event *handpb.RespondEvent) error {
	if s.sendErrAt > 0 && len(s.events)+1 == s.sendErrAt {
		return errors.New("send failed")
	}
	s.events = append(s.events, event)
	return nil
}

func (s *respondStreamServerStub) SetHeader(metadata.MD) error  { return nil }
func (s *respondStreamServerStub) SendHeader(metadata.MD) error { return nil }
func (s *respondStreamServerStub) SetTrailer(metadata.MD)       {}
func (s *respondStreamServerStub) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}
func (s *respondStreamServerStub) SendMsg(any) error { return nil }
func (s *respondStreamServerStub) RecvMsg(any) error { return io.EOF }

func TestNewService_ReturnsService(t *testing.T) {
	require.NotNil(t, NewService(nil))
}

func TestService_RespondReturnsMessage(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	svc := NewService(stub)
	stream := &respondStreamServerStub{}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello", Instruct: "be terse"}, stream)

	require.NoError(t, err)
	require.Equal(t, "hello", stub.ChatInput)
	require.Equal(t, "be terse", stub.RespondOptions.Instruct)
	require.Empty(t, stub.RespondOptions.SessionID)
	require.Equal(t, []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "hello back", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}, stream.events)
}

func TestService_RespondSendsBufferedReplyWhenNotStreamed(t *testing.T) {
	stub := &bufferedReplyStub{reply: "full reply"}
	svc := NewService(stub)
	stream := &respondStreamServerStub{}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello", Id: "ses_1"}, stream)

	require.NoError(t, err)
	require.Equal(t, "ses_1", stub.capturedSessionID)
	require.Equal(t, []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "full reply", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}, stream.events)
}

func TestService_RespondReturnsHandlerError(t *testing.T) {
	stub := &agentstub.AgentServiceStub{RespondErr: errors.New("boom")}
	svc := NewService(stub)
	stream := &respondStreamServerStub{}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.NoError(t, err)
	require.Equal(t, []*handpb.RespondEvent{{Type: handpb.RespondEvent_ERROR, Error: "boom"}}, stream.events)
}

func TestService_RespondRejectsNilRequest(t *testing.T) {
	svc := NewService(&agentstub.AgentServiceStub{})
	stream := &respondStreamServerStub{}

	err := svc.Respond(nil, stream)

	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "respond request is required", status.Convert(err).Message())
}

func TestService_RespondRejectsMissingHandler(t *testing.T) {
	svc := NewService(nil)
	stream := &respondStreamServerStub{}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "agent handler is required", status.Convert(err).Message())
}

func TestService_RespondRejectsNilReceiver(t *testing.T) {
	var svc *Service
	stream := &respondStreamServerStub{}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "service is required", status.Convert(err).Message())
}

func TestService_RespondStreamsDeltas(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Reply: "hello back", Deltas: []string{"hello ", "back"}}
	svc := NewService(stub)
	stream := &respondStreamServerStub{}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.NoError(t, err)
	require.Equal(t, []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "hello ", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "back", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_DONE},
	}, stream.events)
}

func TestService_RespondForwardsStreamOverride(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	svc := NewService(stub)
	stream := &respondStreamServerStub{}
	streaming := false

	err := svc.Respond(&handpb.RespondRequest{Message: "hello", Stream: &streaming}, stream)

	require.NoError(t, err)
	require.NotNil(t, stub.RespondOptions.Stream)
	require.False(t, *stub.RespondOptions.Stream)
}

func TestService_RespondReturnsStreamSendErrorForDelta(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Reply: "hello back", Deltas: []string{"hello "}}
	svc := NewService(stub)
	stream := &respondStreamServerStub{sendErrAt: 1}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.EqualError(t, err, "send failed")
}

func TestService_RespondReturnsStreamSendErrorForErrorEvent(t *testing.T) {
	stub := &agentstub.AgentServiceStub{RespondErr: errors.New("boom")}
	svc := NewService(stub)
	stream := &respondStreamServerStub{sendErrAt: 1}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.EqualError(t, err, "send failed")
}

func TestService_RespondSkipsStreamEventsAfterSendFailure(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Reply: "ignored", Deltas: []string{"first", "second"}}
	svc := NewService(stub)
	stream := &respondStreamServerStub{sendErrAt: 1}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.EqualError(t, err, "send failed")
	require.Len(t, stream.events, 0)
}

func TestService_RespondReturnsStreamSendErrorOnSecondDelta(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Reply: "back", Deltas: []string{"a", "b"}}
	svc := NewService(stub)
	stream := &respondStreamServerStub{sendErrAt: 2}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.EqualError(t, err, "send failed")
	require.Equal(t, []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "a", Channel: handpb.RespondEvent_ASSISTANT},
	}, stream.events)
}

func TestService_RespondReturnsStreamSendErrorForBufferedReply(t *testing.T) {
	stub := &bufferedReplyStub{reply: "only reply"}
	svc := NewService(stub)
	stream := &respondStreamServerStub{sendErrAt: 1}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.EqualError(t, err, "send failed")
	require.Empty(t, stream.events)
}

func TestService_RespondReturnsStreamSendErrorForDone(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Reply: "done", Deltas: []string{"a", "b"}}
	svc := NewService(stub)
	stream := &respondStreamServerStub{sendErrAt: 3}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.EqualError(t, err, "send failed")
	require.Equal(t, []*handpb.RespondEvent{
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "a", Channel: handpb.RespondEvent_ASSISTANT},
		{Type: handpb.RespondEvent_TEXT_DELTA, Text: "b", Channel: handpb.RespondEvent_ASSISTANT},
	}, stream.events)
}

func TestService_RespondMapsStreamChannelFromAgent(t *testing.T) {
	t.Run("reasoning", func(t *testing.T) {
		stub := &channelRespondStub{channel: "reasoning", text: "think"}
		svc := NewService(stub)
		stream := &respondStreamServerStub{}

		err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

		require.NoError(t, err)
		require.Equal(t, handpb.RespondEvent_REASONING, stream.events[0].GetChannel())
	})

	t.Run("assistant default", func(t *testing.T) {
		stub := &channelRespondStub{channel: "assistant", text: "hi"}
		svc := NewService(stub)
		stream := &respondStreamServerStub{}

		err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

		require.NoError(t, err)
		require.Equal(t, handpb.RespondEvent_ASSISTANT, stream.events[0].GetChannel())
	})

	t.Run("unknown maps to assistant", func(t *testing.T) {
		stub := &channelRespondStub{channel: "other", text: "x"}
		svc := NewService(stub)
		stream := &respondStreamServerStub{}

		err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

		require.NoError(t, err)
		require.Equal(t, handpb.RespondEvent_ASSISTANT, stream.events[0].GetChannel())
	})
}

func TestService_RespondMapsGRPCHandlerErrorToErrorEvent(t *testing.T) {
	grpcErr := status.Error(codes.InvalidArgument, "bad request")
	stub := &agentstub.AgentServiceStub{RespondErr: grpcErr}
	svc := NewService(stub)
	stream := &respondStreamServerStub{}

	err := svc.Respond(&handpb.RespondRequest{Message: "hello"}, stream)

	require.NoError(t, err)
	require.Equal(t, []*handpb.RespondEvent{{Type: handpb.RespondEvent_ERROR, Error: "bad request"}}, stream.events)
}

// channelRespondStub emits a single stream event with configurable agent channel name.
type channelRespondStub struct {
	agentstub.AgentServiceStub
	channel string
	text    string
}

func (s *channelRespondStub) Respond(_ context.Context, _ string, opts agent.RespondOptions) (string, error) {
	if opts.OnEvent != nil {
		opts.OnEvent(agent.Event{Channel: s.channel, Text: s.text})
	}
	return "", nil
}

// bufferedReplyStub returns a final reply without invoking OnEvent (non-streaming path).
type bufferedReplyStub struct {
	agentstub.AgentServiceStub
	reply             string
	capturedSessionID string
}

func (s *bufferedReplyStub) Respond(_ context.Context, _ string, opts agent.RespondOptions) (string, error) {
	s.capturedSessionID = opts.SessionID
	return s.reply, nil
}

func TestService_CreateSessionReturnsSummary(t *testing.T) {
	stub := &agentstub.AgentServiceStub{CreatedSession: storage.Session{ID: "project-a"}}
	svc := NewService(stub)

	resp, err := svc.CreateSession(context.Background(), &handpb.CreateSessionRequest{Id: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", resp.GetSession().GetId())
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

		requireStatusError(t, err, codes.Internal, "agent handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{})

		resp, err := svc.CreateSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "create session request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{Err: errors.New("session already exists")})

		resp, err := svc.CreateSession(context.Background(), &handpb.CreateSessionRequest{Id: "project-a"})

		requireStatusError(t, err, codes.AlreadyExists, "session already exists")
		require.Nil(t, resp)
	})
}

func TestService_ListSessionsReturnsItems(t *testing.T) {
	stub := &agentstub.AgentServiceStub{Sessions: []storage.Session{{ID: "default"}, {ID: "project-a"}}}
	svc := NewService(stub)

	resp, err := svc.ListSessions(context.Background(), &handpb.ListSessionsRequest{})

	require.NoError(t, err)
	require.Len(t, resp.GetSessions(), 2)
	require.Equal(t, "default", resp.GetSessions()[0].GetId())
	require.Equal(t, "project-a", resp.GetSessions()[1].GetId())
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

		requireStatusError(t, err, codes.Internal, "agent handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{})

		resp, err := svc.ListSessions(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "list sessions request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{Err: errors.New("boom")})

		resp, err := svc.ListSessions(context.Background(), &handpb.ListSessionsRequest{})

		requireStatusError(t, err, codes.Internal, "boom")
		require.Nil(t, resp)
	})
}

func TestService_UseSessionReturnsSessionID(t *testing.T) {
	svc := NewService(&agentstub.AgentServiceStub{})

	resp, err := svc.UseSession(context.Background(), &handpb.UseSessionRequest{Id: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", resp.GetId())
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

		requireStatusError(t, err, codes.Internal, "agent handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{})

		resp, err := svc.UseSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "use session request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{Err: errors.New("session not found")})

		resp, err := svc.UseSession(context.Background(), &handpb.UseSessionRequest{Id: "project-a"})

		requireStatusError(t, err, codes.NotFound, "session not found")
		require.Nil(t, resp)
	})
}

func TestService_CompactSessionReturnsResult(t *testing.T) {
	now := time.Unix(123, 0).UTC()
	svc := NewService(&agentstub.AgentServiceStub{CompactResult: agent.CompactSessionResult{
		SessionID:            "project-a",
		SourceEndOffset:      12,
		SourceMessageCount:   20,
		UpdatedAt:            now,
		CurrentContextLength: 4000,
		TotalContextLength:   128000,
	}})

	resp, err := svc.CompactSession(context.Background(), &handpb.CompactSessionRequest{Id: "project-a"})

	require.NoError(t, err)
	require.Equal(t, "project-a", resp.GetId())
	require.EqualValues(t, 12, resp.GetSourceEndOffset())
	require.EqualValues(t, 20, resp.GetSourceMessageCount())
	require.Equal(t, now, resp.GetUpdatedAt().AsTime().UTC())
	require.EqualValues(t, 4000, resp.GetCurrentContextLength())
	require.EqualValues(t, 128000, resp.GetTotalContextLength())
}

func TestService_CompactSessionRejectsInvalidState(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var svc *Service

		resp, err := svc.CompactSession(context.Background(), &handpb.CompactSessionRequest{})

		requireStatusError(t, err, codes.Internal, "service is required")
		require.Nil(t, resp)
	})

	t.Run("missing handler", func(t *testing.T) {
		svc := NewService(nil)

		resp, err := svc.CompactSession(context.Background(), &handpb.CompactSessionRequest{})

		requireStatusError(t, err, codes.Internal, "agent handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{})

		resp, err := svc.CompactSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "compact session request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{Err: errors.New("session not found")})

		resp, err := svc.CompactSession(context.Background(), &handpb.CompactSessionRequest{Id: "project-a"})

		requireStatusError(t, err, codes.NotFound, "session not found")
		require.Nil(t, resp)
	})
}

func TestService_GetSessionReturnsResult(t *testing.T) {
	created := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 3, 2, 15, 30, 0, 0, time.UTC)
	svc := NewService(&agentstub.AgentServiceStub{StatusResult: agent.SessionContextStatus{
		SessionID:        "project-a",
		Offset:           12,
		Size:             20,
		Length:           128000,
		Used:             64000,
		Remaining:        64000,
		UsedPct:          0.5,
		RemainingPct:     0.5,
		CreatedAt:        created,
		UpdatedAt:        updated,
		CompactionStatus: "running",
	}})

	resp, err := svc.GetSession(context.Background(), &handpb.GetSessionRequest{
		Context: &handpb.GetSessionRequestContext{Id: "project-a"},
	})

	require.NoError(t, err)
	require.Equal(t, "project-a", resp.GetId())
	require.NotNil(t, resp.GetContext())
	require.EqualValues(t, 12, resp.GetContext().GetOffset())
	require.EqualValues(t, 20, resp.GetSize())
	require.EqualValues(t, 128000, resp.GetContext().GetLength())
	require.EqualValues(t, 64000, resp.GetContext().GetUsed())
	require.EqualValues(t, 64000, resp.GetContext().GetRemaining())
	require.Equal(t, 0.5, resp.GetContext().GetUsedPct())
	require.Equal(t, 0.5, resp.GetContext().GetRemainingPct())
	require.Equal(t, timestamppb.New(created), resp.GetCreatedAt())
	require.Equal(t, timestamppb.New(updated), resp.GetUpdatedAt())
	require.Equal(t, "running", resp.GetCompactionStatus())
}

func TestService_GetSessionRejectsInvalidState(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var svc *Service

		resp, err := svc.GetSession(context.Background(), &handpb.GetSessionRequest{})

		requireStatusError(t, err, codes.Internal, "service is required")
		require.Nil(t, resp)
	})

	t.Run("missing handler", func(t *testing.T) {
		svc := NewService(nil)

		resp, err := svc.GetSession(context.Background(), &handpb.GetSessionRequest{})

		requireStatusError(t, err, codes.Internal, "agent handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{})

		resp, err := svc.GetSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "get session request is required")
		require.Nil(t, resp)
	})

	t.Run("nil context", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{})

		resp, err := svc.GetSession(context.Background(), &handpb.GetSessionRequest{})

		requireStatusError(t, err, codes.InvalidArgument, "get session request context is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{Err: errors.New("session not found")})

		resp, err := svc.GetSession(context.Background(), &handpb.GetSessionRequest{
			Context: &handpb.GetSessionRequestContext{Id: "project-a"},
		})

		requireStatusError(t, err, codes.NotFound, "session not found")
		require.Nil(t, resp)
	})
}

func TestService_CurrentSessionReturnsValue(t *testing.T) {
	svc := NewService(&agentstub.AgentServiceStub{CurrentSessionID: storage.DefaultSessionID})

	resp, err := svc.CurrentSession(context.Background(), &handpb.CurrentSessionRequest{})

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, resp.GetId())
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

		requireStatusError(t, err, codes.Internal, "agent handler is required")
		require.Nil(t, resp)
	})

	t.Run("nil request", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{})

		resp, err := svc.CurrentSession(context.Background(), nil)

		requireStatusError(t, err, codes.InvalidArgument, "current session request is required")
		require.Nil(t, resp)
	})

	t.Run("handler error", func(t *testing.T) {
		svc := NewService(&agentstub.AgentServiceStub{Err: errors.New("boom")})

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
