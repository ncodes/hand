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
	isolated, err := (BrowserRequest{Profile: "isolated", TabTarget: "tab/1", Action: " CLICK "}).Operations()
	require.NoError(t, err)
	personal, err := (BrowserRequest{Profile: "personal", TabTarget: "tab/1", Action: "click", Personal: true}).Operations()
	require.NoError(t, err)
	require.NotContains(t, isolated[0].Effects, EffectCredentialBearing)
	require.Contains(t, personal[0].Effects, EffectCredentialBearing)
	require.NotEqual(t, isolated[0].Target, personal[0].Target)

	_, err = (BrowserRequest{}).Operations()
	require.EqualError(t, err, "browser profile and action are required")
}

func TestBrowserRequest_MapsActionsToConcreteOperations(t *testing.T) {
	navigation, err := NetworkTargetFromURL("https://example.com/news", "GET", NetworkRequestNavigation)
	require.NoError(t, err)
	download, err := NetworkTargetFromURL("https://example.com/file", "GET", NetworkRequestDownload)
	require.NoError(t, err)
	cdp, err := NetworkTargetFromURL("https://example.com", "CONNECT", NetworkRequestCDP)
	require.NoError(t, err)
	tests := []struct {
		action             string
		fileTarget         string
		network            *NetworkTarget
		wantAction         Action
		wantResource       Resource
		wantCount          int
		wantSecondAction   Action
		wantSecondResource Resource
	}{
		{action: "status", wantAction: ActionRead, wantResource: ResourceBrowser, wantCount: 1},
		{action: "snapshot", wantAction: ActionRead, wantResource: ResourceBrowser, wantCount: 1},
		{action: "console", wantAction: ActionRead, wantResource: ResourceBrowser, wantCount: 1},
		{action: "wait", wantAction: ActionRead, wantResource: ResourceBrowser, wantCount: 1},
		{action: "focus", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "profiles", wantAction: ActionList, wantResource: ResourceBrowser, wantCount: 1},
		{action: "tabs", wantAction: ActionList, wantResource: ResourceBrowser, wantCount: 1},
		{action: "start", wantAction: ActionStart, wantResource: ResourceBrowser, wantCount: 1},
		{action: "stop", wantAction: ActionStop, wantResource: ResourceBrowser, wantCount: 1},
		{action: "open", network: &navigation, wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 2, wantSecondAction: ActionRead, wantSecondResource: ResourceNetwork},
		{action: "navigate", network: &navigation, wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 2, wantSecondAction: ActionRead, wantSecondResource: ResourceNetwork},
		{action: "back", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "forward", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "reload", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "click", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "type", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "press", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "scroll", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "select", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "accept_dialog", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "dismiss_dialog", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 1},
		{action: "close", wantAction: ActionDelete, wantResource: ResourceBrowser, wantCount: 1},
		{action: "screenshot", fileTarget: "artifacts/image.png", wantAction: ActionCreate, wantResource: ResourceBrowser, wantCount: 2, wantSecondAction: ActionCreate, wantSecondResource: ResourceFile},
		{action: "pdf", fileTarget: "artifacts/page.pdf", wantAction: ActionCreate, wantResource: ResourceBrowser, wantCount: 2, wantSecondAction: ActionCreate, wantSecondResource: ResourceFile},
		{action: "download", network: &download, wantAction: ActionCreate, wantResource: ResourceBrowser, wantCount: 2, wantSecondAction: ActionCreate, wantSecondResource: ResourceNetwork},
		{action: "upload", fileTarget: "workspace/input.txt", wantAction: ActionUpdate, wantResource: ResourceBrowser, wantCount: 2, wantSecondAction: ActionRead, wantSecondResource: ResourceFile},
		{action: "connect", network: &cdp, wantAction: ActionConnect, wantResource: ResourceBrowser, wantCount: 2, wantSecondAction: ActionConnect, wantSecondResource: ResourceNetwork},
	}
	for _, test := range tests {
		t.Run(test.action, func(t *testing.T) {
			operations, operationErr := (BrowserRequest{
				Profile: "default", Action: test.action, Network: test.network,
				FileTarget: test.fileTarget, TargetScope: TargetScopeWorkspace,
			}).Operations()
			require.NoError(t, operationErr)
			require.Len(t, operations, test.wantCount)
			require.Equal(t, test.wantAction, operations[0].Action)
			require.Equal(t, test.wantResource, operations[0].Resource)
			if test.wantCount == 2 {
				require.Equal(t, test.wantSecondAction, operations[1].Action)
				require.Equal(t, test.wantSecondResource, operations[1].Resource)
			}
		})
	}

	stop, err := (BrowserRequest{Profile: "default", Action: "stop"}).Operations()
	require.NoError(t, err)
	require.True(t, stop[0].OwnerRequired)
	require.ElementsMatch(t, []Effect{EffectWrite, EffectExecution, EffectDestructive}, stop[0].Effects)
	closeTab, err := (BrowserRequest{Profile: "default", Action: "close"}).Operations()
	require.NoError(t, err)
	require.True(t, closeTab[0].OwnerRequired)
	require.ElementsMatch(t, []Effect{EffectWrite, EffectDestructive}, closeTab[0].Effects)
	click, err := (BrowserRequest{Profile: "default", Action: "click"}).Operations()
	require.NoError(t, err)
	require.ElementsMatch(t, []Effect{EffectWrite, EffectNetwork, EffectExternalSystem}, click[0].Effects)

	_, err = (BrowserRequest{Profile: "default", Action: "navigate"}).Operations()
	require.EqualError(t, err, "browser action requires a structured network target")
	_, err = (BrowserRequest{Profile: "default", Action: "upload"}).Operations()
	require.EqualError(t, err, "browser upload requires a file target")
	_, err = (BrowserRequest{Profile: "default", Action: "unknown"}).Operations()
	require.EqualError(t, err, "browser action is invalid")
	_, err = (BrowserRequest{Profile: "default", Action: "connect", Network: &navigation}).Operations()
	require.EqualError(t, err, "browser network target request class does not match the action")
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
