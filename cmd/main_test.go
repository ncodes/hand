package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/agent/internal/config"
	"github.com/wandxy/agent/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestCommandUsesConfigFileValues(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
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
	err := cmd.Run(context.Background(), []string{"agent", "--config", configPath})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "config-key", cfg.ModelKey)
	require.Equal(t, "config-model", cfg.Model)
	require.Equal(t, "https://config.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "error", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
}

func TestCommandUsesEnvOverConfigFile(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
MODEL=env-model
MODEL_ROUTER=openrouter
MODEL_KEY=env-key
MODEL_BASE_URL=https://env.example/v1
LOG_LEVEL=warn
LOG_NO_COLOR=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
log:
  level: error
  noColor: true
`), 0o600))

	require.NoError(t, config.PreloadEnvFile(envPath))
	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{"agent", "--config", configPath})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "env-key", cfg.ModelKey)
	require.Equal(t, "env-model", cfg.Model)
	require.Equal(t, "https://env.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
	require.False(t, cfg.LogNoColor)
}

func TestCommandDefaultsRouterWhenEmpty(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  name: config-model
log:
  level: info
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{"agent", "--config", configPath})
	require.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
}

func TestCommandDefaultsBaseURLWhenRouterIsImplicit(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  name: config-model
  key: config-key
log:
  level: info
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{"agent", "--config", configPath})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "https://openrouter.ai/api/v1", cfg.ModelBaseURL)
}

func TestCommandUsesMappedBaseURLWhenRouterSetAndBaseURLUnset(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  name: config-model
  router: openrouter
  key: config-key
log:
  level: info
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{"agent", "--config", configPath, "--model.router", "openrouter"})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "https://openrouter.ai/api/v1", cfg.ModelBaseURL)
}

func TestCommandFlagsOverrideEnvAndConfig(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(envPath, []byte(`
MODEL=env-model
MODEL_ROUTER=openrouter
MODEL_KEY=env-key
MODEL_BASE_URL=https://env.example/v1
LOG_LEVEL=warn
LOG_NO_COLOR=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
log:
  level: error
  noColor: true
`), 0o600))

	require.NoError(t, config.PreloadEnvFile(envPath))
	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"agent",
		"--config", configPath,
		"--model", "flag-model",
		"--model.router", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", "https://flag.example/v1",
		"--log.level", "debug",
		"--log.no-color=true",
	})
	require.NoError(t, err)

	cfg := config.Get()
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "flag-key", cfg.ModelKey)
	require.Equal(t, "flag-model", cfg.Model)
	require.Equal(t, "https://flag.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "debug", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
}

func TestResolveEnvFilePrefersFlag(t *testing.T) {
	clearEnvKeys(t, "AGENT_ENV_FILE")
	resetGlobals(t)

	require.Equal(t, "/tmp/test.env", resolveEnvFile([]string{"agent", "--env-file", "/tmp/test.env"}))
	require.Equal(t, "/tmp/test2.env", resolveEnvFile([]string{"agent", "--env-file=/tmp/test2.env"}))
}

func TestResolveEnvFilePrefersEnvVar(t *testing.T) {
	clearEnvKeys(t, "AGENT_ENV_FILE")
	resetGlobals(t)

	t.Setenv("AGENT_ENV_FILE", "/tmp/from-env.env")
	require.Equal(t, "/tmp/from-env.env", resolveEnvFile([]string{"agent", "--env-file", "/tmp/ignored.env"}))
}

func TestResolveEnvFileUsesDefaultWhenUnset(t *testing.T) {
	clearEnvKeys(t, "AGENT_ENV_FILE")
	resetGlobals(t)

	require.Equal(t, ".env", resolveEnvFile([]string{"agent"}))
}

func TestCommandRejectsUnsupportedRouter(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  name: config-model
  router: anthropic
  key: config-key
  baseUrl: https://config.example/v1
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{"agent", "--config", configPath})
	require.EqualError(t, err, "model router must be one of: none, openrouter")
}

func TestCommandUsesDirectClientWhenRouterIsNone(t *testing.T) {
	clearEnvKeys(t, "MODEL", "MODEL_ROUTER", "MODEL_KEY", "MODEL_BASE_URL", "LOG_LEVEL", "LOG_NO_COLOR", "AGENT_CONFIG", "AGENT_ENV_FILE")
	resetGlobals(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  name: config-model
  router: none
  key: config-key
log:
  level: info
`), 0o600))

	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{"agent", "--config", configPath})
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
