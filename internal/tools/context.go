package tools

import (
	"context"

	"github.com/wandxy/hand/pkg/agent/runcontext"
)

type TraceRecorder interface {
	Record(string, any)
}

type sessionIDContextKey struct{}
type traceRecorderContextKey struct{}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionIDContextKey{}, sessionID)
}

func WithRunContext(ctx context.Context, runCtx runcontext.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, err := runCtx.Normalize()
	if err != nil {
		return ctx
	}

	ctx = runcontext.WithContext(ctx, runCtx)
	return WithSessionID(ctx, runCtx.StateSessionID())
}

func SessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if runCtx, ok := runcontext.FromContext(ctx); ok {
		return runCtx.StateSessionID()
	}
	sessionID, _ := ctx.Value(sessionIDContextKey{}).(string)
	return sessionID
}

func RunContextFromContext(ctx context.Context) (runcontext.Context, bool) {
	return runcontext.FromContext(ctx)
}

func WithTraceRecorder(ctx context.Context, recorder TraceRecorder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceRecorderContextKey{}, recorder)
}

func TraceRecorderFromContext(ctx context.Context) TraceRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(traceRecorderContextKey{}).(TraceRecorder)
	return recorder
}
