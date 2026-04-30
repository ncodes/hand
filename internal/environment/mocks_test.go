package environment

import (
	"context"

	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/tools"
)

type memoryProviderWithoutSearch struct{}

func (memoryProviderWithoutSearch) Name() string {
	return "without-search"
}

func (memoryProviderWithoutSearch) Capabilities(context.Context) (memory.Capabilities, error) {
	return memory.Capabilities{SupportsSearch: true}, nil
}

func (memoryProviderWithoutSearch) ConfigureObservability(memory.Observability) error {
	return nil
}

func (memoryProviderWithoutSearch) Close() error {
	return nil
}

type memorySearchProviderStub struct {
	caps         memory.Capabilities
	capsErr      error
	searchQuery  memory.SearchQuery
	searchResult memory.SearchResult
	searchErr    error
}

func (p memorySearchProviderStub) Name() string {
	return "search"
}

func (p memorySearchProviderStub) Capabilities(context.Context) (memory.Capabilities, error) {
	return p.caps, p.capsErr
}

func (p memorySearchProviderStub) ConfigureObservability(memory.Observability) error {
	return nil
}

func (p memorySearchProviderStub) Close() error {
	return nil
}

func (p *memorySearchProviderStub) Search(_ context.Context, query memory.SearchQuery) (memory.SearchResult, error) {
	p.searchQuery = query
	return p.searchResult, p.searchErr
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

func (failingRegistry) RegisterGroup(tools.Group) error {
	return nil
}

func (failingRegistry) GetGroup(string) (tools.Group, bool) {
	return tools.Group{}, false
}

func (failingRegistry) List() tools.Definitions {
	return nil
}

func (failingRegistry) ListGroups() []tools.Group {
	return nil
}

func (failingRegistry) Resolve(tools.Policy) (tools.Definitions, error) {
	return nil, nil
}

func (failingRegistry) Invoke(context.Context, tools.Call) (tools.Result, error) {
	return tools.Result{}, nil
}

type failingGroupRegistry struct {
	err error
}

func (failingGroupRegistry) Register(tools.Definition) error {
	return nil
}

func (failingGroupRegistry) Get(string) (tools.Definition, bool) {
	return tools.Definition{}, false
}

func (r failingGroupRegistry) RegisterGroup(tools.Group) error {
	return r.err
}

func (failingGroupRegistry) GetGroup(string) (tools.Group, bool) {
	return tools.Group{}, false
}

func (failingGroupRegistry) List() tools.Definitions {
	return nil
}

func (failingGroupRegistry) ListGroups() []tools.Group {
	return nil
}

func (failingGroupRegistry) Resolve(tools.Policy) (tools.Definitions, error) {
	return nil, nil
}

func (failingGroupRegistry) Invoke(context.Context, tools.Call) (tools.Result, error) {
	return tools.Result{}, nil
}
