package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/pkg/logutils"
)

func TestTurn_LoadTurnContextLoadsPersistedHistoryWithoutHydratingRuntimeContext(t *testing.T) {
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{})

	session, err := manager.ResolveSession(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "previous reply", CreatedAt: time.Now().UTC()},
	}))

	err = turn.loadTurnContext(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, session.ID, turn.sessionID)
	require.Empty(t, turn.emittedMessages)
	require.Len(t, turn.sessionHistory, 1)
	require.Equal(t, "previous reply", turn.sessionHistory[0].Content)
}

func TestTurn_LoadTurnContextRejectsNilExecutionEnvironment(t *testing.T) {
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent"}),
		&mocks.ModelClientStub{},
		mustNewSessionManager(t),
		nil,
		nil,
	)

	err := turn.loadTurnContext(context.Background(), RespondOptions{})
	require.EqualError(t, err, "runtime environment is required")
}

func TestTurn_LoadTurnContextRejectsNilTurn(t *testing.T) {
	var turn *Turn

	err := turn.loadTurnContext(context.Background(), RespondOptions{})
	require.EqualError(t, err, "agent is required")
}

func TestTurn_LoadTurnContextRejectsMissingManager(t *testing.T) {
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err := turn.loadTurnContext(context.Background(), RespondOptions{})
	require.EqualError(t, err, "session manager is required")
}

func TestTurn_LoadTurnContextRejectsMissingConfig(t *testing.T) {
	turn := &Turn{
		modelClient:    &mocks.ModelClientStub{},
		sessionManager: mustNewSessionManager(t),
		runtimeEnv: &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	}

	err := turn.loadTurnContext(context.Background(), RespondOptions{})
	require.EqualError(t, err, "config is required")
}

func TestTurn_LoadTurnContextRejectsMissingModelClient(t *testing.T) {
	turn := &Turn{
		cfg:            testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		sessionManager: mustNewSessionManager(t),
		runtimeEnv: &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	}

	err := turn.loadTurnContext(context.Background(), RespondOptions{})
	require.EqualError(t, err, "model client is required")
}

func TestTurn_LoadTurnContextReturnsResolveError(t *testing.T) {
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{}, false, errors.New("resolve failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent"}),
		&mocks.ModelClientStub{},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err = turn.loadTurnContext(context.Background(), RespondOptions{})
	require.EqualError(t, err, "resolve failed")
}

func TestTurn_LoadTurnContextReturnsGetMessagesError(t *testing.T) {
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, errors.New("get messages failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent"}),
		&mocks.ModelClientStub{},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err = turn.loadTurnContext(context.Background(), RespondOptions{})
	require.EqualError(t, err, "get messages failed")
}

func TestTurn_RunReturnsLoadTurnContextError(t *testing.T) {
	turn := &Turn{}

	_, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "config is required")
}

func TestTurn_RunRejectsEmptyUserMessage(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{})

	_, err := turn.Run(context.Background(), "   ", RespondOptions{})
	require.EqualError(t, err, "message content is required")
}

func TestTurn_RunReturnsAppendSessionErrorAfterUserMessage(t *testing.T) {
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		appendMessagesFn: func(context.Context, string, []handmsg.Message) error {
			return errors.New("append failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	).Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "append failed")
}

func TestTurn_RunReturnsContextErrorAtLoopStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		appendMessagesFn: func(context.Context, string, []handmsg.Message) error {
			cancel()
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	).Run(ctx, "hello", RespondOptions{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestTurn_RunReturnsAppendSessionErrorAfterAssistantResponse(t *testing.T) {
	appendCalls := 0
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		appendMessagesFn: func(context.Context, string, []handmsg.Message) error {
			appendCalls++
			if appendCalls == 2 {
				return errors.New("append assistant failed")
			}
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	).Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "append assistant failed")
}

func TestTurn_RunReturnsAppendSessionErrorAfterAssistantToolCall(t *testing.T) {
	appendCalls := 0
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		appendMessagesFn: func(context.Context, string, []handmsg.Message) error {
			appendCalls++
			if appendCalls == 2 {
				return errors.New("append tool call failed")
			}
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{Responses: []*models.Response{{
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		}}},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	).Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "append tool call failed")
}

func TestTurn_RunReturnsAssistantToolCallNormalizationError(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{{
			RequiresToolCalls: true,
			ToolCalls:         []models.ToolCall{{Name: "time", Input: "{}"}},
		}},
	})

	_, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "tool call id is required")
}

func TestTurn_RunReturnsContextErrorBeforeToolInvocation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	appendCalls := 0
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		appendMessagesFn: func(context.Context, string, []handmsg.Message) error {
			appendCalls++
			if appendCalls == 2 {
				cancel()
			}
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{Responses: []*models.Response{{
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		}}},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     &mocks.ToolRegistryStub{Result: tools.Result{Output: "now"}},
		},
	).Run(ctx, "hello", RespondOptions{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestTurn_RunReturnsToolMessageNormalizationError(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{{
			RequiresToolCalls: true,
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
		}},
	})
	turn.invokeToolFn = func(context.Context, executionEnvironment, models.ToolCall) handmsg.Message {
		return handmsg.Message{
			Role:    handmsg.RoleTool,
			Name:    "time",
			Content: "result",
		}
	}

	_, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "tool call id is required")
}

func TestTurn_RunReturnsAppendSessionErrorAfterToolResult(t *testing.T) {
	appendCalls := 0
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		appendMessagesFn: func(context.Context, string, []handmsg.Message) error {
			appendCalls++
			if appendCalls == 3 {
				return errors.New("append tool result failed")
			}
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{Responses: []*models.Response{{
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		}}},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     &mocks.ToolRegistryStub{Result: tools.Result{Output: "now"}},
		},
	).Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "append tool result failed")
}

func TestTurn_RunReturnsAppendSessionErrorAfterSummaryFallback(t *testing.T) {
	appendCalls := 0
	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{ID: sessionstore.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		getMessagesFn: func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		appendMessagesFn: func(context.Context, string, []handmsg.Message) error {
			appendCalls++
			if appendCalls == 4 {
				return errors.New("append summary failed")
			}
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model", MaxIterations: 1}),
		&mocks.ModelClientStub{Responses: []*models.Response{
			{ToolCalls: []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, RequiresToolCalls: true},
			{OutputText: "summary"},
		}},
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     &mocks.ToolRegistryStub{Result: tools.Result{Output: "now"}},
			IterationBudget:  environment.NewIterationBudget(1),
		},
	).Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "append summary failed")
}

func TestTurn_AvailableToolDefinitionsReturnNilWithoutEnvironment(t *testing.T) {
	turn := &Turn{}
	definitions, err := turn.availableToolDefinitions()
	require.NoError(t, err)
	require.Nil(t, definitions)
}

func TestTurn_AvailableToolDefinitionsReturnResolveError(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, &mocks.ToolRegistryStub{
		ResolveErr: errors.New("resolve failed"),
	}, &mocks.ModelClientStub{})

	definitions, err := turn.availableToolDefinitions()
	require.Nil(t, definitions)
	require.EqualError(t, err, "resolve failed")
}

func TestTurn_InvokeToolReturnsFallbackWithoutInvoker(t *testing.T) {
	turn := &Turn{}

	message := turn.invokeTool(context.Background(), models.ToolCall{ID: "call-1", Name: "time"})
	require.Equal(t, handmsg.RoleTool, message.Role)
	require.Equal(t, "time", message.Name)
	require.Equal(t, "call-1", message.ToolCallID)
	require.Contains(t, message.Content, "tool invocation is required")
}

func TestTurn_SummaryFallbackRejectsToolRequests(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{
		ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
		RequiresToolCalls: true,
	}}}
	turn, _ := newTestTurnHarness(t, instruct.Instructions{{Value: "persona"}}, tools.NewInMemoryRegistry(), client)

	_, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.EqualError(t, err, "iteration limit reached and summary requested more tools")
}

func TestTurn_SummaryFallbackReturnsContextError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := turn.summaryFallback(ctx, environment.NewIterationBudget(0), traceSession)
	require.ErrorIs(t, err, context.Canceled)
}

func TestTurn_SummaryFallbackReturnsAssistantAppendError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "   "}}}
	turn, _ := newTestTurnHarness(t, instruct.Instructions{{Value: "persona"}}, tools.NewInMemoryRegistry(), client)

	_, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.EqualError(t, err, "message content is required")
}

func TestTurn_SummaryFallbackRejectsNilModelResponse(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{nil},
	})

	_, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.EqualError(t, err, "model response is required")
}

func TestTurn_SummaryFallbackUsesExistingInstructions(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "summary"}}}
	turn, _ := newTestTurnHarness(t, instruct.Instructions{
		{Value: "persona"},
		{Name: requestInstructionName, Value: "be terse"},
	}, tools.NewInMemoryRegistry(), client)

	reply, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Contains(t, client.Requests[0].Instructions, "persona")
	require.Contains(t, client.Requests[0].Instructions, "be terse")
	require.Contains(t, client.Requests[0].Instructions, "Remaining iteration budget: 0.")
}

func TestTurn_TurnMessagesReturnsCopy(t *testing.T) {
	turn := &Turn{
		emittedMessages: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello"},
		},
	}

	messages := turn.TurnMessages()
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}, messages)

	messages[0].Content = "changed"
	require.Equal(t, "hello", turn.emittedMessages[0].Content)
}

func TestTurn_TurnMessagesReturnsNilWhenEmpty(t *testing.T) {
	turn := &Turn{}

	require.Nil(t, turn.TurnMessages())
}

func TestSetInstruction_SkipsBlankUnnamedInstruction(t *testing.T) {
	original := instruct.Instructions{{Value: "base"}}

	updated := setInstruction(original, instruct.Instruction{Value: "   "})
	require.Equal(t, original, updated)
}

func TestSetInstruction_AppendsUnnamedInstruction(t *testing.T) {
	original := instruct.Instructions{{Value: "base"}}

	updated := setInstruction(original, instruct.Instruction{Value: " extra "})
	require.Equal(t, instruct.Instructions{{Value: "base"}, {Value: "extra"}}, updated)
}

func TestSetInstruction_RemovesNamedInstruction(t *testing.T) {
	original := instruct.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "temporary"},
	}

	updated := setInstruction(original, instruct.Instruction{Name: " request.instruct ", Value: "   "})
	require.Equal(t, instruct.Instructions{{Value: "base"}}, updated)
}

func TestSetInstruction_UpdatesNamedInstructionWithoutMutatingInput(t *testing.T) {
	original := instruct.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "temporary"},
	}

	updated := setInstruction(original, instruct.Instruction{Name: " request.instruct ", Value: " updated "})
	require.Equal(t, instruct.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "updated"},
	}, updated)
	require.Equal(t, instruct.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "temporary"},
	}, original)
}

func TestSetInstruction_IgnoresEmptyMissingNamedInstruction(t *testing.T) {
	original := instruct.Instructions{{Value: "base"}}

	updated := setInstruction(original, instruct.Instruction{Name: "request.instruct", Value: "   "})
	require.Equal(t, original, updated)
}

func TestSetInstruction_AppendsMissingNamedInstruction(t *testing.T) {
	original := instruct.Instructions{{Value: "base"}}

	updated := setInstruction(original, instruct.Instruction{Name: " request.instruct ", Value: " temporary "})
	require.Equal(t, instruct.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "temporary"},
	}, updated)
}

func TestAgent_RespondAppendsConversationAcrossTurns(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{OutputText: "hello back"},
			{OutputText: "still here"},
		},
	}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model",
		ModelAPIMode: models.APIModeResponses}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: instruct.Instructions{{Value: "system prompt"}},
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Start(context.Background()))

	reply, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello back", reply)

	reply, err = agent.Respond(context.Background(), "again", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "still here", reply)

	require.Len(t, client.Requests, 2)
	require.Equal(t, models.APIModeResponses, client.Requests[0].APIMode)
	require.Equal(t, "system prompt", client.Requests[0].Instructions)
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello",
		CreatedAt: client.Requests[0].Messages[0].CreatedAt}}, client.Requests[0].Messages)

	require.Len(t, client.Requests[1].Messages, 3)
	require.Equal(t, handmsg.RoleUser, client.Requests[1].Messages[0].Role)
	require.Equal(t, "hello", client.Requests[1].Messages[0].Content)
	require.Equal(t, handmsg.RoleAssistant, client.Requests[1].Messages[1].Role)
	require.Equal(t, "hello back", client.Requests[1].Messages[1].Content)
	require.Equal(t, handmsg.RoleUser, client.Requests[1].Messages[2].Role)
	require.Equal(t, "again", client.Requests[1].Messages[2].Content)

	messages := agent.TurnMessages()
	require.Len(t, messages, 2)
	require.Equal(t, "again", messages[0].Content)
	require.Equal(t, "still here", messages[1].Content)
}

func TestAgent_RespondAppendsRequestInstructLast(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "hello back"}},
	}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: instruct.Instructions{
				{Value: "base"},
				{Name: "config.instruct", Value: "configured temporary"},
			},
			ToolRegistry: tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Start(context.Background()))

	reply, err := agent.Respond(context.Background(), "hello", RespondOptions{Instruct: "request temporary"})

	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
	require.Equal(t, "base\nconfigured temporary\nrequest temporary", client.Requests[0].Instructions)
	require.Equal(t, instruct.Instructions{
		{Value: "base"},
		{Name: "config.instruct", Value: "configured temporary"},
	}, agent.env.Instructions())
}

func TestAgent_RespondDoesNotAppendAssistantWhenModelFails(t *testing.T) {
	client := &mocks.ModelClientStub{
		Err: errors.New("upstream failed"),
	}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{
		Name:         "Test Agent",
		Model:        "test-model",
		ModelAPIMode: models.APIModeResponses,
	}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "upstream failed")

	messages := agent.TurnMessages()
	require.Len(t, messages, 1)
	require.Equal(t, handmsg.RoleUser, messages[0].Role)
	require.Equal(t, "hello", messages[0].Content)
}

func TestAgent_RespondRejectsNilAgent(t *testing.T) {
	var agent *Agent

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "agent is required")
}

func TestAgent_RespondRejectsMissingConfig(t *testing.T) {
	agent := &Agent{
		modelClient: &mocks.ModelClientStub{},
		env: &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	}

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "config is required")
}

func TestAgent_RespondRejectsUninitializedEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), &mocks.ModelClientStub{})

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "environment has not been initialized")
}

func TestAgent_RespondRejectsMissingModelClient(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), nil)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "model client is required")
}

func TestAgent_RespondRejectsMissingToolRegistry(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), &mocks.ModelClientStub{})
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     nil,
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "tool registry is required")
}

func TestAgent_RespondRejectsEmptyMessage(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), &mocks.ModelClientStub{})
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "   ", RespondOptions{})
	require.EqualError(t, err, "message is required")
}

func TestAgent_RespondReturnsContextErrorBeforeAppendingUserMessage(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), &mocks.ModelClientStub{})
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agent.Respond(ctx, "hello", RespondOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, agent.TurnMessages())
}

func TestAgent_RespondUsesBackgroundWhenContextIsNil(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "hello back"}},
	}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	reply, err := agent.Respond(nil, "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
}

func TestAgent_RespondReturnsAssistantAppendErrorForEmptyOutput(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{OutputText: "   "},
		},
	}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}

	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "message content is required")
}

func TestAgent_RespondRejectsNilModelResponse(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{nil},
	}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "model response is required")
}

func TestAgent_RespondRejectsMissingToolCallsWhenRequested(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{
			RequiresToolCalls: true,
		}},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
		return tools.NewInMemoryRegistry(), nil
	})

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "model requested tool execution without tool calls")
}

func TestAgent_RespondExecutesToolAndReturnsFinalAnswer(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{
				ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
				RequiresToolCalls: true,
			},
			{
				OutputText: "The current time is 2026-03-23T00:00:00Z",
			},
		},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
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

	reply, err := agent.Respond(context.Background(), "what time is it?", RespondOptions{})

	require.NoError(t, err)
	require.Equal(t, "The current time is 2026-03-23T00:00:00Z", reply)
	require.Len(t, client.Requests, 2)
	require.Len(t, client.Requests[0].Tools, 1)
	require.Len(t, client.Requests[1].Messages, 3)
	require.Equal(t, handmsg.RoleAssistant, client.Requests[1].Messages[1].Role)
	require.Len(t, client.Requests[1].Messages[1].ToolCalls, 1)
	require.Equal(t, handmsg.RoleTool, client.Requests[1].Messages[2].Role)
	require.Contains(t, client.Requests[1].Messages[2].Content, `"output":"2026-03-23T00:00:00Z"`)
}

func TestAgent_RespondExecutesMultipleSequentialToolCalls(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{ToolCalls: []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, RequiresToolCalls: true},
			{ToolCalls: []models.ToolCall{{ID: "call-2", Name: "time", Input: "{}"}}, RequiresToolCalls: true},
			{OutputText: "done"},
		},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
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

	reply, err := agent.Respond(context.Background(), "loop", RespondOptions{})

	require.NoError(t, err)
	require.Equal(t, "done", reply)
	require.Len(t, client.Requests, 3)
	require.Len(t, agent.TurnMessages(), 6)
}

func TestAgent_RespondConvertsMissingToolIntoToolMessage(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{ToolCalls: []models.ToolCall{{ID: "call-1", Name: "missing", Input: "{}"}}, RequiresToolCalls: true},
			{OutputText: "fallback"},
		},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
		return tools.NewInMemoryRegistry(), nil
	})

	reply, err := agent.Respond(context.Background(), "use a missing tool", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "fallback", reply)
	require.Len(t, client.Requests, 2)
	logutils.PrettyPrint(client.Requests)
	require.Contains(t, client.Requests[1].Messages[2].Content, `tool_not_registered`)
	require.Contains(t, client.Requests[1].Messages[2].Content, `tool is not registered`)
}

func TestAgent_RespondPreservesAssistantToolCallsAcrossSQLiteBackedTurns(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{ToolCalls: []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, RequiresToolCalls: true},
			{OutputText: "first reply"},
			{OutputText: "second reply"},
		},
	}

	originalRuntimeFactory := newRuntimeEnvironment
	originalOpenStore := openSessionStore
	t.Cleanup(func() {
		newRuntimeEnvironment = originalRuntimeFactory
		openSessionStore = originalOpenStore
	})

	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		registry := tools.NewInMemoryRegistry()
		require.NoError(t, registry.Register(tools.Definition{
			Name:        "time",
			Description: "Returns time",
			Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
				return tools.Result{Output: "2026-03-23T00:00:00Z"}, nil
			}),
		}))
		return &mocks.EnvironmentStub{
			InstructionsList: instruct.Instructions{{Value: "system prompt"}},
			ToolRegistry:     registry,
			IterationBudget:  environment.NewIterationBudget(config.DefaultMaxIterations),
			TraceSession:     &mocks.TraceSessionStub{},
		}
	}

	openSessionStore = func(*config.Config) (storage.SessionStore, error) {
		return sessionstore.NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	}

	agent := NewAgent(context.Background(), &config.Config{
		Name:           "Test Agent",
		Model:          "test-model",
		SessionBackend: "sqlite",
	}, client)
	require.NoError(t, agent.Start(context.Background()))

	reply, err := agent.Respond(context.Background(), "what time is it?", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "first reply", reply)

	reply, err = agent.Respond(context.Background(), "and again?", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "second reply", reply)
	require.Len(t, client.Requests, 3)
	require.Len(t, client.Requests[2].Messages, 5)
	require.Equal(t, handmsg.RoleAssistant, client.Requests[2].Messages[1].Role)
	require.Len(t, client.Requests[2].Messages[1].ToolCalls, 1)
	require.Equal(t, "call-1", client.Requests[2].Messages[1].ToolCalls[0].ID)
}

func TestAgent_RespondUsesSummaryFallbackWhenIterationBudgetIsExhausted(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{
				ToolCalls:         []models.ToolCall{{ID: "call", Name: "time", Input: "{}"}},
				RequiresToolCalls: true,
			},
			{
				OutputText: "summary",
			},
		},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		MaxIterations: 1,
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
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

	reply, err := agent.Respond(context.Background(), "loop forever", RespondOptions{})

	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Len(t, client.Requests, 2)
	require.Nil(t, client.Requests[1].Tools)
	require.Contains(t, client.Requests[1].Instructions, "The maximum number of tool-calling iterations has been reached.")
	require.Contains(t, client.Requests[1].Instructions, "Remaining iteration budget: 0.")
}

func TestAgent_RespondReturnsSummaryFailureWhenFallbackCallFails(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{
				ToolCalls:         []models.ToolCall{{ID: "call", Name: "time", Input: "{}"}},
				RequiresToolCalls: true,
			},
		},
		Errors: []error{nil, errors.New("summary failed")},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		MaxIterations: 1,
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
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

	_, err := agent.Respond(context.Background(), "loop forever", RespondOptions{})

	require.EqualError(t, err, "iteration limit reached and summary failed: summary failed")
}

func TestAgent_RespondRejectsSummaryFallbackThatRequestsMoreTools(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{
				ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
				RequiresToolCalls: true,
			},
			{
				ToolCalls:         []models.ToolCall{{ID: "call-2", Name: "time", Input: "{}"}},
				RequiresToolCalls: true,
			},
		},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		MaxIterations: 1,
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
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

	_, err := agent.Respond(context.Background(), "loop forever", RespondOptions{})

	require.EqualError(t, err, "iteration limit reached and summary requested more tools")
}

func TestAgent_RespondReturnsContextErrorBeforeToolInvocation(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		}},
	}
	agent := newTestAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		MaxIterations: 1,
		DebugRequests: false,
	}, client, func() (tools.Registry, error) {
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agent.Respond(ctx, "loop forever", RespondOptions{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestAgent_RespondReturnsResolveError(t *testing.T) {
	client := &mocks.ModelClientStub{}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry: &mocks.ToolRegistryStub{
				ResolveErr: errors.New("resolve failed"),
			},
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})

	require.EqualError(t, err, "resolve failed")
	require.Empty(t, client.Requests)
}

func TestToContextToolCalls_ReturnsNilWhenEmpty(t *testing.T) {
	require.Nil(t, toContextToolCalls(nil))
}

func TestAgent_SummaryFallbackReturnsContextError(t *testing.T) {
	agent := &Agent{
		cfg:         &config.Config{Name: "Test Agent", Model: "test-model"},
		modelClient: &mocks.ModelClientStub{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agent.summaryFallback(ctx, environment.NewIterationBudget(0), nil, nil, &mocks.TraceSessionStub{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestAgent_SummaryFallbackReturnsAssistantAppendError(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "   "}},
	}
	agent := &Agent{
		cfg:         &config.Config{Name: "Test Agent", Model: "test-model"},
		modelClient: client,
	}

	_, err := agent.summaryFallback(
		context.Background(),
		environment.NewIterationBudget(0),
		instruct.Instructions{{Value: "system prompt"}},
		nil,
		&mocks.TraceSessionStub{},
	)
	require.EqualError(t, err, "message content is required")
}

func TestAgent_SummaryFallbackRejectsNilModelResponse(t *testing.T) {
	agent := &Agent{
		cfg:         &config.Config{Name: "Test Agent", Model: "test-model"},
		modelClient: &mocks.ModelClientStub{Responses: []*models.Response{nil}},
	}

	_, err := agent.summaryFallback(context.Background(), environment.NewIterationBudget(0), nil, nil, &mocks.TraceSessionStub{})
	require.EqualError(t, err, "model response is required")
}

func TestAgent_RespondRecordsTraceEventsOnSuccess(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "hello back"}}}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)

	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
			TraceSession:     traceSession,
		}
	}

	require.NoError(t, agent.Start(context.Background()))

	reply, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello back", reply)

	require.True(t, traceSession.Closed)
	expectedEvents := []string{"user.message.accepted", "model.request", "model.response", "final.assistant.response"}
	actualEvents := []string{traceSession.Events[0].Type, traceSession.Events[1].Type, traceSession.Events[2].Type, traceSession.Events[3].Type}
	require.Equal(t, expectedEvents, actualEvents)
}

func TestAgent_RespondRecordsTraceFailure(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Err: errors.New("upstream failed")}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
			TraceSession:     traceSession,
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "upstream failed")
	require.Equal(t, "session.failed", traceSession.Events[len(traceSession.Events)-1].Type)
}

func TestAgent_SummaryFallbackRecordsTraceEvent(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "summary"}}}
	agent := &Agent{cfg: &config.Config{Name: "Test Agent", Model: "test-model"}, modelClient: client}

	reply, err := agent.summaryFallback(context.Background(), environment.NewIterationBudget(0), nil, nil, traceSession)
	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Equal(t, "summary.fallback.started", traceSession.Events[0].Type)
	require.Equal(t, "final.assistant.response", traceSession.Events[len(traceSession.Events)-1].Type)
}

func TestAgent_SummaryFallback_UsesExistingInstructionsAndInstruct(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "summary"}}}
	agent := &Agent{cfg: &config.Config{Name: "Test Agent", Model: "test-model"}, modelClient: client}
	reply, err := agent.summaryFallback(
		context.Background(),
		environment.NewIterationBudget(0),
		instruct.Instructions{
			{Value: "persona"},
			{Value: "workspace rules"},
			{Name: "request.instruct", Value: "be terse"}},
		nil,
		traceSession,
	)

	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Contains(t, client.Requests[0].Instructions, "persona")
	require.Contains(t, client.Requests[0].Instructions, "workspace rules")
	require.Contains(t, client.Requests[0].Instructions, "be terse")
	require.Contains(t, client.Requests[0].Instructions, "The maximum number of tool-calling iterations has been reached.")
}

func newTestAgent(
	t *testing.T,
	cfg *config.Config,
	client *mocks.ModelClientStub,
	registryFactory func() (tools.Registry, error),
) *Agent {
	t.Helper()

	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
		registry, err := registryFactory()
		require.NoError(t, err)
		budget := environment.NewIterationBudget(config.DefaultMaxIterations)
		if cfg != nil && cfg.MaxIterations > 0 {
			budget = environment.NewIterationBudget(cfg.MaxIterations)
		}
		return &mocks.EnvironmentStub{
			InstructionsList: instruct.Instructions{{Value: "system prompt"}},
			ToolRegistry:     registry,
			IterationBudget:  budget,
			TraceSession:     &mocks.TraceSessionStub{},
		}
	}

	agent := NewAgent(context.Background(), testSessionConfig(cfg), client)
	require.NoError(t, agent.Start(context.Background()))
	return agent
}

func newTestTurnHarness(
	t *testing.T,
	instructions instruct.Instructions,
	registry environment.ToolRegistry,
	client *mocks.ModelClientStub,
) (*Turn, *sessionstore.Manager) {
	t.Helper()

	manager := mustNewSessionManager(t)
	runtimeEnv := &mocks.EnvironmentStub{
		InstructionsList: instructions,
		ToolRegistry:     registry,
		TraceSession:     &mocks.TraceSessionStub{},
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		client,
		manager,
		nil,
		runtimeEnv,
	)
	session, err := manager.ResolveSession(context.Background(), "")
	require.NoError(t, err)
	turn.ctx = context.Background()
	turn.instructions = runtimeEnv.Instructions()
	turn.sessionID = session.ID
	return turn, manager
}

func mustNewSessionManager(t *testing.T) *sessionstore.Manager {
	t.Helper()

	manager, err := sessionstore.NewManager(sessionstore.NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)
	return manager
}
