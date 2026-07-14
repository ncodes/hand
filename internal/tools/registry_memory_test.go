package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/trace"
)

func TestInMemoryRegistry_RegisterAndGet(t *testing.T) {
	registry := NewInMemoryRegistry()
	definition := Definition{
		Name: "echo",
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			return Result{Output: "ok"}, nil
		}),
	}

	require.NoError(t, registry.Register(definition))

	loaded, ok := registry.Get("echo")
	require.True(t, ok)
	require.Equal(t, "echo", loaded.Name)
	require.Nil(t, loaded.Groups)
}

func TestInMemoryRegistry_InvokeCallsHand(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.Register(Definition{
		Name: "echo",
		Handler: HandlerFunc(func(_ context.Context, call Call) (Result, error) {
			return Result{Output: call.Input}, nil
		}),
	}))

	result, err := registry.Invoke(context.Background(), Call{Name: "echo", Input: "hello"})

	require.NoError(t, err)
	require.Equal(t, "hello", result.Output)
}

func TestInMemoryRegistry_ListReturnsSortedDefinitions(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) {
		return Result{}, nil
	})

	require.NoError(t, registry.Register(Definition{Name: "zeta", Handler: handler}))
	require.NoError(t, registry.Register(Definition{Name: "alpha", Handler: handler}))

	definitions := registry.List()

	require.Len(t, definitions, 2)
	require.Equal(t, []string{"alpha", "zeta"}, definitions.Names())
}

func TestInMemoryRegistry_RejectsInvalidDefinitions(t *testing.T) {
	registry := NewInMemoryRegistry()

	require.EqualError(t, registry.Register(Definition{}), "tool name is required")
	require.EqualError(t, registry.Register(Definition{Name: "echo"}), "tool handler is required")
	require.EqualError(t, registry.RegisterGroup(Group{}), "tool group name is required")
}

func TestInMemoryRegistry_RejectsDuplicateTools(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) {
		return Result{}, nil
	})

	require.NoError(t, registry.Register(Definition{Name: "echo", Handler: handler}))
	require.EqualError(t, registry.Register(Definition{Name: "echo", Handler: handler}), "tool is already registered")
}

func TestInMemoryRegistry_RejectsDuplicateToolGroups(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(Group{Name: "core"}))
	require.EqualError(t, registry.RegisterGroup(Group{Name: "core"}), "tool group is already registered")
}

func TestInMemoryRegistry_InvokeReturnsNormalizedUnknownToolError(t *testing.T) {
	registry := NewInMemoryRegistry()

	result, err := registry.Invoke(context.Background(), Call{Name: "missing"})

	require.NoError(t, err)
	require.Equal(t, Error{Code: "tool_not_registered", Message: "tool is not registered"}.String(), result.Error)
}

func TestInMemoryRegistry_MutationsRejectNilRegistry(t *testing.T) {
	var registry *InMemoryRegistry
	handler := HandlerFunc(func(context.Context, Call) (Result, error) {
		return Result{}, nil
	})

	require.EqualError(t, registry.Register(Definition{Name: "echo", Handler: handler}), "tool registry is required")
	require.EqualError(t, registry.RegisterGroup(Group{Name: "core"}), "tool registry is required")
}

func TestInMemoryRegistry_GetHandlesNilRegistry(t *testing.T) {
	var registry *InMemoryRegistry

	definition, ok := registry.Get("echo")
	require.False(t, ok)
	require.Equal(t, Definition{}, definition)

	group, ok := registry.GetGroup("core")
	require.False(t, ok)
	require.Equal(t, Group{}, group)
}

func TestInMemoryRegistry_ListHandlesNilRegistry(t *testing.T) {
	var registry *InMemoryRegistry

	require.Nil(t, registry.List())
	require.Nil(t, registry.ListGroups())
}

func TestInMemoryRegistry_GetTrimsName(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.Register(Definition{
		Name: "echo",
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			return Result{}, nil
		}),
	}))

	definition, ok := registry.Get("  echo  ")

	require.True(t, ok)
	require.Equal(t, "echo", definition.Name)
}

func TestInMemoryRegistry_RegisterAndGetGroup(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(Group{
		Name:     " core ",
		Tools:    []string{" echo ", "", "echo"},
		Includes: []string{" shared ", "shared", " "},
	}))

	group, ok := registry.GetGroup("core")

	require.True(t, ok)
	require.Equal(t, "core", group.Name)
	require.Equal(t, []string{"echo"}, group.Tools)
	require.Equal(t, []string{"shared"}, group.Includes)
}

func TestInMemoryRegistry_ListGroupsReturnsSortedGroups(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(Group{Name: "zeta"}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "alpha"}))

	groups := registry.ListGroups()

	require.Equal(t, []Group{{Name: "alpha"}, {Name: "zeta"}}, groups)
}

func TestInMemoryRegistry_ResolveReturnsAllToolsWhenNoGroupsRequested(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "beta", Handler: handler}))
	require.NoError(t, registry.Register(Definition{Name: "alpha", Handler: handler}))

	definitions, err := registry.Resolve(Policy{})

	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, definitions.Names())
}

func TestInMemoryRegistry_ResolveByGroupMembership(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "echo", Groups: []string{"core"}, Handler: handler}))
	require.NoError(t, registry.Register(Definition{Name: "search", Handler: handler}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "core"}))

	definitions, err := registry.Resolve(Policy{GroupNames: []string{"core"}})

	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "echo", definitions[0].Name)
}

func TestInMemoryRegistry_ResolveThroughIncludedGroups(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "echo", Groups: []string{"core"}, Handler: handler}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "core"}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "coding", Includes: []string{"core"}}))

	definitions, err := registry.Resolve(Policy{GroupNames: []string{"coding"}})

	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "echo", definitions[0].Name)
}

func TestInMemoryRegistry_ResolveDeduplicatesTools(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "echo", Groups: []string{"core"}, Handler: handler}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "core"}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "coding", Tools: []string{"echo"}, Includes: []string{"core"}}))

	definitions, err := registry.Resolve(Policy{GroupNames: []string{"coding"}})

	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "echo", definitions[0].Name)
}

func TestInMemoryRegistry_ResolveSharedIncludedGroupOnceAndSortsTools(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "zeta", Handler: handler}))
	require.NoError(t, registry.Register(Definition{Name: "alpha", Handler: handler}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "shared", Tools: []string{"zeta", "alpha"}}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "first", Includes: []string{"shared"}}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "second", Includes: []string{"shared"}}))

	definitions, err := registry.Resolve(Policy{GroupNames: []string{"second", "first"}})

	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "zeta"}, definitions.Names())
}

func TestInMemoryRegistry_ResolveRejectsNilRegistry(t *testing.T) {
	var registry *InMemoryRegistry

	definitions, err := registry.Resolve(Policy{})

	require.EqualError(t, err, "tool registry is required")
	require.Nil(t, definitions)
}

func TestInMemoryRegistry_ResolveRejectsMissingReferencedTool(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(Group{Name: "core", Tools: []string{"missing"}}))

	_, err := registry.Resolve(Policy{GroupNames: []string{"core"}})

	require.EqualError(t, err, "tool ('missing') referenced by group is not registered")
}

func TestInMemoryRegistry_ResolveRejectsMissingIncludedGroup(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(Group{Name: "core", Includes: []string{"missing"}}))

	_, err := registry.Resolve(Policy{GroupNames: []string{"core"}})

	require.EqualError(t, err, "tool group ('missing') is not registered")
}

func TestInMemoryRegistry_ResolveRejectsGroupCycles(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(Group{Name: "core", Includes: []string{"coding"}}))
	require.NoError(t, registry.RegisterGroup(Group{Name: "coding", Includes: []string{"core"}}))

	_, err := registry.Resolve(Policy{GroupNames: []string{"core"}})

	require.EqualError(t, err, "tool group ('core') cycle detected")
}

func TestInMemoryRegistry_ResolveFiltersByCapabilities(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "read_file", Requires: Capabilities{Filesystem: true}, Handler: handler}))

	definitions, err := registry.Resolve(Policy{})
	require.NoError(t, err)
	require.Empty(t, definitions)

	definitions, err = registry.Resolve(Policy{Capabilities: Capabilities{Filesystem: true}})
	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "read_file", definitions[0].Name)
}

func TestInMemoryRegistry_ResolveFiltersByPlatform(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "browser", Platforms: []string{"desktop"}, Handler: handler}))
	require.NoError(t, registry.Register(Definition{Name: "time", Handler: handler}))

	definitions, err := registry.Resolve(Policy{Platform: "desktop"})
	require.NoError(t, err)
	require.Len(t, definitions, 2)

	definitions, err = registry.Resolve(Policy{Platform: "slack"})
	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "time", definitions[0].Name)
}

func TestInMemoryRegistry_InvokeNormalizesHandlerErrors(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.Register(Definition{
		Name: "echo",
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			return Result{}, context.DeadlineExceeded
		}),
	}))

	result, err := registry.Invoke(context.Background(), Call{Name: "echo"})

	require.NoError(t, err)
	require.Equal(t, Error{Code: "tool_invocation_failed", Message: context.DeadlineExceeded.Error()}.String(), result.Error)
}

func TestInMemoryRegistry_InvokeNormalizesResultErrors(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.Register(Definition{
		Name: "echo",
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			return Result{Error: "failed"}, nil
		}),
	}))

	result, err := registry.Invoke(context.Background(), Call{Name: "echo"})

	require.NoError(t, err)
	require.Equal(t, Error{Code: "tool_failed", Message: "failed"}.String(), result.Error)
}

func TestInMemoryRegistry_InvokePreservesStructuredResultErrors(t *testing.T) {
	registry := NewInMemoryRegistry()
	expected := Error{Code: "rate_limited", Message: "retry later", Retryable: true}
	raw, err := json.Marshal(expected)
	require.NoError(t, err)

	require.NoError(t, registry.Register(Definition{
		Name: "echo",
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			return Result{Error: string(raw)}, nil
		}),
	}))

	result, err := registry.Invoke(context.Background(), Call{Name: "echo"})

	require.NoError(t, err)
	require.Equal(t, string(raw), result.Error)
}

func TestInMemoryRegistry_RegisterNormalizesPermissionMetadata(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.Register(Definition{
		Name: "write",
		Permission: permissions.Operation{
			Resource: " FILE ",
			Action:   " UPDATE ",
			Effects:  []permissions.Effect{" WRITE ", permissions.EffectWrite},
		},
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil }),
	}))

	definition, ok := registry.Get("write")
	require.True(t, ok)
	require.Equal(t, permissions.Operation{
		Tool:     "write",
		Resource: permissions.ResourceFile,
		Action:   permissions.ActionUpdate,
		Effects:  []permissions.Effect{permissions.EffectWrite},
	}, definition.Permission)
}

func TestInMemoryRegistry_RegisterRejectsInvalidPermissionMetadata(t *testing.T) {
	registry := NewInMemoryRegistry()
	err := registry.Register(Definition{
		Name:       "write",
		Permission: permissions.Operation{Resource: "database", Action: permissions.ActionUpdate},
		Handler:    HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil }),
	})

	require.EqualError(t, err, "permission resource is invalid")
}

func TestInMemoryRegistry_InvokeObservesPermissionWithoutEnforcingDecision(t *testing.T) {
	called := false
	registry := NewInMemoryRegistry(RegistryOptions{PermissionPolicy: permissions.Policy{
		Default: permissions.DecisionDeny,
		Rules: []permissions.Rule{{
			Name:       "ask owner writes",
			ActorKinds: []permissions.ActorKind{permissions.ActorLocalOwner},
			Effects:    []permissions.Effect{permissions.EffectWrite},
			Decision:   permissions.DecisionAsk,
		}},
	}})
	require.NoError(t, registry.Register(Definition{
		Name: "write",
		Permission: permissions.Operation{
			Resource: permissions.ResourceFile,
			Action:   permissions.ActionUpdate,
			Effects:  []permissions.Effect{permissions.EffectWrite},
		},
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			called = true
			return Result{Output: "written"}, nil
		}),
	}))
	recorder := &permissionTraceRecorder{}
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor:   permissions.Actor{Kind: permissions.ActorLocalOwner},
		Surface: permissions.SurfaceCLI,
	})
	ctx = WithTraceRecorder(ctx, recorder)

	result, err := registry.Invoke(ctx, Call{
		Name:  "write",
		Input: `{"actor":"local_owner","surface":"cli"}`,
	})

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "written", result.Output)
	require.Equal(t, []permissionTraceEvent{{
		eventType: trace.EvtPermissionDecisionObserved,
		payload: trace.PermissionDecisionPayload{
			ActorKind:     string(permissions.ActorLocalOwner),
			SurfaceKind:   string(permissions.SurfaceKindLocal),
			Surface:       string(permissions.SurfaceCLI),
			Tool:          "write",
			Resource:      string(permissions.ResourceFile),
			Action:        string(permissions.ActionUpdate),
			Effects:       []string{string(permissions.EffectWrite)},
			Decision:      string(permissions.DecisionAsk),
			ReasonCode:    permissions.ReasonRuleMatched,
			Rule:          "ask owner writes",
			Mode:          string(permissions.ModeObserve),
			OwnerRequired: false,
		},
	}}, recorder.events)
}

func TestInMemoryRegistry_InvokeObservesUnknownActorAndSkipsUnclassifiedTools(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{Output: "ok"}, nil })
	require.NoError(t, registry.Register(Definition{
		Name:       "write",
		Permission: permissions.Operation{Resource: permissions.ResourceFile, Action: permissions.ActionUpdate},
		Handler:    handler,
	}))
	require.NoError(t, registry.Register(Definition{Name: "echo", Handler: handler}))
	recorder := &permissionTraceRecorder{}
	ctx := WithTraceRecorder(context.Background(), recorder)

	_, err := registry.Invoke(ctx, Call{Name: "write"})
	require.NoError(t, err)
	_, err = registry.Invoke(ctx, Call{Name: "echo"})
	require.NoError(t, err)

	require.Len(t, recorder.events, 1)
	payload := recorder.events[0].payload
	require.Equal(t, string(permissions.ActorUnknown), payload.ActorKind)
	require.Equal(t, string(permissions.SurfaceKindUnknown), payload.SurfaceKind)
	require.Equal(t, string(permissions.SurfaceUnknown), payload.Surface)
	require.Equal(t, string(permissions.DecisionDeny), payload.Decision)
	require.Equal(t, permissions.ReasonPolicyDefault, payload.ReasonCode)
}

func TestInMemoryRegistry_InvokeClassifiedToolWithoutRecorder(t *testing.T) {
	registry := NewInMemoryRegistry()
	require.NoError(t, registry.Register(Definition{
		Name:       "read",
		Permission: permissions.Operation{Resource: permissions.ResourceFile, Action: permissions.ActionRead},
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			return Result{Output: "contents"}, nil
		}),
	}))

	result, err := registry.Invoke(context.Background(), Call{Name: "read"})

	require.NoError(t, err)
	require.Equal(t, "contents", result.Output)
}

type permissionTraceEvent struct {
	eventType string
	payload   trace.PermissionDecisionPayload
}

type permissionTraceRecorder struct {
	events []permissionTraceEvent
}

func (r *permissionTraceRecorder) Record(eventType string, payload any) {
	r.events = append(r.events, permissionTraceEvent{
		eventType: eventType,
		payload:   payload.(trace.PermissionDecisionPayload),
	})
}
