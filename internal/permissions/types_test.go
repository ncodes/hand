package permissions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorizationContext_Normalize(t *testing.T) {
	authorization, err := (AuthorizationContext{
		Actor:           Actor{Kind: " LOCAL_OWNER ", ID: " owner "},
		Surface:         " CLI ",
		Profile:         " default ",
		SessionID:       " session ",
		RunID:           " run ",
		ParentActorKind: " SUBAGENT ",
		ParentRunID:     " parent ",
	}).Normalize()
	require.NoError(t, err)
	require.Equal(t, AuthorizationContext{
		Actor:           Actor{Kind: ActorLocalOwner, ID: "owner"},
		SurfaceKind:     SurfaceKindLocal,
		Surface:         SurfaceCLI,
		Profile:         "default",
		SessionID:       "session",
		RunID:           "run",
		ParentActorKind: ActorSubagent,
		ParentRunID:     "parent",
	}, authorization)
}

func TestAuthorizationContext_NormalizeRejectsInvalidIdentity(t *testing.T) {
	tests := []struct {
		name          string
		authorization AuthorizationContext
		errorMessage  string
	}{
		{
			name:          "actor",
			authorization: AuthorizationContext{Actor: Actor{Kind: "root"}, Surface: SurfaceCLI},
			errorMessage:  "permission actor kind is invalid",
		},
		{
			name: "surface kind",
			authorization: AuthorizationContext{
				Actor: Actor{Kind: ActorLocalOwner}, SurfaceKind: "remote", Surface: SurfaceCLI,
			},
			errorMessage: "permission surface kind is invalid",
		},
		{
			name:          "surface",
			authorization: AuthorizationContext{Actor: Actor{Kind: ActorLocalOwner}, SurfaceKind: SurfaceKindLocal},
			errorMessage:  "permission surface is invalid",
		},
		{
			name: "surface kind mismatch",
			authorization: AuthorizationContext{
				Actor: Actor{Kind: ActorLocalOwner}, SurfaceKind: SurfaceKindGateway, Surface: SurfaceCLI,
			},
			errorMessage: "permission surface does not match its kind",
		},
		{
			name: "parent actor",
			authorization: AuthorizationContext{
				Actor:           Actor{Kind: ActorSubagent},
				Surface:         SurfaceCLI,
				ParentActorKind: "root",
			},
			errorMessage: "permission parent actor kind is invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.authorization.Normalize()
			require.EqualError(t, err, test.errorMessage)
		})
	}
}

func TestOperation_Normalize(t *testing.T) {
	operation, err := (Operation{
		Tool:          " write_file ",
		Resource:      " FILE ",
		Action:        " UPDATE ",
		Effects:       []Effect{" WRITE ", EffectRead, EffectWrite, ""},
		Target:        " workspace/file.txt ",
		OwnerID:       " owner ",
		OwnerRequired: true,
	}).Normalize()
	require.NoError(t, err)
	require.Equal(t, Operation{
		Tool:          "write_file",
		Resource:      ResourceFile,
		Action:        ActionUpdate,
		Effects:       []Effect{EffectRead, EffectWrite},
		Target:        "workspace/file.txt",
		OwnerID:       "owner",
		OwnerRequired: true,
	}, operation)
	require.False(t, operation.IsZero())
	require.True(t, (Operation{}).IsZero())
}

func TestOperation_NormalizeRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name         string
		operation    Operation
		errorMessage string
	}{
		{name: "resource", operation: Operation{Resource: "database", Action: ActionRead}, errorMessage: "permission resource is invalid"},
		{name: "action", operation: Operation{Resource: ResourceFile, Action: "download"}, errorMessage: "permission action is invalid"},
		{name: "effect", operation: Operation{Resource: ResourceFile, Action: ActionRead, Effects: []Effect{"unknown"}}, errorMessage: "permission effect is invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.operation.Normalize()
			require.EqualError(t, err, test.errorMessage)
		})
	}
}

func TestValidationHelpers_AcceptKnownAndUnknownValues(t *testing.T) {
	require.True(t, isValidActorKind(ActorLocalOwner, false))
	require.True(t, isValidActorKind(ActorUnknown, true))
	require.False(t, isValidActorKind(ActorUnknown, false))
	require.True(t, isValidSurface(SurfaceCLI, false))
	require.True(t, isValidSurface("discord", false))
	require.True(t, isValidSurface(SurfaceUnknown, true))
	require.False(t, isValidSurface(SurfaceUnknown, false))
	require.True(t, isValidSurfaceKind(SurfaceKindGateway, false))
	require.True(t, isValidSurfaceKind(SurfaceKindUnknown, true))
	require.False(t, isValidSurfaceKind(SurfaceKindUnknown, false))
	require.Equal(t, SurfaceKindGateway, getSurfaceKind(SurfaceTelegram))
	require.Equal(t, SurfaceKindLocal, getSurfaceKind(SurfaceTUI))
	require.Equal(t, SurfaceKindAutomation, getSurfaceKind(SurfaceAutomation))
	require.Equal(t, SurfaceKindRPC, getSurfaceKind(SurfaceRPC))
	require.Equal(t, SurfaceKindACP, getSurfaceKind(SurfaceACP))
	require.Equal(t, SurfaceKindUnknown, getSurfaceKind("discord"))
	require.True(t, isValidResource(ResourceFile, false))
	require.True(t, isValidResource(ResourceUnknown, true))
	require.False(t, isValidResource(ResourceUnknown, false))
	require.True(t, isValidAction(ActionRead, false))
	require.True(t, isValidAction(ActionUnknown, true))
	require.False(t, isValidAction(ActionUnknown, false))
	require.True(t, isValidEffect(EffectRead))
	require.False(t, isValidEffect("unknown"))
	require.True(t, isValidDecision(DecisionAllow))
	require.False(t, isValidDecision("unknown"))
}
