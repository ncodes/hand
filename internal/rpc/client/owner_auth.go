package client

import (
	"context"
	"errors"
	"sync"

	"github.com/wandxy/morph/internal/rpc/rpcauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func ownerUnaryClientInterceptor(credential []byte) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		request any,
		reply any,
		conn *grpc.ClientConn,
		invoke grpc.UnaryInvoker,
		callOptions ...grpc.CallOption,
	) error {
		authenticated, err := rpcauth.WithOutgoingProof(ctx, method, credential, request)
		if err != nil {
			return err
		}

		return invoke(authenticated, method, request, reply, conn, callOptions...)
	}
}

func ownerStreamClientInterceptor(credential []byte) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		conn *grpc.ClientConn,
		method string,
		stream grpc.Streamer,
		callOptions ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if len(credential) == 0 {
			return stream(ctx, desc, conn, method, callOptions...)
		}

		return &ownerClientStream{
			ctx: ctx, desc: desc, conn: conn, method: method, stream: stream,
			credential: append([]byte(nil), credential...), callOptions: callOptions,
		}, nil
	}
}

type ownerClientStream struct {
	mu          sync.Mutex
	ctx         context.Context
	desc        *grpc.StreamDesc
	conn        *grpc.ClientConn
	method      string
	stream      grpc.Streamer
	credential  []byte
	callOptions []grpc.CallOption
	client      grpc.ClientStream
	startErr    error
}

func (s *ownerClientStream) Header() (metadata.MD, error) {
	client, err := s.getStartedClient()
	if err != nil {
		return nil, err
	}

	return client.Header()
}

func (s *ownerClientStream) Trailer() metadata.MD {
	client, err := s.getStartedClient()
	if err != nil {
		return nil
	}

	return client.Trailer()
}

func (s *ownerClientStream) CloseSend() error {
	client, err := s.getStartedClient()
	if err != nil {
		return err
	}

	return client.CloseSend()
}

func (s *ownerClientStream) Context() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client.Context()
	}

	return s.ctx
}

func (s *ownerClientStream) SendMsg(message any) error {
	client, err := s.start(message)
	if err != nil {
		return err
	}

	return client.SendMsg(message)
}

func (s *ownerClientStream) RecvMsg(message any) error {
	client, err := s.getStartedClient()
	if err != nil {
		return err
	}

	return client.RecvMsg(message)
}

func (s *ownerClientStream) start(request any) (grpc.ClientStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil || s.startErr != nil {
		return s.client, s.startErr
	}
	authenticated, err := rpcauth.WithOutgoingProof(s.ctx, s.method, s.credential, request)
	if err != nil {
		s.startErr = err
		return nil, err
	}
	s.client, s.startErr = s.stream(
		authenticated, s.desc, s.conn, s.method, s.callOptions...,
	)

	return s.client, s.startErr
}

func (s *ownerClientStream) getStartedClient() (grpc.ClientStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil || s.startErr != nil {
		return s.client, s.startErr
	}

	return nil, errors.New("RPC owner stream request must be sent before receiving a response")
}
