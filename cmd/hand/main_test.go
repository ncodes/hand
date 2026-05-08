package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	doctorcmd "github.com/wandxy/hand/cmd/doctor"
	upcmd "github.com/wandxy/hand/cmd/up"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestNewCommand_UsesConfigFileValues(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
    baseUrl: `+serverURL+`
log:
  level: error
  noColor: true
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "config-agent", cfg.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "config-key", cfg.Models.Key)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Models.Main.Name)
	require.Equal(t, serverURL, cfg.Models.Main.BaseURL)
	require.Equal(t, "error", cfg.Log.Level)
	require.True(t, cfg.Log.NoColor)
}

func TestNewCommand_UsesEnvOverConfigFile(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_NAME=env-agent
HAND_MODEL=openai/gpt-4o-mini
HAND_MODEL_PROVIDER=openrouter
HAND_MODEL_KEY=env-key
HAND_MODEL_BASE_URL=`+serverURL+`
HAND_LOG_LEVEL=warn
HAND_LOG_NO_COLOR=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
    baseUrl: `+serverURL+`
log:
  level: error
  noColor: true
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--env-file", envPath,
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "env-agent", cfg.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "env-key", cfg.Models.Key)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Models.Main.Name)
	require.Equal(t, serverURL, cfg.Models.Main.BaseURL)
	require.Equal(t, "warn", cfg.Log.Level)
	require.False(t, cfg.Log.NoColor)
}

func TestNewCommand_DefaultsProviderWhenEmpty(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  main:
    name: openai/gpt-4o-mini
log:
  level: info
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.EqualError(t, err, "model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
}

func TestNewCommand_DefaultsBaseURLWhenProviderIsImplicit(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
log:
  level: info
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "https://openrouter.ai/api/v1", cfg.Models.Main.BaseURL)
}

func TestNewCommand_UsesMappedBaseURLWhenProviderSetAndBaseURLUnset(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
log:
  level: info
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"--model.provider", "openrouter",
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "https://openrouter.ai/api/v1", cfg.Models.Main.BaseURL)
}

func TestNewCommand_FlagsOverrideEnvAndConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_NAME=env-agent
HAND_MODEL=openai/gpt-4o-mini
HAND_MODEL_PROVIDER=openrouter
HAND_MODEL_KEY=env-key
HAND_MODEL_BASE_URL=`+serverURL+`
HAND_LOG_LEVEL=warn
HAND_LOG_NO_COLOR=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
    baseUrl: `+serverURL+`
log:
  level: error
  noColor: true
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--env-file", envPath,
		"--config", configPath,
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", serverURL,
		"--rpc.port", nextTestPort(t),
		"--log.level", "debug",
		"--log.no-color=true",
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "flag-agent", cfg.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "flag-key", cfg.Models.Key)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Models.Main.Name)
	require.Equal(t, serverURL, cfg.Models.Main.BaseURL)
	require.Equal(t, "debug", cfg.Log.Level)
	require.True(t, cfg.Log.NoColor)
}

func TestNewCommand_RunsUpCommandExplicitly(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
    baseUrl: `+serverURL+`
log:
  level: info
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"agent",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "config-agent", cfg.Name)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "config-key", cfg.Models.Key)
	require.Equal(t, serverURL, cfg.Models.Main.BaseURL)
}

func TestResolveEnvFilePrefersFlag(t *testing.T) {
	clearEnvKeys(t, "HAND_ENV_FILE")
	resetGlobals(t)
	require.Equal(t, "/tmp/test.env", getEnvFile([]string{"hand", "--env-file", "/tmp/test.env"}))
	require.Equal(t, "/tmp/test2.env", getEnvFile([]string{"hand", "--env-file=/tmp/test2.env"}))
}

func TestResolveEnvFilePrefersEnvVar(t *testing.T) {
	clearEnvKeys(t, "HAND_ENV_FILE")
	resetGlobals(t)
	t.Setenv("HAND_ENV_FILE", "/tmp/from-env.env")
	require.Equal(t, "/tmp/from-env.env", getEnvFile([]string{"hand", "--env-file", "/tmp/ignored.env"}))
}

func TestResolveEnvFileUsesDefaultWhenUnset(t *testing.T) {
	clearEnvKeys(t, "HAND_ENV_FILE")
	resetGlobals(t)
	require.Equal(t, ".env", getEnvFile([]string{"hand"}))
}

func TestNewCommand_RootActionShowsHelp(t *testing.T) {
	clearEnvKeys(t, "HAND_ENV_FILE")
	resetGlobals(t)

	var output bytes.Buffer
	cmd := newCommand()
	cmd.Writer = &output
	cmd.ErrWriter = &output
	err := cmd.Run(context.Background(), []string{"hand"})
	require.NoError(t, err)
	require.Contains(t, output.String(), "EXAMPLES:")
	require.Contains(t, output.String(), "hand up")
	require.Contains(t, output.String(), "hand --config ./config.yaml --trace.enabled up")
	require.Contains(t, output.String(), `hand "summarize the failing tests"`)
	require.Contains(t, output.String(), `hand --session ses_abc123 --instruct "be brief" "continue from the last debugging step"`)
	require.Contains(t, output.String(), "hand trace view")
	require.Contains(t, output.String(), "hand --config ./config.yaml trace view --listen 127.0.0.1:9090")
}

func TestNewCommand_RunsDoctorCommandExplicitly(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_DEBUG_REQUESTS", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"doctor",
	})
	require.NoError(t, err)
}

func TestNewCommand_RootActionTreatsUnknownArgsAsChat(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	originalRootOutput := rootOutput
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
		rootOutput = originalRootOutput
	})

	var output bytes.Buffer
	rootOutput = &output

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"hello",
		"world",
	})
	require.NoError(t, err)
	require.Equal(t, "hello world", stub.ChatInput)
	require.Empty(t, stub.RespondOptions.Instruct)
	require.Empty(t, stub.RespondOptions.SessionID)
	require.True(t, stub.Closed)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewCommand_RootActionForwardsInstruct(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
	})

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--instruct", "be terse",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "be terse", stub.RespondOptions.Instruct)
}

func TestNewCommand_RootActionForwardsSessionID(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
	})

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--session", "project-a",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "project-a", stub.RespondOptions.SessionID)
}

func TestNewCommand_RootActionStreamsOutput(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	originalRootOutput := rootOutput
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
		rootOutput = originalRootOutput
	})

	var output bytes.Buffer
	rootOutput = &output

	stub := &agentstub.AgentServiceStub{Reply: "hello back", Deltas: []string{"hello ", "back"}}
	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model.stream=true",
		"hello",
	})
	require.NoError(t, err)
	require.NotNil(t, stub.RespondOptions.Stream)
	require.True(t, *stub.RespondOptions.Stream)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewCommand_RootActionStylesReasoningOutput(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	originalRootOutput := rootOutput
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
		rootOutput = originalRootOutput
	})

	var output bytes.Buffer
	rootOutput = &output

	stub := &agentstub.AgentServiceStub{
		Reply: "thinking done",
		Events: []rpcclient.Event{
			{Channel: "reasoning", Text: "thinking"},
			{Channel: "assistant", Text: " done"},
		},
	}
	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model.stream=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "\x1b[90mthinking\x1b[0m done\n", output.String())
}

func TestNewCommand_RootActionDoesNotStyleReasoningWhenNoColor(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	originalRootOutput := rootOutput
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
		rootOutput = originalRootOutput
	})

	var output bytes.Buffer
	rootOutput = &output

	stub := &agentstub.AgentServiceStub{
		Reply: "thinking done",
		Events: []rpcclient.Event{
			{Channel: "reasoning", Text: "thinking"},
			{Channel: "assistant", Text: " done"},
		},
	}
	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model.stream=true",
		"--log.no-color=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "thinking done\n", output.String())
}

func TestNewCommand_RootActionDoesNotForwardConfiguredInstruct(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
session:
  instruct: be terse
`), 0o600))

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--config", configPath,
		"hello",
	})
	require.NoError(t, err)
	require.Empty(t, stub.RespondOptions.Instruct)
}

func TestNewCommand_RootActionReturnsRPCError(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	originalNewChatClient := newChatClient
	t.Cleanup(func() {
		newChatClient = originalNewChatClient
	})

	newChatClient = func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return nil, status.Error(codes.Unavailable, "connection refused")
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"hello",
	})
	require.EqualError(t, err, "rpc error: code = Unavailable desc = connection refused")
}

func TestNewCommand_RejectsUnsupportedProvider(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: anthropic
    baseUrl: https://config.example/v1
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.EqualError(t, err, "model provider must be one of: openai, openrouter")
}

func TestNewCommand_UsesDirectClientWhenProviderIsOpenai(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_MODEL_BASE_URL", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: openai/gpt-4o-mini
    provider: openai
log:
  level: info
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openai", cfg.Models.Main.Provider)
	require.Equal(t, "https://api.openai.com/v1", cfg.Models.Main.BaseURL)
}

func TestNewCommand_DatabaseResetDeletesConfiguredSQLiteDatabase(t *testing.T) {
	clearEnvKeys(t, "HAND_HOME", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
storage:
  backend: sqlite
`), 0o600))

	statePath := datadir.StateDBPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))
	for _, path := range getConfiguredDatabasePaths() {
		require.NoError(t, os.WriteFile(path, []byte("database"), 0o600))
	}

	var output bytes.Buffer
	rootOutput = &output
	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--config", configPath,
		"db",
		"reset",
		"--force",
	})
	require.NoError(t, err)

	for _, path := range getConfiguredDatabasePaths() {
		require.NoFileExists(t, path)
	}
	require.Contains(t, output.String(), "Reset database: "+statePath)
}

func TestNewCommand_DatabaseResetRequiresForce(t *testing.T) {
	clearEnvKeys(t, "HAND_HOME", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
storage:
  backend: sqlite
`), 0o600))

	statePath := datadir.StateDBPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))
	require.NoError(t, os.WriteFile(statePath, []byte("database"), 0o600))

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--config", configPath,
		"db",
		"reset",
	})
	require.EqualError(t, err, "database reset requires --force")
	require.FileExists(t, statePath)
}

func TestNewCommand_DatabaseResetRejectsMemoryStorage(t *testing.T) {
	clearEnvKeys(t, "HAND_HOME", "HAND_CONFIG", "HAND_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
storage:
  backend: memory
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--config", configPath,
		"db",
		"reset",
		"--force",
	})
	require.EqualError(t, err, "database reset requires sqlite storage backend")
}

func clearEnvKeys(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		original, ok := os.LookupEnv(key)
		if ok {
			t.Cleanup(func() {
				_ = os.Setenv(key, original)
			})
		} else {
			t.Cleanup(func() {
				_ = os.Unsetenv(key)
			})
		}
		_ = os.Unsetenv(key)
	}
}

func resetGlobals(t *testing.T) {
	t.Helper()
	originalEnvFile := envFile
	originalConfigFile := configFile
	originalRootOutput := rootOutput
	originalDoctorOutput := doctorcmd.SetOutput(io.Discard)
	originalStartupOutput := upcmd.SetOutput(io.Discard)
	originalConfig := config.Get()
	t.Cleanup(func() {
		envFile = originalEnvFile
		configFile = originalConfigFile
		rootOutput = originalRootOutput
		doctorcmd.SetOutput(originalDoctorOutput)
		upcmd.SetOutput(originalStartupOutput)
		config.Set(originalConfig)
	})
	envFile = ".env"
	configFile = "config.yaml"
	rootOutput = io.Discard
	logutils.SetOutput(io.Discard)
	config.Set(nil)
	t.Setenv("HAND_HOME", t.TempDir())
}

func newOpenRouterModelsServer(t *testing.T, model string) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"` + model + `","context_length":128000}]}`))
	}))
	t.Cleanup(server.Close)

	return server.URL
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func nextTestPort(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()
	port := lis.Addr().(*net.TCPAddr).Port
	return strconv.Itoa(port)
}
