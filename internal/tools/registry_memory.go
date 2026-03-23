package tools

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

// InMemoryRegistry stores registered tool definitions in memory.
type InMemoryRegistry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

// NewInMemoryRegistry creates an empty in-memory tool registry.
func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{
		definitions: make(map[string]Definition),
	}
}

// Register adds a tool definition to the registry.
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

// Get retrieves a tool definition by name.
func (r *InMemoryRegistry) Get(name string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.definitions[strings.TrimSpace(name)]
	return def, ok
}

// List returns all registered tool definitions.
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

// Invoke dispatches a tool call to the registered handler.
func (r *InMemoryRegistry) Invoke(ctx context.Context, call Call) (Result, error) {
	def, ok := r.Get(call.Name)
	if !ok {
		return Result{}, errors.New("tool is not registered")
	}

	return def.Handler.Invoke(ctx, call)
}
