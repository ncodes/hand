package memory

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wandxy/hand/pkg/nanoid"
)

const ProviderInMemory = "memory"

type InMemoryProvider struct {
	mu         sync.Mutex
	items      map[string]MemoryItem
	guardrails Guardrails
	obs        Observability
	now        func() time.Time
}

func NewInMemoryProvider(opts Options) *InMemoryProvider {
	return &InMemoryProvider{
		items:      make(map[string]MemoryItem),
		guardrails: opts.Guardrails,
		obs:        opts.Observability,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (p *InMemoryProvider) Name() string {
	return ProviderInMemory
}

func (p *InMemoryProvider) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{
		SupportsPinned:        true,
		SupportsSearch:        true,
		SupportsWrite:         true,
		SupportsDelete:        true,
		SupportsObservability: true,
	}, nil
}

func (p *InMemoryProvider) ConfigureObservability(obs Observability) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.obs = obs
	return nil
}

func (p *InMemoryProvider) Close() error {
	return nil
}

func (p *InMemoryProvider) LoadPinned(ctx context.Context, query SearchQuery) ([]MemoryItem, error) {
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

func (p *InMemoryProvider) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if err := validateSearch(ctx, p.guardrails, query); err != nil {
		return SearchResult{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	hits := make([]SearchHit, 0, len(p.items))
	for _, item := range p.items {
		if !matchesQuery(item, query) {
			continue
		}

		score := simpleScore(item, query.Text)
		redacted, err := redactItem(ctx, p.guardrails, item)
		if err != nil {
			return SearchResult{}, err
		}

		if query.MaxChars > 0 && len([]rune(redacted.Text)) > query.MaxChars {
			redacted.Text = string([]rune(redacted.Text)[:query.MaxChars])
		}

		hits = append(hits, SearchHit{Item: cloneItem(redacted), Score: score})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if !hits[i].Item.UpdatedAt.Equal(hits[j].Item.UpdatedAt) {
			return hits[i].Item.UpdatedAt.After(hits[j].Item.UpdatedAt)
		}
		return hits[i].Item.ID < hits[j].Item.ID
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}

	logDebug(p.obs, "memory search completed", map[string]any{"provider": p.Name(), "operation": "search", "result_count": len(hits)})
	traceRecord(ctx, p.obs, "memory.search.completed", map[string]any{"provider": p.Name(), "operation": "search", "result_count": len(hits)})

	return SearchResult{Hits: hits}, nil
}

func (p *InMemoryProvider) Upsert(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if err := validateWrite(ctx, p.guardrails, item); err != nil {
		return MemoryItem{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now().UTC()
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = nanoid.MustGenerate("mem_")
	}
	if item.Status == "" {
		item.Status = StatusCandidate
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	p.items[item.ID] = cloneItem(item)
	logDebug(p.obs, "memory item upserted", map[string]any{"provider": p.Name(), "operation": "upsert", "memory_id": item.ID})
	traceRecord(ctx, p.obs, "memory.upsert.completed", map[string]any{"provider": p.Name(), "operation": "upsert", "memory_id": item.ID})

	return cloneItem(item), nil
}

func (p *InMemoryProvider) Delete(ctx context.Context, req DeleteRequest) error {
	if err := validateDelete(ctx, p.guardrails, req); err != nil {
		return err
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		return errors.New("memory id is required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.items[id]
	if !ok {
		return nil
	}
	item.Status = StatusDeleted
	item.UpdatedAt = p.now().UTC()
	p.items[id] = item

	traceRecord(ctx, p.obs, "memory.delete.completed", map[string]any{"provider": p.Name(), "operation": "delete", "memory_id": id})
	return nil
}

func matchesQuery(item MemoryItem, query SearchQuery) bool {
	if len(query.Kinds) > 0 && !containsKind(query.Kinds, item.Kind) {
		return false
	}
	if len(query.Statuses) > 0 {
		if !containsStatus(query.Statuses, item.Status) {
			return false
		}
	} else if item.Status != StatusActive {
		return false
	}
	if len(query.Tags) > 0 && !containsAllTags(item.Tags, query.Tags) {
		return false
	}

	text := strings.TrimSpace(strings.ToLower(query.Text))
	if text == "" {
		return true
	}

	return strings.Contains(strings.ToLower(item.Title), text) || strings.Contains(strings.ToLower(item.Text), text)
}

func simpleScore(item MemoryItem, query string) float64 {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return 0
	}

	score := 0.0
	if strings.Contains(strings.ToLower(item.Title), query) {
		score += 2
	}
	if strings.Contains(strings.ToLower(item.Text), query) {
		score++
	}
	return score
}

func containsKind(values []Kind, target Kind) bool {
	return slices.Contains(values, target)
}

func containsStatus(values []Status, target Status) bool {
	return slices.Contains(values, target)
}

func containsAllTags(itemTags []string, queryTags []string) bool {
	tags := make(map[string]struct{}, len(itemTags))
	for _, tag := range itemTags {
		tags[strings.TrimSpace(strings.ToLower(tag))] = struct{}{}
	}
	for _, tag := range queryTags {
		if _, ok := tags[strings.TrimSpace(strings.ToLower(tag))]; !ok {
			return false
		}
	}
	return true
}

func cloneItem(item MemoryItem) MemoryItem {
	item.Tags = append([]string(nil), item.Tags...)
	if len(item.Metadata) > 0 {
		metadata := make(map[string]string, len(item.Metadata))
		for key, value := range item.Metadata {
			metadata[key] = value
		}
		item.Metadata = metadata
	}
	if len(item.SourceLinks) > 0 {
		links := make([]SourceLink, 0, len(item.SourceLinks))
		for _, link := range item.SourceLinks {
			link.MessageIDs = append([]uint(nil), link.MessageIDs...)
			link.Offsets = append([]int(nil), link.Offsets...)
			links = append(links, link)
		}
		item.SourceLinks = links
	}
	return item
}
