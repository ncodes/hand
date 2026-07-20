package server

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/rpc/rpcauth"
	"github.com/wandxy/morph/internal/rpc/rpcmeta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const testOwnerMethod = "/morph.BrowserService/Start"

func TestOwnerUnaryServerInterceptor_AuthenticatesLocalOwnerProof(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	interceptor := ownerUnaryServerInterceptor(rpcauth.NewValidator(credential), "default")
	ctx := getOwnerProofContext(t, testOwnerMethod, credential, permissions.SurfaceCLI, true)

	response, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{FullMethod: testOwnerMethod}, func(
		ctx context.Context,
		req any,
	) (any, error) {
		require.Equal(t, "request", req)
		return rpcmeta.PermissionActorFromIncomingContext(ctx), nil
	})
	require.NoError(t, err)
	require.Equal(t, permissions.Actor{Kind: permissions.ActorLocalOwner, ID: "default"}, response)
}

func TestOwnerUnaryServerInterceptor_DoesNotTrustMetadataWithoutProof(t *testing.T) {
	interceptor := ownerUnaryServerInterceptor(rpcauth.NewValidator([]byte("credential")), "default")
	ctx := getOwnerProofContext(t, testOwnerMethod, nil, permissions.SurfaceTUI, true)

	response, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: testOwnerMethod}, func(
		ctx context.Context,
		_ any,
	) (any, error) {
		return rpcmeta.PermissionActorFromIncomingContext(ctx), nil
	})
	require.NoError(t, err)
	require.Equal(t, permissions.Actor{Kind: permissions.ActorRPCClient}, response)
}

func TestOwnerUnaryServerInterceptor_RejectsInvalidProof(t *testing.T) {
	interceptor := ownerUnaryServerInterceptor(rpcauth.NewValidator([]byte("expected")), "default")
	ctx := getOwnerProofContext(t, testOwnerMethod, []byte("wrong"), permissions.SurfaceCLI, true)

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: testOwnerMethod}, func(
		context.Context,
		any,
	) (any, error) {
		t.Fatal("handler must not run")
		return nil, nil
	})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestOwnerStreamServerInterceptor_UsesAuthenticatedStreamContext(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	interceptor := ownerStreamServerInterceptor(rpcauth.NewValidator(credential), "default")
	stream := &ownerAuthTestStream{ctx: getOwnerProofContext(
		t, testOwnerMethod, credential, permissions.SurfaceCLI, true,
	)}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: testOwnerMethod}, func(
		_ any,
		stream grpc.ServerStream,
	) error {
		var request string
		require.NoError(t, stream.RecvMsg(&request))
		require.Equal(t, "request", request)
		require.Equal(
			t, permissions.Actor{Kind: permissions.ActorLocalOwner, ID: "default"},
			rpcmeta.PermissionActorFromIncomingContext(stream.Context()),
		)
		return nil
	})
	require.NoError(t, err)
}

func TestOwnerStreamServerInterceptor_RejectsInvalidProofOnFirstRequest(t *testing.T) {
	interceptor := ownerStreamServerInterceptor(rpcauth.NewValidator([]byte("expected")), "default")
	stream := &ownerAuthTestStream{ctx: getOwnerProofContext(
		t, testOwnerMethod, []byte("wrong"), permissions.SurfaceCLI, true,
	)}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: testOwnerMethod}, func(
		_ any,
		stream grpc.ServerStream,
	) error {
		var request string
		return stream.RecvMsg(&request)
	})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestOwnerStreamServerInterceptor_PassesThroughStreamWithoutProof(t *testing.T) {
	interceptor := ownerStreamServerInterceptor(rpcauth.NewValidator([]byte("credential")), "default")
	stream := &ownerAuthTestStream{ctx: context.Background()}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: testOwnerMethod}, func(
		_ any,
		actual grpc.ServerStream,
	) error {
		require.Same(t, stream, actual)
		return nil
	})
	require.NoError(t, err)
}

func TestOwnerStreamServerInterceptor_RejectsOwnerProofForClientStreaming(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	interceptor := ownerStreamServerInterceptor(rpcauth.NewValidator(credential), "default")
	stream := &ownerAuthTestStream{ctx: getOwnerProofContext(
		t, testOwnerMethod, credential, permissions.SurfaceCLI, true,
	)}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{
		FullMethod: testOwnerMethod, IsClientStream: true, IsServerStream: true,
	}, func(any, grpc.ServerStream) error {
		t.Fatal("handler must not run")
		return nil
	})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestOwnerServerStream_ContextIsSafeDuringFirstReceive(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	stream := &ownerServerStream{
		ServerStream: &ownerAuthTestStream{ctx: getOwnerProofContext(
			t, testOwnerMethod, credential, permissions.SurfaceCLI, true,
		)},
		ctx:    getOwnerProofContext(t, testOwnerMethod, credential, permissions.SurfaceCLI, true),
		method: testOwnerMethod, validator: rpcauth.NewValidator(credential), principalID: "default",
	}
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 100 {
				_ = stream.Context()
			}
		}()
	}
	var request string
	require.NoError(t, stream.RecvMsg(&request))
	wait.Wait()
}

func TestAuthenticatedOwnerProof_StillRequiresLoopbackAndSupportedSurface(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	for _, test := range []struct {
		name     string
		surface  permissions.Surface
		loopback bool
	}{
		{name: "RPC surface", surface: permissions.SurfaceRPC, loopback: true},
		{name: "remote peer", surface: permissions.SurfaceCLI, loopback: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			interceptor := ownerUnaryServerInterceptor(rpcauth.NewValidator(credential), "default")
			ctx := getOwnerProofContext(t, testOwnerMethod, credential, test.surface, test.loopback)
			response, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{FullMethod: testOwnerMethod}, func(
				ctx context.Context,
				_ any,
			) (any, error) {
				return rpcmeta.PermissionActorFromIncomingContext(ctx), nil
			})
			require.NoError(t, err)
			require.Equal(t, permissions.Actor{Kind: permissions.ActorRPCClient}, response)
		})
	}
}

type ownerAuthTestStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *ownerAuthTestStream) Context() context.Context {
	return s.ctx
}

func (s *ownerAuthTestStream) RecvMsg(message any) error {
	request, ok := message.(*string)
	if !ok {
		return errors.New("unexpected request type")
	}
	*request = "request"
	return nil
}

func getOwnerProofContext(
	t *testing.T,
	method string,
	credential []byte,
	surface permissions.Surface,
	loopback bool,
) context.Context {
	t.Helper()
	ctx := rpcmeta.WithOutgoingPermissionSurface(context.Background(), surface)
	var err error
	ctx, err = rpcauth.WithOutgoingProof(ctx, method, credential, "request")
	require.NoError(t, err)
	values, _ := metadata.FromOutgoingContext(ctx)
	incoming := metadata.NewIncomingContext(context.Background(), values)
	ip := net.ParseIP("192.0.2.10")
	if loopback {
		ip = net.ParseIP("127.0.0.1")
	}
	return peer.NewContext(incoming, &peer.Peer{Addr: &net.TCPAddr{IP: ip, Port: 50051}})
}
