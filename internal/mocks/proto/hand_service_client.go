package proto

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type HandServiceClientStub struct {
	Req         *handpb.RespondRequest
	Events      []*handpb.RespondEvent
	Err         error
	CreateResp  *handpb.CreateSessionResponse
	CreateReq   *handpb.CreateSessionRequest
	ListResp    *handpb.ListSessionsResponse
	ListReq     *handpb.ListSessionsRequest
	UseReq      *handpb.UseSessionRequest
	CurrentResp *handpb.CurrentSessionResponse
	CompactResp *handpb.CompactSessionResponse
	CompactReq  *handpb.CompactSessionRequest
	StatusResp  *handpb.GetSessionResponse
	StatusReq   *handpb.GetSessionRequest
}

func (s *HandServiceClientStub) Respond(_ context.Context, req *handpb.RespondRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[handpb.RespondEvent], error) {
	s.Req = req
	if s.Err != nil {
		return nil, s.Err
	}
	return &respondStreamStub{events: s.Events}, nil
}

func (s *HandServiceClientStub) CreateSession(_ context.Context, req *handpb.CreateSessionRequest, _ ...grpc.CallOption) (*handpb.CreateSessionResponse, error) {
	s.CreateReq = req
	return s.CreateResp, s.Err
}

func (s *HandServiceClientStub) ListSessions(_ context.Context, req *handpb.ListSessionsRequest, _ ...grpc.CallOption) (*handpb.ListSessionsResponse, error) {
	s.ListReq = req
	return s.ListResp, s.Err
}

func (s *HandServiceClientStub) UseSession(_ context.Context, req *handpb.UseSessionRequest, _ ...grpc.CallOption) (*handpb.UseSessionResponse, error) {
	s.UseReq = req
	return &handpb.UseSessionResponse{Id: req.GetId()}, s.Err
}

func (s *HandServiceClientStub) CurrentSession(context.Context, *handpb.CurrentSessionRequest, ...grpc.CallOption) (*handpb.CurrentSessionResponse, error) {
	return s.CurrentResp, s.Err
}

func (s *HandServiceClientStub) CompactSession(_ context.Context, req *handpb.CompactSessionRequest, _ ...grpc.CallOption) (*handpb.CompactSessionResponse, error) {
	s.CompactReq = req
	return s.CompactResp, s.Err
}

func (s *HandServiceClientStub) GetSession(_ context.Context, req *handpb.GetSessionRequest, _ ...grpc.CallOption) (*handpb.GetSessionResponse, error) {
	s.StatusReq = req
	return s.StatusResp, s.Err
}

type respondStreamStub struct {
	events []*handpb.RespondEvent
	index  int
}

func (s *respondStreamStub) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (s *respondStreamStub) Trailer() metadata.MD {
	return metadata.MD{}
}

func (s *respondStreamStub) CloseSend() error {
	return nil
}

func (s *respondStreamStub) Context() context.Context {
	return context.Background()
}

func (s *respondStreamStub) SendMsg(any) error {
	return nil
}

func (s *respondStreamStub) RecvMsg(any) error {
	return nil
}

func (s *respondStreamStub) Recv() (*handpb.RespondEvent, error) {
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
