package rpc

import (
	"context"
	"errors"

	"github.com/wandxy/hand/internal/agent"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
)

type chatter interface {
	Chat(context.Context, string, agent.ChatOptions) (string, error)
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
		return nil, errors.New("service is required")
	}
	if s.chatter == nil {
		return nil, errors.New("chat handler is required")
	}
	if req == nil {
		return nil, errors.New("chat request is required")
	}

	reply, err := s.chatter.Chat(ctx, req.Message, agent.ChatOptions{Instruct: req.Instruct})
	if err != nil {
		return nil, err
	}

	return &handpb.ChatResponse{Message: reply}, nil
}
