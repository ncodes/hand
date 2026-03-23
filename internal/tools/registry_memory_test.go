package tools

import (
	"context"
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
}

func TestInMemoryRegistry_RejectsDuplicateTools(t *testing.T) {
	registry := NewInMemoryRegistry()
	handler := HandlerFunc(func(context.Context, Call) (Result, error) {
		return Result{}, nil
	})

	require.NoError(t, registry.Register(Definition{Name: "echo", Handler: handler}))
	require.EqualError(t, registry.Register(Definition{Name: "echo", Handler: handler}), "tool is already registered")
}

func TestInMemoryRegistry_InvokeRejectsUnknownTool(t *testing.T) {
	registry := NewInMemoryRegistry()

	_, err := registry.Invoke(context.Background(), Call{Name: "missing"})

	require.EqualError(t, err, "tool is not registered")
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
