package permissions

import (
	"errors"
	"net/url"
	"path"
	"strings"

	"github.com/wandxy/morph/pkg/str"
)

type MCPRequest struct {
	ServerID  string
	Transport string
	Tool      string
	Target    string
}

func (r MCPRequest) Operation() (Operation, error) {
	r.ServerID = str.String(r.ServerID).Trim()
	r.Transport = str.String(r.Transport).Normalized()
	r.Tool = str.String(r.Tool).Trim()
	r.Target = normalizeExtensionTarget(r.Target)
	if r.ServerID == "" || r.Transport == "" || r.Tool == "" {
		return Operation{}, errors.New("MCP server, transport, and tool are required")
	}
	if r.Transport != "stdio" && r.Transport != "http" && r.Transport != "sse" && r.Transport != "streamable_http" {
		return Operation{}, errors.New("MCP transport must be one of: stdio, http, sse, streamable_http")
	}
	target := getStructuredTarget(url.Values{
		"server": {r.ServerID}, "transport": {r.Transport}, "tool": {r.Tool}, "target": {r.Target},
	})

	return Operation{
		Tool:     "mcp:" + r.ServerID + ":" + r.Tool,
		Resource: ResourceMCP,
		Action:   ActionExecute,
		Effects:  []Effect{EffectExecution, EffectExternalSystem, EffectNetwork},
		Target:   target,
	}.Normalize()
}

type BrowserRequest struct {
	Profile     string
	CDPEndpoint string
	TabTarget   string
	Action      string
	Personal    bool
}

type ExtensionRequest struct {
	Tool     string
	Resource Resource
	Action   Action
	Effects  []Effect
	Target   string
}

func (r ExtensionRequest) Operation(authorization AuthorizationContext) (Operation, error) {
	authorization, err := authorization.Normalize()
	if err != nil {
		return Operation{}, err
	}
	if authorization.Actor.ID == "" || !authorization.Scope.Restricted {
		return Operation{}, errors.New("extension operation requires an authenticated actor and constrained scope")
	}
	if r.Resource != ResourceExecuteCode && r.Resource != ResourceACP {
		return Operation{}, errors.New("extension resource must be execute_code or acp")
	}
	if r.Resource == ResourceExecuteCode && authorization.Actor.Kind != ActorRPCClient {
		return Operation{}, errors.New("execute-code operation requires an RPC client actor")
	}
	if r.Resource == ResourceACP && authorization.Actor.Kind != ActorACPClient {
		return Operation{}, errors.New("ACP operation requires an ACP client actor")
	}
	operation, err := (Operation{
		Tool: str.String(r.Tool).Trim(), Resource: r.Resource, Action: r.Action,
		Effects: append([]Effect(nil), r.Effects...), Target: normalizeExtensionTarget(r.Target),
	}).Normalize()
	if err != nil {
		return Operation{}, err
	}
	if !authorization.Scope.Allows(operation) {
		return Operation{}, errors.New("extension operation exceeds its constrained scope")
	}

	return operation, nil
}

func (r BrowserRequest) Operation() (Operation, error) {
	r.Profile = str.String(r.Profile).Trim()
	r.CDPEndpoint = normalizeExtensionTarget(r.CDPEndpoint)
	r.TabTarget = normalizeExtensionTarget(r.TabTarget)
	r.Action = str.String(r.Action).Normalized()
	if r.Profile == "" || r.CDPEndpoint == "" || r.Action == "" {
		return Operation{}, errors.New("browser profile, CDP endpoint, and action are required")
	}
	target := getStructuredTarget(url.Values{
		"profile": {r.Profile}, "cdp": {r.CDPEndpoint}, "tab": {r.TabTarget},
		"action": {r.Action}, "mode": {getBrowserMode(r.Personal)},
	})
	effects := []Effect{EffectExternalSystem, EffectNetwork}
	if r.Personal {
		effects = append(effects, EffectCredentialBearing)
	}

	return Operation{
		Tool:     "browser:" + r.Action,
		Resource: ResourceBrowser,
		Action:   ActionExecute,
		Effects:  effects,
		Target:   target,
	}.Normalize()
}

func NewConstrainedExtensionAuthorization(
	kind ActorKind,
	id string,
	surface Surface,
	profile string,
	sessionID string,
	runID string,
	scope PermissionScope,
) (AuthorizationContext, error) {
	if kind != ActorACPClient && kind != ActorRPCClient {
		return AuthorizationContext{}, errors.New("extension actor must be an ACP or RPC client")
	}
	if kind == ActorACPClient && surface != SurfaceACP || kind == ActorRPCClient && surface != SurfaceRPC {
		return AuthorizationContext{}, errors.New("extension actor does not match its surface")
	}
	id = str.String(id).Trim()
	if id == "" {
		return AuthorizationContext{}, errors.New("extension actor id is required")
	}
	scope, err := scope.Normalize()
	if err != nil {
		return AuthorizationContext{}, err
	}
	if !scope.Restricted {
		return AuthorizationContext{}, errors.New("extension permission scope must be restricted")
	}

	return AuthorizationContext{
		Actor:     Actor{Kind: kind, ID: id},
		Surface:   surface,
		Profile:   str.String(profile).Trim(),
		SessionID: str.String(sessionID).Trim(),
		RunID:     str.String(runID).Trim(),
		Scope:     scope,
	}.Normalize()
}

func normalizeExtensionTarget(raw string) string {
	raw = str.String(raw).Trim()
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return path.Clean(strings.ReplaceAll(raw, "\\", "/"))
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.RawQuery = parsed.Query().Encode()
	parsed.Path = path.Clean(parsed.Path)
	if parsed.Path == "." || parsed.Path == "/" {
		parsed.Path = ""
	}

	return parsed.String()
}

func getStructuredTarget(values url.Values) string {
	return values.Encode()
}

func getBrowserMode(personal bool) string {
	if personal {
		return "personal"
	}

	return "isolated"
}
