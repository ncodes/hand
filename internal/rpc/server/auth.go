package server

import (
	"context"
	"sync"

	"github.com/wandxy/morph/internal/rpc/rpcauth"
	"github.com/wandxy/morph/internal/rpc/rpcmeta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ownerUnaryServerInterceptor(
	validator *rpcauth.Validator,
	principalID string,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ownerCtx, err := getAuthenticatedOwnerContext(ctx, info.FullMethod, req, validator, principalID)
		if err != nil {
			return nil, err
		}

		return handler(ownerCtx, req)
	}
}

func ownerStreamServerInterceptor(
	validator *rpcauth.Validator,
	principalID string,
) grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if !rpcauth.HasIncomingProof(stream.Context()) {
			return handler(srv, stream)
		}
		if info.IsClientStream {
			return status.Error(codes.Unimplemented, "RPC owner proof is not supported for client-streaming methods")
		}
		return handler(srv, &ownerServerStream{
			ServerStream: stream, ctx: stream.Context(), method: info.FullMethod,
			validator: validator, principalID: principalID,
		})
	}
}

type ownerServerStream struct {
	grpc.ServerStream
	mu          sync.RWMutex
	ctx         context.Context
	method      string
	validator   *rpcauth.Validator
	principalID string
	validate    sync.Once
	validateErr error
}

func (s *ownerServerStream) Context() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.ctx
}

func (s *ownerServerStream) RecvMsg(message any) error {
	if err := s.ServerStream.RecvMsg(message); err != nil {
		return err
	}
	s.validate.Do(func() {
		ownerCtx, err := getAuthenticatedOwnerContext(
			s.Context(), s.method, message, s.validator, s.principalID,
		)
		if ownerCtx != nil {
			s.mu.Lock()
			s.ctx = ownerCtx
			s.mu.Unlock()
		}
		s.validateErr = err
	})

	return s.validateErr
}

func getAuthenticatedOwnerContext(
	ctx context.Context,
	method string,
	request any,
	validator *rpcauth.Validator,
	principalID string,
) (context.Context, error) {
	if !rpcauth.HasIncomingProof(ctx) {
		return ctx, nil
	}
	if validator == nil {
		return nil, status.Error(codes.Unauthenticated, "RPC owner authentication is unavailable")
	}
	if err := validator.Validate(ctx, method, request); err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	return rpcmeta.WithAuthenticatedLocalOwner(ctx, principalID), nil
}
