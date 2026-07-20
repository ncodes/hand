package client

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/profile"
	"github.com/wandxy/morph/internal/rpc/rpcauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestOwnerClientInterceptors_AttachMethodBoundProof(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	validator := rpcauth.NewValidator(credential)
	unary := ownerUnaryClientInterceptor(credential)
	unaryRequest := "unary request"
	err := unary(
		context.Background(), "/morph.BrowserService/Start", unaryRequest, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			return validator.Validate(
				outgoingToIncomingContext(t, ctx), "/morph.BrowserService/Start", unaryRequest,
			)
		},
	)
	require.NoError(t, err)

	streamValidator := rpcauth.NewValidator(credential)
	stream := ownerStreamClientInterceptor(credential)
	clientStream, err := stream(
		context.Background(), &grpc.StreamDesc{}, nil, "/morph.BrowserService/ListSessions",
		func(
			ctx context.Context,
			_ *grpc.StreamDesc,
			_ *grpc.ClientConn,
			method string,
			_ ...grpc.CallOption,
		) (grpc.ClientStream, error) {
			return &ownerAuthClientStream{}, streamValidator.Validate(
				outgoingToIncomingContext(t, ctx), method, "stream request",
			)
		},
	)
	require.NoError(t, err)
	require.NoError(t, clientStream.SendMsg("stream request"))
}

func TestOwnerClientInterceptors_PassThroughWithoutCredential(t *testing.T) {
	ctx := context.Background()
	unaryCalled := false
	err := ownerUnaryClientInterceptor(nil)(
		ctx, "/morph.MorphService/Respond", "request", nil, nil,
		func(actual context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			unaryCalled = true
			require.Equal(t, ctx, actual)
			return nil
		},
	)
	require.NoError(t, err)
	require.True(t, unaryCalled)

	underlying := &ownerAuthClientStream{}
	streamCalled := false
	stream, err := ownerStreamClientInterceptor(nil)(
		ctx, &grpc.StreamDesc{}, nil, "/morph.MorphService/Respond",
		func(
			actual context.Context,
			_ *grpc.StreamDesc,
			_ *grpc.ClientConn,
			_ string,
			_ ...grpc.CallOption,
		) (grpc.ClientStream, error) {
			streamCalled = true
			require.Equal(t, ctx, actual)
			return underlying, nil
		},
	)
	require.NoError(t, err)
	require.True(t, streamCalled)
	require.Same(t, underlying, stream)
}

type ownerAuthClientStream struct {
	grpc.ClientStream
}

func (s *ownerAuthClientStream) SendMsg(any) error {
	return nil
}

func TestOwnerClientStream_RequiresRequestBeforeResponseOperations(t *testing.T) {
	stream := &ownerClientStream{ctx: context.Background()}

	header, err := stream.Header()
	require.EqualError(t, err, "RPC owner stream request must be sent before receiving a response")
	require.Nil(t, header)
	require.Nil(t, stream.Trailer())
	require.EqualError(t, stream.CloseSend(), "RPC owner stream request must be sent before receiving a response")
	require.Equal(t, stream.ctx, stream.Context())
	require.EqualError(
		t, stream.RecvMsg(nil), "RPC owner stream request must be sent before receiving a response",
	)
}

func TestOwnerClientStream_DelegatesOperationsAfterSendingRequest(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	validator := rpcauth.NewValidator(credential)
	underlying := &recordingOwnerClientStream{ctx: context.WithValue(context.Background(), testContextKey{}, "stream")}
	stream := &ownerClientStream{
		ctx: context.Background(), desc: &grpc.StreamDesc{}, method: "/morph.BrowserService/ReadArtifact",
		credential: credential,
		stream: func(
			ctx context.Context,
			_ *grpc.StreamDesc,
			_ *grpc.ClientConn,
			method string,
			_ ...grpc.CallOption,
		) (grpc.ClientStream, error) {
			require.NoError(t, validator.Validate(outgoingToIncomingContext(t, ctx), method, "request"))
			return underlying, nil
		},
	}

	require.NoError(t, stream.SendMsg("request"))
	header, err := stream.Header()
	require.NoError(t, err)
	require.Equal(t, metadata.Pairs("header", "value"), header)
	require.Equal(t, metadata.Pairs("trailer", "value"), stream.Trailer())
	require.Equal(t, "stream", stream.Context().Value(testContextKey{}))
	require.NoError(t, stream.RecvMsg(nil))
	require.NoError(t, stream.CloseSend())
	require.Equal(t, 1, underlying.sent)
	require.Equal(t, 1, underlying.received)
	require.Equal(t, 1, underlying.closed)
}

func TestOwnerClientStream_PreservesStartupFailure(t *testing.T) {
	streamError := errors.New("stream unavailable")
	starts := 0
	stream := &ownerClientStream{
		ctx: context.Background(), desc: &grpc.StreamDesc{}, method: "/morph.BrowserService/ReadArtifact",
		credential: []byte("credential"),
		stream: func(
			context.Context,
			*grpc.StreamDesc,
			*grpc.ClientConn,
			string,
			...grpc.CallOption,
		) (grpc.ClientStream, error) {
			starts++
			return nil, streamError
		},
	}

	require.ErrorIs(t, stream.SendMsg("request"), streamError)
	require.ErrorIs(t, stream.SendMsg("request"), streamError)
	require.ErrorIs(t, stream.RecvMsg(nil), streamError)
	require.Equal(t, 1, starts)
}

type testContextKey struct{}

type recordingOwnerClientStream struct {
	grpc.ClientStream
	ctx      context.Context
	sent     int
	received int
	closed   int
}

func (s *recordingOwnerClientStream) Header() (metadata.MD, error) {
	return metadata.Pairs("header", "value"), nil
}

func (s *recordingOwnerClientStream) Trailer() metadata.MD {
	return metadata.Pairs("trailer", "value")
}

func (s *recordingOwnerClientStream) CloseSend() error {
	s.closed++
	return nil
}

func (s *recordingOwnerClientStream) Context() context.Context {
	return s.ctx
}

func (s *recordingOwnerClientStream) SendMsg(any) error {
	s.sent++
	return nil
}

func (s *recordingOwnerClientStream) RecvMsg(any) error {
	s.received++
	return nil
}

func TestNewClient_RejectsInvalidLocalOwnerCredential(t *testing.T) {
	original := profile.Active()
	t.Cleanup(func() { profile.SetActive(original) })
	home := t.TempDir()
	require.NoError(t, os.Chmod(home, 0o700))
	profile.SetActive(profile.Profile{Name: "default", HomeDir: home})
	require.NoError(t, os.WriteFile(rpcauth.CredentialPath(home), []byte("invalid\n"), 0o600))

	client, err := NewClient(context.Background(), Options{
		Address: "127.0.0.1", Port: 50051, PermissionSurface: permissions.SurfaceCLI,
	})
	require.EqualError(
		t, err,
		"load RPC owner credential: RPC owner credential is invalid; run morph browser auth rotate, then restart the daemon",
	)
	require.Nil(t, client)
}

func TestNewClient_IgnoresMissingCredentialForNonOwnerAndPreDaemonClients(t *testing.T) {
	original := profile.Active()
	t.Cleanup(func() { profile.SetActive(original) })
	profile.SetActive(profile.Profile{Name: "default", HomeDir: t.TempDir()})

	for _, surface := range []permissions.Surface{permissions.SurfaceCLI, permissions.SurfaceRPC} {
		client, err := NewClient(context.Background(), Options{
			Address: "127.0.0.1", Port: 50051, PermissionSurface: surface,
		})
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NoError(t, client.Close())
	}
}

func outgoingToIncomingContext(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	values, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	return metadata.NewIncomingContext(context.Background(), values)
}
