package environment

import (
	gctx "context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/identity"
)

func TestNewEnvironment_InitializesDependencies(t *testing.T) {
	baseCtx := gctx.WithValue(gctx.Background(), "requestID", "req-123")
	cfg := &config.Config{Name: "Test Agent"}

	env := NewEnvironment(baseCtx, cfg)

	require.Same(t, baseCtx, env.ctx)
	require.Same(t, cfg, env.cfg)
	require.NotNil(t, env.hctx)
	require.Empty(t, env.hctx.GetInstructions())
	require.True(t, env.hctx.GetConversation().Empty())
}

func TestEnvironment_PrepareAddsBaseIdentityInstruction(t *testing.T) {
	cfg := &config.Config{Name: "Test Agent"}
	env := NewEnvironment(gctx.Background(), cfg)

	err := env.Prepare()

	require.NoError(t, err)
	require.Len(t, env.hctx.GetInstructions(), 1)
	require.Equal(t, identity.GetBaseIdentity(cfg.Name), env.hctx.GetInstructions()[0])
}

func TestEnvironment_ContextUsesContextState(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent"})
	runtimeContext := env.Context()

	require.NoError(t, runtimeContext.AddUserMessage("hello"))
	require.NoError(t, runtimeContext.AddAssistantMessage("hi"))

	messages := runtimeContext.GetMessages()
	require.Len(t, messages, 2)
	require.Equal(t, context.RoleUser, messages[0].Role)
	require.Equal(t, context.RoleAssistant, messages[1].Role)

	conversation := runtimeContext.GetConversation()
	require.Len(t, conversation.Messages(), 2)
	messages[0].Content = "changed"
	require.Equal(t, "hello", runtimeContext.GetMessages()[0].Content)
}
