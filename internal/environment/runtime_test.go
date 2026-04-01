package environment

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
)

func TestNewRuntime_DefaultsRootToCWDAndNormalizesPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	runtime := NewRuntime(nil, guardrails.CommandPolicy{
		Ask:  []string{" git push "},
		Deny: []string{"git push", "git push"},
	})

	require.Equal(t, []string{dir}, runtime.FilePolicy().Roots)
	require.Equal(t, []string{"git push"}, runtime.CommandPolicy().Ask)
	require.Equal(t, []string{"git push"}, runtime.CommandPolicy().Deny)
	require.IsType(t, &MemoryTodoStore{}, runtime.todos)
}

func TestNewRuntime_FallsBackWhenGetwdFails(t *testing.T) {
	originalGetwd := getwd
	t.Cleanup(func() {
		getwd = originalGetwd
	})
	getwd = func() (string, error) {
		return "", errors.New("getwd failed")
	}

	runtime := NewRuntime(nil, guardrails.CommandPolicy{})

	expectedRoot, err := filepath.Abs(".")
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Clean(expectedRoot)}, runtime.FilePolicy().Roots)
}

func TestNewRuntime_NormalizesConfiguredRoots(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "workspace")
	runtime := NewRuntime([]string{root, filepath.Join(root, ".")}, guardrails.CommandPolicy{})
	require.Equal(t, []string{root}, runtime.FilePolicy().Roots)
}

func TestRuntime_FilePolicyHandlesNilReceiver(t *testing.T) {
	var runtime *Runtime
	require.Equal(t, guardrails.NormalizeRoots(nil), runtime.FilePolicy().Roots)
}

func TestRuntime_CommandPolicyHandlesNilReceiver(t *testing.T) {
	var runtime *Runtime
	require.Equal(t, guardrails.CommandPolicy{}.Normalize(), runtime.CommandPolicy())
}

func TestMemoryTodoStore_ReturnsDefensiveCopies(t *testing.T) {
	store := &MemoryTodoStore{}

	replaced := store.Replace([]envtypes.TodoItem{{Text: "first", Done: false}})
	replaced[0].Text = "mutated"

	listed := store.List()
	require.Equal(t, []envtypes.TodoItem{{Text: "first", Done: false}}, listed)

	listed[0].Done = true
	require.Equal(t, []envtypes.TodoItem{{Text: "first", Done: false}}, store.List())
}

func TestMemoryTodoStore_ClearRemovesItems(t *testing.T) {
	store := &MemoryTodoStore{}
	store.Replace([]envtypes.TodoItem{{Text: "first", Done: false}})
	cleared := store.Clear()
	require.Nil(t, cleared)
	require.Empty(t, store.List())
}

func TestRuntime_TodoMethodsDelegateToStore(t *testing.T) {
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{})
	items := []envtypes.TodoItem{{Text: "first", Done: false}}

	replaced := runtime.ReplaceTodos(items)
	listed := runtime.ListTodos()
	cleared := runtime.ClearTodos()

	require.Equal(t, items, replaced)
	require.Equal(t, items, listed)
	require.Nil(t, cleared)
	require.Empty(t, runtime.ListTodos())
}

func TestRuntime_TodoMethodsHandleNilReceiver(t *testing.T) {
	var runtime *Runtime
	items := []envtypes.TodoItem{{Text: "first", Done: false}}
	require.Nil(t, runtime.ListTodos())
	require.Equal(t, items, runtime.ReplaceTodos(items))
	require.Nil(t, runtime.ClearTodos())
}
