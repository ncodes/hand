package agent

import (
	"context"

	"github.com/wandxy/hand/internal/memory"
)

type memoryProviderStub struct {
	name         string
	caps         memory.Capabilities
	capsErr      error
	configureErr error
	searchResult memory.SearchResult
	searchErr    error
}

func (p *memoryProviderStub) Name() string {
	if p.name == "" {
		return "stub"
	}
	return p.name
}

func (p *memoryProviderStub) Capabilities(context.Context) (memory.Capabilities, error) {
	return p.caps, p.capsErr
}

func (p *memoryProviderStub) ConfigureObservability(memory.Observability) error {
	return p.configureErr
}

func (p *memoryProviderStub) Close() error {
	return nil
}

func (p *memoryProviderStub) Search(context.Context, memory.SearchQuery) (memory.SearchResult, error) {
	return p.searchResult, p.searchErr
}

type nonSearchMemoryProvider struct{}

func (nonSearchMemoryProvider) Name() string {
	return "non-search"
}

func (nonSearchMemoryProvider) Capabilities(context.Context) (memory.Capabilities, error) {
	return memory.Capabilities{}, nil
}

func (nonSearchMemoryProvider) ConfigureObservability(memory.Observability) error {
	return nil
}

func (nonSearchMemoryProvider) Close() error {
	return nil
}
