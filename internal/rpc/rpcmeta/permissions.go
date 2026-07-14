package rpcmeta

import (
	"context"
	"net"
	"strings"

	"github.com/wandxy/morph/internal/permissions"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
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

func PermissionActorFromIncomingContext(ctx context.Context) permissions.Actor {
	surface := PermissionSurfaceFromIncomingContext(ctx)
	if isSupportedClientSurface(surface) && isLoopbackPeer(ctx) {
		return permissions.Actor{Kind: permissions.ActorLocalOwner}
	}

	return permissions.Actor{Kind: permissions.ActorRPCClient}
}

func isSupportedClientSurface(surface permissions.Surface) bool {
	return surface == permissions.SurfaceCLI || surface == permissions.SurfaceTUI
}

func isLoopbackPeer(ctx context.Context) bool {
	remotePeer, ok := peer.FromContext(ctx)
	if !ok || remotePeer.Addr == nil {
		return false
	}

	host, _, err := net.SplitHostPort(remotePeer.Addr.String())
	if err != nil {
		host = remotePeer.Addr.String()
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))

	return ip != nil && ip.IsLoopback()
}
