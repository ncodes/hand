package agent

import (
	"context"
	"errors"

	"github.com/wandxy/hand/internal/memory"
	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
	storagememory "github.com/wandxy/hand/internal/state/storememory"
)

type memoryProviderStub struct {
	name         string
	caps         memory.Capabilities
	capsErr      error
	configureErr error
	pinnedItems  []memory.MemoryItem
	pinnedQuery  memory.SearchQuery
	pinnedErr    error
	searchResult memory.SearchResult
	searchQuery  memory.SearchQuery
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

func (p *memoryProviderStub) Search(_ context.Context, query memory.SearchQuery) (memory.SearchResult, error) {
	p.searchQuery = query
	return p.searchResult, p.searchErr
}

func (p *memoryProviderStub) LoadPinned(_ context.Context, query memory.SearchQuery) ([]memory.MemoryItem, error) {
	p.pinnedQuery = query
	return p.pinnedItems, p.pinnedErr
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

type timelineErrorStore struct {
	*storagememory.Store
	messageErr error
	traceErr   error
}

func (s *timelineErrorStore) Get(ctx context.Context, id string) (storage.Session, bool, error) {
	return storage.Session{ID: storage.DefaultSessionID}, true, nil
}

func (s *timelineErrorStore) GetMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
	if s.messageErr != nil {
		return nil, s.messageErr
	}

	return nil, nil
}

func (s *timelineErrorStore) ListTraceEvents(ctx context.Context, query storage.TraceQuery) (storage.TraceResult, error) {
	if s.traceErr != nil {
		return storage.TraceResult{}, s.traceErr
	}

	return storage.TraceResult{}, nil
}

func (s *timelineErrorStore) PruneTraceEvents(context.Context, string, int) error {
	return nil
}

func (s *timelineErrorStore) AppendTraceEvent(context.Context, storage.TraceEvent) (storage.TraceEvent, error) {
	return storage.TraceEvent{}, nil
}

type timelineTraceGapErrorStore struct {
	*timelineErrorStore
}

func (s *timelineTraceGapErrorStore) Get(ctx context.Context, id string) (storage.Session, bool, error) {
	return storage.Session{ID: storage.DefaultSessionID}, true, nil
}

func (s *timelineTraceGapErrorStore) GetMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
	return nil, nil
}

func (s *timelineTraceGapErrorStore) ListTraceEvents(ctx context.Context, query storage.TraceQuery) (storage.TraceResult, error) {
	if query.MinSequence > 0 {
		return storage.TraceResult{}, nil
	}

	return storage.TraceResult{}, errors.New("trace gap check failed")
}

func (s *timelineTraceGapErrorStore) PruneTraceEvents(context.Context, string, int) error {
	return nil
}

func (s *timelineTraceGapErrorStore) AppendTraceEvent(context.Context, storage.TraceEvent) (storage.TraceEvent, error) {
	return storage.TraceEvent{}, nil
}

type timelineTraceGapUnsupportedStore struct {
	timelineTraceGapErrorStore
}

func (s *timelineTraceGapUnsupportedStore) ListTraceEvents(ctx context.Context, query storage.TraceQuery) (storage.TraceResult, error) {
	if query.MinSequence > 0 {
		return storage.TraceResult{}, nil
	}

	return storage.TraceResult{}, storage.ErrTraceStoreUnsupported
}
