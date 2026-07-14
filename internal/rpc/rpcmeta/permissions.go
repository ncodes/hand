package rpcmeta

import (
	"context"

	"github.com/wandxy/morph/internal/permissions"
	"google.golang.org/grpc/metadata"
)

const permissionSurfaceKey = "x-morph-permission-surface"

func WithOutgoingPermissionSurface(ctx context.Context, surface permissions.Surface) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !isSupportedClientSurface(surface) {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, permissionSurfaceKey, string(surface))
}

func PermissionSurfaceFromIncomingContext(ctx context.Context) permissions.Surface {
	if ctx == nil {
		return permissions.SurfaceRPC
	}

	values := metadata.ValueFromIncomingContext(ctx, permissionSurfaceKey)
	if len(values) == 0 {
		return permissions.SurfaceRPC
	}
	surface := permissions.Surface(values[len(values)-1])
	if !isSupportedClientSurface(surface) {
		return permissions.SurfaceRPC
	}

	return surface
}

func isSupportedClientSurface(surface permissions.Surface) bool {
	return surface == permissions.SurfaceCLI || surface == permissions.SurfaceTUI
}
