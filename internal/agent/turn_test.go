package agent

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent/memory"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	storagememory "github.com/wandxy/hand/internal/storage/memory"
	storagemock "github.com/wandxy/hand/internal/storage/mock"
	storagesqlite "github.com/wandxy/hand/internal/storage/sqlite"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestTurn_LoadLoadsPersistedHistoryWithoutHydratingRuntimeContext(t *testing.T) {
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "previous reply", CreatedAt: time.Now().UTC()},
	}))

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, session.ID, turn.sessionID)
	require.Empty(t, turn.emittedMessages)
	require.Len(t, turn.sessionHistory, 1)
	require.Equal(t, "previous reply", turn.sessionHistory[0].Content)
}

func TestTurn_LoadHydratesPlanUsingFilteredToolQueries(t *testing.T) {
	var capturedGetOpts []storage.MessageQueryOptions

	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			capturedGetOpts = append(capturedGetOpts, opts)
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				return []handmsg.Message{{
					Role:    handmsg.RoleTool,
					Name:    "plan_tool",
					Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`,
				}}, nil
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{
		InstructionsList: nil,
		ToolRegistry:     tools.NewInMemoryRegistry(),
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Len(t, capturedGetOpts, 2)
	require.Equal(t, storage.MessageQueryOptions{Offset: 0}, capturedGetOpts[0])
	require.Equal(t, storage.MessageQueryOptions{Role: handmsg.RoleTool, Name: "plan_tool", Order: "desc", Limit: 1}, capturedGetOpts[1])
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "Implement feature", Status: envtypes.PlanStatusInProgress}},
		Explanation: "current plan",
	}, env.Plan)
}

func TestTurn_LoadRejectsNilExecutionEnvironment(t *testing.T) {
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent"}),
		&mocks.ModelClientStub{},
		nil,
		mustNewSessionManager(t),
		nil,
		nil,
	)

	err := turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "runtime environment is required")
}

func TestTurn_LoadRejectsNilTurn(t *testing.T) {
	var turn *Turn
	err := turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "agent is required")
}

func TestTurn_LoadRejectsMissingManager(t *testing.T) {
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		nil,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err := turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "session manager is required")
}

func TestTurn_LoadRejectsMissingConfig(t *testing.T) {
	turn := &Turn{
		modelClient:    &mocks.ModelClientStub{},
		sessionManager: mustNewSessionManager(t),
		env: &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	}

	err := turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "config is required")
}

func TestTurn_LoadRejectsMissingModelClient(t *testing.T) {
	turn := &Turn{
		cfg:            testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		sessionManager: mustNewSessionManager(t),
		env: &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	}

	err := turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "model client is required")
}

func TestTurn_LoadReturnsResolveError(t *testing.T) {
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, errors.New("resolve failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "resolve failed")
}

func TestTurn_LoadReturnsGetMessagesError(t *testing.T) {
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, errors.New("get messages failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "get messages failed")
}

func TestTurn_LoadReturnsGetSummaryError(t *testing.T) {
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{}, false, errors.New("get summary failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "get summary failed")
}

func TestTurn_RunReturnsLoadError(t *testing.T) {
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
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
			return errors.New("append failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
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
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
			cancel()
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	).Run(ctx, "hello", RespondOptions{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestTurn_RunReturnsPromptTokenPersistenceError(t *testing.T) {
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
			return nil
		},
		SaveFunc: func(context.Context, storage.Session) error {
			return errors.New("save failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{Responses: []*models.Response{{
			OutputText:   "reply",
			PromptTokens: 42,
		}}},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
			TraceSession:     &mocks.TraceSessionStub{},
		},
	)

	_, err = turn.Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "save failed")
}

func TestTurn_RunReturnsAppendSessionErrorAfterAssistantResponse(t *testing.T) {
	appendCalls := 0
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
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
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	).Run(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "append assistant failed")
}

func TestTurn_RunStreamsDeltasImmediatelyWhenNoToolsAreAvailable(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "reply"}},
		Deltas: [][]models.StreamDelta{{
			{Channel: models.StreamChannelAssistant, Text: "re"},
			{Channel: models.StreamChannelAssistant, Text: "ply"},
		}},
	})

	var events []Event
	reply, err := turn.Run(context.Background(), "hello", RespondOptions{
		Stream: new(true),
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})

	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Equal(t, []Event{
		{Channel: "assistant", Text: "re"},
		{Channel: "assistant", Text: "ply"},
	}, events)
}

func TestTurn_RunStreamIgnoresEmptyDeltas(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "reply"}},
		Deltas: [][]models.StreamDelta{{
			{Channel: models.StreamChannelAssistant, Text: ""},
			{Channel: models.StreamChannelAssistant, Text: "reply"},
		}},
	})

	var events []Event
	reply, err := turn.Run(context.Background(), "hello", RespondOptions{
		Stream: new(true),
		OnEvent: func(event Event) {
			events = append(events, event)
		},
	})

	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Equal(t, []Event{{Channel: "assistant", Text: "reply"}}, events)
}

func TestTurn_RunSupportsStreamingWithoutEventCallback(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "reply"}},
		Deltas: [][]models.StreamDelta{{
			{Channel: models.StreamChannelAssistant, Text: "re"},
			{Channel: models.StreamChannelAssistant, Text: "ply"},
		}},
	})

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{
		Stream: new(true),
	})

	require.NoError(t, err)
	require.Equal(t, "reply", reply)
}

func TestTurn_RunUsesNonStreamingCompletionWhenDisabled(t *testing.T) {
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "reply"}},
		Deltas: [][]models.StreamDelta{{
			{Channel: models.StreamChannelAssistant, Text: "stream"},
		}},
	})

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{
		Stream: new(false),
	})

	require.NoError(t, err)
	require.Equal(t, "reply", reply)
}

func TestTurn_RunReturnsAppendSessionErrorAfterAssistantToolCall(t *testing.T) {
	appendCalls := 0
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
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
		nil,
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
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
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
		nil,
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
	turn.invokeToolFn = func(context.Context, environment.Environment, models.ToolCall) handmsg.Message {
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
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
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
		nil,
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
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
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
		nil,
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
	turn, _ := newTestTurnHarness(t, instructions.Instructions{{Value: "persona"}}, tools.NewInMemoryRegistry(), client)

	_, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.EqualError(t, err, "iteration limit reached and summary requested more tools")
}

func TestTurn_SummaryFallbackReturnsModelError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{Err: errors.New("summary failed")})
	_, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.EqualError(t, err, "iteration limit reached and summary failed: summary failed")
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
	turn, _ := newTestTurnHarness(t, instructions.Instructions{{Value: "persona"}}, tools.NewInMemoryRegistry(), client)
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
	turn, _ := newTestTurnHarness(t, instructions.Instructions{
		{Value: "persona"},
		{Value: "workspace rules"},
		{Name: requestInstructionName, Value: "be terse"},
	}, tools.NewInMemoryRegistry(), client)

	reply, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Contains(t, client.Requests[0].Instructions, "persona")
	require.Contains(t, client.Requests[0].Instructions, "workspace rules")
	require.Contains(t, client.Requests[0].Instructions, "be terse")
	require.Contains(t, client.Requests[0].Instructions, "Remaining iteration budget: 0.")
}

func TestTurn_SummaryFallbackReturnsPromptTokenPersistenceError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
			return nil
		},
		SaveFunc: func(context.Context, storage.Session) error {
			return errors.New("save failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{Responses: []*models.Response{{
			OutputText:   "summary",
			PromptTokens: 42,
		}}},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
			TraceSession:     traceSession,
		},
	)
	require.NoError(t, turn.load(context.Background(), RespondOptions{}))

	_, err = turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.EqualError(t, err, "save failed")
}

func TestTurn_SummaryFallbackRecordsTraceEvent(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "summary"}}}
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)

	reply, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Equal(t, trace.EvtSummaryFallbackStarted, traceSession.Events[0].Type)
	require.Equal(t, trace.EvtContextPreflight, traceSession.Events[1].Type)

	payload, ok := traceSession.Events[1].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "estimated", payload["source"])
	require.Equal(t, trace.EvtFinalAssistantResponse, traceSession.Events[len(traceSession.Events)-1].Type)
}

func TestTurn_SummaryFallbackSkipsCompactionTraceWhenDisabled(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "summary"}}}
	turn, _ := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)
	turn.cfg = testSessionConfig(&config.Config{
		Name:              "Test Agent",
		Model:             "test-model",
		CompactionEnabled: new(false),
	})

	reply, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Equal(t, trace.EvtSummaryFallbackStarted, traceSession.Events[0].Type)
	require.Equal(t, trace.EvtModelRequest, traceSession.Events[1].Type)

	eventTypes := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.NotContains(t, eventTypes, trace.EvtContextPreflight)
	require.NotContains(t, eventTypes, trace.EvtContextCompactionTriggered)
	require.NotContains(t, eventTypes, trace.EvtContextCompactionWarning)
}

func TestTurn_SummaryFallbackRecordsEstimatedPreflightPayload(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "summary"}}}
	turn, _ := newTestTurnHarness(t, instructions.Instructions{{Value: "persona"}}, tools.NewInMemoryRegistry(), client)
	turn.cfg = testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            1000,
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.8,
	})
	turn.sessionHistory = []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}

	reply, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.NoError(t, err)
	require.Equal(t, "summary", reply)
	require.Equal(t, trace.EvtContextPreflight, traceSession.Events[1].Type)

	payload, ok := traceSession.Events[1].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "estimated", payload["source"])
	require.Equal(t, 1000, payload["context_limit"])
	require.Greater(t, payload["prompt_tokens"].(int), 0)
}

func TestTurn_SummaryFallbackRecordsTriggerAndWarningWhenThresholdExceeded(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "summary"}}}
	turn, _ := newTestTurnHarness(t, instructions.Instructions{{Value: strings.Repeat("a", 80)}}, tools.NewInMemoryRegistry(), client)
	turn.cfg = testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.6,
	})
	turn.sessionHistory = []handmsg.Message{{Role: handmsg.RoleUser, Content: strings.Repeat("b", 300)}}

	reply, err := turn.summaryFallback(context.Background(), environment.NewIterationBudget(0), traceSession)
	require.NoError(t, err)
	require.Equal(t, "summary", reply)

	eventTypes := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.Contains(t, eventTypes, trace.EvtContextPreflight)
	require.Contains(t, eventTypes, trace.EvtContextCompactionTriggered)
	require.Contains(t, eventTypes, trace.EvtContextCompactionWarning)
}

func TestTurn_RecordPostflightUsageReturnsNilForMissingResponseData(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	turn := &Turn{}

	require.NoError(t, turn.recordPostflightUsage(traceSession, nil))
	require.NoError(t, turn.recordPostflightUsage(traceSession, &models.Response{}))
	require.Empty(t, traceSession.Events)
}

func TestTurn_TurnMessagesReturnsCopy(t *testing.T) {
	turn := &Turn{
		emittedMessages: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello"},
		},
	}

	messages := turn.Messages()
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}, messages)

	messages[0].Content = "changed"
	require.Equal(t, "hello", turn.emittedMessages[0].Content)
}

func TestTurn_TurnMessagesReturnsNilWhenEmpty(t *testing.T) {
	turn := &Turn{}
	require.Nil(t, turn.Messages())
}

func TestTurn_RequestMessagesReturnsSessionHistoryThenEmittedMessages(t *testing.T) {
	turn := &Turn{
		sessionHistory:  []handmsg.Message{{Role: handmsg.RoleUser, Content: "before"}},
		emittedMessages: []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "after"}},
	}

	messages := turn.Context()

	require.Equal(t, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "before"},
		{Role: handmsg.RoleAssistant, Content: "after"},
	}, messages)
}

func TestTurn_RequestMessagesDefaultsBuilderWhenUnset(t *testing.T) {
	turn := &Turn{
		sessionHistory: []handmsg.Message{{Role: handmsg.RoleAssistant, ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}}},
	}
	messages := turn.Context()
	messages[0].ToolCalls[0].Name = "changed"
	require.Equal(t, "time", turn.sessionHistory[0].ToolCalls[0].Name)
}

func TestTurn_RequestMessagesIncludesPersistedSummaryBeforeUnsummarizedHistory(t *testing.T) {
	turn := &Turn{
		memory: &memory.Memory{
			Summary: &memory.SummaryState{
				SessionID:          storage.DefaultSessionID,
				SourceEndOffset:    1,
				SourceMessageCount: 3,
				SessionSummary:     "Older context",
				CurrentTask:        "Fix tests",
			},
		},
		sessionHistory: []handmsg.Message{
			{Role: handmsg.RoleAssistant, Content: "recent-1"},
			{Role: handmsg.RoleUser, Content: "recent-2"},
		},
		emittedMessages: []handmsg.Message{
			{Role: handmsg.RoleAssistant, Content: "new"},
		},
	}

	messages := turn.Context()

	require.Len(t, messages, 3)
	require.Equal(t, "recent-1", messages[0].Content)
	require.Equal(t, "recent-2", messages[1].Content)
	require.Equal(t, "new", messages[2].Content)
	require.Contains(t, turn.buildRequestInstructions(), "Session Summary:\nOlder context")
}

func TestTurn_RequestInstructions_HandlesNilTurnAndAppendsExtra(t *testing.T) {
	var turn *Turn
	require.Equal(t, "", turn.buildRequestInstructions())

	turn = &Turn{
		instructions: instructions.New("base"),
		memory:       &memory.Memory{},
	}
	require.Equal(t, "base\nextra", turn.buildRequestInstructions(instructions.New("extra")))
}

func TestTurn_RequestInstructions_IncludeActivePlanOnly(t *testing.T) {
	turn := &Turn{
		sessionID: "session-1",
		env: &mocks.EnvironmentStub{
			Plan: envtypes.Plan{
				Steps: []envtypes.PlanStep{
					{ID: "step-1", Content: "Implement feature", Status: envtypes.PlanStatusInProgress},
					{ID: "step-2", Content: "Write tests", Status: envtypes.PlanStatusPending},
					{ID: "step-3", Content: "Done", Status: envtypes.PlanStatusCompleted},
				},
				Explanation: "current focus",
			},
		},
		instructions: instructions.New("base"),
	}

	rendered := turn.buildRequestInstructions()
	require.Contains(t, rendered, "Active Plan:")
	require.Contains(t, rendered, "- [in_progress] Implement feature")
	require.Contains(t, rendered, "- [pending] Write tests")
	require.Contains(t, rendered, "Plan Update Reason:\ncurrent focus")
	require.NotContains(t, rendered, "Done")
}

func TestTurn_TrimSessionHistoryToSummary_TrimsRelativeToLoadedOffset(t *testing.T) {
	turn := &Turn{
		memory: &memory.Memory{
			Summary: &memory.SummaryState{SourceEndOffset: 5},
		},
		sessionHistory: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "m3"},
			{Role: handmsg.RoleUser, Content: "m4"},
			{Role: handmsg.RoleUser, Content: "m5"},
			{Role: handmsg.RoleUser, Content: "m6"},
		},
		sessionHistoryOffset: 2,
	}

	turn.trimSessionHistoryToSummary()

	require.Equal(t, 5, turn.sessionHistoryOffset)
	require.Equal(t, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "m6"},
	}, turn.sessionHistory)
}

func TestTurn_TrimSessionHistoryToSummary_NoopsWhenSummaryDoesNotAdvanceOffset(t *testing.T) {
	turn := &Turn{
		memory: &memory.Memory{
			Summary: &memory.SummaryState{SourceEndOffset: 2},
		},
		sessionHistory: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "m2"},
			{Role: handmsg.RoleUser, Content: "m3"},
		},
		sessionHistoryOffset: 2,
	}

	turn.trimSessionHistoryToSummary()

	require.Equal(t, 2, turn.sessionHistoryOffset)
	require.Equal(t, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "m2"},
		{Role: handmsg.RoleUser, Content: "m3"},
	}, turn.sessionHistory)
}

func TestTurn_TrimSessionHistoryToSummary_ClearsHistoryWhenSummaryConsumesAllLoadedMessages(t *testing.T) {
	turn := &Turn{
		memory: &memory.Memory{
			Summary: &memory.SummaryState{SourceEndOffset: 5},
		},
		sessionHistory: []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "m2"},
			{Role: handmsg.RoleUser, Content: "m3"},
			{Role: handmsg.RoleUser, Content: "m4"},
		},
		sessionHistoryOffset: 2,
	}

	turn.trimSessionHistoryToSummary()

	require.Equal(t, 5, turn.sessionHistoryOffset)
	require.Nil(t, turn.sessionHistory)
}

func TestTurn_LoadLoadsPersistedSummary(t *testing.T) {
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.SaveSummary(context.Background(), storage.SessionSummary{
		SessionID:          session.ID,
		SourceEndOffset:    2,
		SourceMessageCount: 10,
		SessionSummary:     "Older work",
		CurrentTask:        "Finish phase 3",
	}))

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.NotNil(t, turn.memory)
	require.NotNil(t, turn.memory.Summary)
	require.Equal(t, "Older work", turn.memory.Summary.SessionSummary)
	require.Equal(t, "Finish phase 3", turn.memory.Summary.CurrentTask)
}

func TestTurn_LoadLoadsOnlyUnsummarizedTailWhenSummaryExists(t *testing.T) {
	var capturedOpts []storage.MessageQueryOptions
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{
				SessionID:       storage.DefaultSessionID,
				SourceEndOffset: 3,
				SessionSummary:  "Older work",
			}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			capturedOpts = append(capturedOpts, opts)
			return []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "tail"}}, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.Len(t, capturedOpts, 2)
	require.Equal(t, 3, capturedOpts[0].Offset)
	require.Equal(t, storage.MessageQueryOptions{Role: handmsg.RoleTool, Name: "plan_tool", Order: "desc", Limit: 1}, capturedOpts[1])
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "tail"}}, turn.sessionHistory)
}

func TestTurn_LoadHydratesLatestValidPlanFromHistory(t *testing.T) {
	manager := mustNewSessionManager(t)
	env := &mocks.EnvironmentStub{
		InstructionsList: instructions.New("base"),
		ToolRegistry:     tools.NewInMemoryRegistry(),
		TraceSession:     &mocks.TraceSessionStub{},
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, []handmsg.Message{
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"old\",\"content\":\"Old\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"old\",\"explanation\":\"old plan\"}"}`},
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`},
	}))

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "Implement feature", Status: envtypes.PlanStatusInProgress}},
		Explanation: "current plan",
	}, env.Plan)
}

func TestTurn_LoadIgnoresMalformedPlanHistory(t *testing.T) {
	manager := mustNewSessionManager(t)
	env := &mocks.EnvironmentStub{
		InstructionsList: instructions.New("base"),
		ToolRegistry:     tools.NewInMemoryRegistry(),
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, []handmsg.Message{
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"not-json"}`},
	}))

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.False(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{}, env.Plan)
}

func TestTurn_LoadHydratesPlanFromHistoryBeforeSummaryOffset(t *testing.T) {
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{
				SessionID:       storage.DefaultSessionID,
				SourceEndOffset: 3,
				SessionSummary:  "Older work",
			}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" && opts.Order == "desc" {
				return []handmsg.Message{
					{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`},
				}, nil
			}
			all := []handmsg.Message{
				{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`},
				{Role: handmsg.RoleAssistant, Content: "older-1"},
				{Role: handmsg.RoleAssistant, Content: "older-2"},
				{Role: handmsg.RoleAssistant, Content: "tail"},
			}
			if opts.Offset > 0 {
				return all[opts.Offset:], nil
			}
			return all, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{
		InstructionsList: instructions.New("base"),
		ToolRegistry:     tools.NewInMemoryRegistry(),
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "Implement feature", Status: envtypes.PlanStatusInProgress}},
		Explanation: "current plan",
	}, env.Plan)
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "tail"}}, turn.sessionHistory)
}

func TestTurn_LoadReturnsHydratePlanLookupError(t *testing.T) {
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				return nil, errors.New("hydrate lookup failed")
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()},
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "hydrate lookup failed")
}

func TestTurn_RunRecordsHydratedPlanTrace(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}}
	manager, err := session.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" && opts.Order == "desc" {
				return []handmsg.Message{{
					Role:    handmsg.RoleTool,
					Name:    "plan_tool",
					Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Do first\",\"status\":\"pending\"},{\"id\":\"step-2\",\"content\":\"Do now\",\"status\":\"in_progress\"},{\"id\":\"step-3\",\"content\":\"Done\",\"status\":\"completed\"},{\"id\":\"step-4\",\"content\":\"Skip\",\"status\":\"cancelled\"}],\"summary\":{\"total\":4,\"pending\":1,\"in_progress\":1,\"completed\":1,\"cancelled\":1},\"active_step_id\":\"step-2\",\"explanation\":\"hydrate\"}"}`,
				}}, nil
			}
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		client,
		nil,
		manager,
		nil,
		env,
	)

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)

	var found bool
	for _, event := range traceSession.Events {
		if event.Type != trace.EvtPlanHydrated {
			continue
		}
		found = true
		payload, ok := event.Payload.(map[string]any)
		require.True(t, ok)
		require.Equal(t, storage.DefaultSessionID, payload["session_id"])
		require.Equal(t, "step-2", payload["active_step_id"])
		require.Equal(t, "hydrate", payload["explanation"])
		summary, ok := payload["summary"].(envtypes.PlanSummary)
		require.True(t, ok)
		require.Equal(t, envtypes.PlanSummary{
			Total:      4,
			Pending:    1,
			InProgress: 1,
			Completed:  1,
			Cancelled:  1,
		}, summary)
	}
	require.True(t, found)
}

func TestTurn_HydratePlanFromMessagesHandlesNilTurn(t *testing.T) {
	var turn *Turn
	require.False(t, turn.hydratePlanFromMessages([]handmsg.Message{{Role: handmsg.RoleTool, Name: "plan_tool"}}))
}

func TestTurn_HydratePlanFromMessagesHydratesEmptyPlanWhenNoValidToolMessageExists(t *testing.T) {
	env := &mocks.EnvironmentStub{}
	turn := &Turn{env: env, sessionID: storage.DefaultSessionID}

	ok := turn.hydratePlanFromMessages([]handmsg.Message{{Role: handmsg.RoleAssistant, Content: "hello"}})

	require.False(t, ok)
	require.Equal(t, envtypes.Plan{}, env.Plan)
}

func TestTurn_DecodeHydratedPlanPayloadRejectsInvalidStepsEncoding(t *testing.T) {
	plan, ok := decodeHydratedPlanPayload(`{"steps":"bad"}`)

	require.False(t, ok)
	require.Equal(t, envtypes.Plan{}, plan)
}

func TestTurn_DecodeHydratedPlanPayloadRejectsInvalidPlanState(t *testing.T) {
	plan, ok := decodeHydratedPlanPayload(`{"steps":[{"id":"step-1","content":"Work","status":"pending"}]}`)

	require.False(t, ok)
	require.Equal(t, envtypes.Plan{}, plan)
}

func TestTurn_ActiveHydratedPlanStepIDReturnsEmptyWhenNoActiveStepExists(t *testing.T) {
	require.Empty(t, activeHydratedPlanStepID(envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "Done", Status: envtypes.PlanStatusCompleted}},
	}))
}

func TestTurn_RunGeneratesAndAppliesStructuredSummaryWhenCompactionTriggers(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{
		{OutputText: `{
			"session_summary": "Older work",
			"current_task": "Fix tests",
			"discoveries": ["one"],
			"open_questions": ["two"],
			"next_actions": ["three"]
		}`},
		{OutputText: "reply"},
	}}
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}
	turn.cfg = testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.8,
	})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)

	history := make([]handmsg.Message, 0, 10)
	for range 10 {
		history = append(history, handmsg.Message{
			Role:      handmsg.RoleUser,
			Content:   strings.Repeat("a", 40),
			CreatedAt: time.Now().UTC(),
		})
	}
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, history))

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Len(t, client.Requests, 2)
	require.Nil(t, client.Requests[0].Tools)
	require.Len(t, client.Requests[0].Messages, 3)
	require.Contains(t, client.Requests[1].Instructions, "Session Summary:\nOlder work")
	require.Len(t, client.Requests[1].Messages, 8)

	summary, ok, err := manager.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Older work", summary.SessionSummary)
	require.Equal(t, 3, summary.SourceEndOffset)

	compactionSession, ok, err := manager.Get(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.CompactionStatusSucceeded, compactionSession.Compaction.Status)
	require.Equal(t, 3, compactionSession.Compaction.TargetOffset)
	require.Equal(t, 11, compactionSession.Compaction.TargetMessageCount)
	require.Empty(t, compactionSession.Compaction.LastError)

	persisted, err := manager.GetMessages(context.Background(), session.ID, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, persisted, 12)

	eventTypes := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.Contains(t, eventTypes, trace.EvtSummaryRequested)
	require.Contains(t, eventTypes, trace.EvtSummarySaved)
	require.Contains(t, eventTypes, trace.EvtSummaryApplied)
	require.Contains(t, eventTypes, trace.EvtContextCompactionPending)
	require.Contains(t, eventTypes, trace.EvtContextCompactionRunning)
	require.Contains(t, eventTypes, trace.EvtContextCompactionSucceeded)
}

func TestTurn_RunSkipsSummaryGenerationWhenHistoryIsTooShort(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}}
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}
	turn.cfg = testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 0.1,
		CompactionWarnPercent:    0.2,
	})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	history := make([]handmsg.Message, 0, 7)
	for range 7 {
		history = append(history, handmsg.Message{Role: handmsg.RoleUser, Content: strings.Repeat("a", 40), CreatedAt: time.Now().UTC()})
	}
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, history))

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Len(t, client.Requests, 1)

	_, ok, err := manager.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestTurn_RunContinuesWhenSummaryParsingFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{
		{OutputText: `not-json`},
		{OutputText: "reply"},
	}}
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}
	turn.cfg = testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.8,
	})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	history := make([]handmsg.Message, 0, 10)
	for range 10 {
		history = append(history, handmsg.Message{Role: handmsg.RoleUser, Content: strings.Repeat("a", 40), CreatedAt: time.Now().UTC()})
	}
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, history))

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Len(t, client.Requests, 2)
	require.NotEmpty(t, client.Requests[1].Messages)
	require.NotEqual(t, handmsg.RoleDeveloper, client.Requests[1].Messages[0].Role)

	_, ok, err := manager.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)

	compactionSession, ok, err := manager.Get(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.CompactionStatusSucceeded, compactionSession.Compaction.Status)
	require.Equal(t, 3, compactionSession.Compaction.TargetOffset)
	require.Equal(t, 11, compactionSession.Compaction.TargetMessageCount)
	require.Empty(t, compactionSession.Compaction.LastError)

	eventTypes := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.Contains(t, eventTypes, trace.EvtSummaryParseFailed)
	require.Contains(t, eventTypes, trace.EvtSummarySaved)
	require.Contains(t, eventTypes, trace.EvtContextCompactionPending)
	require.Contains(t, eventTypes, trace.EvtContextCompactionRunning)
	require.Contains(t, eventTypes, trace.EvtContextCompactionSucceeded)
}

func TestTurn_RunSkipsSummaryGenerationWhenCompactionIsDisabled(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}}
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}
	turn.cfg = testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
		CompactionEnabled:        new(false),
		CompactionTriggerPercent: 0.1,
		CompactionWarnPercent:    0.2,
	})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	history := make([]handmsg.Message, 0, 10)
	for i := 0; i < 10; i++ {
		history = append(history, handmsg.Message{Role: handmsg.RoleUser, Content: strings.Repeat("a", 40), CreatedAt: time.Now().UTC()})
	}
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, history))

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Len(t, client.Requests, 1)

	_, ok, err := manager.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.False(t, ok)

	eventTypes := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.NotContains(t, eventTypes, trace.EvtSummaryRequested)
	require.NotContains(t, eventTypes, trace.EvtSummarySaved)
	require.NotContains(t, eventTypes, trace.EvtSummaryFailed)
}

func TestTurn_RunRefreshesSummaryIncrementally(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{
		{OutputText: `{
			"session_summary": "Updated summary",
			"current_task": "Fix tests",
			"discoveries": ["delta"],
			"open_questions": [],
			"next_actions": []
		}`},
		{OutputText: "reply"},
	}}
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}
	turn.cfg = testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.8,
	})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	history := make([]handmsg.Message, 0, 12)
	for range 12 {
		history = append(history, handmsg.Message{Role: handmsg.RoleUser, Content: strings.Repeat("a", 40), CreatedAt: time.Now().UTC()})
	}
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, history))
	require.NoError(t, manager.SaveSummary(context.Background(), storage.SessionSummary{
		SessionID:          session.ID,
		SourceEndOffset:    2,
		SourceMessageCount: 10,
		SessionSummary:     "Older summary",
		CurrentTask:        "Initial task",
	}))

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Len(t, client.Requests, 2)
	require.Len(t, client.Requests[0].Messages, 3)
	require.Contains(t, client.Requests[0].Instructions, "Session Summary:\nOlder summary")

	summary, ok, err := manager.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Updated summary", summary.SessionSummary)
	require.Equal(t, 5, summary.SourceEndOffset)
}

func TestSetInstruction_SkipsBlankUnnamedInstruction(t *testing.T) {
	original := instructions.Instructions{{Value: "base"}}
	updated := setInstruction(original, instructions.Instruction{Value: "   "})
	require.Equal(t, original, updated)
}

func TestSetInstruction_AppendsUnnamedInstruction(t *testing.T) {
	original := instructions.Instructions{{Value: "base"}}
	updated := setInstruction(original, instructions.Instruction{Value: " extra "})
	require.Equal(t, instructions.Instructions{{Value: "base"}, {Value: "extra"}}, updated)
}

func TestSetInstruction_RemovesNamedInstruction(t *testing.T) {
	original := instructions.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "temporary"},
	}
	updated := setInstruction(original, instructions.Instruction{Name: " request.instruct ", Value: "   "})
	require.Equal(t, instructions.Instructions{{Value: "base"}}, updated)
}

func TestSetInstruction_UpdatesNamedInstructionWithoutMutatingInput(t *testing.T) {
	original := instructions.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "temporary"},
	}

	updated := setInstruction(original, instructions.Instruction{Name: " request.instruct ", Value: " updated "})
	require.Equal(t, instructions.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "updated"},
	}, updated)
	require.Equal(t, instructions.Instructions{
		{Value: "base"},
		{Name: "request.instruct", Value: "temporary"},
	}, original)
}

func TestSetInstruction_IgnoresEmptyMissingNamedInstruction(t *testing.T) {
	original := instructions.Instructions{{Value: "base"}}
	updated := setInstruction(original, instructions.Instruction{Name: "request.instruct", Value: "   "})
	require.Equal(t, original, updated)
}

func TestSetInstruction_AppendsMissingNamedInstruction(t *testing.T) {
	original := instructions.Instructions{{Value: "base"}}
	updated := setInstruction(original, instructions.Instruction{Name: " request.instruct ", Value: " temporary "})
	require.Equal(t, instructions.Instructions{
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		return &mocks.EnvironmentStub{
			InstructionsList: instructions.Instructions{{Value: "system prompt"}},
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		return &mocks.EnvironmentStub{
			InstructionsList: instructions.Instructions{
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
	require.Equal(t, instructions.Instructions{
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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

	originalRuntimeFactory := newEnvironment
	originalOpenStore := openSessionStore
	t.Cleanup(func() {
		newEnvironment = originalRuntimeFactory
		openSessionStore = originalOpenStore
	})

	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		registry := tools.NewInMemoryRegistry()
		require.NoError(t, registry.Register(tools.Definition{
			Name:        "time",
			Description: "Returns time",
			Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
				return tools.Result{Output: "2026-03-23T00:00:00Z"}, nil
			}),
		}))
		return &mocks.EnvironmentStub{
			InstructionsList: instructions.Instructions{{Value: "system prompt"}},
			ToolRegistry:     registry,
			IterationBudget:  environment.NewIterationBudget(config.DefaultMaxIterations),
			TraceSession:     &mocks.TraceSessionStub{},
		}
	}

	openSessionStore = func(*config.Config) (storage.SessionStore, error) {
		return storagesqlite.NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	}

	agent := NewAgent(context.Background(), &config.Config{
		Name:           "Test Agent",
		Model:          "test-model",
		StorageBackend: "sqlite",
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
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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

func TestAgent_RespondRecordsTraceEventsOnSuccess(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "hello back"}}}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)

	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
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
	expectedEvents := []string{
		trace.EvtUserMessageAccepted,
		trace.EvtContextPreflight,
		trace.EvtModelRequest,
		trace.EvtModelResponse,
		trace.EvtFinalAssistantResponse,
	}
	actualEvents := []string{traceSession.Events[0].Type, traceSession.Events[1].Type, traceSession.Events[2].Type, traceSession.Events[3].Type, traceSession.Events[4].Type}
	require.Equal(t, expectedEvents, actualEvents)
	payload, ok := traceSession.Events[1].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "estimated", payload["source"])
}

func TestAgent_RespondRecordsTraceFailure(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Err: errors.New("upstream failed")}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
			TraceSession:     traceSession,
		}
	}
	require.NoError(t, agent.Start(context.Background()))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "upstream failed")
	require.Equal(t, trace.EvtSessionFailed, traceSession.Events[len(traceSession.Events)-1].Type)
}

func TestTurn_RunStoresActualPromptTokensForFutureTurns(t *testing.T) {
	client := &mocks.ModelClientStub{Responses: []*models.Response{{
		OutputText:   "hello back",
		PromptTokens: 4321,
	}}}
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), client)

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello back", reply)

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, 4321, session.LastPromptTokens)
}

func TestTurn_RunReusesActualPromptTokensDuringPreflight(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	manager := mustNewSessionManager(t)
	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.UpdateLastPromptTokens(context.Background(), session.ID, 2048))

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		&mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			ToolRegistry: tools.NewInMemoryRegistry(),
			TraceSession: traceSession,
		},
	)

	_, err = turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, trace.EvtContextPreflight, traceSession.Events[1].Type)
	payload, ok := traceSession.Events[1].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 2048, payload["prompt_tokens"])
	require.Equal(t, "actual", payload["source"])
}

func TestTurn_RunUsesEstimatedPromptTokensWhenRequestGrowsPastStoredActual(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	manager := mustNewSessionManager(t)
	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.UpdateLastPromptTokens(context.Background(), session.ID, 50))

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model", ContextLength: 1000}),
		&mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{
			ToolRegistry: tools.NewInMemoryRegistry(),
			TraceSession: traceSession,
		},
	)

	_, err = turn.Run(context.Background(), strings.Repeat("a", 800), RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, trace.EvtContextPreflight, traceSession.Events[1].Type)
	payload, ok := traceSession.Events[1].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "estimated", payload["source"])
	require.Greater(t, payload["prompt_tokens"].(int), 50)
}

func TestTurn_RunRecordsCompactionTriggerAndWarningWithoutMutatingHistory(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	cfg := testSessionConfig(&config.Config{
		Name:                     "Test Agent",
		Model:                    "test-model",
		ContextLength:            100,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 0.5,
		CompactionWarnPercent:    0.6,
	})
	turn, manager := newTestTurnHarness(t, nil, tools.NewInMemoryRegistry(), &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "reply"}},
	})
	turn.cfg = cfg
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}

	message := strings.Repeat("a", 400)
	reply, err := turn.Run(context.Background(), message, RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)

	eventTypes := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.Contains(t, eventTypes, trace.EvtContextCompactionTriggered)
	require.Contains(t, eventTypes, trace.EvtContextCompactionWarning)

	messages, err := manager.GetMessages(context.Background(), turn.sessionID, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, message, messages[0].Content)
	require.Equal(t, "reply", messages[1].Content)
}

func newTestAgent(
	t *testing.T,
	cfg *config.Config,
	client *mocks.ModelClientStub,
	registryFactory func() (tools.Registry, error),
) *Agent {
	t.Helper()

	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		registry, err := registryFactory()
		require.NoError(t, err)
		budget := environment.NewIterationBudget(config.DefaultMaxIterations)
		if cfg != nil && cfg.MaxIterations > 0 {
			budget = environment.NewIterationBudget(cfg.MaxIterations)
		}

		return &mocks.EnvironmentStub{
			InstructionsList: instructions.Instructions{{Value: "system prompt"}},
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
	instructions instructions.Instructions,
	registry environment.ToolRegistry,
	client *mocks.ModelClientStub,
) (*Turn, *session.Manager) {
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
		nil,
		manager,
		nil,
		runtimeEnv,
	)
	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	turn.ctx = context.Background()
	turn.instructions = runtimeEnv.Instructions()
	turn.sessionID = session.ID
	return turn, manager
}

func mustNewSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	manager, err := session.NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)
	return manager
}
