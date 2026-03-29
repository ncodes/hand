package native

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

func TestTodo_ToolStoresSessionState(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	_, err := registry.Invoke(context.Background(), tools.Call{Name: "todo", Input: `{"action":"replace","items":[{"text":"ship it","done":false}]}`})
	require.NoError(t, err)
	result, err := registry.Invoke(context.Background(), tools.Call{Name: "todo", Input: `{"action":"list"}`})

	require.NoError(t, err)
	var payload struct {
		Items []envtypes.TodoItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, []envtypes.TodoItem{{Text: "ship it", Done: false}}, payload.Items)
}

func TestTodo_ToolRemainsAvailableWithoutFilesystemCapability(t *testing.T) {
	registry := tools.NewInMemoryRegistry()
	dependencies := &testDependencies{
		filePolicy:    guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots([]string{t.TempDir()})},
		commandPolicy: guardrails.CommandPolicy{}.Normalize(),
	}
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	require.NoError(t, registry.Register(TimeDefinition()))
	require.NoError(t, registry.Register(TodoDefinition(dependencies)))
	require.NoError(t, registry.Register(ReadFileDefinition(dependencies)))

	definitions, err := registry.Resolve(tools.Policy{
		Capabilities: tools.Capabilities{Exec: true, Memory: true},
	})

	require.NoError(t, err)
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	require.Contains(t, names, "todo")
	require.NotContains(t, names, "read_file")
}
