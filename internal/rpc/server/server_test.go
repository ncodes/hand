package server

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/browser"
	"github.com/wandxy/morph/internal/config"
	agentstub "github.com/wandxy/morph/internal/mocks/agentstub"
	"github.com/wandxy/morph/internal/permissions"
	morphpb "github.com/wandxy/morph/internal/rpc/proto"
	"github.com/wandxy/morph/internal/rpc/rpcauth"
	"github.com/wandxy/morph/internal/rpc/rpcmeta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestNew_RegistersMorphServiceWithoutHealth(t *testing.T) {
	server := New(&agentstub.AgentRunnerStub{}, Options{})

	serviceInfo := server.GetServiceInfo()
	require.Contains(t, serviceInfo, morphpb.MorphService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, morphpb.SessionService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, morphpb.BrowserService_ServiceDesc.ServiceName)
	require.NotContains(t, serviceInfo, healthgrpc.Health_ServiceDesc.ServiceName)
}

func TestNew_RegistersHealthWhenEnabled(t *testing.T) {
	server := New(&agentstub.AgentRunnerStub{}, Options{Health: true})

	serviceInfo := server.GetServiceInfo()
	require.Contains(t, serviceInfo, morphpb.MorphService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, morphpb.SessionService_ServiceDesc.ServiceName)
	require.Contains(t, serviceInfo, healthgrpc.Health_ServiceDesc.ServiceName)
}

func TestBrowserService_EndToEndRequiresOwnerProof(t *testing.T) {
	credential := []byte("01234567890123456789012345678901")
	cfg := config.NewDefaultConfig()
	runtime := &browserServerRuntime{status: browser.Status{
		Enabled:  true,
		Profiles: []browser.Profile{{Name: "default", Mode: config.BrowserProfileManagedEphemeral, Default: true, Available: true}},
	}, artifact: browser.ArtifactContent{Artifact: browser.Artifact{Handle: "artifact_1"}}}
	server := New(&agentstub.AgentRunnerStub{}, Options{
		Browser: runtime, BrowserConfig: cfg.Browser, BrowserCapability: true, ProfileName: "default",
		PermissionPolicy: permissions.Policy{Rules: []permissions.Rule{{Name: "allow", Decision: permissions.DecisionAllow}}},
		OwnerCredential:  credential, OwnerPrincipal: "default",
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	go func() { _ = server.Serve(listener) }()
	connection, err := grpc.NewClient(
		listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	client := morphpb.NewBrowserServiceClient(connection)

	_, err = client.Status(context.Background(), &morphpb.GetBrowserStatusRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	spoofed := rpcmeta.WithOutgoingPermissionSurface(context.Background(), permissions.SurfaceCLI)
	spoofed = rpcmeta.WithOutgoingPermissionPreset(spoofed, permissions.PresetFullAccess)
	_, err = client.Status(spoofed, &morphpb.GetBrowserStatusRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	request := &morphpb.GetBrowserStatusRequest{}
	authenticated, err := rpcauth.WithOutgoingProof(
		spoofed, morphpb.BrowserService_Status_FullMethodName, credential, request,
	)
	require.NoError(t, err)
	response, err := client.Status(authenticated, request)
	require.NoError(t, err)
	require.True(t, response.GetStatus().GetEnabled())

	artifactRequest := &morphpb.ReadBrowserArtifactRequest{Handle: "artifact_1"}
	unauthenticatedStream, err := client.ReadArtifact(spoofed, artifactRequest)
	require.NoError(t, err)
	_, err = unauthenticatedStream.Recv()
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	authenticated, err = rpcauth.WithOutgoingProof(
		spoofed, morphpb.BrowserService_ReadArtifact_FullMethodName, credential, artifactRequest,
	)
	require.NoError(t, err)
	authenticatedStream, err := client.ReadArtifact(authenticated, artifactRequest)
	require.NoError(t, err)
	artifactResponse, err := authenticatedStream.Recv()
	require.NoError(t, err)
	require.Equal(t, "artifact_1", artifactResponse.GetArtifact().GetHandle())
}

type browserServerRuntime struct {
	status   browser.Status
	artifact browser.ArtifactContent
}

func (s *browserServerRuntime) Status() browser.Status {
	return s.status
}

func (*browserServerRuntime) Start(context.Context, browser.StartRequest) (browser.Session, error) {
	return browser.Session{}, nil
}

func (*browserServerRuntime) Stop(context.Context, string) (browser.Session, error) {
	return browser.Session{}, nil
}

func (s *browserServerRuntime) ReadArtifact(context.Context, string) (browser.ArtifactContent, error) {
	return s.artifact, nil
}
