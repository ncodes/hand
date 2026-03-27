package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestInMemoryRegistry_InvokeCallsHandler(t *testing.T) {
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
	require.Equal(t, "alpha", definitions[0].Name)
	require.Equal(t, "zeta", definitions[1].Name)
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

func TestInMemoryRegistry_RejectsNilRegistry(t *testing.T) {
	var registry *InMemoryRegistry

	err := registry.Register(Definition{
		Name: "echo",
		Handler: HandlerFunc(func(context.Context, Call) (Result, error) {
			return Result{}, nil
		}),
	})

	require.EqualError(t, err, "tool registry is required")
}

func TestInMemoryRegistry_GetHandlesNilRegistry(t *testing.T) {
	var registry *InMemoryRegistry

	definition, ok := registry.Get("echo")

	require.False(t, ok)
	require.Equal(t, Definition{}, definition)
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
	require.NoError(t, registry.RegisterGroup(Group{Name: " core ", Tools: []string{"echo"}, Includes: []string{"shared"}}))

	group, ok := registry.GetGroup("core")

	require.True(t, ok)
	require.Equal(t, "core", group.Name)
	require.Equal(t, []string{"echo"}, group.Tools)
	require.Equal(t, []string{"shared"}, group.Includes)
}

func TestInMemoryRegistry_ResolveReturnsAllToolsWhenNoGroupsRequested(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) { return Result{}, nil })
	require.NoError(t, registry.Register(Definition{Name: "beta", Handler: handler}))
	require.NoError(t, registry.Register(Definition{Name: "alpha", Handler: handler}))

	definitions, err := registry.Resolve(Policy{})

	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, []string{definitions[0].Name, definitions[1].Name})
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
