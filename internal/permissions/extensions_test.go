package permissions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPRequest_OperationBindsServerTransportToolAndNormalizedTarget(t *testing.T) {
	left, err := (MCPRequest{
		ServerID: "workspace", Transport: " HTTP ", Tool: "read", Target: "HTTPS://EXAMPLE.COM/a/../b#secret",
	}).Operation()
	require.NoError(t, err)
	right, err := (MCPRequest{
		ServerID: "workspace", Transport: "http", Tool: "read", Target: "https://example.com/b",
	}).Operation()
	require.NoError(t, err)
	require.Equal(t, left.Target, right.Target)
	require.Equal(t, "mcp:workspace:read", left.Tool)
	require.Contains(t, left.Target, "server=workspace")

	changed, err := (MCPRequest{
		ServerID: "personal", Transport: "http", Tool: "read", Target: "https://example.com/b",
	}).Operation()
	require.NoError(t, err)
	require.NotEqual(t, left.Target, changed.Target)

	_, err = (MCPRequest{}).Operation()
	require.EqualError(t, err, "MCP server, transport, and tool are required")
	_, err = (MCPRequest{ServerID: "server", Transport: "socket", Tool: "read"}).Operation()
	require.EqualError(t, err, "MCP transport must be one of: stdio, http, sse, streamable_http")
}

func TestBrowserRequest_PersonalProfileAddsCredentialEffectAndFingerprintIdentity(t *testing.T) {
	isolated, err := (BrowserRequest{
		Profile: "isolated", CDPEndpoint: "HTTP://LOCALHOST:9222/", TabTarget: "tab/1", Action: " CLICK ",
	}).Operation()
	require.NoError(t, err)
	personal, err := (BrowserRequest{
		Profile: "personal", CDPEndpoint: "http://localhost:9222", TabTarget: "tab/1", Action: "click", Personal: true,
	}).Operation()
	require.NoError(t, err)
	require.NotContains(t, isolated.Effects, EffectCredentialBearing)
	require.Contains(t, personal.Effects, EffectCredentialBearing)
	require.NotEqual(t, isolated.Target, personal.Target)

	_, err = (BrowserRequest{}).Operation()
	require.EqualError(t, err, "browser profile, CDP endpoint, and action are required")
}

func TestExtensionAuthorization_RequiresAuthenticatedConstrainedActor(t *testing.T) {
	scope := PermissionScope{
		Restricted: true, Resources: []Resource{ResourceExecuteCode}, Actions: []Action{ActionExecute},
		Effects: []Effect{EffectExecution}, TargetPrefixes: []string{"workspace/"},
	}
	authorization, err := NewConstrainedExtensionAuthorization(
		ActorRPCClient, "executor", SurfaceRPC, "default", "session", "run", scope,
	)
	require.NoError(t, err)
	operation, err := (ExtensionRequest{
		Tool: "execute_code", Resource: ResourceExecuteCode, Action: ActionExecute,
		Effects: []Effect{EffectExecution}, Target: "workspace/script.go",
	}).Operation(authorization)
	require.NoError(t, err)
	require.Equal(t, ResourceExecuteCode, operation.Resource)

	_, err = NewConstrainedExtensionAuthorization(ActorSubagent, "child", SurfaceRPC, "", "", "", scope)
	require.EqualError(t, err, "extension actor must be an ACP or RPC client")
	_, err = NewConstrainedExtensionAuthorization(ActorRPCClient, "", SurfaceRPC, "", "", "", scope)
	require.EqualError(t, err, "extension actor id is required")
	_, err = NewConstrainedExtensionAuthorization(ActorRPCClient, "executor", SurfaceRPC, "", "", "", PermissionScope{})
	require.EqualError(t, err, "extension permission scope must be restricted")
	_, err = NewConstrainedExtensionAuthorization(ActorRPCClient, "executor", SurfaceACP, "", "", "", scope)
	require.EqualError(t, err, "extension actor does not match its surface")
	_, err = NewConstrainedExtensionAuthorization(
		ActorRPCClient, "executor", SurfaceRPC, "", "", "",
		PermissionScope{Restricted: true, Actions: []Action{"bad"}},
	)
	require.EqualError(t, err, "permission scope contains an invalid action")

	_, err = (ExtensionRequest{Resource: ResourceFile, Action: ActionRead}).Operation(authorization)
	require.EqualError(t, err, "extension resource must be execute_code or acp")
	_, err = (ExtensionRequest{
		Resource: ResourceExecuteCode, Action: ActionExecute, Effects: []Effect{EffectExecution}, Target: "private/script.go",
	}).Operation(authorization)
	require.EqualError(t, err, "extension operation exceeds its constrained scope")
	_, err = (ExtensionRequest{Resource: ResourceExecuteCode, Action: "bad"}).Operation(authorization)
	require.EqualError(t, err, "permission action is invalid")
	_, err = (ExtensionRequest{Resource: ResourceExecuteCode, Action: ActionExecute}).Operation(AuthorizationContext{})
	require.EqualError(t, err, "permission actor kind is invalid")
	_, err = (ExtensionRequest{Resource: ResourceExecuteCode, Action: ActionExecute}).Operation(AuthorizationContext{
		Actor: Actor{Kind: ActorRPCClient}, Surface: SurfaceRPC,
		Scope: PermissionScope{Restricted: true, Resources: []Resource{ResourceExecuteCode}, Actions: []Action{ActionExecute}},
	})
	require.EqualError(t, err, "extension operation requires an authenticated actor and constrained scope")
	acpAuthorization, err := NewConstrainedExtensionAuthorization(
		ActorACPClient, "editor", SurfaceACP, "", "", "",
		PermissionScope{
			Restricted: true, Resources: []Resource{ResourceACP}, Actions: []Action{ActionExecute},
			Effects: []Effect{EffectExecution},
		},
	)
	require.NoError(t, err)
	_, err = (ExtensionRequest{
		Resource: ResourceExecuteCode, Action: ActionExecute, Effects: []Effect{EffectExecution},
	}).Operation(acpAuthorization)
	require.EqualError(t, err, "execute-code operation requires an RPC client actor")
	_, err = (ExtensionRequest{
		Resource: ResourceACP, Action: ActionExecute, Effects: []Effect{EffectExecution},
	}).Operation(authorization)
	require.EqualError(t, err, "ACP operation requires an ACP client actor")
	operation, err = (ExtensionRequest{
		Tool: "editor", Resource: ResourceACP, Action: ActionExecute, Effects: []Effect{EffectExecution},
	}).Operation(acpAuthorization)
	require.NoError(t, err)
	require.Equal(t, ResourceACP, operation.Resource)
}

func TestFingerprint_BindsDelegationLineageAndScope(t *testing.T) {
	operation := Operation{Resource: ResourceFile, Action: ActionRead, Effects: []Effect{EffectRead}, Target: "workspace/a"}
	base := AuthorizationContext{Actor: Actor{Kind: ActorSubagent, ID: "child"}, Surface: SurfaceTUI}
	parentA := base
	parentA.ParentActorKind = ActorLocalOwner
	parentA.ParentActorID = "owner-a"
	parentB := parentA
	parentB.ParentActorID = "owner-b"
	require.NotEqual(t, Fingerprint(parentA, operation), Fingerprint(parentB, operation))

	scoped := parentA
	scoped.Scope = PermissionScope{Restricted: true, Resources: []Resource{ResourceFile}, Actions: []Action{ActionRead}}
	require.NotEqual(t, Fingerprint(parentA, operation), Fingerprint(scoped, operation))
}
