package memory

import "context"

type Logger interface {
	Debug(string, map[string]any)
	Info(string, map[string]any)
	Warn(string, map[string]any)
	Error(string, map[string]any)
}

type Tracer interface {
	Record(context.Context, string, map[string]any)
}

type Observability interface {
	Logger() Logger
	Tracer() Tracer
}

func logDebug(obs Observability, message string, fields map[string]any) {
	if obs == nil || obs.Logger() == nil {
		return
	}
	obs.Logger().Debug(message, fields)
}

func traceRecord(ctx context.Context, obs Observability, event string, fields map[string]any) {
	if obs == nil || obs.Tracer() == nil {
		return
	}
	obs.Tracer().Record(ctx, event, fields)
}

func validateSearch(ctx context.Context, guardrails Guardrails, query SearchQuery) error {
	if guardrails == nil {
		return nil
	}
	return guardrails.ValidateSearch(ctx, query)
}

func validateWrite(ctx context.Context, guardrails Guardrails, item MemoryItem) error {
	if guardrails == nil {
		return nil
	}
	if err := guardrails.ValidateWrite(ctx, item); err != nil {
		return err
	}
	return guardrails.SafetyScan(ctx, item)
}

func validateDelete(ctx context.Context, guardrails Guardrails, req DeleteRequest) error {
	if guardrails == nil {
		return nil
	}
	return guardrails.ValidateDelete(ctx, req)
}

func redactItem(ctx context.Context, guardrails Guardrails, item MemoryItem) (MemoryItem, error) {
	if guardrails == nil {
		return item, nil
	}
	return guardrails.Redact(ctx, item)
}
