package proto

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

// HandServiceClientStub is a test stub for hand service client.
type HandServiceClientStub struct {
	Req                *handpb.RespondRequest
	Events             []*handpb.RespondEvent
	Err                error
	RecvErr            error
	CreateResp         *handpb.CreateSessionResponse
	CreateReq          *handpb.CreateSessionRequest
	ListResp           *handpb.ListSessionsResponse
	ListReq            *handpb.ListSessionsRequest
	UseReq             *handpb.UseSessionRequest
	ArchiveReq         *handpb.ArchiveSessionRequest
	UnarchiveReq       *handpb.UnarchiveSessionRequest
	UnarchiveResp      *handpb.UnarchiveSessionResponse
	RenameReq          *handpb.RenameSessionRequest
	RenameResp         *handpb.RenameSessionResponse
	CurrentResp        *handpb.CurrentSessionResponse
	CompactResp        *handpb.CompactSessionResponse
	CompactReq         *handpb.CompactSessionRequest
	RepairResp         *handpb.RepairSessionResponse
	RepairReq          *handpb.RepairSessionRequest
	StatusResp         *handpb.GetSessionStatusResponse
	StatusReq          *handpb.GetSessionStatusRequest
	TimelineResp       *handpb.GetSessionTimelineResponse
	TimelineReq        *handpb.GetSessionTimelineRequest
	ProvidersResp      *handpb.ListProvidersResponse
	ProvidersReq       *handpb.ListProvidersRequest
	ModelsResp         *handpb.ListModelsResponse
	ModelsReq          *handpb.ListModelsRequest
	SelectReq          *handpb.SelectModelRequest
	SelectResp         *handpb.SelectModelResponse
	APIKeyReq          *handpb.SetProviderAPIKeyRequest
	APIKeyResp         *handpb.SetProviderAPIKeyResponse
	GatewayStatusReq   *handpb.GetGatewayStatusRequest
	GatewayStatusResp  *handpb.GetGatewayStatusResponse
	GatewayStartReq    *handpb.StartGatewayRequest
	GatewayStartResp   *handpb.StartGatewayResponse
	GatewayStopReq     *handpb.StopGatewayRequest
	GatewayStopResp    *handpb.StopGatewayResponse
	GatewayRestartReq  *handpb.RestartGatewayRequest
	GatewayRestartResp *handpb.RestartGatewayResponse
	PairingsReq        *handpb.ListGatewayPairingsRequest
	PairingsResp       *handpb.ListGatewayPairingsResponse
	ApproveReq         *handpb.ApproveGatewayPairingRequest
	ApproveResp        *handpb.ApproveGatewayPairingResponse
	RevokeReq          *handpb.RevokeGatewayPairingRequest
	ClearReq           *handpb.ClearPendingGatewayPairingsRequest
	OnRespond          func()
	OnListModels       func()
}

func (s *HandServiceClientStub) Respond(_ context.Context, req *handpb.RespondRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[handpb.RespondEvent], error) {
	if s.OnRespond != nil {
		s.OnRespond()
	}
	s.Req = req
	if s.Err != nil {
		return nil, s.Err
	}
	return &respondStreamStub{events: s.Events, err: s.RecvErr}, nil
}

func (s *HandServiceClientStub) Create(_ context.Context, req *handpb.CreateSessionRequest, _ ...grpc.CallOption) (*handpb.CreateSessionResponse, error) {
	s.CreateReq = req
	return s.CreateResp, s.Err
}

func (s *HandServiceClientStub) List(_ context.Context, req *handpb.ListSessionsRequest, _ ...grpc.CallOption) (*handpb.ListSessionsResponse, error) {
	s.ListReq = req
	return s.ListResp, s.Err
}

func (s *HandServiceClientStub) Use(_ context.Context, req *handpb.UseSessionRequest, _ ...grpc.CallOption) (*handpb.UseSessionResponse, error) {
	s.UseReq = req
	return &handpb.UseSessionResponse{Id: req.GetId()}, s.Err
}

func (s *HandServiceClientStub) Archive(_ context.Context, req *handpb.ArchiveSessionRequest, _ ...grpc.CallOption) (*handpb.ArchiveSessionResponse, error) {
	s.ArchiveReq = req
	return &handpb.ArchiveSessionResponse{Id: req.GetId()}, s.Err
}

func (s *HandServiceClientStub) Unarchive(_ context.Context, req *handpb.UnarchiveSessionRequest, _ ...grpc.CallOption) (*handpb.UnarchiveSessionResponse, error) {
	s.UnarchiveReq = req
	return s.UnarchiveResp, s.Err
}

func (s *HandServiceClientStub) Rename(_ context.Context, req *handpb.RenameSessionRequest, _ ...grpc.CallOption) (*handpb.RenameSessionResponse, error) {
	s.RenameReq = req
	return s.RenameResp, s.Err
}

func (s *HandServiceClientStub) Current(context.Context, *handpb.CurrentSessionRequest, ...grpc.CallOption) (*handpb.CurrentSessionResponse, error) {
	return s.CurrentResp, s.Err
}

func (s *HandServiceClientStub) Compact(_ context.Context, req *handpb.CompactSessionRequest, _ ...grpc.CallOption) (*handpb.CompactSessionResponse, error) {
	s.CompactReq = req
	return s.CompactResp, s.Err
}

func (s *HandServiceClientStub) Repair(_ context.Context, req *handpb.RepairSessionRequest, _ ...grpc.CallOption) (*handpb.RepairSessionResponse, error) {
	s.RepairReq = req
	return s.RepairResp, s.Err
}

func (s *HandServiceClientStub) Status(_ context.Context, req *handpb.GetSessionStatusRequest, _ ...grpc.CallOption) (*handpb.GetSessionStatusResponse, error) {
	s.StatusReq = req
	return s.StatusResp, s.Err
}

func (s *HandServiceClientStub) Timeline(_ context.Context, req *handpb.GetSessionTimelineRequest, _ ...grpc.CallOption) (*handpb.GetSessionTimelineResponse, error) {
	s.TimelineReq = req
	return s.TimelineResp, s.Err
}

func (s *HandServiceClientStub) ListModels(_ context.Context, req *handpb.ListModelsRequest, _ ...grpc.CallOption) (*handpb.ListModelsResponse, error) {
	if s.OnListModels != nil {
		s.OnListModels()
	}
	s.ModelsReq = req
	return s.ModelsResp, s.Err
}

func (s *HandServiceClientStub) ListProviders(_ context.Context, req *handpb.ListProvidersRequest, _ ...grpc.CallOption) (*handpb.ListProvidersResponse, error) {
	s.ProvidersReq = req
	return s.ProvidersResp, s.Err
}

func (s *HandServiceClientStub) SelectModel(_ context.Context, req *handpb.SelectModelRequest, _ ...grpc.CallOption) (*handpb.SelectModelResponse, error) {
	s.SelectReq = req
	return s.SelectResp, s.Err
}

func (s *HandServiceClientStub) SetProviderAPIKey(_ context.Context, req *handpb.SetProviderAPIKeyRequest, _ ...grpc.CallOption) (*handpb.SetProviderAPIKeyResponse, error) {
	s.APIKeyReq = req
	return s.APIKeyResp, s.Err
}

func (s *HandServiceClientStub) GatewayStatus(_ context.Context, req *handpb.GetGatewayStatusRequest, _ ...grpc.CallOption) (*handpb.GetGatewayStatusResponse, error) {
	s.GatewayStatusReq = req
	return s.GatewayStatusResp, s.Err
}

func (s *HandServiceClientStub) Start(_ context.Context, req *handpb.StartGatewayRequest, _ ...grpc.CallOption) (*handpb.StartGatewayResponse, error) {
	s.GatewayStartReq = req
	return s.GatewayStartResp, s.Err
}

func (s *HandServiceClientStub) Stop(_ context.Context, req *handpb.StopGatewayRequest, _ ...grpc.CallOption) (*handpb.StopGatewayResponse, error) {
	s.GatewayStopReq = req
	return s.GatewayStopResp, s.Err
}

func (s *HandServiceClientStub) Restart(_ context.Context, req *handpb.RestartGatewayRequest, _ ...grpc.CallOption) (*handpb.RestartGatewayResponse, error) {
	s.GatewayRestartReq = req
	return s.GatewayRestartResp, s.Err
}

func (s *HandServiceClientStub) ListPairings(_ context.Context, req *handpb.ListGatewayPairingsRequest, _ ...grpc.CallOption) (*handpb.ListGatewayPairingsResponse, error) {
	s.PairingsReq = req
	return s.PairingsResp, s.Err
}

func (s *HandServiceClientStub) ApprovePairing(_ context.Context, req *handpb.ApproveGatewayPairingRequest, _ ...grpc.CallOption) (*handpb.ApproveGatewayPairingResponse, error) {
	s.ApproveReq = req
	return s.ApproveResp, s.Err
}

func (s *HandServiceClientStub) RevokePairing(_ context.Context, req *handpb.RevokeGatewayPairingRequest, _ ...grpc.CallOption) (*handpb.RevokeGatewayPairingResponse, error) {
	s.RevokeReq = req
	return &handpb.RevokeGatewayPairingResponse{}, s.Err
}

func (s *HandServiceClientStub) ClearPendingPairings(_ context.Context, req *handpb.ClearPendingGatewayPairingsRequest, _ ...grpc.CallOption) (*handpb.ClearPendingGatewayPairingsResponse, error) {
	s.ClearReq = req
	return &handpb.ClearPendingGatewayPairingsResponse{}, s.Err
}

type respondStreamStub struct {
	events []*handpb.RespondEvent
	err    error
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
		if s.err != nil {
			return nil, s.err
		}
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
