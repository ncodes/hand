package tools

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

type Call struct {
	Name   string
	Input  string
	Source string
}

type Result struct {
	Output string
	Error  string
}

type Handler interface {
	Invoke(context.Context, Call) (Result, error)
}

type HandlerFunc func(context.Context, Call) (Result, error)

func (f HandlerFunc) Invoke(ctx context.Context, call Call) (Result, error) {
	return f(ctx, call)
}

type Definition struct {
	Name        string
	Description string
	Handler     Handler
}

type Registry interface {
	Register(Definition) error
	Get(string) (Definition, bool)
	List() []Definition
	Invoke(context.Context, Call) (Result, error)
}

type InMemoryRegistry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

func NewRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{
		definitions: make(map[string]Definition),
	}
}

func (r *InMemoryRegistry) Register(def Definition) error {
	if r == nil {
		return errors.New("tool registry is required")
	}

	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return errors.New("tool name is required")
	}
	if def.Handler == nil {
		return errors.New("tool handler is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[def.Name]; exists {
		return errors.New("tool is already registered")
	}

	r.definitions[def.Name] = def
	return nil
}

func (r *InMemoryRegistry) Get(name string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.definitions[strings.TrimSpace(name)]
	return def, ok
}

func (r *InMemoryRegistry) List() []Definition {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]Definition, 0, len(r.definitions))
	for _, def := range r.definitions {
		definitions = append(definitions, def)
	}

	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})

	return definitions
}

func (r *InMemoryRegistry) Invoke(ctx context.Context, call Call) (Result, error) {
	def, ok := r.Get(call.Name)
	if !ok {
		return Result{}, errors.New("tool is not registered")
	}

	return def.Handler.Invoke(ctx, call)
}
