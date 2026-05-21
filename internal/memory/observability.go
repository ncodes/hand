package memory

import (
	"context"
	"maps"
)

// Logger is intentionally smaller than zerolog/logrus/etc. Memory code only
// needs leveled structured messages and should not depend on a concrete logger.
type Logger interface {
	Debug(string, map[string]any)
	Info(string, map[string]any)
	Warn(string, map[string]any)
	Error(string, map[string]any)
}

// Tracer records structured events that can be inspected after a run. Logs are
// optimized for humans watching the process; traces are optimized for debugging
// complete timelines.
type Tracer interface {
	Record(context.Context, string, any)
}

// Observability bundles optional log and trace sinks. Nil sinks are valid so
// memory code can run in tests and embedded contexts without instrumentation.
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

func traceRecord(ctx context.Context, obs Observability, event string, payload any) {
	if obs == nil || obs.Tracer() == nil {
		return
	}
	obs.Tracer().Record(ctx, event, payload)
}

// buildObservationFields standardizes the fields present on provider events so log
// streams remain searchable by provider and operation even as individual events
// add their own payload.
func buildObservationFields(provider string, operation string, fields map[string]any) map[string]any {
	shared := map[string]any{
		"provider":  provider,
		"operation": operation,
	}
	maps.Copy(shared, fields)
	return shared
}

// logDebugAndTrace emits the same event through both observability surfaces.
// Keeping these paired makes the stdout sequence line up with trace inspection.
func logDebugAndTrace(
	ctx context.Context,
	obs Observability,
	message string,
	event string,
	fields map[string]any,
) {
	logDebug(obs, message, fields)
	traceRecord(ctx, obs, event, fields)
}
