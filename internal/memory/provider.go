package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/memory/episodic"
	pinnedmemory "github.com/wandxy/hand/internal/memory/pinned"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	statecore "github.com/wandxy/hand/internal/state/core"
)

const ProviderDefaultMemory = constants.MemoryProviderDefault

var ErrUnknownProvider = errors.New("unknown memory provider")
var ErrUnknownBackend = errors.New("unknown memory backend")

type Options struct {
	Guardrails           Guardrails
	Observability        Observability
	StateManager         StateManager
	StorageBackend       string
	MemoryBackend        string
	Pinned               PinnedOptions
	EpisodicBackground   EpisodicBackgroundOptions
	ReflectionBackground ReflectionBackgroundOptions
	PromotionBackground  PromotionBackgroundOptions
	ModelClient          models.Client
	Model                string
	APIMode              string
	DebugRequests        bool
	ReflectionGenerator  ReflectionGenerator
	PromotionPolicy      PromotionPolicy
}

type PinnedOptions = pinnedmemory.Options

type StateManager interface {
	SearchMemory(context.Context, SearchQuery) (SearchResult, error)
	UpsertMemory(context.Context, MemoryItem) (MemoryItem, error)
	PatchMemory(context.Context, MemoryPatch) (MemoryItem, error)
	DeleteMemory(context.Context, DeleteRequest) error
	CurrentSession(context.Context) (string, error)
	CountMessages(context.Context, string, statecore.MessageQueryOptions) (int, error)
	GetMessages(context.Context, string, statecore.MessageQueryOptions) ([]handmsg.Message, error)
	ListTraceEvents(context.Context, statecore.TraceQuery) (statecore.TraceResult, error)
	UpdateEpisodicCheckpoint(context.Context, string, int) error
}

type MemoryProvider struct {
	mu                            sync.RWMutex
	manager                       StateManager
	guardrails                    Guardrails
	obs                           Observability
	pinned                        PinnedOptions
	episodicExtractor             *episodic.Service
	episodicBackground            EpisodicBackgroundOptions
	episodicBackgroundStartOnce   sync.Once
	reflectionBackground          ReflectionBackgroundOptions
	reflectionBackgroundStartOnce sync.Once
	promotionBackground           PromotionBackgroundOptions
	promotionBackgroundStartOnce  sync.Once
	reflectionGenerator           ReflectionGenerator
	promotionPolicy               PromotionPolicy
}

func NewProvider(name string, opts Options) (Provider, error) {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "", ProviderDefaultMemory:
		switch effectiveBackend(opts) {
		case "memory", "sqlite":
			return NewFromManager(opts.StateManager, opts)
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
	return constants.DefaultStorageBackend
}

func NewFromManager(manager StateManager, opts Options) (*MemoryProvider, error) {
	if manager == nil {
		return nil, errors.New("state manager is required")
	}

	provider := &MemoryProvider{
		manager:              manager,
		guardrails:           opts.Guardrails,
		obs:                  opts.Observability,
		pinned:               pinnedmemory.NormalizeOptions(opts.Pinned),
		episodicBackground:   episodic.NormalizeBackgroundOptions(opts.EpisodicBackground),
		reflectionBackground: normalizeReflectionBackgroundOptions(opts.ReflectionBackground),
		promotionBackground:  normalizePromotionBackgroundOptions(opts.PromotionBackground),
		reflectionGenerator:  opts.ReflectionGenerator,
		promotionPolicy:      opts.PromotionPolicy,
	}

	if opts.ModelClient != nil {
		extractor, err := episodic.NewLLMExtractor(episodic.LLMExtractorOptions{
			Client:        opts.ModelClient,
			Model:         opts.Model,
			APIMode:       opts.APIMode,
			DebugRequests: opts.DebugRequests,
		})
		if err != nil {
			return nil, err
		}

		service, err := episodic.NewService(manager, provider, extractor)
		if err != nil {
			return nil, err
		}
		provider.episodicExtractor = service

		if provider.reflectionGenerator == nil {
			generator, err := NewLLMReflectionGenerator(LLMReflectionGeneratorOptions{
				Client:        opts.ModelClient,
				Model:         opts.Model,
				APIMode:       opts.APIMode,
				DebugRequests: opts.DebugRequests,
			})
			if err != nil {
				return nil, err
			}
			provider.reflectionGenerator = generator
		}
	}

	return provider, nil
}

func (p *MemoryProvider) Name() string {
	return ProviderDefaultMemory
}

func (p *MemoryProvider) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{
		SupportsPinned:                      true,
		SupportsSearch:                      true,
		SupportsWrite:                       true,
		SupportsDelete:                      true,
		SupportsEpisodeRecording:            true,
		SupportsSemanticProceduralRecording: true,
		SupportsReflection:                  p != nil && p.reflectionGenerator != nil,
		SupportsVectors:                     p != nil && supportsVectorSearch(p.manager),
		SupportsReranking:                   true,
		SupportsAudit:                       p != nil && p.manager != nil,
		SupportsObservability:               true,
	}, nil
}

func supportsVectorSearch(manager StateManager) bool {
	vectorManager, ok := manager.(interface{ SupportsVectorSearch() bool })
	return ok && vectorManager.SupportsVectorSearch()
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
	if p == nil || p.manager == nil {
		return nil, errors.New("memory provider is required")
	}
	if !pinnedmemory.Enabled(p.pinned) {
		return nil, nil
	}
	if err := validateSearch(ctx, p.guardrails, query); err != nil {
		return nil, err
	}

	fileItems, err := pinnedmemory.LoadFile()
	if err != nil {
		return nil, err
	}

	dbItems, err := p.loadStorePinned(ctx, query)
	if err != nil {
		return nil, err
	}

	items := append(fileItems, dbItems...)
	items, err = pinnedmemory.PrepareItems(ctx, items, query, p.pinned, p.safetyScanPinnedItem, p.redactPinnedItem)
	if err != nil {
		return nil, err
	}
	if query.Limit > 0 && len(items) > query.Limit {
		items = items[:query.Limit]
	}

	fields := observationFields(p.Name(), "load_pinned", map[string]any{"result_count": len(items)})
	logDebugAndTrace(ctx, p.observability(), "pinned memory loaded", "memory.pinned.loaded", fields)

	return items, nil
}

func (p *MemoryProvider) loadStorePinned(ctx context.Context, query SearchQuery) ([]MemoryItem, error) {
	storeQuery := query
	storeQuery.Text = ""
	storeQuery.Kinds = []Kind{KindPinned}
	storeQuery.Statuses = []Status{StatusActive}

	result, err := p.manager.SearchMemory(ctx, storeQuery)
	if err != nil {
		return nil, err
	}

	items := make([]MemoryItem, 0, len(result.Hits))
	for _, hit := range result.Hits {
		items = append(items, hit.Item.Clone())
	}
	return items, nil
}

func (p *MemoryProvider) safetyScanPinnedItem(ctx context.Context, item MemoryItem) error {
	if p.guardrails == nil {
		return nil
	}
	return p.guardrails.SafetyScan(ctx, item)
}

func (p *MemoryProvider) redactPinnedItem(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	return redactItem(ctx, p.guardrails, item)
}

func (p *MemoryProvider) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if p == nil || p.manager == nil {
		return SearchResult{}, errors.New("memory provider is required")
	}
	if err := validateSearch(ctx, p.guardrails, query); err != nil {
		return SearchResult{}, err
	}

	obs := p.observability()
	logDebugAndTrace(ctx, obs, "memory search started", "memory.search.started", observationFields(p.Name(), "search", nil))

	result, err := p.manager.SearchMemory(ctx, query)
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

	fields := observationFields(p.Name(), "search", map[string]any{"result_count": len(hits)})
	logDebugAndTrace(ctx, obs, "memory search completed", "memory.search.completed", fields)

	return SearchResult{Hits: hits}, nil
}

func (p *MemoryProvider) Upsert(ctx context.Context, item MemoryItem) (MemoryItem, error) {
	if p == nil || p.manager == nil {
		return MemoryItem{}, errors.New("memory provider is required")
	}
	if err := validateWrite(ctx, p.guardrails, item); err != nil {
		return MemoryItem{}, err
	}

	item, err := p.manager.UpsertMemory(ctx, item)
	if err != nil {
		return MemoryItem{}, err
	}

	obs := p.observability()
	fields := observationFields(p.Name(), "upsert", map[string]any{"memory_id": item.ID})
	logDebugAndTrace(ctx, obs, "memory item upserted", "memory.upsert.completed", fields)

	return item.Clone(), nil
}

func (p *MemoryProvider) Delete(ctx context.Context, req DeleteRequest) error {
	if p == nil || p.manager == nil {
		return errors.New("memory provider is required")
	}
	if err := validateDelete(ctx, p.guardrails, req); err != nil {
		return err
	}

	memoryID := strings.TrimSpace(req.ID)
	if memoryID == "" {
		return errors.New("memory id is required")
	}
	item, err := p.loadLifecycleMemory(ctx, memoryID, []Status{StatusActive, StatusCandidate, StatusSuperseded})
	if err != nil {
		return err
	}
	previousStatus := item.Status
	item.Status = StatusDeleted
	item.Metadata = lifecycleMetadata(item.Metadata, "delete", req.Reason, previousStatus)

	if _, err := p.manager.UpsertMemory(ctx, item); err != nil {
		return err
	}

	fields := observationFields(p.Name(), "delete", map[string]any{"memory_id": memoryID})
	traceRecord(ctx, p.observability(), "memory.delete.completed", fields)
	return nil
}

func (p *MemoryProvider) RecordEpisode(ctx context.Context, record EpisodeRecord) (MemoryItem, error) {
	item := record.Item.Clone()
	item.Kind = KindEpisodic
	if item.Status == "" {
		item.Status = StatusActive
	}
	return p.Upsert(ctx, item)
}

func (p *MemoryProvider) ExtractEpisodes(ctx context.Context, req ExtractionRequest) (ExtractionResult, error) {
	if p == nil || p.episodicExtractor == nil {
		return ExtractionResult{}, errors.New("memory extraction is not configured")
	}

	return p.episodicExtractor.Extract(ctx, req)
}

func (p *MemoryProvider) StartBackground(ctx context.Context) error {
	if p == nil {
		return errors.New("memory provider is required")
	}

	ctx = backgroundContext(ctx)
	for _, start := range p.backgroundStarters() {
		if err := start(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (p *MemoryProvider) backgroundStarters() []func(context.Context) error {
	return []func(context.Context) error{
		p.startEpisodicRecordingBackground,
		p.startReflectionBackground,
		p.startPromotionBackground,
	}
}

func (p *MemoryProvider) startEpisodicRecordingBackground(ctx context.Context) error {
	if !p.episodicBackground.Enabled {
		return nil
	}
	if p.episodicExtractor == nil {
		return errors.New("memory extraction is not configured")
	}

	opts := episodic.NormalizeBackgroundOptions(p.episodicBackground)
	p.episodicBackgroundStartOnce.Do(func() {
		go p.runEpisodicRecordingBackgroundLoop(ctx, opts)
	})

	return nil
}

func backgroundContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

func (p *MemoryProvider) runEpisodicRecordingBackgroundLoop(ctx context.Context, opts EpisodicBackgroundOptions) {
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = p.episodicExtractor.RunBackground(ctx, episodic.BackgroundRequest{
				Options: opts,
				Trace: providerTraceRecorder{
					ctx: ctx,
					obs: p.observability(),
				},
			})
		}
	}
}

func (p *MemoryProvider) observability() Observability {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.obs
}

type providerTraceRecorder struct {
	ctx context.Context
	obs Observability
}

func (r providerTraceRecorder) Record(event string, payload any) {
	fields, ok := payload.(map[string]any)
	if !ok {
		fields = map[string]any{"payload": payload}
	}
	traceRecord(r.ctx, r.obs, event, fields)
}
