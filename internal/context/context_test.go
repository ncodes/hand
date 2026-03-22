package context

import (
	gctx "context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
)

func TestNewContext_InitializesDependencies(t *testing.T) {
	baseCtx := gctx.WithValue(gctx.Background(), "requestID", "req-123")
	cfg := &config.Config{Name: "Test Agent"}

	ctx := NewContext(baseCtx, cfg)

	require.Same(t, cfg, ctx.cfg)
	require.Same(t, baseCtx, ctx.ctx)
	require.Empty(t, ctx.instructions)
}

func TestContext_AddInstructionAppendsInstructions(t *testing.T) {
	ctx := NewContext(gctx.Background(), &config.Config{Name: "Test Agent"})

	ctx.AddInstruction(Instruction{Value: "first"})
	ctx.AddInstruction(Instruction{Value: "second"})

	require.Equal(t, Instructions{
		{Value: "first"},
		{Value: "second"},
	}, ctx.GetInstructions())
}
