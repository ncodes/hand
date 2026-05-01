package memory

import (
	"context"
	"errors"
	"strings"
	"sync"

	statecore "github.com/wandxy/hand/internal/state/core"
)

const ProviderDefaultMemory = "default-memory"

var ErrUnknownProvider = errors.New("unknown memory provider")
var ErrUnknownBackend = errors.New("unknown memory backend")

type Options struct {
	Guardrails     Guardrails
	Observability  Observability
	MemoryStore    statecore.MemoryStore
	StorageBackend string
	MemoryBackend  string
}

type MemoryProvider struct {
	mu         sync.RWMutex
	store      statecore.MemoryStore
	guardrails Guardrails
	obs        Observability
}

func NewProvider(name string, opts Options) (Provider, error) {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "", ProviderDefaultMemory:
		switch effectiveBackend(opts) {
		case "memory", "sqlite":
			return NewFromStore(opts.MemoryStore, opts)
		default:
			return nil, ErrUnknownBackend
		}
	default:
		return nil, ErrUnknownProvider
	}
}

func effectiveBackend(opts Options) string {
	if backend := strings.TrimSpace(strings.ToLower(opts.MemoryBackend)); backend != "" {
		return backend
	}
	if backend := strings.TrimSpace(strings.ToLower(opts.StorageBackend)); backend != "" {
		return backend
	}
	return "sqlite"
}

func NewFromStore(store statecore.MemoryStore, opts Options) (*MemoryProvider, error) {
	if store == nil {
		return nil, errors.New("memory store is required")
	}

	return &MemoryProvider{
		store:      store,
		guardrails: opts.Guardrails,
		obs:        opts.Observability,
	}, nil
}

func (p *MemoryProvider) Name() string {
	return ProviderDefaultMemory
}

func (p *MemoryProvider) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{
		SupportsPinned:        true,
		SupportsSearch:        true,
		SupportsWrite:         true,
		SupportsDelete:        true,
		SupportsReranking:     true,
		SupportsObservability: true,
	}, nil
}

func (p *MemoryProvider) ConfigureObservability(obs Observability) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.obs = obs
	return nil
}

func (p *MemoryProvider) Close() error {
	return nil
}

func (p *MemoryProvider) LoadPinned(ctx context.Context, query SearchQuery) ([]MemoryItem, error) {
	query.Kinds = []Kind{KindPinned}
	result, err := p.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	items := make([]MemoryItem, 0, len(result.Hits))
	for _, hit := range result.Hits {
		items = append(items, hit.Item)
	}
	return items, nil
}

func (p *MemoryProvider) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if p == nil || p.store == nil {
		return SearchResult{}, errors.New("memory provider is required")
	}
	if err := validateSearch(ctx, p.guardrails, query); err != nil {
		return SearchResult{}, err
	}

	result, err := p.store.SearchMemory(ctx, query)
	if err != nil {
		return SearchResult{}, err
	}

	hits := make([]SearchHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		redacted, err := redactItem(ctx, p.guardrails, hit.Item)
		if err != nil {
			return SearchResult{}, err
		}

		if query.MaxChars > 0 && len([]rune(redacted.Text)) > query.MaxChars {
			redacted.Text = string([]rune(redacted.Text)[:query.MaxChars])
		}

		hits = append(hits, SearchHit{Item: redacted.Clone(), Score: hit.Score})
	}

	obs := p.observability()
	fields := observationFields(p.Name(), "search", map[string]any{"result_count": len(hits)})
	logDebugAndTrace(ctx, obs, "memory search completed", "memory.search.completed", fields)

	return SearchResult{Hits: hits}, nil
}

func (p *MemoryProvider) Upsert(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if p == nil || p.store == nil {
		return MemoryItem{}, errors.New("memory provider is required")
	}
	if err := validateWrite(ctx, p.guardrails, item); err != nil {
		return MemoryItem{}, err
	}

	item, err := p.store.UpsertMemory(ctx, item)
	if err != nil {
		return MemoryItem{}, err
	}

	obs := p.observability()
	fields := observationFields(p.Name(), "upsert", map[string]any{"memory_id": item.ID})
	logDebugAndTrace(ctx, obs, "memory item upserted", "memory.upsert.completed", fields)

	return item.Clone(), nil
}

func (p *MemoryProvider) Delete(ctx context.Context, req DeleteRequest) error {
	if p == nil || p.store == nil {
		return errors.New("memory provider is required")
	}
	if err := validateDelete(ctx, p.guardrails, req); err != nil {
		return err
	}

	if err := p.store.DeleteMemory(ctx, req); err != nil {
		return err
	}

	fields := observationFields(p.Name(), "delete", map[string]any{"memory_id": strings.TrimSpace(req.ID)})
	traceRecord(ctx, p.observability(), "memory.delete.completed", fields)
	return nil
}

func (p *MemoryProvider) observability() Observability {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.obs
}
