package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := NewRegistry()
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
}

func TestRegistry_InvokeCallsHandler(t *testing.T) {
	registry := NewRegistry()
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

func TestRegistry_ListReturnsSortedDefinitions(t *testing.T) {
	registry := NewRegistry()
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

func TestRegistry_RejectsInvalidDefinitions(t *testing.T) {
	registry := NewRegistry()

	require.EqualError(t, registry.Register(Definition{}), "tool name is required")
	require.EqualError(t, registry.Register(Definition{Name: "echo"}), "tool handler is required")
}

func TestRegistry_RejectsDuplicateTools(t *testing.T) {
	registry := NewRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) {
		return Result{}, nil
	})

	require.NoError(t, registry.Register(Definition{Name: "echo", Handler: handler}))
	require.EqualError(t, registry.Register(Definition{Name: "echo", Handler: handler}), "tool is already registered")
}

func TestRegistry_InvokeRejectsUnknownTool(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Invoke(context.Background(), Call{Name: "missing"})

	require.EqualError(t, err, "tool is not registered")
}
