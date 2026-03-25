package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreloadEnvFile_LoadsValues(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(`
NAME=env-agent
MODEL=env-model
MODEL_ROUTER=openrouter
MODEL_KEY=env-key
OPENAI_API_KEY=openai-env-key
OPENROUTER_API_KEY=openrouter-env-key
MODEL_BASE_URL=https://env.example/v1
RPC_ADDRESS=0.0.0.0
RPC_PORT=6000
MAX_ITERATIONS=45
LOG_LEVEL=warn
LOG_NO_COLOR=true
DEBUG_REQUESTS=true
`), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "env-agent", os.Getenv("NAME"))
	require.Equal(t, "env-model", os.Getenv("MODEL"))
	require.Equal(t, "openrouter", os.Getenv("MODEL_ROUTER"))
	require.Equal(t, "env-key", os.Getenv("MODEL_KEY"))
	require.Equal(t, "openai-env-key", os.Getenv("OPENAI_API_KEY"))
	require.Equal(t, "openrouter-env-key", os.Getenv("OPENROUTER_API_KEY"))
	require.Equal(t, "https://env.example/v1", os.Getenv("MODEL_BASE_URL"))
	require.Equal(t, "0.0.0.0", os.Getenv("RPC_ADDRESS"))
	require.Equal(t, "6000", os.Getenv("RPC_PORT"))
	require.Equal(t, "45", os.Getenv("MAX_ITERATIONS"))
	require.Equal(t, "warn", os.Getenv("LOG_LEVEL"))
	require.Equal(t, "true", os.Getenv("LOG_NO_COLOR"))
	require.Equal(t, "true", os.Getenv("DEBUG_REQUESTS"))
}

func TestPreloadEnvFile_DoesNotOverrideShellEnv(t *testing.T) {
	clearEnvKeys(t, "MODEL_KEY")
	t.Setenv("MODEL_KEY", "shell-key")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("MODEL_KEY=env-key\n"), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "shell-key", os.Getenv("MODEL_KEY"))
}

func TestPreloadEnvFile_ReturnsErrorForUnreadablePath(t *testing.T) {
	dir := t.TempDir()
	require.EqualError(t, PreloadEnvFile(dir), `failed to load env file "`+dir+`": read `+dir+`: is a directory`)
}

func TestPreloadEnvFile_UsesDefaultPathWhenEmpty(t *testing.T) {
	originalLoadDotEnv := loadDotEnv
	t.Cleanup(func() {
		loadDotEnv = originalLoadDotEnv
	})

	calledWith := ""
	loadDotEnv = func(filenames ...string) error {
		if len(filenames) > 0 {
			calledWith = filenames[0]
		}
		return nil
	}

	require.NoError(t, PreloadEnvFile(""))
	require.Equal(t, ".env", calledWith)
}

func TestPreloadEnvFile_IgnoresMissingFile(t *testing.T) {
	originalLoadDotEnv := loadDotEnv
	t.Cleanup(func() {
		loadDotEnv = originalLoadDotEnv
	})

	loadDotEnv = func(...string) error {
		return os.ErrNotExist
	}

	require.NoError(t, PreloadEnvFile("missing.env"))
}

func TestLoad_UsesConfigFileValues(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
rpc:
  address: 0.0.0.0
  port: 6000
agent:
  maxIterations: 45
log:
  level: error
  noColor: true
debug:
  requests: true
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.Equal(t, "config-agent", cfg.Name)
	require.Equal(t, "config-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "config-key", cfg.ModelKey)
	require.Equal(t, "https://config.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "0.0.0.0", cfg.RPCAddress)
	require.Equal(t, 6000, cfg.RPCPort)
	require.Equal(t, 45, cfg.MaxIterations)
	require.Equal(t, "error", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
	require.True(t, cfg.DebugRequests)
}

func TestLoad_UsesEnvOverConfigFile(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
NAME=env-agent
MODEL=env-model
MODEL_ROUTER=openrouter
MODEL_KEY=env-key
MODEL_BASE_URL=https://env.example/v1
RPC_ADDRESS=127.0.0.1
RPC_PORT=7000
MAX_ITERATIONS=55
LOG_LEVEL=warn
LOG_NO_COLOR=false
DEBUG_REQUESTS=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
  baseUrl: https://config.example/v1
rpc:
  address: 0.0.0.0
  port: 6000
agent:
  maxIterations: 45
log:
  level: error
  noColor: true
debug:
  requests: true
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.Equal(t, "env-agent", cfg.Name)
	require.Equal(t, "env-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "env-key", cfg.ModelKey)
	require.Equal(t, "https://env.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "127.0.0.1", cfg.RPCAddress)
	require.Equal(t, 7000, cfg.RPCPort)
	require.Equal(t, 55, cfg.MaxIterations)
	require.Equal(t, "warn", cfg.LogLevel)
	require.False(t, cfg.LogNoColor)
	require.False(t, cfg.DebugRequests)
}

func TestLoad_IgnoresInvalidMaxIterationsEnvOverride(t *testing.T) {
	clearEnvKeys(t, "MAX_ITERATIONS", "MODEL_API_MODE")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("MAX_ITERATIONS=invalid\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: openrouter
  key: config-key
rpc:
  address: 127.0.0.1
  port: 50051
agent:
  maxIterations: 45
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.Equal(t, 45, cfg.MaxIterations)
}

func TestLoad_IgnoresMissingConfigFile(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS")

	cfg, err := Load("", filepath.Join(t.TempDir(), "missing.yaml"))

	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, defaultModel, cfg.Model)
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, "127.0.0.1", cfg.RPCAddress)
	require.Equal(t, 50051, cfg.RPCPort)
	require.Equal(t, defaultMaxIterations, cfg.MaxIterations)
	require.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_ReturnsErrorForInvalidConfigFile(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_ROUTER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: [\n"), 0o600))

	_, err := Load("", configPath)

	require.Error(t, err)
	require.Contains(t, err.Error(), `failed to parse config file`)
}

func TestGet_ReturnsDefaultsWhenConfigIsUnset(t *testing.T) {
	original := Get()
	Set(nil)
	t.Cleanup(func() {
		Set(original)
	})

	cfg := Get()
	require.Empty(t, cfg.Name)
	require.Equal(t, defaultModel, cfg.Model)
	require.Equal(t, "info", cfg.LogLevel)
	require.Empty(t, cfg.ModelRouter)
	require.Empty(t, cfg.ModelBaseURL)
	require.Empty(t, cfg.RPCAddress)
	require.Zero(t, cfg.RPCPort)
	require.Equal(t, defaultMaxIterations, cfg.MaxIterations)
}

func TestSet_StoresConfigGlobally(t *testing.T) {
	original := Get()
	t.Cleanup(func() {
		Set(original)
	})

	cfg := &Config{Name: "Test Agent", Model: "test-model", ModelRouter: "none", ModelKey: "test-key", LogLevel: "debug"}
	Set(cfg)
	require.Same(t, cfg, Get())
}

func TestConfig_ValidateRequiresKey(t *testing.T) {
	cfg := &Config{
		Name:     "test-agent",
		Model:    defaultModel,
		LogLevel: "info",
	}
	require.EqualError(t, cfg.Validate(), "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
}

func TestConfig_ResolveModelAuthUsesOpenRouterSpecificKey(t *testing.T) {
	cfg := &Config{
		Name:             "test-agent",
		Model:            defaultModel,
		ModelRouter:      "openrouter",
		OpenRouterAPIKey: "openrouter-key",
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openrouter", auth.Router)
	require.Equal(t, "openrouter-key", auth.APIKey)
	require.Equal(t, supportedRouters["openrouter"], auth.BaseURL)
}

func TestConfig_ResolveModelAuthUsesOpenAISpecificKey(t *testing.T) {
	cfg := &Config{
		Name:         "test-agent",
		Model:        defaultModel,
		ModelRouter:  "none",
		OpenAIAPIKey: "openai-key",
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "none", auth.Router)
	require.Equal(t, "openai-key", auth.APIKey)
	require.Empty(t, auth.BaseURL)
}

func TestConfig_ResolveModelAuthFallsBackToModelKey(t *testing.T) {
	cfg := &Config{
		Name:        "test-agent",
		Model:       defaultModel,
		ModelRouter: "openrouter",
		ModelKey:    "generic-key",
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "generic-key", auth.APIKey)
}

func TestConfig_ValidateAllowsProviderSpecificAuthWithoutModelKey(t *testing.T) {
	cfg := &Config{
		Name:             "test-agent",
		Model:            defaultModel,
		ModelRouter:      "openrouter",
		OpenRouterAPIKey: "openrouter-key",
		LogLevel:         "info",
	}

	require.NoError(t, cfg.Validate())
}

func TestConfig_ValidateNormalizesFields(t *testing.T) {
	cfg := &Config{
		Name:        "  Test Agent  ",
		Model:       "  test-model  ",
		ModelRouter: " OpenRouter ",
		ModelKey:    "  test-key  ",
		LogLevel:    " WARN ",
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "test-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "test-key", cfg.ModelKey)
	require.Equal(t, supportedRouters["openrouter"], cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
}

func TestConfig_ValidateRequiresName(t *testing.T) {
	err := (&Config{Model: defaultModel, ModelKey: "test-key", LogLevel: "info"}).Validate()
	require.EqualError(t, err, "name is required; set NAME, provide it in config, or use --name")
}

func TestConfig_ValidateDefaultsModelWhenEmpty(t *testing.T) {
	cfg := &Config{Name: "test-agent", ModelKey: "test-key", LogLevel: "info"}

	require.NoError(t, cfg.Validate())
	require.Equal(t, defaultModel, cfg.Model)
}

func TestConfig_ValidateRejectsUnsupportedRouter(t *testing.T) {
	err := (&Config{
		Name:         "test-agent",
		Model:        defaultModel,
		ModelRouter:  "anthropic",
		ModelKey:     "test-key",
		ModelBaseURL: supportedRouters[defaultModelRouter],
		LogLevel:     "info",
	}).Validate()
	require.EqualError(t, err, "model router must be one of: none, openrouter")
}

func TestConfig_ValidateRejectsInvalidLogLevel(t *testing.T) {
	err := (&Config{
		Name:        "test-agent",
		Model:       defaultModel,
		ModelRouter: "none",
		ModelKey:    "test-key",
		LogLevel:    "trace",
	}).Validate()
	require.EqualError(t, err, "log level must be one of debug, info, warn, or error; use --log.level")
}

func TestConfig_ValidateAllowsEmptyRouterAndLogLevel(t *testing.T) {
	err := (&Config{
		Name:     "test-agent",
		Model:    defaultModel,
		ModelKey: "test-key",
	}).Validate()
	require.NoError(t, err)
}

func TestConfig_ValidateRejectsEmptyRPCAddress(t *testing.T) {
	cfg := &Config{
		Name:        "test-agent",
		Model:       defaultModel,
		ModelRouter: "none",
		ModelKey:    "test-key",
		RPCAddress:  "   ",
		RPCPort:     50051,
		LogLevel:    "info",
	}

	require.EqualError(t, cfg.Validate(), "rpc address is required; set RPC_ADDRESS, provide it in config, or use --rpc.address")
}

func TestConfig_ValidateRejectsInvalidRPCPort(t *testing.T) {
	cfg := &Config{
		Name:        "test-agent",
		Model:       defaultModel,
		ModelRouter: "none",
		ModelKey:    "test-key",
		RPCAddress:  "127.0.0.1",
		RPCPort:     -1,
		LogLevel:    "info",
	}

	require.EqualError(t, cfg.Validate(), "rpc port must be greater than zero; set RPC_PORT, provide it in config, or use --rpc.port")
}

func TestConfig_ValidateRejectsInvalidMaxIterations(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelRouter:   "none",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		MaxIterations: -1,
		LogLevel:      "info",
	}

	require.EqualError(t, cfg.Validate(), "max iterations must be greater than zero; set MAX_ITERATIONS, provide it in config, or use --max-iterations")
}

func TestConfig_NormalizeDefaultsRouterWhenEmpty(t *testing.T) {
	cfg := &Config{
		Model:    defaultModel,
		LogLevel: "info",
	}
	cfg.Normalize()
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
}

func TestConfig_NormalizeIgnoresNilReceiver(t *testing.T) {
	var cfg *Config
	cfg.Normalize()
}

func TestConfig_NormalizeDefaultsModelAndLogLevel(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Empty(t, cfg.Name)
	require.Equal(t, defaultModel, cfg.Model)
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
	require.Equal(t, "127.0.0.1", cfg.RPCAddress)
	require.Equal(t, 50051, cfg.RPCPort)
	require.Equal(t, defaultMaxIterations, cfg.MaxIterations)
	require.Equal(t, "info", cfg.LogLevel)
}

func TestConfig_NormalizeUsesMappedBaseURLWhenRouterWasExplicitlySet(t *testing.T) {
	cfg := &Config{
		Model:       defaultModel,
		ModelRouter: defaultModelRouter,
		LogLevel:    "info",
	}
	cfg.Normalize()
	require.Equal(t, defaultModelRouter, cfg.ModelRouter)
	require.Equal(t, supportedRouters[defaultModelRouter], cfg.ModelBaseURL)
}

func TestConfig_NormalizeLeavesBaseURLUnsetForNoneRouter(t *testing.T) {
	cfg := &Config{
		Model:       defaultModel,
		ModelRouter: "none",
		ModelKey:    "test-key",
		LogLevel:    "info",
	}
	cfg.Normalize()
	require.Equal(t, "none", cfg.ModelRouter)
	require.Equal(t, "", cfg.ModelBaseURL)
}

func TestConfig_NormalizeTrimsAndLowercasesFields(t *testing.T) {
	cfg := &Config{
		Name:         "  Test Agent  ",
		Model:        "  test-model  ",
		ModelRouter:  " OpenRouter ",
		ModelKey:     "  test-key  ",
		ModelBaseURL: "  https://example.com/v1  ",
		LogLevel:     " WARN ",
	}
	cfg.Normalize()
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "test-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "test-key", cfg.ModelKey)
	require.Equal(t, "https://example.com/v1", cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
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

func TestLoad_UsesModelAPIModeFromConfig(t *testing.T) {
	clearEnvKeys(t, "MODEL_API_MODE")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: none
  key: config-key
  apiMode: responses
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, "responses", cfg.ModelAPIMode)
}

func TestLoad_UsesModelAPIModeFromEnvOverride(t *testing.T) {
	clearEnvKeys(t, "MODEL_API_MODE")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("MODEL_API_MODE=responses\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: none
  key: config-key
  apiMode: chat-completions
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)
	require.NoError(t, err)
	require.Equal(t, "responses", cfg.ModelAPIMode)
}

func TestConfig_ValidateRejectsInvalidAPIMode(t *testing.T) {
	err := (&Config{
		Name:         "test-agent",
		Model:        defaultModel,
		ModelRouter:  "none",
		ModelAPIMode: "invalid",
		ModelKey:     "test-key",
		RPCAddress:   "127.0.0.1",
		RPCPort:      50051,
		LogLevel:     "info",
	}).Validate()
	require.EqualError(t, err, "model api mode must be one of: chat-completions, responses; use --model.api-mode")
}

func TestConfig_ValidateRejectsResponsesModeWithOpenRouter(t *testing.T) {
	err := (&Config{
		Name:         "test-agent",
		Model:        defaultModel,
		ModelRouter:  "openrouter",
		ModelAPIMode: "responses",
		ModelKey:     "test-key",
		RPCAddress:   "127.0.0.1",
		RPCPort:      50051,
		LogLevel:     "info",
	}).Validate()
	require.EqualError(t, err, "model api mode 'responses' is only supported with model router 'none'; use --model.router 'none' or --model.api-mode 'chat-completions'")
}

func TestLoad_UsesDebugTraceSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t, "DEBUG_TRACES", "DEBUG_TRACE_DIR")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: none
  key: config-key
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
debug:
  traces: true
  traceDir: /tmp/hand-traces
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.True(t, cfg.DebugTraces)
	require.Equal(t, "/tmp/hand-traces", cfg.DebugTraceDir)
}

func TestLoad_UsesDebugTraceSettingsFromEnvOverride(t *testing.T) {
	clearEnvKeys(t, "DEBUG_TRACES", "DEBUG_TRACE_DIR")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("DEBUG_TRACES=true\nDEBUG_TRACE_DIR=/tmp/env-traces\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  router: none
  key: config-key
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
debug:
  traces: false
  traceDir: /tmp/config-traces
`), 0o600))

	cfg, err := Load(envPath, configPath)
	require.NoError(t, err)
	require.True(t, cfg.DebugTraces)
	require.Equal(t, "/tmp/env-traces", cfg.DebugTraceDir)
}

func TestConfig_NormalizeDefaultsDebugTraceDir(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, DefaultDebugTraceDir, cfg.DebugTraceDir)
}
