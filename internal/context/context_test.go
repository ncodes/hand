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
	require.True(t, ctx.conversation.Empty())
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

func TestContext_MessageAndConversationAccessorsUseConversationState(t *testing.T) {
	ctx := NewContext(gctx.Background(), &config.Config{Name: "Test Agent"})

	require.NoError(t, ctx.AddUserMessage("hello"))
	require.NoError(t, ctx.AddAssistantMessage("hi"))

	messages := ctx.GetMessages()
	require.Len(t, messages, 2)
	require.Equal(t, RoleUser, messages[0].Role)
	require.Equal(t, RoleAssistant, messages[1].Role)

	conversation := ctx.GetConversation()
	require.Len(t, conversation.Messages(), 2)
	messages[0].Content = "changed"
	require.Equal(t, "hello", ctx.GetMessages()[0].Content)
}

func TestContext_AddMessageUsesConversationState(t *testing.T) {
	ctx := NewContext(gctx.Background(), &config.Config{Name: "Test Agent"})

	require.NoError(t, ctx.AddMessage(Message{
		Role:       RoleTool,
		Content:    "tool output",
		ToolCallID: "call-1",
		Name:       "time",
	}))

	messages := ctx.GetMessages()
	require.Len(t, messages, 1)
	require.Equal(t, RoleTool, messages[0].Role)
	require.Equal(t, "tool output", messages[0].Content)
	require.Equal(t, "call-1", messages[0].ToolCallID)
	require.Equal(t, "time", messages[0].Name)
}
