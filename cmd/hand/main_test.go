package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

type chatRunnerStub struct {
	runCalled bool
	chatInput string
	reply     string
	runErr    error
	chatErr   error
}

func (s *chatRunnerStub) Run(context.Context) error {
	s.runCalled = true
	return s.runErr
}

func (s *chatRunnerStub) Chat(_ context.Context, msg string) (string, error) {
	s.chatInput = msg
	return s.reply, s.chatErr
}

func TestNewCommand_UsesConfigFileValues(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
log:
  level: error
  noColor: true
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
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "config-key", cfg.ModelKey)
	require.Equal(t, "config-model", cfg.Model)
	require.Equal(t, "https://config.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "error", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
}

func TestNewCommand_UsesEnvOverConfigFile(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
NAME=env-agent
MODEL=env-model
MODEL_ROUTER=openrouter
MODEL_KEY=env-key
MODEL_BASE_URL=https://env.example/v1
LOG_LEVEL=warn
LOG_NO_COLOR=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
log:
  level: error
  noColor: true
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
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "env-key", cfg.ModelKey)
	require.Equal(t, "env-model", cfg.Model)
	require.Equal(t, "https://env.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
	require.False(t, cfg.LogNoColor)
}

func TestNewCommand_DefaultsRouterWhenEmpty(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
log:
  level: info
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
}

func TestNewCommand_DefaultsBaseURLWhenRouterIsImplicit(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  key: config-key
log:
  level: info
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
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "https://openrouter.ai/api/v1", cfg.ModelBaseURL)
}

func TestNewCommand_UsesMappedBaseURLWhenRouterSetAndBaseURLUnset(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
log:
  level: info
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"--model.router", "openrouter",
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "https://openrouter.ai/api/v1", cfg.ModelBaseURL)
}

func TestNewCommand_FlagsOverrideEnvAndConfig(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(envPath, []byte(`
NAME=env-agent
MODEL=env-model
MODEL_ROUTER=openrouter
MODEL_KEY=env-key
MODEL_BASE_URL=https://env.example/v1
LOG_LEVEL=warn
LOG_NO_COLOR=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
log:
  level: error
  noColor: true
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--env-file", envPath,
		"--config", configPath,
		"--name", "flag-agent",
		"--model", "flag-model",
		"--model.router", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", "https://flag.example/v1",
		"--rpc.port", nextTestPort(t),
		"--log.level", "debug",
		"--log.no-color=true",
		"up",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "flag-agent", cfg.Name)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "flag-key", cfg.ModelKey)
	require.Equal(t, "flag-model", cfg.Model)
	require.Equal(t, "https://flag.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "debug", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
}

func TestNewCommand_RunsUpCommandExplicitly(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
log:
  level: info
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
	require.Equal(t, "config-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "config-key", cfg.ModelKey)
	require.Equal(t, "https://config.example/v1", cfg.ModelBaseURL)
}

func TestResolveEnvFilePrefersFlag(t *testing.T) {
	clearEnvKeys(t, "AGENT_ENV_FILE")
	resetGlobals(t)

	require.Equal(t, "/tmp/test.env", resolveEnvFile([]string{"hand", "--env-file", "/tmp/test.env"}))
	require.Equal(t, "/tmp/test2.env", resolveEnvFile([]string{"hand", "--env-file=/tmp/test2.env"}))
}

func TestResolveEnvFilePrefersEnvVar(t *testing.T) {
	clearEnvKeys(t, "AGENT_ENV_FILE")
	resetGlobals(t)

	t.Setenv("AGENT_ENV_FILE", "/tmp/from-env.env")
	require.Equal(t, "/tmp/from-env.env", resolveEnvFile([]string{"hand", "--env-file", "/tmp/ignored.env"}))
}

func TestResolveEnvFileUsesDefaultWhenUnset(t *testing.T) {
	clearEnvKeys(t, "AGENT_ENV_FILE")
	resetGlobals(t)

	require.Equal(t, ".env", resolveEnvFile([]string{"hand"}))
}

func TestNewCommand_RootActionShowsHelp(t *testing.T) {
	clearEnvKeys(t, "AGENT_ENV_FILE")
	resetGlobals(t)

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{"hand"})
	require.NoError(t, err)
}

func TestNewCommand_RunsDoctorCommandExplicitly(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "flag-model",
		"--model.router", "openrouter",
		"--model.key", "flag-key",
		"doctor",
	})
	require.NoError(t, err)
}

func TestNewCommand_RootActionTreatsUnknownArgsAsChat(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	originalNewChatAgent := newChatAgent
	originalRootOutput := rootOutput
	t.Cleanup(func() {
		newChatAgent = originalNewChatAgent
		rootOutput = originalRootOutput
	})

	var output bytes.Buffer
	rootOutput = &output

	stub := &chatRunnerStub{reply: "hello back"}
	newChatAgent = func(context.Context, *config.Config, models.Client) chatRunner {
		return stub
	}

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "flag-model",
		"--model.router", "none",
		"--model.key", "flag-key",
		"hello",
		"world",
	})
	require.NoError(t, err)
	require.True(t, stub.runCalled)
	require.Equal(t, "hello world", stub.chatInput)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewCommand_RejectsUnsupportedRouter(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: anthropic
  key: config-key
  baseUrl: https://config.example/v1
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.EqualError(t, err, "model router must be one of: none, openrouter")
}

func TestNewCommand_UsesDirectClientWhenRouterIsNone(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: none
  key: config-key
log:
  level: info
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
	require.Equal(t, "none", cfg.ModelRouter)
	require.Equal(t, "", cfg.ModelBaseURL)
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
	originalConfig := config.Get()
	t.Cleanup(func() {
		envFile = originalEnvFile
		configFile = originalConfigFile
		config.Set(originalConfig)
	})
	envFile = ".env"
	configFile = "config.yaml"
	config.Set(nil)
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
