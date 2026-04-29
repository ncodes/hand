package memory

import (
	"context"
)

const ProviderNoop = "noop"

type NoopProvider struct {
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
	traceRecord(ctx, p.obs, "memory.pinned.noop", map[string]any{"provider": p.Name()})
	return nil, nil
}

func (p *NoopProvider) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if err := validateSearch(ctx, p.guardrails, query); err != nil {
		return SearchResult{}, err
	}
	logDebug(p.obs, "memory search skipped", map[string]any{"provider": p.Name()})
	traceRecord(ctx, p.obs, "memory.search.noop", map[string]any{"provider": p.Name()})
	return SearchResult{}, nil
}

func (p *NoopProvider) Upsert(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if err := validateWrite(ctx, p.guardrails, item); err != nil {
		return MemoryItem{}, err
	}
	traceRecord(ctx, p.obs, "memory.upsert.noop", map[string]any{"provider": p.Name()})
	return cloneItem(item), nil
}

func (p *NoopProvider) Delete(ctx context.Context, req DeleteRequest) error {
	if err := validateDelete(ctx, p.guardrails, req); err != nil {
		return err
	}
	traceRecord(ctx, p.obs, "memory.delete.noop", map[string]any{"provider": p.Name()})
	return nil
}
