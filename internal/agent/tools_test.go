package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/mocks"
	models "github.com/wandxy/hand/internal/model"
	handtools "github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
	agenttool "github.com/wandxy/hand/pkg/agent/tool"
)

func TestToolRegistry_ResolveUsesEnvironmentPolicyAndConvertsDefinitions(t *testing.T) {
	stub := &mocks.ToolRegistryStub{
		Definitions: handtools.Definitions{{
			Name:         "memory_extract",
			Description:  "Extract memory",
			InputSchema:  map[string]any{"type": "object"},
			ParallelSafe: true,
			Groups:       []string{"core"},
			Requires:     handtools.Capabilities{Memory: true},
			Platforms:    []string{"darwin"},
		}},
	}
	env := &mocks.EnvironmentStub{
		ToolRegistry: stub,
		Policy: handtools.Policy{
			GroupNames:   []string{"core"},
			Capabilities: handtools.Capabilities{Memory: true},
			Platform:     "darwin",
		},
	}
	registry := NewToolRegistry(env, nil)

	definitions, err := registry.Resolve(agenttool.Policy{})
	require.NoError(t, err)
	require.Equal(t, []agenttool.Definition{{
		Name:         "memory_extract",
		Description:  "Extract memory",
		InputSchema:  map[string]any{"type": "object"},
		ParallelSafe: true,
		Groups:       []string{"core"},
		Requires:     agenttool.Capabilities{Memory: true},
		Platforms:    []string{"darwin"},
	}}, definitions)
	require.Equal(t, env.Policy, stub.LastToolPolicy)
}

func TestToolRegistry_InvokeDelegatesToHostInvoker(t *testing.T) {
	env := &mocks.EnvironmentStub{}
	var capturedEnv any
	var capturedCall models.ToolCall
	registry := NewToolRegistry(
		env,
		func(_ context.Context, runtimeEnv environment.Environment, toolCall models.ToolCall) handmsg.Message {
			capturedEnv = runtimeEnv
			capturedCall = toolCall
			return handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       toolCall.Name,
				ToolCallID: toolCall.ID,
				Content:    `{"ok":true}`,
			}
		},
	)

	message := registry.Invoke(context.Background(), agenttool.Call{ID: "call-1", Name: "time", Input: "{}"})

	require.Same(t, env, capturedEnv)
	require.Equal(t, models.ToolCall{ID: "call-1", Name: "time", Input: "{}"}, capturedCall)
	require.Equal(t, handmsg.RoleTool, message.Role)
	require.Equal(t, "time", message.Name)
	require.Equal(t, "call-1", message.ToolCallID)
	require.Equal(t, `{"ok":true}`, message.Content)
}
func TestTurn_ExecuteToolCallsParallel_AppendsResultsInModelOrder(t *testing.T) {
	completed := make(chan string, 2)
	secondCompleted := make(chan struct{})
	turn := &Turn{
		invokeToolFn: func(_ context.Context, toolCall models.ToolCall) handmsg.Message {
			switch toolCall.ID {
			case "call-1":
				select {
				case <-secondCompleted:
				case <-time.After(250 * time.Millisecond):
				}
			case "call-2":
				completed <- toolCall.ID
				close(secondCompleted)

				return toolExecutionTestMessage(toolCall, `{"ok":true}`)
			}
			completed <- toolCall.ID

			return toolExecutionTestMessage(toolCall, `{"ok":true}`)
		},
	}

	messages, err := turn.executeToolCalls(
		context.Background(),
		trace.NoopSession(),
		[]models.ToolCall{
			{ID: "call-1", Name: "time", Input: "{}"},
			{ID: "call-2", Name: "time", Input: "{}"},
		},
		[]models.ToolDefinition{{Name: "time", ParallelSafe: true}},
	)

	require.NoError(t, err)
	require.Equal(t, "call-2", <-completed)
	require.Equal(t, "call-1", <-completed)
	require.Equal(t, []string{"call-1", "call-2"}, toolExecutionTestMessageIDs(messages))
}

func TestAssistantToolCallMessageFromResponse_PreservesMultipleToolCalls(t *testing.T) {
	message, err := assistantToolCallMessageFromResponse(&models.Response{
		OutputText: "checking both",
		ToolCalls: []models.ToolCall{
			{ID: "call-1", Name: "time", Input: "{}"},
			{ID: "call-2", Name: "web_search", Input: `{"query":"hand"}`},
		},
	})

	require.NoError(t, err)
	require.Equal(t, handmsg.RoleAssistant, message.Role)
	require.Equal(t, "checking both", message.Content)
	require.Equal(t, []handmsg.ToolCall{
		{ID: "call-1", Name: "time", Input: "{}"},
		{ID: "call-2", Name: "web_search", Input: `{"query":"hand"}`},
	}, message.ToolCalls)
}

func TestTurn_ExecuteToolCallsParallel_PreservesToolErrorPayloads(t *testing.T) {
	turn := &Turn{
		invokeToolFn: func(_ context.Context, toolCall models.ToolCall) handmsg.Message {
			if toolCall.ID == "call-2" {
				return toolExecutionTestMessage(toolCall, `{"error":"blocked"}`)
			}

			return toolExecutionTestMessage(toolCall, `{"ok":true}`)
		},
	}

	messages, err := turn.executeToolCalls(
		context.Background(),
		trace.NoopSession(),
		[]models.ToolCall{
			{ID: "call-1", Name: "time", Input: "{}"},
			{ID: "call-2", Name: "time", Input: "{}"},
		},
		[]models.ToolDefinition{{Name: "time", ParallelSafe: true}},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"call-1", "call-2"}, toolExecutionTestMessageIDs(messages))
	require.Equal(t, `{"error":"blocked"}`, messages[1].Content)
}

func TestTurn_ExecuteToolCalls_RunsAdjacentSafeGroupsInParallel(t *testing.T) {
	completed := make(chan string, 4)
	fourthStarted := make(chan struct{})
	turn := &Turn{
		invokeToolFn: func(_ context.Context, toolCall models.ToolCall) handmsg.Message {
			switch toolCall.ID {
			case "call-3":
				select {
				case <-fourthStarted:
				case <-time.After(250 * time.Millisecond):
				}
			case "call-4":
				close(fourthStarted)
				completed <- toolCall.ID

				return toolExecutionTestMessage(toolCall, `{"ok":true}`)
			}
			completed <- toolCall.ID

			return toolExecutionTestMessage(toolCall, `{"ok":true}`)
		},
	}

	messages, err := turn.executeToolCalls(
		context.Background(),
		trace.NoopSession(),
		[]models.ToolCall{
			{ID: "call-1", Name: "web_search"},
			{ID: "call-2", Name: "memory_add"},
			{ID: "call-3", Name: "web_extract"},
			{ID: "call-4", Name: "time"},
		},
		[]models.ToolDefinition{
			{Name: "web_search", ParallelSafe: true},
			{Name: "web_extract", ParallelSafe: true},
			{Name: "time", ParallelSafe: true},
			{Name: "memory_add"},
		},
	)

	require.NoError(t, err)
	require.Equal(t, "call-1", <-completed)
	require.Equal(t, "call-2", <-completed)
	require.Equal(t, "call-4", <-completed)
	require.Equal(t, "call-3", <-completed)
	require.Equal(t, []string{"call-1", "call-2", "call-3", "call-4"}, toolExecutionTestMessageIDs(messages))
}

func TestTurn_ExecuteToolCallsParallel_CancelsSiblingsOnFatalError(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var once sync.Once
	turn := &Turn{
		invokeToolFn: func(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
			if toolCall.ID == "call-1" {
				once.Do(func() { close(started) })
				<-ctx.Done()
				close(cancelled)
				return toolExecutionTestMessage(toolCall, `{"cancelled":true}`)
			}

			<-started
			return handmsg.Message{
				Role:    handmsg.RoleTool,
				Name:    toolCall.Name,
				Content: `{"invalid":true}`,
			}
		},
	}

	_, err := turn.executeToolCalls(
		context.Background(),
		trace.NoopSession(),
		[]models.ToolCall{
			{ID: "call-1", Name: "time", Input: "{}"},
			{ID: "call-2", Name: "time", Input: "{}"},
		},
		[]models.ToolDefinition{{Name: "time", ParallelSafe: true}},
	)

	require.EqualError(t, err, "tool call id is required")
	require.Eventually(t, func() bool {
		select {
		case <-cancelled:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestTurn_ExecuteToolCalls_UsesSequentialPathForUnsafeTools(t *testing.T) {
	var calls []string
	turn := &Turn{
		invokeToolFn: func(_ context.Context, toolCall models.ToolCall) handmsg.Message {
			calls = append(calls, toolCall.ID)

			return toolExecutionTestMessage(toolCall, `{"ok":true}`)
		},
	}

	messages, err := turn.executeToolCalls(
		context.Background(),
		trace.NoopSession(),
		[]models.ToolCall{
			{ID: "call-1", Name: "memory_add", Input: "{}"},
			{ID: "call-2", Name: "memory_add", Input: "{}"},
		},
		[]models.ToolDefinition{{Name: "memory_add"}},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"call-1", "call-2"}, calls)
	require.Equal(t, []string{"call-1", "call-2"}, toolExecutionTestMessageIDs(messages))
}

func TestTurn_ExecuteToolCalls_ReturnsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	turn := &Turn{
		invokeToolFn: func(_ context.Context, toolCall models.ToolCall) handmsg.Message {
			return toolExecutionTestMessage(toolCall, `{"ok":true}`)
		},
	}

	_, err := turn.executeToolCalls(
		ctx,
		trace.NoopSession(),
		[]models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
		[]models.ToolDefinition{{Name: "time", ParallelSafe: true}},
	)

	require.ErrorIs(t, err, context.Canceled)
}

func toolExecutionTestMessage(toolCall models.ToolCall, content string) handmsg.Message {
	return handmsg.Message{
		Role:       handmsg.RoleTool,
		Name:       toolCall.Name,
		ToolCallID: toolCall.ID,
		Content:    content,
	}
}

func toolExecutionTestMessageIDs(messages []handmsg.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ToolCallID)
	}

	return ids
}

func toolExecutionTestCallIDs(toolCalls []models.ToolCall) []string {
	ids := make([]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		ids = append(ids, toolCall.ID)
	}

	return ids
}
