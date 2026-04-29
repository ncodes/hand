package memory

import "context"

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
