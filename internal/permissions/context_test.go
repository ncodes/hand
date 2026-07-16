package permissions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContext_RoundTripsTrustedAuthorization(t *testing.T) {
	want := AuthorizationContext{
		Actor: Actor{Kind: ActorGatewayUser, ID: "123"}, SurfaceKind: SurfaceKindGateway, Surface: SurfaceTelegram,
	}

	ctx := WithContext(nil, want)
	got, ok := FromContext(ctx)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestContext_RejectsMissingOrInvalidAuthorization(t *testing.T) {
	_, ok := FromContext(nil)
	require.False(t, ok)
	_, ok = FromContext(context.Background())
	require.False(t, ok)

	ctx := WithContext(context.Background(), AuthorizationContext{Actor: Actor{Kind: "owner"}, Surface: SurfaceCLI})
	_, ok = FromContext(ctx)
	require.False(t, ok)

	ctx = context.WithValue(context.Background(), authorizationContextKey{}, AuthorizationContext{
		Actor:   Actor{Kind: ActorLocalOwner},
		Surface: "terminal",
	})
	_, ok = FromContext(ctx)
	require.False(t, ok)
}

func TestContext_TracksFullAccessExecution(t *testing.T) {
	require.False(t, HasFullAccess(nil))
	require.False(t, HasFullAccess(context.Background()))

	ctx := WithFullAccess(nil)

	require.True(t, HasFullAccess(ctx))
}
