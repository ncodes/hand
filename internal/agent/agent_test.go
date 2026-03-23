package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	handcontext "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/tools"
)

func TestAgent_RunInitializesConversationState(t *testing.T) {
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}

	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &mocks.ModelClientStub{})

	require.NoError(t, agent.Run(context.Background()))
	require.True(t, agent.Conversation().Empty())
}

func TestAgent_RunRejectsNilAgent(t *testing.T) {
	var agent *Agent

	err := agent.Run(context.Background())
	require.EqualError(t, err, "agent is required")
}

func TestAgent_RunRejectsNilConfig(t *testing.T) {
	agent := NewAgent(context.Background(), nil, &mocks.ModelClientStub{})

	err := agent.Run(context.Background())
	require.EqualError(t, err, "config is required")
}

func TestAgent_RunReturnsEnvironmentPrepareError(t *testing.T) {
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			PrepareErr:     errors.New("prepare failed"),
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}

	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &mocks.ModelClientStub{})

	err := agent.Run(context.Background())
	require.EqualError(t, err, "prepare failed")
}

func TestAgent_ChatAppendsConversationAcrossTurns(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{
			{OutputText: "hello back"},
			{OutputText: "still here"},
		},
	}
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		runtimeContext := handcontext.NewContext(context.Background(), &config.Config{})
		runtimeContext.AddInstruction(handcontext.Instruction{Value: "system prompt"})
		return &mocks.EnvironmentStub{
			RuntimeContext: runtimeContext,
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Run(context.Background()))

	reply, err := agent.Chat(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "hello back", reply)

	reply, err = agent.Chat(context.Background(), "again")
	require.NoError(t, err)
	require.Equal(t, "still here", reply)

	require.Len(t, client.Requests, 2)
	require.Equal(t, "system prompt", client.Requests[0].Instructions)
	require.Equal(t, []handcontext.Message{{Role: handcontext.RoleUser, Content: "hello", CreatedAt: client.Requests[0].Messages[0].CreatedAt}}, client.Requests[0].Messages)

	require.Len(t, client.Requests[1].Messages, 3)
	require.Equal(t, handcontext.RoleUser, client.Requests[1].Messages[0].Role)
	require.Equal(t, "hello", client.Requests[1].Messages[0].Content)
	require.Equal(t, handcontext.RoleAssistant, client.Requests[1].Messages[1].Role)
	require.Equal(t, "hello back", client.Requests[1].Messages[1].Content)
	require.Equal(t, handcontext.RoleUser, client.Requests[1].Messages[2].Role)
	require.Equal(t, "again", client.Requests[1].Messages[2].Content)

	conversation := agent.Conversation()
	require.Len(t, conversation.Messages(), 4)
	require.Equal(t, "still here", conversation.Messages()[3].Content)
}

func TestAgent_ChatDoesNotAppendAssistantWhenModelFails(t *testing.T) {
	client := &mocks.ModelClientStub{
		Err: errors.New("upstream failed"),
	}
	agent := NewAgent(context.Background(), &config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "upstream failed")

	conversation := agent.Conversation()
	require.Len(t, conversation.Messages(), 1)
	require.Equal(t, handcontext.RoleUser, conversation.Messages()[0].Role)
	require.Equal(t, "hello", conversation.Messages()[0].Content)
}

func TestAgent_ChatRejectsNilAgent(t *testing.T) {
	var agent *Agent

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "agent is required")
}

func TestAgent_ChatRejectsUninitializedEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, &mocks.ModelClientStub{})

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "environment has not been initialized")
}

func TestAgent_ChatRejectsMissingModelClient(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, nil)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "model client is required")
}

func TestAgent_ChatRejectsMissingToolRegistry(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, &mocks.ModelClientStub{})
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   nil,
		}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "tool registry is required")
}

func TestAgent_ChatRejectsEmptyMessage(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, &mocks.ModelClientStub{})
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "   ")
	require.EqualError(t, err, "message is required")
}

func TestAgent_ChatReturnsUserAppendError(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, &mocks.ModelClientStub{})
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: &mocks.ContextStub{AddUserMessageErr: errors.New("append user failed")},
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "append user failed")
}

func TestAgent_ConversationReturnsCopy(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{
			{OutputText: "hello back"},
		},
	}
	agent := NewAgent(context.Background(), &config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Run(context.Background()))
	_, err := agent.Chat(context.Background(), "hello")
	require.NoError(t, err)

	conversation := agent.Conversation()
	messages := conversation.Messages()
	messages[0].Content = "changed"

	require.Equal(t, "hello", agent.Conversation().Messages()[0].Content)
}

func TestAgent_ConversationReturnsEmptyForNilAgent(t *testing.T) {
	var agent *Agent

	conversation := agent.Conversation()
	require.True(t, conversation.Empty())
}

func TestAgent_ChatReturnsAssistantAppendErrorForEmptyOutput(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{
			{OutputText: "   "},
		},
	}
	agent := NewAgent(context.Background(), &config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &mocks.EnvironmentStub{
			RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
			ToolRegistry:   tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "message content is required")
}

func TestAgent_ChatReturnsAssistantAppendError(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{
			{OutputText: "hello back"},
		},
	}
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		ctx := &mocks.ContextStub{
			Instructions:       handcontext.Instructions{{Value: "system prompt"}},
			AddAssistantMsgErr: errors.New("append assistant failed"),
			Conversation:       handcontext.NewConversation(),
		}
		return &mocks.EnvironmentStub{RuntimeContext: ctx, ToolRegistry: tools.NewInMemoryRegistry()}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "append assistant failed")
}

func TestAgent_ChatRejectsMissingToolCallsWhenRequested(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{{
			RequiresToolCalls: true,
		}},
	}
	agent := newTestAgent(t, client, func() (tools.Registry, error) {
		return tools.NewInMemoryRegistry(), nil
	})

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "model requested tool execution without tool calls")
}

func TestAgent_ChatReturnsAssistantToolCallAppendError(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{{
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		}},
	}
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		ctx := &mocks.ContextStub{
			Instructions:  handcontext.Instructions{{Value: "system prompt"}},
			Conversation:  handcontext.NewConversation(),
			AddMessageErr: errors.New("append message failed"),
		}
		return &mocks.EnvironmentStub{RuntimeContext: ctx, ToolRegistry: tools.NewInMemoryRegistry()}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "append message failed")
}

func TestAgent_ChatReturnsToolResultAppendError(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{{
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		}},
	}
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		ctx := &mocks.ContextStub{
			Instructions:        handcontext.Instructions{{Value: "system prompt"}},
			Conversation:        handcontext.NewConversation(),
			AddMessageErr:       errors.New("append tool result failed"),
			AddMessageErrOnCall: 2,
		}
		return &mocks.EnvironmentStub{
			RuntimeContext: ctx,
			ToolRegistry: &mocks.ToolRegistryStub{
				Result: tools.Result{Output: "2026-03-23T00:00:00Z"},
			},
		}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "append tool result failed")
}

func TestAgent_ChatExecutesToolAndReturnsFinalAnswer(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{
			{
				ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
				RequiresToolCalls: true,
			},
			{
				OutputText: "The current time is 2026-03-23T00:00:00Z",
			},
		},
	}
	agent := newTestAgent(t, client, func() (tools.Registry, error) {
		registry := tools.NewInMemoryRegistry()
		require.NoError(t, registry.Register(tools.Definition{
			Name:        "time",
			Description: "Returns time",
			Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
				return tools.Result{Output: "2026-03-23T00:00:00Z"}, nil
			}),
		}))
		return registry, nil
	})

	reply, err := agent.Chat(context.Background(), "what time is it?")

	require.NoError(t, err)
	require.Equal(t, "The current time is 2026-03-23T00:00:00Z", reply)
	require.Len(t, client.Requests, 2)
	require.Len(t, client.Requests[0].Tools, 1)
	require.Len(t, client.Requests[1].Messages, 3)
	require.Equal(t, handcontext.RoleAssistant, client.Requests[1].Messages[1].Role)
	require.Len(t, client.Requests[1].Messages[1].ToolCalls, 1)
	require.Equal(t, handcontext.RoleTool, client.Requests[1].Messages[2].Role)
	require.Contains(t, client.Requests[1].Messages[2].Content, `"output":"2026-03-23T00:00:00Z"`)
}

func TestAgent_ChatExecutesMultipleSequentialToolCalls(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{
			{ToolCalls: []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, RequiresToolCalls: true},
			{ToolCalls: []models.ToolCall{{ID: "call-2", Name: "time", Input: "{}"}}, RequiresToolCalls: true},
			{OutputText: "done"},
		},
	}
	agent := newTestAgent(t, client, func() (tools.Registry, error) {
		registry := tools.NewInMemoryRegistry()
		require.NoError(t, registry.Register(tools.Definition{
			Name:        "time",
			Description: "Returns time",
			Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
				return tools.Result{Output: "2026-03-23T00:00:00Z"}, nil
			}),
		}))
		return registry, nil
	})

	reply, err := agent.Chat(context.Background(), "loop")

	require.NoError(t, err)
	require.Equal(t, "done", reply)
	require.Len(t, client.Requests, 3)
	require.Len(t, agent.Conversation().Messages(), 6)
}

func TestAgent_ChatConvertsMissingToolIntoToolMessage(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.GenerateResponse{
			{ToolCalls: []models.ToolCall{{ID: "call-1", Name: "missing", Input: "{}"}}, RequiresToolCalls: true},
			{OutputText: "fallback"},
		},
	}
	agent := newTestAgent(t, client, func() (tools.Registry, error) {
		return tools.NewInMemoryRegistry(), nil
	})

	reply, err := agent.Chat(context.Background(), "use a missing tool")

	require.NoError(t, err)
	require.Equal(t, "fallback", reply)
	require.Len(t, client.Requests, 2)
	require.Contains(t, client.Requests[1].Messages[2].Content, `"error":"tool is not registered"`)
}

func TestAgent_ChatRejectsIterationOverflow(t *testing.T) {
	responses := make([]*models.GenerateResponse, 0, maxToolIterations)
	for index := 0; index < maxToolIterations; index++ {
		responses = append(responses, &models.GenerateResponse{
			ToolCalls:         []models.ToolCall{{ID: "call", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		})
	}
	client := &mocks.ModelClientStub{Responses: responses}
	agent := newTestAgent(t, client, func() (tools.Registry, error) {
		registry := tools.NewInMemoryRegistry()
		require.NoError(t, registry.Register(tools.Definition{
			Name:        "time",
			Description: "Returns time",
			Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
				return tools.Result{Output: "2026-03-23T00:00:00Z"}, nil
			}),
		}))
		return registry, nil
	})

	_, err := agent.Chat(context.Background(), "loop forever")

	require.EqualError(t, err, "tool loop exceeded 8 iterations")
}

func TestAgent_ConversationReturnsEmptyForUninitializedAgent(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &mocks.ModelClientStub{})

	conversation := agent.Conversation()
	require.True(t, conversation.Empty())
}

func TestAgent_ToolDefinitionsReturnNilWithoutEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &mocks.ModelClientStub{})
	require.Nil(t, agent.toolDefinitions())
}

func TestAgent_ToolDefinitionsReturnDefinitionsFromEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
		ToolRegistry: &mocks.ToolRegistryStub{
			Definitions: []tools.Definition{{
				Name:        "time",
				Description: "Returns time",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
	}

	require.Equal(t, []models.ToolDefinition{{
		Name:        "time",
		Description: "Returns time",
		InputSchema: map[string]any{"type": "object"},
	}}, agent.toolDefinitions())
}

func TestAgent_InvokeToolIncludesRegistryAndToolErrors(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
		ToolRegistry: &mocks.ToolRegistryStub{
			Result: tools.Result{Error: "tool failed", Output: "ignored"},
			Err:    errors.New("invoke failed"),
		},
	}

	message := agent.invokeTool(context.Background(), models.ToolCall{ID: "call-1", Name: "time", Input: "{}"})

	require.Equal(t, handcontext.RoleTool, message.Role)
	require.Equal(t, "time", message.Name)
	require.Equal(t, "call-1", message.ToolCallID)
	require.Contains(t, message.Content, `"error":"tool failed"`)
	require.Contains(t, message.Content, `"output":"ignored"`)
}

func TestAgent_InvokeToolHandlesMarshalError(t *testing.T) {
	originalMarshal := jsonMarshal
	t.Cleanup(func() {
		jsonMarshal = originalMarshal
	})
	jsonMarshal = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}

	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		RuntimeContext: handcontext.NewContext(context.Background(), &config.Config{}),
		ToolRegistry: &mocks.ToolRegistryStub{
			Result: tools.Result{Output: "2026-03-23T00:00:00Z"},
		},
	}

	message := agent.invokeTool(context.Background(), models.ToolCall{ID: "call-1", Name: "time", Input: "{}"})

	require.Equal(t, `{"name":"time","error":"marshal failed"}`, message.Content)
}

func TestToContextToolCallsReturnsNilWhenEmpty(t *testing.T) {
	require.Nil(t, toContextToolCalls(nil))
}

func newTestAgent(t *testing.T, client *mocks.ModelClientStub, registryFactory func() (tools.Registry, error)) *Agent {
	t.Helper()

	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		runtimeContext := handcontext.NewContext(context.Background(), &config.Config{})
		runtimeContext.AddInstruction(handcontext.Instruction{Value: "system prompt"})
		registry, err := registryFactory()
		require.NoError(t, err)
		return &mocks.EnvironmentStub{
			RuntimeContext: runtimeContext,
			ToolRegistry:   registry,
		}
	}

	agent := NewAgent(context.Background(), &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		DebugRequests: false,
	}, client)
	require.NoError(t, agent.Run(context.Background()))
	return agent
}
