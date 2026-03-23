package rpc

import (
	"context"

	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type Service struct {
	handpb.UnimplementedHandServiceServer
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Echo(_ context.Context, req *handpb.EchoRequest) (*handpb.EchoResponse, error) {
	if s == nil || req == nil {
		return &handpb.EchoResponse{}, nil
	}
	return &handpb.EchoResponse{Message: req.Message}, nil
}
