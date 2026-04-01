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
	"github.com/wandxy/hand/internal/tools"
)

func testSessionConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.SessionBackend = "memory"
	return &cloned
}

func TestAgent_StartInitializesConversationState(t *testing.T) {
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
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
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
	env := newRuntimeEnvironment(context.Background(), testSessionConfig(&config.Config{Name: "Test Agent"}))
	require.NotNil(t, env)
}

func TestAgent_StartUsesProvidedContext(t *testing.T) {
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})

	type contextKey string
	const key contextKey = "request_id"

	var captured context.Context
	newRuntimeEnvironment = func(ctx context.Context, _ *config.Config) executionEnvironment {
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
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", SessionBackend: "invalid"}, &mocks.ModelClientStub{})

	err := agent.Start(context.Background())
	require.EqualError(t, err, "session backend must be one of: memory, sqlite")
}

func TestAgent_StartReturnsManagerStartError(t *testing.T) {
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

	manager, err := sessionstore.NewManager(&sessionStoreStub{
		getFn: func(context.Context, string) (sessionstore.Session, bool, error) {
			return sessionstore.Session{}, false, errors.New("resolve failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	agent := &Agent{
		ctx:         context.Background(),
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent"}),
		modelClient: &mocks.ModelClientStub{},
		manager:     manager,
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

	err = agent.UseSession(context.Background(), sessionstore.DefaultSessionID)
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

	err = agent.UseSession(context.Background(), sessionstore.DefaultSessionID)
	require.EqualError(t, err, "agent is required")

	_, err = agent.CurrentSession(context.Background())
	require.EqualError(t, err, "agent is required")
}

func TestAgent_SessionLifecycleMethods(t *testing.T) {
	manager, err := sessionstore.NewManager(sessionstore.NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	agent := &Agent{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent"}),
		modelClient: &mocks.ModelClientStub{},
		manager:     manager,
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
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) executionEnvironment {
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
		manager:     manager,
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
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})

	type contextKey string
	const key contextKey = "request_id"

	var captured []context.Context
	newRuntimeEnvironment = func(ctx context.Context, _ *config.Config) executionEnvironment {
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
	agent := &Agent{cfg: testSessionConfig(&config.Config{Name: "Test Agent"}), manager: manager}

	err := agent.ensureSessionManager()
	require.NoError(t, err)
	require.Same(t, manager, agent.manager)
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
		return sessionstore.NewStore(), nil
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

func TestAgent_SummaryFallbackReturnsModelError(t *testing.T) {
	agent := &Agent{
		cfg:         &config.Config{Name: "Test Agent", Model: "test-model"},
		modelClient: &mocks.ModelClientStub{Err: errors.New("summary failed")},
	}

	_, err := agent.summaryFallback(context.Background(), environment.NewIterationBudget(0), nil, nil, &mocks.TraceSessionStub{})
	require.EqualError(t, err, "iteration limit reached and summary failed: summary failed")
}

func TestAgent_SummaryFallbackRejectsToolRequests(t *testing.T) {
	agent := &Agent{
		cfg: &config.Config{Name: "Test Agent", Model: "test-model"},
		modelClient: &mocks.ModelClientStub{Responses: []*models.Response{{
			ToolCalls:         []models.ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
			RequiresToolCalls: true,
		}}},
	}

	_, err := agent.summaryFallback(context.Background(), environment.NewIterationBudget(0), nil, nil, &mocks.TraceSessionStub{})
	require.EqualError(t, err, "iteration limit reached and summary requested more tools")
}

func mustSessionManager(t *testing.T) *sessionstore.Manager {
	t.Helper()

	manager, err := sessionstore.NewManager(sessionstore.NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)
	return manager
}

type sessionStoreStub struct {
	saveFn           func(context.Context, sessionstore.Session) error
	getFn            func(context.Context, string) (sessionstore.Session, bool, error)
	listFn           func(context.Context) ([]sessionstore.Session, error)
	deleteFn         func(context.Context, string) error
	setCurrentFn     func(context.Context, string) error
	currentFn        func(context.Context) (string, bool, error)
	appendMessagesFn func(context.Context, string, []handmsg.Message) error
	getMessageFn     func(context.Context, string, int, sessionstore.MessageQueryOptions) (handmsg.Message, bool, error)
	getMessagesFn    func(context.Context, string, sessionstore.MessageQueryOptions) ([]handmsg.Message, error)
	clearMessagesFn  func(context.Context, string, sessionstore.MessageQueryOptions) error
	createArchiveFn  func(context.Context, sessionstore.ArchivedSession) error
	getArchiveFn     func(context.Context, string) (sessionstore.ArchivedSession, bool, error)
	listArchivesFn   func(context.Context, string) ([]sessionstore.ArchivedSession, error)
	deleteArchivesFn func(context.Context, string) error
	deleteExpiredFn  func(context.Context, time.Time) error
}

func (s *sessionStoreStub) Save(ctx context.Context, session sessionstore.Session) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, session)
	}
	return nil
}

func (s *sessionStoreStub) Get(ctx context.Context, id string) (sessionstore.Session, bool, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return sessionstore.Session{}, false, nil
}

func (s *sessionStoreStub) List(ctx context.Context) ([]sessionstore.Session, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func (s *sessionStoreStub) Delete(ctx context.Context, id string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func (s *sessionStoreStub) SetCurrent(ctx context.Context, id string) error {
	if s.setCurrentFn != nil {
		return s.setCurrentFn(ctx, id)
	}
	return nil
}

func (s *sessionStoreStub) Current(ctx context.Context) (string, bool, error) {
	if s.currentFn != nil {
		return s.currentFn(ctx)
	}
	return "", false, nil
}

func (s *sessionStoreStub) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s.appendMessagesFn != nil {
		return s.appendMessagesFn(ctx, id, messages)
	}
	return nil
}

func (s *sessionStoreStub) GetMessage(ctx context.Context, id string, index int, opts sessionstore.MessageQueryOptions) (handmsg.Message, bool, error) {
	if s.getMessageFn != nil {
		return s.getMessageFn(ctx, id, index, opts)
	}
	return handmsg.Message{}, false, nil
}

func (s *sessionStoreStub) GetMessages(ctx context.Context, id string, opts sessionstore.MessageQueryOptions) ([]handmsg.Message, error) {
	if s.getMessagesFn != nil {
		return s.getMessagesFn(ctx, id, opts)
	}
	return nil, nil
}

func (s *sessionStoreStub) ClearMessages(ctx context.Context, id string, opts sessionstore.MessageQueryOptions) error {
	if s.clearMessagesFn != nil {
		return s.clearMessagesFn(ctx, id, opts)
	}
	return nil
}

func (s *sessionStoreStub) CreateArchive(ctx context.Context, archive sessionstore.ArchivedSession) error {
	if s.createArchiveFn != nil {
		return s.createArchiveFn(ctx, archive)
	}
	return nil
}

func (s *sessionStoreStub) GetArchive(ctx context.Context, id string) (sessionstore.ArchivedSession, bool, error) {
	if s.getArchiveFn != nil {
		return s.getArchiveFn(ctx, id)
	}
	return sessionstore.ArchivedSession{}, false, nil
}

func (s *sessionStoreStub) ListArchives(ctx context.Context, sourceSessionID string) ([]sessionstore.ArchivedSession, error) {
	if s.listArchivesFn != nil {
		return s.listArchivesFn(ctx, sourceSessionID)
	}
	return nil, nil
}

func (s *sessionStoreStub) DeleteArchives(ctx context.Context, archiveID string) error {
	if s.deleteArchivesFn != nil {
		return s.deleteArchivesFn(ctx, archiveID)
	}
	return nil
}

func (s *sessionStoreStub) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s.deleteExpiredFn != nil {
		return s.deleteExpiredFn(ctx, now)
	}
	return nil
}
