package native

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/tools"
)

func TestRegister_RegistersTimeTool(t *testing.T) {
	originalNow := now
	t.Cleanup(func() {
		now = originalNow
	})
	now = func() time.Time {
		return time.Date(2026, time.March, 23, 0, 0, 0, 0, time.UTC)
	}

	registry := tools.NewInMemoryRegistry()
	require.NoError(t, Register(registry))

	definition, ok := registry.Get("time")
	require.True(t, ok)
	require.Equal(t, "Returns the current server time in RFC3339 format.", definition.Description)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "time"})
	require.NoError(t, err)
	require.Equal(t, "2026-03-23T00:00:00Z", result.Output)
}

func TestRegister_HandlesNilRegistry(t *testing.T) {
	require.NoError(t, Register(nil))
}

type failingRegistry struct {
	err error
}

func (r failingRegistry) Register(tools.Definition) error {
	return r.err
}

func (failingRegistry) Get(string) (tools.Definition, bool) {
	return tools.Definition{}, false
}

func (failingRegistry) List() []tools.Definition {
	return nil
}

func (failingRegistry) Invoke(context.Context, tools.Call) (tools.Result, error) {
	return tools.Result{}, nil
}

func TestRegister_PropagatesRegisterError(t *testing.T) {
	expectedErr := errors.New("register failed")

	err := Register(failingRegistry{err: expectedErr})

	require.ErrorIs(t, err, expectedErr)
}
