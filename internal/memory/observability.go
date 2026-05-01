package memory

import (
	"context"
	"maps"
)

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

func observationFields(provider string, operation string, fields map[string]any) map[string]any {
	shared := map[string]any{
		"provider":  provider,
		"operation": operation,
	}
	maps.Copy(shared, fields)
	return shared
}

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
