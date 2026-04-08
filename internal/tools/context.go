package tools

import "context"

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

func SessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	sessionID, _ := ctx.Value(sessionIDContextKey{}).(string)
	return sessionID
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
