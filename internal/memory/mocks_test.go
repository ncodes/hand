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
