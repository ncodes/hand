package rpcmeta

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

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
