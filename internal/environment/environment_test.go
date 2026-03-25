package environment

import (
	gctx "context"
	stdctx "context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
	instructionpkg "github.com/wandxy/hand/internal/instruction"
	"github.com/wandxy/hand/internal/tools"
)

func TestNewEnvironment_InitializesDependencies(t *testing.T) {
	baseCtx := gctx.WithValue(gctx.Background(), "requestID", "req-123")
	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}

	env := NewEnvironment(baseCtx, cfg)

	require.Same(t, baseCtx, env.ctx)
	require.Same(t, cfg, env.cfg)
	require.NotNil(t, env.handCtx)
	require.Empty(t, env.handCtx.GetInstructions())
	require.True(t, env.handCtx.GetConversation().Empty())
}

func TestEnvironment_PrepareAddsBaseInstruction(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}
	env := NewEnvironment(gctx.Background(), cfg)

	err := env.Prepare()

	require.NoError(t, err)
	require.Len(t, env.handCtx.GetInstructions(), 1)
	require.Equal(t, instructionpkg.BuildBase(cfg.Name).First(), env.handCtx.GetInstructions()[0])
}

func TestEnvironment_PrepareRegistersNativeTools(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})

	require.NoError(t, env.Prepare())

	tools := env.Tools()
	require.NotNil(t, tools)

	definitions := tools.List()
	require.Len(t, definitions, 1)
	require.Equal(t, "time", definitions[0].Name)
}

type failingRegistry struct {
	err error
}

func (r failingRegistry) Register(tools.Definition) error {
	return r.err
}

func (failingRegistry) Get(string) (tools.Definition, bool) {
	return tools.Definition{}, false
}

func (failingRegistry) List() []tools.Definition {
	return nil
}

func (failingRegistry) Invoke(stdctx.Context, tools.Call) (tools.Result, error) {
	return tools.Result{}, nil
}

func TestEnvironment_PrepareReturnsToolRegistrationError(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
	env.tools = failingRegistry{err: errors.New("register failed")}

	err := env.Prepare()

	require.EqualError(t, err, "register failed")
	require.Empty(t, env.handCtx.GetInstructions())
}

func TestEnvironment_ContextUsesContextState(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
	runtimeContext := env.Context()

	require.NoError(t, runtimeContext.AddUserMessage("hello"))
	require.NoError(t, runtimeContext.AddAssistantMessage("hi"))

	messages := runtimeContext.GetMessages()
	require.Len(t, messages, 2)
	require.Equal(t, handctx.RoleUser, messages[0].Role)
	require.Equal(t, handctx.RoleAssistant, messages[1].Role)

	conversation := runtimeContext.GetConversation()
	require.Len(t, conversation.Messages(), 2)
	messages[0].Content = "changed"
	require.Equal(t, "hello", runtimeContext.GetMessages()[0].Content)
}

func TestEnvironment_NewIterationBudgetUsesConfigValue(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{MaxIterations: 12, DebugTraceDir: dir})
	require.Equal(t, 12, env.NewIterationBudget().Remaining())
}

func TestEnvironment_NewIterationBudgetUsesDefaultWhenUnset(t *testing.T) {
	require.Equal(t, config.DefaultMaxIterations, (&Environment{}).NewIterationBudget().Remaining())
}

func TestNewEnvironment_ConfiguresTraceFactoryWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Model: "gpt-5.1", ModelAPIMode: "responses", DebugTraces: true, DebugTraceDir: dir})

	session := env.NewTraceSession()
	require.NotEmpty(t, session.ID())
	session.Close()
}

func TestNewEnvironment_ReturnsNoopTraceSessionWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})

	session := env.NewTraceSession()
	require.Equal(t, "", session.ID())
}

func TestNewEnvironment_UsesDefaultTraceDirWhenEnabledWithoutConfiguredDir(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraces: true, DebugTraceDir: dir})

	session := env.NewTraceSession()
	require.NotEmpty(t, session.ID())
	session.Close()
}
