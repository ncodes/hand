package rpcmeta

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"github.com/wandxy/morph/internal/permissions"
)

func TestPermissionSurface_RoundTripsSupportedClientSurface(t *testing.T) {
	for _, surface := range []permissions.Surface{permissions.SurfaceCLI, permissions.SurfaceTUI} {
		t.Run(string(surface), func(t *testing.T) {
			outgoing := WithOutgoingPermissionSurface(nil, surface)
			outgoingMetadata, ok := metadata.FromOutgoingContext(outgoing)
			require.True(t, ok)
			incoming := metadata.NewIncomingContext(context.Background(), outgoingMetadata)

			require.Equal(t, surface, PermissionSurfaceFromIncomingContext(incoming))
		})
	}
}

func TestPermissionSurface_DefaultsUnsupportedOrMissingValuesToRPC(t *testing.T) {
	require.Equal(t, permissions.SurfaceRPC, PermissionSurfaceFromIncomingContext(nil))
	require.Equal(t, permissions.SurfaceRPC, PermissionSurfaceFromIncomingContext(context.Background()))

	outgoing := WithOutgoingPermissionSurface(context.Background(), permissions.SurfaceSlack)
	_, ok := metadata.FromOutgoingContext(outgoing)
	require.False(t, ok)

	incoming := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		permissionSurfaceKey, string(permissions.SurfaceCLI),
		permissionSurfaceKey, string(permissions.SurfaceSlack),
	))
	require.Equal(t, permissions.SurfaceRPC, PermissionSurfaceFromIncomingContext(incoming))
}

func TestPermissionActor_ClassifiesOnlyLoopbackInteractiveClientsAsLocalOwner(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		expects permissions.ActorKind
	}{
		{
			name: "loopback TUI",
			ctx: incomingPermissionContext(
				permissions.SurfaceTUI,
				&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50051},
			),
			expects: permissions.ActorLocalOwner,
		},
		{
			name: "loopback CLI",
			ctx: incomingPermissionContext(
				permissions.SurfaceCLI,
				&net.TCPAddr{IP: net.ParseIP("::1"), Port: 50051},
			),
			expects: permissions.ActorLocalOwner,
		},
		{
			name: "remote TUI spoof",
			ctx: incomingPermissionContext(
				permissions.SurfaceTUI,
				&net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 50051},
			),
			expects: permissions.ActorRPCClient,
		},
		{
			name:    "missing peer",
			ctx:     incomingPermissionContext(permissions.SurfaceTUI, nil),
			expects: permissions.ActorRPCClient,
		},
		{
			name: "loopback generic RPC",
			ctx: peer.NewContext(
				context.Background(),
				&peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50051}},
			),
			expects: permissions.ActorRPCClient,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expects, PermissionActorFromIncomingContext(test.ctx).Kind)
		})
	}
}

func TestPermissionActor_UsesAuthenticatedPrincipalWithoutGrantingOwnerAuthority(t *testing.T) {
	ctx := WithAuthenticatedPermissionPrincipal(nil, " client-123 ")
	actor := PermissionActorFromIncomingContext(ctx)
	require.Equal(t, permissions.Actor{Kind: permissions.ActorRPCClient, ID: "client-123"}, actor)

	unchanged := WithAuthenticatedPermissionPrincipal(context.Background(), " ")
	require.Equal(t, permissions.ActorRPCClient, PermissionActorFromIncomingContext(unchanged).Kind)
	require.Equal(t, permissions.ActorRPCClient, PermissionActorFromIncomingContext(nil).Kind)
}

func incomingPermissionContext(surface permissions.Surface, address net.Addr) context.Context {
	outgoing := WithOutgoingPermissionSurface(context.Background(), surface)
	outgoingMetadata, _ := metadata.FromOutgoingContext(outgoing)
	ctx := metadata.NewIncomingContext(context.Background(), outgoingMetadata)
	if address != nil {
		ctx = peer.NewContext(ctx, &peer.Peer{Addr: address})
	}
	return ctx
}
