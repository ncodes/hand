package memory

import (
	"context"
	"sync"
)

const ProviderNoop = "noop"

type NoopProvider struct {
	mu         sync.RWMutex
	guardrails Guardrails
	obs        Observability
}

func NewNoopProvider(opts Options) *NoopProvider {
	return &NoopProvider{
		guardrails: opts.Guardrails,
		obs:        opts.Observability,
	}
}

func (p *NoopProvider) Name() string {
	return ProviderNoop
}

func (p *NoopProvider) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{SupportsObservability: true}, nil
}

func (p *NoopProvider) ConfigureObservability(obs Observability) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.obs = obs
	return nil
}

func (p *NoopProvider) Close() error {
	return nil
}

func (p *NoopProvider) LoadPinned(ctx context.Context, query SearchQuery) ([]MemoryItem, error) {
	if err := validateSearch(ctx, p.guardrails, query); err != nil {
		return nil, err
	}
	traceRecord(ctx, p.observability(), "memory.pinned.noop", map[string]any{"provider": p.Name(), "operation": "load_pinned"})
	return nil, nil
}

func (p *NoopProvider) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if err := validateSearch(ctx, p.guardrails, query); err != nil {
		return SearchResult{}, err
	}
	obs := p.observability()
	logDebug(obs, "memory search skipped", map[string]any{"provider": p.Name(), "operation": "search"})
	traceRecord(ctx, obs, "memory.search.noop", map[string]any{"provider": p.Name(), "operation": "search"})
	return SearchResult{}, nil
}

func (p *NoopProvider) Upsert(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if err := validateWrite(ctx, p.guardrails, item); err != nil {
		return MemoryItem{}, err
	}
	traceRecord(ctx, p.observability(), "memory.upsert.noop", map[string]any{"provider": p.Name(), "operation": "upsert"})
	return cloneItem(item), nil
}

func (p *NoopProvider) Delete(ctx context.Context, req DeleteRequest) error {
	if err := validateDelete(ctx, p.guardrails, req); err != nil {
		return err
	}
	traceRecord(ctx, p.observability(), "memory.delete.noop", map[string]any{"provider": p.Name(), "operation": "delete"})
	return nil
}

func (p *NoopProvider) observability() Observability {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.obs
}
