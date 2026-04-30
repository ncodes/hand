package observability

import (
	"context"
	"strings"

	"github.com/rs/zerolog"

	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/trace"
)

type Observability struct {
	logger       *zerolog.Logger
	traceSession trace.Session
}

func New(logger *zerolog.Logger, traceSession trace.Session) memory.Observability {
	return Observability{logger: logger, traceSession: traceSession}
}

func (o Observability) Logger() memory.Logger {
	if o.logger == nil {
		return nil
	}
	return logger{logger: o.logger}
}

func (o Observability) Tracer() memory.Tracer {
	if o.traceSession == nil {
		return nil
	}
	return tracer{traceSession: o.traceSession}
}

type logger struct {
	logger *zerolog.Logger
}

func (l logger) Debug(message string, fields map[string]any) {
	l.event(l.logger.Debug(), message, fields)
}

func (l logger) Info(message string, fields map[string]any) {
	l.event(l.logger.Info(), message, fields)
}

func (l logger) Warn(message string, fields map[string]any) {
	l.event(l.logger.Warn(), message, fields)
}

func (l logger) Error(message string, fields map[string]any) {
	l.event(l.logger.Error(), message, fields)
}

func (l logger) event(event *zerolog.Event, message string, fields map[string]any) {
	if event == nil {
		return
	}
	if len(fields) > 0 {
		event = event.Fields(fields)
	}
	event.Msg(message)
}

type tracer struct {
	traceSession trace.Session
}

func (t tracer) Record(_ context.Context, event string, fields map[string]any) {
	if t.traceSession == nil || strings.TrimSpace(event) == "" {
		return
	}
	t.traceSession.Record(event, fields)
}
