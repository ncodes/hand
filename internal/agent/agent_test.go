package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
	storagememory "github.com/wandxy/hand/internal/storage/memory"
	storagemock "github.com/wandxy/hand/internal/storage/mock"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

func testSessionConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.StorageBackend = "memory"
	return &cloned
}

func TestAgent_StartInitializesConversationState(t *testing.T) {
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

	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})

	require.NoError(t, agent.Start(context.Background()))
	require.Empty(t, agent.TurnMessages())
}

func TestAgent_StartRejectsNilAgent(t *testing.T) {
	var agent *Agent
	err := agent.Start(context.Background())
	require.EqualError(t, err, "agent is required")
}

func TestAgent_StartRejectsNilConfig(t *testing.T) {
	agent := NewAgent(context.Background(), nil, &mocks.ModelClientStub{})
	err := agent.Start(context.Background())
	require.EqualError(t, err, "config is required")
}

func TestAgent_StartReturnsEnvironmentPrepareError(t *testing.T) {
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		return &mocks.EnvironmentStub{
			PrepareErr:       errors.New("prepare failed"),
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}

	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})

	err := agent.Start(context.Background())
	require.EqualError(t, err, "prepare failed")
}

func TestNewRuntimeEnvironmentReturnsEnvironment(t *testing.T) {
	env := newEnvironment(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}))
	require.NotNil(t, env)
}

func TestAgent_StartUsesProvidedContext(t *testing.T) {
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})

	type contextKey string
	const key contextKey = "request_id"

	var captured context.Context
	newEnvironment = func(ctx context.Context, _ *config.Config) environment.Environment {
		captured = ctx
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}

	ctx := context.WithValue(context.Background(), key, "start-ctx")
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})

	require.NoError(t, agent.Start(ctx))
	require.Same(t, ctx, captured)
	require.Same(t, ctx, agent.ctx)
}

func TestAgent_StartReturnsEnsureSessionManagerError(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", StorageBackend: "invalid"}, &mocks.ModelClientStub{})
	err := agent.Start(context.Background())
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func TestAgent_StartReturnsManagerStartError(t *testing.T) {
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

	manager, err := sessionstore.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, errors.New("resolve failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	agent := &Agent{
		ctx:         context.Background(),
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent"}),
		modelClient: &mocks.ModelClientStub{},
		sessionMgr:  manager,
	}

	err = agent.Start(context.Background())
	require.EqualError(t, err, "resolve failed")
}

func TestAgent_TurnMessagesReturnsCopy(t *testing.T) {
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{
			{OutputText: "hello back"},
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
	require.NoError(t, err)

	messages := agent.TurnMessages()
	messages[0].Content = "changed"

	require.Equal(t, "hello", agent.TurnMessages()[0].Content)
}

func TestAgent_TurnMessagesReturnsEmptyForNilAgent(t *testing.T) {
	var agent *Agent
	require.Nil(t, agent.TurnMessages())
}

func TestAgent_TurnMessagesReturnsEmptyForUninitializedAgent(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	require.Nil(t, agent.TurnMessages())
}

func TestAgent_AvailableToolDefinitionsReturnNilWithoutEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	definitions, err := agent.availableToolDefinitions()
	require.NoError(t, err)
	require.Nil(t, definitions)
}

func TestAgent_AvailableToolDefinitionsReturnDefinitionsFromEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		InstructionsList: nil,
		Policy: tools.Policy{
			Capabilities: tools.Capabilities{Filesystem: true},
			Platform:     "cli",
		},
		ToolRegistry: &mocks.ToolRegistryStub{
			Definitions: []tools.Definition{{
				Name:        "time",
				Description: "Returns time",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
	}

	definitions, err := agent.availableToolDefinitions()
	require.NoError(t, err)
	require.Equal(t, []models.ToolDefinition{{
		Name:        "time",
		Description: "Returns time",
		InputSchema: map[string]any{"type": "object"},
	}}, definitions)
	require.Equal(t, tools.Policy{
		Capabilities: tools.Capabilities{Filesystem: true},
		Platform:     "cli",
	}, agent.env.(*mocks.EnvironmentStub).ToolRegistry.(*mocks.ToolRegistryStub).LastToolPolicy)
}

func TestAgent_AvailableToolDefinitionsReturnResolveError(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		InstructionsList: nil,
		ToolRegistry: &mocks.ToolRegistryStub{
			ResolveErr: errors.New("resolve failed"),
		},
	}

	definitions, err := agent.availableToolDefinitions()
	require.Nil(t, definitions)
	require.EqualError(t, err, "resolve failed")
}

func TestAgent_InvokeToolIncludesRegistryAndToolErrors(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		InstructionsList: nil,
		ToolRegistry: &mocks.ToolRegistryStub{
			Result: tools.Result{Error: tools.Error{Code: "tool_failed", Message: "tool failed"}.String(), Output: "ignored"},
			Err:    errors.New("invoke failed"),
		},
	}

	message := agent.invokeTool(context.Background(), models.ToolCall{ID: "call-1", Name: "time", Input: "{}"})

	require.Equal(t, handmsg.RoleTool, message.Role)
	require.Equal(t, "time", message.Name)
	require.Equal(t, "call-1", message.ToolCallID)
	require.Contains(t, message.Content, `"error":{"code":"tool_failed","message":"tool failed"}`)
	require.Contains(t, message.Content, `"output":"ignored"`)
}

func TestAgent_InvokeToolPreservesPlainStringErrors(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		InstructionsList: nil,
		ToolRegistry: &mocks.ToolRegistryStub{
			Result: tools.Result{Error: "plain failure"},
		},
	}

	message := agent.invokeTool(context.Background(), models.ToolCall{ID: "call-1", Name: "time", Input: "{}"})

	require.Contains(t, message.Content, `"error":"plain failure"`)
}

func TestAgent_InvokeToolHandlesMarshalError(t *testing.T) {
	originalMarshal := jsonMarshal
	t.Cleanup(func() {
		jsonMarshal = originalMarshal
	})
	jsonMarshal = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}

	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	agent.env = &mocks.EnvironmentStub{
		InstructionsList: nil,
		ToolRegistry: &mocks.ToolRegistryStub{
			Result: tools.Result{Output: "2026-03-23T00:00:00Z"},
		},
	}

	message := agent.invokeTool(context.Background(), models.ToolCall{ID: "call-1", Name: "time", Input: "{}"})

	require.Equal(t, `{"name":"time","error":"marshal failed"}`, message.Content)
}

func TestAgent_InvokeToolReturnsRegistryRequiredWithoutRuntimeEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})
	message := agent.invokeToolWithEnvironment(context.Background(), nil, models.ToolCall{ID: "call-1", Name: "time", Input: "{}"})
	require.Contains(t, message.Content, `"error":"tool registry is required"`)
}

func TestAgent_SessionMethodsRejectUninitializedAgent(t *testing.T) {
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}), &mocks.ModelClientStub{})

	_, err := agent.CreateSession(context.Background(), "")
	require.EqualError(t, err, "environment has not been initialized")

	_, err = agent.ListSessions(context.Background())
	require.EqualError(t, err, "environment has not been initialized")

	err = agent.UseSession(context.Background(), storage.DefaultSessionID)
	require.EqualError(t, err, "environment has not been initialized")

	_, err = agent.CurrentSession(context.Background())
	require.EqualError(t, err, "environment has not been initialized")
}

func TestAgent_SessionMethodsRejectNilAgent(t *testing.T) {
	var agent *Agent

	_, err := agent.CreateSession(context.Background(), "")
	require.EqualError(t, err, "agent is required")

	_, err = agent.ListSessions(context.Background())
	require.EqualError(t, err, "agent is required")

	err = agent.UseSession(context.Background(), storage.DefaultSessionID)
	require.EqualError(t, err, "agent is required")

	_, err = agent.CurrentSession(context.Background())
	require.EqualError(t, err, "agent is required")
}

func TestAgent_SessionLifecycleMethods(t *testing.T) {
	manager, err := sessionstore.NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	agent := &Agent{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent"}),
		modelClient: &mocks.ModelClientStub{},
		sessionMgr:  manager,
		initialized: true,
	}

	created, err := agent.CreateSession(context.Background(), "")
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	sessions, err := agent.ListSessions(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, sessions)

	err = agent.UseSession(context.Background(), created.ID)
	require.NoError(t, err)

	current, err := agent.CurrentSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, created.ID, current)
}

func TestAgent_RespondRejectsMissingManagerWhenInitialized(t *testing.T) {
	agent := &Agent{
		ctx:         context.Background(),
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		modelClient: &mocks.ModelClientStub{},
		env: &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
		initialized: true,
	}

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "environment has not been initialized")
}

func TestAgent_RespondReturnsRecreatedEnvironmentPrepareError(t *testing.T) {
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		return &mocks.EnvironmentStub{
			PrepareErr:       errors.New("prepare failed"),
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}

	manager := mustSessionManager(t)
	agent := &Agent{
		ctx:         context.Background(),
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}),
		modelClient: &mocks.ModelClientStub{},
		sessionMgr:  manager,
		initialized: true,
		env: &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		},
	}

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})
	require.EqualError(t, err, "prepare failed")
}

func TestAgent_RespondUsesProvidedContextForExecutionEnvironment(t *testing.T) {
	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})

	type contextKey string
	const key contextKey = "request_id"

	var captured []context.Context
	newEnvironment = func(ctx context.Context, _ *config.Config) environment.Environment {
		captured = append(captured, ctx)
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
		}
	}

	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "hello back"}}}
	agent := NewAgent(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent", Model: "test-model"}), client)
	startCtx := context.WithValue(context.Background(), key, "start-ctx")
	runCtx := context.WithValue(context.Background(), key, "run-ctx")

	require.NoError(t, agent.Start(startCtx))

	reply, err := agent.Respond(runCtx, "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello back", reply)
	require.Len(t, captured, 2)
	require.Same(t, startCtx, captured[0])
	require.Same(t, runCtx, captured[1])
}

func TestAgent_EnsureSessionManagerRejectsNilAgent(t *testing.T) {
	var agent *Agent
	err := agent.ensureSessionManager()
	require.EqualError(t, err, "agent is required")
}

func TestAgent_EnsureSessionManagerRejectsNilConfig(t *testing.T) {
	agent := &Agent{}
	err := agent.ensureSessionManager()
	require.EqualError(t, err, "config is required")
}

func TestAgent_EnsureSessionManagerReturnsExistingManager(t *testing.T) {
	manager := mustSessionManager(t)
	agent := &Agent{cfg: testSessionConfig(&config.Config{Name: "Test Agent"}), sessionMgr: manager}
	err := agent.ensureSessionManager()
	require.NoError(t, err)
	require.Same(t, manager, agent.sessionMgr)
}

func TestAgent_EnsureSessionManagerReturnsOpenStoreError(t *testing.T) {
	originalOpen := openSessionStore
	t.Cleanup(func() {
		openSessionStore = originalOpen
	})
	openSessionStore = func(*config.Config) (storage.SessionStore, error) {
		return nil, errors.New("open store failed")
	}

	agent := &Agent{cfg: testSessionConfig(&config.Config{Name: "Test Agent"})}
	err := agent.ensureSessionManager()
	require.EqualError(t, err, "open store failed")
}

func TestAgent_EnsureSessionManagerReturnsNewManagerError(t *testing.T) {
	originalOpen := openSessionStore
	originalNewManager := newSessionManager
	t.Cleanup(func() {
		openSessionStore = originalOpen
		newSessionManager = originalNewManager
	})
	openSessionStore = func(*config.Config) (storage.SessionStore, error) {
		return storagememory.NewSessionStore(), nil
	}
	newSessionManager = func(storage.SessionStore, time.Duration, time.Duration) (*sessionstore.Manager, error) {
		return nil, errors.New("new manager failed")
	}

	agent := &Agent{cfg: testSessionConfig(&config.Config{Name: "Test Agent"})}
	err := agent.ensureSessionManager()
	require.EqualError(t, err, "new manager failed")
}

func TestDurationOrDefault(t *testing.T) {
	require.Equal(t, 5*time.Second, durationOrDefault(5*time.Second, time.Second))
	require.Equal(t, time.Second, durationOrDefault(0, time.Second))
}

func mustSessionManager(t *testing.T) *sessionstore.Manager {
	t.Helper()
	manager, err := sessionstore.NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)
	return manager
}

func newSessionOpsAgent(
	t *testing.T,
	cfg *config.Config,
	client models.Client,
	traceSession trace.Session,
) *Agent {
	t.Helper()

	originalFactory := newEnvironment
	originalStore := openSessionStore
	t.Cleanup(func() {
		newEnvironment = originalFactory
		openSessionStore = originalStore
	})

	store := storagememory.NewSessionStore()
	openSessionStore = func(*config.Config) (storage.SessionStore, error) {
		return store, nil
	}
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		return &mocks.EnvironmentStub{
			InstructionsList: nil,
			ToolRegistry:     tools.NewInMemoryRegistry(),
			TraceSession:     traceSession,
		}
	}

	agent := NewAgent(context.Background(), testSessionConfig(cfg), client)
	require.NoError(t, agent.Start(context.Background()))

	return agent
}

func appendUserMessages(t *testing.T, agent *Agent, sessionID string, count int, content string) {
	t.Helper()

	messages := make([]handmsg.Message, 0, count)
	for range count {
		msg, err := handmsg.NewMessage(handmsg.RoleUser, content)
		require.NoError(t, err)
		messages = append(messages, msg)
	}

	require.NoError(t, agent.sessionMgr.AppendMessages(context.Background(), sessionID, messages))
}

func TestAgent_CompactSessionRefreshesSummary(t *testing.T) {
	client := &mocks.ModelClientStub{Responses: []*models.Response{{
		OutputText: `{"session_summary":"Earlier work","current_task":"Investigate compact","discoveries":["d1"],"open_questions":["q1"],"next_actions":["n1"]}`,
	}}}
	agent := newSessionOpsAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		ContextLength: 128000,
	}, client, &mocks.TraceSessionStub{})

	session, err := agent.CreateSession(context.Background(), "ses_Z4VxN3E3h5cQH1sYq2k8a")
	require.NoError(t, err)

	appendUserMessages(t, agent, session.ID, 10, "message")
	require.NoError(t, agent.sessionMgr.UpdateLastPromptTokens(context.Background(), session.ID, 50))

	result, err := agent.CompactSession(context.Background(), session.ID)

	require.NoError(t, err)
	require.Equal(t, session.ID, result.SessionID)
	require.Equal(t, 2, result.SourceEndOffset)
	require.Equal(t, 10, result.SourceMessageCount)
	require.Equal(t, 50, result.CurrentContextLength)
	require.Equal(t, 128000, result.TotalContextLength)

	summary, ok, err := agent.sessionMgr.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Earlier work", summary.SessionSummary)
	require.Equal(t, 1, client.CallCount)
}

func TestAgent_SessionContextStatusUsesStoredPromptTokens(t *testing.T) {
	agent := newSessionOpsAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		ContextLength: 128000,
	}, &mocks.ModelClientStub{}, nil)

	session, err := agent.CreateSession(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
	require.NoError(t, err)
	require.NoError(t, agent.sessionMgr.UpdateLastPromptTokens(context.Background(), session.ID, 64000))

	status, err := agent.SessionContextStatus(context.Background(), session.ID)

	require.NoError(t, err)
	require.Equal(t, session.ID, status.SessionID)
	require.Zero(t, status.Offset)
	require.Zero(t, status.Size)
	require.Equal(t, 128000, status.Length)
	require.Equal(t, 64000, status.Used)
	require.Equal(t, 64000, status.Remaining)
	require.Equal(t, 0.5, status.UsedPct)
	require.Equal(t, 0.5, status.RemainingPct)
}

func TestAgent_CompactSession_validationErrors(t *testing.T) {
	cfg := testSessionConfig(&config.Config{Name: "Test Agent", Model: "m", ContextLength: 128000})
	manager := mustSessionManager(t)

	t.Run("nil_agent", func(t *testing.T) {
		var a *Agent
		_, err := a.CompactSession(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
		require.EqualError(t, err, "agent is required")
	})

	t.Run("nil_config", func(t *testing.T) {
		a := &Agent{modelClient: &mocks.ModelClientStub{}, sessionMgr: manager, initialized: true}
		_, err := a.CompactSession(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
		require.EqualError(t, err, "config is required")
	})

	t.Run("uninitialized", func(t *testing.T) {
		a := NewAgent(context.Background(), cfg, &mocks.ModelClientStub{})
		_, err := a.CompactSession(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
		require.EqualError(t, err, "environment has not been initialized")
	})

	t.Run("nil_model_client", func(t *testing.T) {
		a := &Agent{cfg: cfg, sessionMgr: manager, initialized: true}
		_, err := a.CompactSession(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
		require.EqualError(t, err, "model client is required")
	})
}

func TestAgent_CompactSession_returnsResolveError(t *testing.T) {
	manager, err := sessionstore.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, errors.New("store get failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	a := &Agent{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent", Model: "m", ContextLength: 128000}),
		modelClient: &mocks.ModelClientStub{},
		sessionMgr:  manager,
		initialized: true,
	}

	_, err = a.CompactSession(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
	require.EqualError(t, err, "store get failed")
}

func TestAgent_CompactSession_returnsMemoryErrorWhenHistoryTooShort(t *testing.T) {
	a := newSessionOpsAgent(t, &config.Config{
		Name:          "Test Agent",
		Model:         "test-model",
		ContextLength: 128000,
	}, &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "{}"}}}, nil)

	session, err := a.CreateSession(context.Background(), "")
	require.NoError(t, err)

	appendUserMessages(t, a, session.ID, 8, "x")

	_, err = a.CompactSession(context.Background(), session.ID)
	require.EqualError(t, err, "session history is too short to compact")
}

func TestAgent_CompactSession_withNilEnvironmentUsesNoopTrace(t *testing.T) {
	manager, err := sessionstore.NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	client := &mocks.ModelClientStub{Responses: []*models.Response{{
		OutputText: `{"session_summary":"noop trace","current_task":"t","discoveries":[],"open_questions":[],"next_actions":[]}`,
	}}}

	a := &Agent{
		cfg: testSessionConfig(&config.Config{
			Name:          "Test Agent",
			Model:         "test-model",
			ContextLength: 128000,
		}),
		modelClient: client,
		sessionMgr:  manager,
		initialized: true,
		env:         nil,
	}

	session, err := a.CreateSession(context.Background(), "")
	require.NoError(t, err)

	appendUserMessages(t, a, session.ID, 10, "message")
	require.NoError(t, a.sessionMgr.UpdateLastPromptTokens(context.Background(), session.ID, 50))

	result, err := a.CompactSession(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, result.SessionID)
	require.Equal(t, 1, client.CallCount)
}

func TestAgent_SessionContextStatus_validationErrors(t *testing.T) {
	cfg := testSessionConfig(&config.Config{Name: "Test Agent", ContextLength: 128000})
	manager := mustSessionManager(t)

	t.Run("nil_agent", func(t *testing.T) {
		var a *Agent
		_, err := a.SessionContextStatus(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
		require.EqualError(t, err, "agent is required")
	})

	t.Run("nil_config", func(t *testing.T) {
		a := &Agent{sessionMgr: manager, initialized: true}
		_, err := a.SessionContextStatus(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
		require.EqualError(t, err, "config is required")
	})

	t.Run("uninitialized", func(t *testing.T) {
		a := NewAgent(context.Background(), cfg, &mocks.ModelClientStub{})
		_, err := a.SessionContextStatus(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
		require.EqualError(t, err, "environment has not been initialized")
	})
}

func TestAgent_SessionContextStatus_returnsResolveError(t *testing.T) {
	manager, err := sessionstore.NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, errors.New("session not found")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	a := &Agent{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent", ContextLength: 128000}),
		sessionMgr:  manager,
		initialized: true,
		modelClient: &mocks.ModelClientStub{},
	}

	_, err = a.SessionContextStatus(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
	require.EqualError(t, err, "session not found")
}

func TestAgent_SessionContextStatus_returnsSummaryError(t *testing.T) {
	manager, err := sessionstore.NewManager(&storagemock.SessionStore{
		GetFunc: func(_ context.Context, id string) (storage.Session, bool, error) {
			return storage.Session{ID: id}, true, nil
		},
		GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{}, false, errors.New("summary load failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	a := &Agent{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent", ContextLength: 128000}),
		sessionMgr:  manager,
		initialized: true,
		modelClient: &mocks.ModelClientStub{},
	}

	_, err = a.SessionContextStatus(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
	require.EqualError(t, err, "summary load failed")
}

func TestAgent_SessionContextStatus_zeroTotalSkipsPercentages(t *testing.T) {
	manager, err := sessionstore.NewManager(&storagemock.SessionStore{
		GetFunc: func(_ context.Context, id string) (storage.Session, bool, error) {
			return storage.Session{ID: id, LastPromptTokens: 0}, true, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	a := &Agent{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent", ContextLength: 0}),
		sessionMgr:  manager,
		initialized: true,
		modelClient: &mocks.ModelClientStub{},
	}

	st, err := a.SessionContextStatus(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
	require.NoError(t, err)
	require.Zero(t, st.Offset)
	require.Zero(t, st.Size)
	require.Zero(t, st.Length)
	require.Zero(t, st.Used)
	require.Zero(t, st.Remaining)
	require.Zero(t, st.UsedPct)
	require.Zero(t, st.RemainingPct)
}

func TestAgent_SessionContextStatus_clampsNegativeTotalsAndUsed(t *testing.T) {
	manager, err := sessionstore.NewManager(&storagemock.SessionStore{
		GetFunc: func(_ context.Context, id string) (storage.Session, bool, error) {
			return storage.Session{ID: id, LastPromptTokens: -100}, true, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	a := &Agent{
		cfg: testSessionConfig(&config.Config{
			Name:          "Test Agent",
			ContextLength: -500,
		}),
		sessionMgr:  manager,
		initialized: true,
		modelClient: &mocks.ModelClientStub{},
	}

	st, err := a.SessionContextStatus(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
	require.NoError(t, err)
	require.Zero(t, st.Offset)
	require.Zero(t, st.Size)
	require.Zero(t, st.Length)
	require.Zero(t, st.Used)
	require.Zero(t, st.Remaining)
}

func TestAgent_GetSessionReturnsConcreteValues(t *testing.T) {
	created := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC)
	manager, err := sessionstore.NewManager(&storagemock.SessionStore{
		GetFunc: func(_ context.Context, id string) (storage.Session, bool, error) {
			return storage.Session{
				ID:               id,
				LastPromptTokens: 100,
				CreatedAt:        created,
				UpdatedAt:        updated,
				Compaction: storage.SessionCompaction{
					Status: storage.CompactionStatusFailed,
				},
			}, true, nil
		},
		GetSummaryFunc: func(_ context.Context, id string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{
				SessionID:          id,
				SourceEndOffset:    4,
				SourceMessageCount: 16,
			}, true, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	a := &Agent{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent", ContextLength: 400}),
		sessionMgr:  manager,
		initialized: true,
		modelClient: &mocks.ModelClientStub{},
	}

	got, err := a.GetSession(context.Background(), "ses_N8wM2fL7p9rT4vXc1q6b3")
	require.NoError(t, err)
	require.Equal(t, "ses_N8wM2fL7p9rT4vXc1q6b3", got.SessionID)
	require.Equal(t, 4, got.Offset)
	require.Equal(t, 16, got.Size)
	require.Equal(t, 400, got.Length)
	require.Equal(t, 100, got.Used)
	require.Equal(t, 300, got.Remaining)
	require.Equal(t, 0.25, got.UsedPct)
	require.Equal(t, 0.75, got.RemainingPct)
	require.True(t, created.Equal(got.CreatedAt))
	require.True(t, updated.Equal(got.UpdatedAt))
	require.Equal(t, string(storage.CompactionStatusFailed), got.CompactionStatus)
}
