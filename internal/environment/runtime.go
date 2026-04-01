package environment

import (
	"os"
	"path/filepath"
	"sync"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
)

var getwd = os.Getwd

type Runtime struct {
	filePolicy    guardrails.FilesystemPolicy
	commandPolicy guardrails.CommandPolicy
	todos         TodoStore
}

type TodoStore interface {
	Replace([]envtypes.TodoItem) []envtypes.TodoItem
	List() []envtypes.TodoItem
	Clear() []envtypes.TodoItem
}

func NewRuntime(roots []string, policy guardrails.CommandPolicy) *Runtime {
	if len(roots) == 0 {
		cwd, err := getwd()
		if err != nil {
			cwd = "."
		}
		roots = []string{filepath.Clean(cwd)}
	}

	return &Runtime{
		filePolicy:    guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots(roots)},
		commandPolicy: policy.Normalize(),
		todos:         &MemoryTodoStore{},
	}
}

func (r *Runtime) FilePolicy() guardrails.FilesystemPolicy {
	if r == nil {
		return guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots(nil)}
	}
	return r.filePolicy
}

func (r *Runtime) CommandPolicy() guardrails.CommandPolicy {
	if r == nil {
		return guardrails.CommandPolicy{}.Normalize()
	}
	return r.commandPolicy
}

func (r *Runtime) ListTodos() []envtypes.TodoItem {
	if r == nil || r.todos == nil {
		return nil
	}
	return r.todos.List()
}

func (r *Runtime) ReplaceTodos(items []envtypes.TodoItem) []envtypes.TodoItem {
	if r == nil || r.todos == nil {
		return append([]envtypes.TodoItem(nil), items...)
	}
	return r.todos.Replace(items)
}

func (r *Runtime) ClearTodos() []envtypes.TodoItem {
	if r == nil || r.todos == nil {
		return nil
	}
	return r.todos.Clear()
}

type MemoryTodoStore struct {
	mu    sync.Mutex
	items []envtypes.TodoItem
}

func (s *MemoryTodoStore) Replace(items []envtypes.TodoItem) []envtypes.TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append([]envtypes.TodoItem(nil), items...)
	return append([]envtypes.TodoItem(nil), s.items...)
}

func (s *MemoryTodoStore) List() []envtypes.TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]envtypes.TodoItem(nil), s.items...)
}

func (s *MemoryTodoStore) Clear() []envtypes.TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
	return nil
}
