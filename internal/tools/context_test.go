package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent/runcontext"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/pkg/nanoid"
)

func TestSessionIDFromContextPrefersEffectiveRunContext(t *testing.T) {
	parentID := nanoid.MustFromSeed(storage.SessionIDPrefix, "parent", "ToolsContextTestSeed")
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "ToolsContextTestSeed")
	parent, err := runcontext.NewParent(parentID)
	require.NoError(t, err)
	child, err := parent.NewChild(runcontext.ChildOptions{
		ChildSessionID: childID,
		RunID:          "run_tools",
	})
	require.NoError(t, err)

	ctx := WithRunContext(WithSessionID(context.Background(), parentID), child)

	require.Equal(t, childID, SessionIDFromContext(ctx))
	runCtx, ok := RunContextFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, parentID, runCtx.Session.PublicID)
	require.Equal(t, childID, runCtx.Session.EffectiveID)
}

func TestSessionIDFromContextFallsBackToLegacySessionID(t *testing.T) {
	ctx := WithSessionID(context.Background(), "session-1")

	require.Equal(t, "session-1", SessionIDFromContext(ctx))
	_, ok := RunContextFromContext(ctx)
	require.False(t, ok)
}

func TestWithSessionID_HandlesNilContext(t *testing.T) {
	ctx := WithSessionID(nil, "session-1")

	require.Equal(t, "session-1", SessionIDFromContext(ctx))
}

func TestWithRunContext_HandlesNilContext(t *testing.T) {
	runCtx, err := runcontext.NewParent(storage.DefaultSessionID)
	require.NoError(t, err)

	ctx := WithRunContext(nil, runCtx)

	require.Equal(t, storage.DefaultSessionID, SessionIDFromContext(ctx))
}

func TestWithRunContext_IgnoresInvalidRunContext(t *testing.T) {
	ctx := WithRunContext(WithSessionID(context.Background(), "legacy"), runcontext.Context{
		Session: runcontext.Session{PublicID: "session-1"},
	})

	require.Equal(t, "legacy", SessionIDFromContext(ctx))
	_, ok := RunContextFromContext(ctx)
	require.False(t, ok)
}

func TestSessionIDFromContext_ReturnsEmptyForNilContext(t *testing.T) {
	require.Empty(t, SessionIDFromContext(nil))
}

func TestTraceRecorderFromContext_ReturnsRecorder(t *testing.T) {
	recorder := &traceRecorderStub{}

	ctx := WithTraceRecorder(context.Background(), recorder)

	require.Same(t, recorder, TraceRecorderFromContext(ctx))
}

func TestWithTraceRecorder_HandlesNilContext(t *testing.T) {
	recorder := &traceRecorderStub{}

	ctx := WithTraceRecorder(nil, recorder)

	require.Same(t, recorder, TraceRecorderFromContext(ctx))
}

func TestTraceRecorderFromContext_ReturnsNilForNilOrMissingRecorder(t *testing.T) {
	require.Nil(t, TraceRecorderFromContext(nil))
	require.Nil(t, TraceRecorderFromContext(context.Background()))
}

type traceRecorderStub struct{}

func (s *traceRecorderStub) Record(string, any) {}
