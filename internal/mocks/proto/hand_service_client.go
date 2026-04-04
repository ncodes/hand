package proto

import (
	"context"

	"google.golang.org/grpc"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type HandServiceClientStub struct {
	Req         *handpb.RespondRequest
	Resp        *handpb.RespondResponse
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

func (s *HandServiceClientStub) Respond(_ context.Context, req *handpb.RespondRequest, _ ...grpc.CallOption) (*handpb.RespondResponse, error) {
	s.Req = req
	return s.Resp, s.Err
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
