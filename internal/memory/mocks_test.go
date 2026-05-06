package memory

import (
	"context"

	handmsg "github.com/wandxy/hand/internal/messages"
	statecore "github.com/wandxy/hand/internal/state/core"
)

type fakeLogger struct {
	debug []map[string]any
}

func (l *fakeLogger) Debug(_ string, fields map[string]any) {
	l.debug = append(l.debug, fields)
}

func (l *fakeLogger) Info(string, map[string]any)  {}
func (l *fakeLogger) Warn(string, map[string]any)  {}
func (l *fakeLogger) Error(string, map[string]any) {}

type fakeTracer struct {
	events []string
	fields []map[string]any
}

func (t *fakeTracer) Record(_ context.Context, event string, fields map[string]any) {
	t.events = append(t.events, event)
	t.fields = append(t.fields, fields)
}

type fakeObservability struct {
	logger *fakeLogger
	tracer *fakeTracer
}

func (o fakeObservability) Logger() Logger {
	if o.logger == nil {
		return nil
	}
	return o.logger
}

func (o fakeObservability) Tracer() Tracer {
	if o.tracer == nil {
		return nil
	}
	return o.tracer
}

type fakeGuardrails struct {
	validateSearchCalls int
	validateWriteCalls  int
	validateDeleteCalls int
	safetyScanCalls     int
	redactCalls         int
	searchErr           error
	writeErr            error
	safetyErr           error
	deleteErr           error
	redactErr           error
	redactText          string
}

func (g *fakeGuardrails) ValidateSearch(context.Context, SearchQuery) error {
	g.validateSearchCalls++
	return g.searchErr
}

func (g *fakeGuardrails) ValidateWrite(context.Context, MemoryItem) error {
	g.validateWriteCalls++
	return g.writeErr
}

func (g *fakeGuardrails) ValidateDelete(context.Context, DeleteRequest) error {
	g.validateDeleteCalls++
	return g.deleteErr
}

func (g *fakeGuardrails) SafetyScan(context.Context, MemoryItem) error {
	g.safetyScanCalls++
	return g.safetyErr
}

func (g *fakeGuardrails) Redact(_ context.Context, item MemoryItem) (MemoryItem, error) {
	g.redactCalls++
	if g.redactErr != nil {
		return MemoryItem{}, g.redactErr
	}
	if g.redactText != "" {
		item.Text = g.redactText
	}
	return item, nil
}

type fakeMemoryManager struct {
	searchResult SearchResult
	searchErr    error
	upsertItem   MemoryItem
	upsertErr    error
	deleteErr    error
}

func (s fakeMemoryManager) SearchMemory(context.Context, SearchQuery) (SearchResult, error) {
	return s.searchResult, s.searchErr
}

func (s fakeMemoryManager) UpsertMemory(_ context.Context, item MemoryItem) (MemoryItem, error) {
	if s.upsertErr != nil {
		return MemoryItem{}, s.upsertErr
	}
	if s.upsertItem.ID != "" {
		return s.upsertItem, nil
	}
	return item, nil
}

func (s fakeMemoryManager) DeleteMemory(context.Context, DeleteRequest) error {
	return s.deleteErr
}

func (s fakeMemoryManager) CurrentSession(context.Context) (string, error) {
	return "", nil
}

func (s fakeMemoryManager) CountMessages(context.Context, string, statecore.MessageQueryOptions) (int, error) {
	return 0, nil
}

func (s fakeMemoryManager) GetMessages(context.Context, string, statecore.MessageQueryOptions) ([]handmsg.Message, error) {
	return nil, nil
}

func (s fakeMemoryManager) ListTraceEvents(context.Context, statecore.TraceQuery) (statecore.TraceResult, error) {
	return statecore.TraceResult{}, nil
}

func (s fakeMemoryManager) UpdateEpisodicCheckpoint(context.Context, string, int) error {
	return nil
}

type recordingMemoryManager struct {
	fakeMemoryManager
	searchResults     []SearchResult
	searchErrs        []error
	upsertErrs        []error
	upsertItems       []MemoryItem
	upsertErr         error
	currentSessionErr error
}

func (m *recordingMemoryManager) SearchMemory(ctx context.Context, query SearchQuery) (SearchResult, error) {
	if len(m.searchErrs) > 0 {
		err := m.searchErrs[0]
		m.searchErrs = m.searchErrs[1:]
		if err != nil {
			return SearchResult{}, err
		}
	}
	if len(m.searchResults) > 0 {
		result := m.searchResults[0]
		m.searchResults = m.searchResults[1:]
		return result, nil
	}

	return m.fakeMemoryManager.SearchMemory(ctx, query)
}

func (m *recordingMemoryManager) UpsertMemory(_ context.Context, item MemoryItem) (MemoryItem, error) {
	m.upsertItems = append(m.upsertItems, item.Clone())
	if len(m.upsertErrs) > 0 {
		err := m.upsertErrs[0]
		m.upsertErrs = m.upsertErrs[1:]
		if err != nil {
			return MemoryItem{}, err
		}
	}
	if m.upsertErr != nil {
		return MemoryItem{}, m.upsertErr
	}
	if m.upsertItem.ID != "" {
		return m.upsertItem, nil
	}

	return item, nil
}

func (m *recordingMemoryManager) CurrentSession(ctx context.Context) (string, error) {
	if m.currentSessionErr != nil {
		return "", m.currentSessionErr
	}

	return m.fakeMemoryManager.CurrentSession(ctx)
}

type fakeReflectionGenerator struct {
	requests []ReflectionGenerationRequest
	result   ReflectionGenerationResult
	err      error
}

func (g *fakeReflectionGenerator) GenerateReflectionCandidates(
	_ context.Context,
	req ReflectionGenerationRequest,
) (ReflectionGenerationResult, error) {
	g.requests = append(g.requests, req)
	if g.err != nil {
		return ReflectionGenerationResult{}, g.err
	}
	return g.result, nil
}
