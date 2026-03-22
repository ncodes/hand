package environment

import (
	gctx "context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/identity"
)

func TestNewEnvironment_InitializesDependencies(t *testing.T) {
	baseCtx := gctx.WithValue(gctx.Background(), "requestID", "req-123")
	cfg := &config.Config{Name: "Test Agent"}

	env := NewEnvironment(baseCtx, cfg)

	require.Same(t, baseCtx, env.gctx)
	require.Same(t, cfg, env.cfg)
	require.NotNil(t, env.ctx)
	require.Empty(t, env.ctx.GetInstructions())
}

func TestEnvironment_PrepareAddsBaseIdentityInstruction(t *testing.T) {
	cfg := &config.Config{Name: "Test Agent"}
	env := NewEnvironment(gctx.Background(), cfg)

	err := env.Prepare()

	require.NoError(t, err)
	require.Len(t, env.ctx.GetInstructions(), 1)
	require.Equal(t, identity.GetBaseIdentity(cfg.Name), env.ctx.GetInstructions()[0])
}
