package native

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

type testDependencies struct {
	filePolicy    guardrails.FilesystemPolicy
	commandPolicy guardrails.CommandPolicy
	todos         []envtypes.TodoItem
}

func (d testDependencies) FilePolicy() guardrails.FilesystemPolicy { return d.filePolicy }
func (d testDependencies) CommandPolicy() guardrails.CommandPolicy { return d.commandPolicy }
func (d *testDependencies) ListTodos() []envtypes.TodoItem {
	return append([]envtypes.TodoItem(nil), d.todos...)
}
func (d *testDependencies) ReplaceTodos(items []envtypes.TodoItem) []envtypes.TodoItem {
	d.todos = append([]envtypes.TodoItem(nil), items...)
	return append([]envtypes.TodoItem(nil), d.todos...)
}
func (d *testDependencies) ClearTodos() []envtypes.TodoItem {
	d.todos = nil
	return nil
}

func registerTestRuntime(t *testing.T, root string, policy guardrails.CommandPolicy) tools.Registry {
	t.Helper()
	registry := tools.NewInMemoryRegistry()
	dependencies := &testDependencies{
		filePolicy:    guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots([]string{root})},
		commandPolicy: policy.Normalize(),
	}
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	for _, definition := range []tools.Definition{
		TimeDefinition(),
		ListFilesDefinition(dependencies),
		ReadFileDefinition(dependencies),
		SearchFilesDefinition(dependencies),
		WriteFileDefinition(dependencies),
		PatchDefinition(dependencies),
		TodoDefinition(dependencies),
		RunCommandDefinition(dependencies),
	} {
		require.NoError(t, registry.Register(definition))
	}

	return registry
}

func quoteJSON(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
