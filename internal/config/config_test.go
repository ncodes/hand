package config

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/datadir"
)

func stubModelMetadataResolver(t *testing.T, fn func(context.Context, *Config, ModelAuth) (ModelMetadata, error)) {
	t.Helper()

	original := resolveModelMeta
	resolveModelMeta = fn
	t.Cleanup(func() {
		resolveModelMeta = original
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestPreloadEnvFile_LoadsValues(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_PROVIDER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
		"MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL",
		"LOG_NO_COLOR", "DEBUG_REQUESTS", "RULES_FILES", "INSTRUCT", "PLATFORM", "AGENT_CAP_FS", "AGENT_CAP_NET",
		"AGENT_CAP_EXEC", "AGENT_CAP_MEM", "AGENT_CAP_BROWSER")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(`
NAME=env-agent
MODEL=env-model
MODEL_PROVIDER=openrouter
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
RULES_FILES=hand.md,custom.md
INSTRUCT=be terse
PLATFORM=desktop
AGENT_CAP_FS=false
AGENT_CAP_NET=false
AGENT_CAP_EXEC=false
AGENT_CAP_MEM=false
AGENT_CAP_BROWSER=true
`), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "env-agent", os.Getenv("NAME"))
	require.Equal(t, "env-model", os.Getenv("MODEL"))
	require.Equal(t, "openrouter", os.Getenv("MODEL_PROVIDER"))
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
	require.Equal(t, "hand.md,custom.md", os.Getenv("RULES_FILES"))
	require.Equal(t, "be terse", os.Getenv("INSTRUCT"))
	require.Equal(t, "desktop", os.Getenv("PLATFORM"))
	require.Equal(t, "false", os.Getenv("AGENT_CAP_FS"))
	require.Equal(t, "false", os.Getenv("AGENT_CAP_NET"))
	require.Equal(t, "false", os.Getenv("AGENT_CAP_EXEC"))
	require.Equal(t, "false", os.Getenv("AGENT_CAP_MEM"))
	require.Equal(t, "true", os.Getenv("AGENT_CAP_BROWSER"))
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

func TestLoad_ReturnsPreloadEnvFileError(t *testing.T) {
	originalLoadDotEnv := loadDotEnv
	t.Cleanup(func() {
		loadDotEnv = originalLoadDotEnv
	})

	loadDotEnv = func(...string) error {
		return errors.New("boom")
	}

	_, err := Load("broken.env", "")
	require.EqualError(t, err, `failed to load env file "broken.env": boom`)
}

func TestLoad_UsesConfigFileValues(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_PROVIDER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
		"MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR",
		"WEB_PROVIDER", "WEB_API_KEY", "WEB_BASE_URL", "WEB_MAX_CHAR_PER_RESULT", "WEB_MAX_EXTRACT_CHAR_PER_RESULT",
		"DEBUG_REQUESTS", "RULES_FILES", "INSTRUCT", "PLATFORM", "AGENT_CAP_FS", "AGENT_CAP_NET", "AGENT_CAP_EXEC", "AGENT_CAP_MEM", "AGENT_CAP_BROWSER")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  provider: openrouter
  key: config-key
  baseUrl: https://config.example/v1
rpc:
  address: 0.0.0.0
  port: 6000
maxIterations: 45
instruct: be terse
cap:
  fs: false
  net: false
  exec: false
  mem: false
  browser: true
platform: desktop
log:
  level: error
  noColor: true
debug:
  requests: true
web:
  provider: exa
  apiKey: web-key
  baseUrl: https://web.example
  maxCharPerResult: 2400
  maxExtractCharPerResult: 9600
rules:
  files:
    - hand.md
    - custom.md
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.Equal(t, "config-agent", cfg.Name)
	require.Equal(t, "config-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelProvider)
	require.Equal(t, "config-key", cfg.ModelKey)
	require.Equal(t, "https://config.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "0.0.0.0", cfg.RPCAddress)
	require.Equal(t, 6000, cfg.RPCPort)
	require.Equal(t, 45, cfg.MaxIterations)
	require.Equal(t, "error", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
	require.True(t, cfg.DebugRequests)
	require.Equal(t, "exa", cfg.WebProvider)
	require.Equal(t, "web-key", cfg.WebAPIKey)
	require.Equal(t, "https://web.example", cfg.WebBaseURL)
	require.Equal(t, 2400, cfg.WebMaxCharPerResult)
	require.Equal(t, 9600, cfg.WebMaxExtractCharPerResult)
	require.Equal(t, []string{"hand.md", "custom.md"}, cfg.RulesFiles)
	require.Equal(t, "be terse", cfg.Instruct)
	require.Equal(t, "desktop", cfg.Platform)
	require.False(t, boolValue(cfg.CapFilesystem))
	require.False(t, boolValue(cfg.CapNetwork))
	require.False(t, boolValue(cfg.CapExec))
	require.False(t, boolValue(cfg.CapMemory))
	require.True(t, boolValue(cfg.CapBrowser))
}

func TestLoad_UsesEnvOverConfigFile(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_PROVIDER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
		"MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR",
		"WEB_PROVIDER", "WEB_API_KEY", "WEB_BASE_URL", "WEB_MAX_CHAR_PER_RESULT", "WEB_MAX_EXTRACT_CHAR_PER_RESULT",
		"DEBUG_REQUESTS", "RULES_FILES", "INSTRUCT", "PLATFORM", "AGENT_CAP_FS", "AGENT_CAP_NET", "AGENT_CAP_EXEC", "AGENT_CAP_MEM", "AGENT_CAP_BROWSER")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
NAME=env-agent
MODEL=env-model
MODEL_PROVIDER=openrouter
MODEL_KEY=env-key
MODEL_BASE_URL=https://env.example/v1
RPC_ADDRESS=127.0.0.1
RPC_PORT=7000
MAX_ITERATIONS=55
LOG_LEVEL=warn
LOG_NO_COLOR=false
DEBUG_REQUESTS=false
WEB_PROVIDER=tavily
WEB_API_KEY=web-env-key
WEB_BASE_URL=https://env-web.example
WEB_MAX_CHAR_PER_RESULT=3100
WEB_MAX_EXTRACT_CHAR_PER_RESULT=12400
RULES_FILES=hand.md,custom.md
INSTRUCT=be terse
PLATFORM=editor
AGENT_CAP_FS=true
AGENT_CAP_NET=true
AGENT_CAP_EXEC=true
AGENT_CAP_MEM=true
AGENT_CAP_BROWSER=false
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  provider: openrouter
  key: config-key
  baseUrl: https://config.example/v1
rpc:
  address: 0.0.0.0
  port: 6000
maxIterations: 45
instruct: be formal
web:
  provider: firecrawl
  apiKey: config-web-key
  baseUrl: https://config-web.example
  maxCharPerResult: 1800
  maxExtractCharPerResult: 7200
cap:
  fs: false
  net: false
  exec: false
  mem: false
  browser: true
platform: desktop
log:
  level: error
  noColor: true
debug:
  requests: true
rules:
  files:
    - agents.md
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.Equal(t, "env-agent", cfg.Name)
	require.Equal(t, "env-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelProvider)
	require.Equal(t, "env-key", cfg.ModelKey)
	require.Equal(t, "https://env.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "127.0.0.1", cfg.RPCAddress)
	require.Equal(t, 7000, cfg.RPCPort)
	require.Equal(t, 55, cfg.MaxIterations)
	require.Equal(t, "warn", cfg.LogLevel)
	require.False(t, cfg.LogNoColor)
	require.False(t, cfg.DebugRequests)
	require.Equal(t, "tavily", cfg.WebProvider)
	require.Equal(t, "web-env-key", cfg.WebAPIKey)
	require.Equal(t, "https://env-web.example", cfg.WebBaseURL)
	require.Equal(t, 3100, cfg.WebMaxCharPerResult)
	require.Equal(t, 12400, cfg.WebMaxExtractCharPerResult)
	require.Equal(t, []string{"hand.md", "custom.md"}, cfg.RulesFiles)
	require.Equal(t, "be terse", cfg.Instruct)
	require.Equal(t, "editor", cfg.Platform)
	require.True(t, boolValue(cfg.CapFilesystem))
	require.True(t, boolValue(cfg.CapNetwork))
	require.True(t, boolValue(cfg.CapExec))
	require.True(t, boolValue(cfg.CapMemory))
	require.False(t, boolValue(cfg.CapBrowser))
}

func TestLoad_UsesModelStreamFromConfigAndEnv(t *testing.T) {
	clearEnvKeys(t, "MODEL_STREAM")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("MODEL_STREAM=true\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  stream: false
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.True(t, cfg.StreamEnabled())
}

func TestConfig_StreamEnabledDefaultsToTrue(t *testing.T) {
	require.True(t, (&Config{}).StreamEnabled())
	require.False(t, (&Config{Stream: new(false)}).StreamEnabled())
}

func TestLoad_UsesOpenRouterModelMetadataWhenContextLengthIsUnset(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 222222}, nil
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: openai/gpt-4o-mini
  provider: openrouter
  key: config-key
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 222222, cfg.ContextLength)
}

func TestLoad_UsesProviderMetadataWhenConfiguredContextLengthIsTooLarge(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 64000}, nil
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: gpt-4.1-nano
  provider: openai
  key: config-key
  contextLength: 999999
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 64000, cfg.ContextLength)
}

func TestLoad_PreservesSmallerConfiguredContextLengthThanProviderMetadata(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 128000}, nil
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: gpt-4.1-nano
  provider: openai
  key: config-key
  contextLength: 32000
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 32000, cfg.ContextLength)
}

func TestLoad_SkipsProviderModelMetadataWhenVerificationIsDisabled(t *testing.T) {
	called := false
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		called = true
		return ModelMetadata{Exists: true, ContextLength: 64000}, nil
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: openai/gpt-4o-mini
  provider: openrouter
  key: config-key
  verifyModel: false
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.False(t, called)
	require.Equal(t, defaultContextLength, cfg.ContextLength)
	require.False(t, boolValueDefault(cfg.VerifyModel, true))
}

func TestConfig_NormalizeLeavesRulesFilesEmptyWhenUnset(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Empty(t, cfg.RulesFiles)
}

func TestConfig_NormalizeNormalizesRulesFiles(t *testing.T) {
	cfg := &Config{RulesFiles: []string{" ./Hand.md ", "custom.md", "Hand.md", ""}}
	cfg.Normalize()
	require.Equal(t, []string{"Hand.md", "custom.md"}, cfg.RulesFiles)
}

func TestConfig_NormalizeTrimsInstruct(t *testing.T) {
	cfg := &Config{Instruct: "  be terse  "}
	cfg.Normalize()
	require.Equal(t, "be terse", cfg.Instruct)
}

func TestFetchOpenRouterModelMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "Bearer openrouter-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","context_length":555555}]}`))
	}))
	t.Cleanup(server.Close)

	meta, err := fetchOpenRouterModelMetadata(
		context.Background(),
		server.URL,
		"openai/gpt-4o-mini",
		"openrouter-key",
	)
	require.NoError(t, err)
	require.True(t, meta.Exists)
	require.Equal(t, 555555, meta.ContextLength)
}

func TestFetchOpenAIModelMetadata(t *testing.T) {
	originalHTTPClient := httpClient
	originalModelDocsBaseURL := modelDocsBaseURL
	t.Cleanup(func() {
		httpClient = originalHTTPClient
		modelDocsBaseURL = originalModelDocsBaseURL
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/docs/models/gpt-4.1-nano", r.URL.Path)
		_, _ = w.Write([]byte(`<html><body><p>1,047,576 context window</p></body></html>`))
	}))
	t.Cleanup(server.Close)

	httpClient = server.Client()
	modelDocsBaseURL = server.URL + "/api/docs/models"

	meta, err := fetchOpenAIModelMetadata(context.Background(), "openai/gpt-4.1-nano")
	require.NoError(t, err)
	require.True(t, meta.Exists)
	require.Equal(t, 1047576, meta.ContextLength)
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
  provider: openrouter
  key: config-key
rpc:
  address: 127.0.0.1
  port: 50051
maxIterations: 45
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.Equal(t, 45, cfg.MaxIterations)
}

func TestLoad_IgnoresMissingConfigFile(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_PROVIDER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS")

	cfg, err := Load("", filepath.Join(t.TempDir(), "missing.yaml"))

	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, defaultModel, cfg.Model)
	require.Equal(t, defaultModelProvider, cfg.ModelProvider)
	require.Equal(t, "127.0.0.1", cfg.RPCAddress)
	require.Equal(t, 50051, cfg.RPCPort)
	require.Equal(t, defaultMaxIterations, cfg.MaxIterations)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, boolValue(cfg.CapFilesystem))
	require.True(t, boolValue(cfg.CapNetwork))
	require.True(t, boolValue(cfg.CapExec))
	require.True(t, boolValue(cfg.CapMemory))
	require.False(t, boolValue(cfg.CapBrowser))
	require.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_ReturnsErrorForInvalidConfigFile(t *testing.T) {
	clearEnvKeys(t, "NAME", "MODEL", "MODEL_PROVIDER", "MODEL_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "MODEL_BASE_URL", "MODEL_API_MODE", "RPC_ADDRESS", "RPC_PORT", "MAX_ITERATIONS", "LOG_LEVEL", "LOG_NO_COLOR", "DEBUG_REQUESTS")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: [\n"), 0o600))

	_, err := Load("", configPath)

	require.Error(t, err)
	require.Contains(t, err.Error(), `failed to parse config file`)
}

func TestLoadConfigFile_UsesDefaultPathWhenEmpty(t *testing.T) {
	cfg, err := loadConfigFile("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

func TestLoadConfigFile_ReturnsReadError(t *testing.T) {
	dir := t.TempDir()

	_, err := loadConfigFile(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), `failed to read config file`)
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
	require.Empty(t, cfg.ModelProvider)
	require.Empty(t, cfg.ModelBaseURL)
	require.Empty(t, cfg.RPCAddress)
	require.Zero(t, cfg.RPCPort)
	require.Equal(t, defaultMaxIterations, cfg.MaxIterations)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, boolValue(cfg.CapFilesystem))
	require.True(t, boolValue(cfg.CapNetwork))
	require.True(t, boolValue(cfg.CapExec))
	require.True(t, boolValue(cfg.CapMemory))
	require.False(t, boolValue(cfg.CapBrowser))
}

func TestSet_StoresConfigGlobally(t *testing.T) {
	original := Get()
	t.Cleanup(func() {
		Set(original)
	})

	cfg := &Config{Name: "Test Agent", Model: "test-model", ModelProvider: "openai", ModelKey: "test-key", LogLevel: "debug"}
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
	require.Equal(t, defaultModelProvider, cfg.ModelProvider)
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.ModelBaseURL)
}

func TestConfig_ValidateNilConfig(t *testing.T) {
	var cfg *Config
	require.EqualError(t, cfg.Validate(), "config is required")
}

func TestConfig_ResolveModelAuthUsesOpenRouterSpecificKey(t *testing.T) {
	cfg := &Config{
		Name:             "test-agent",
		Model:            defaultModel,
		ModelProvider:    "openrouter",
		OpenRouterAPIKey: "openrouter-key",
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openrouter", auth.Provider)
	require.Equal(t, "openrouter-key", auth.APIKey)
	require.Equal(t, defaultBaseURLForProvider("openrouter", DefaultModelAPIMode), auth.BaseURL)
}

func TestConfig_ResolveModelAuthUsesOpenAISpecificKey(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openai",
		OpenAIAPIKey:  "openai-key",
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openai", auth.Provider)
	require.Equal(t, "openai-key", auth.APIKey)
	require.Empty(t, auth.BaseURL)
}

func TestConfig_ResolveModelAuthAcceptsOpenAIProviderAlias(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openai",
		OpenAIAPIKey:  "openai-key",
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openai", auth.Provider)
	require.Equal(t, "openai-key", auth.APIKey)
	require.Empty(t, auth.BaseURL)
}

func TestConfig_ResolveModelAuthFallsBackToModelKey(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openrouter",
		ModelKey:      "generic-key",
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "generic-key", auth.APIKey)
}

func TestConfig_ValidateAllowsProviderSpecificAuthWithoutModelKey(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	cfg := &Config{
		Name:             "test-agent",
		Model:            defaultModel,
		ModelProvider:    "openrouter",
		OpenRouterAPIKey: "openrouter-key",
		LogLevel:         "info",
	}

	require.NoError(t, cfg.Validate())
}

func TestConfig_ValidateNormalizesFields(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	cfg := &Config{
		Name:          "  Test Agent  ",
		Model:         "  openai/test-model  ",
		ModelProvider: " OpenRouter ",
		ModelKey:      "  test-key  ",
		LogLevel:      " WARN ",
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "openai/test-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelProvider)
	require.Equal(t, "test-key", cfg.ModelKey)
	require.Equal(t, defaultBaseURLForProvider("openrouter", DefaultModelAPIMode), cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
}

func TestConfig_ValidateRequiresName(t *testing.T) {
	err := (&Config{Model: defaultModel, ModelKey: "test-key", LogLevel: "info"}).Validate()
	require.EqualError(t, err, "name is required; set NAME, provide it in config, or use --name")
}

func TestConfig_ValidateDefaultsModelWhenEmpty(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	cfg := &Config{Name: "test-agent", ModelKey: "test-key", LogLevel: "info"}
	require.NoError(t, cfg.Validate())
	require.Equal(t, defaultModel, cfg.Model)
}

func TestConfig_ValidateRejectsModelWithoutOwnerPrefix(t *testing.T) {
	err := (&Config{
		Name:          "test-agent",
		Model:         "gpt-4o-mini",
		ModelProvider: "openai",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
	}).Validate()

	require.EqualError(t, err, "model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
}

func TestConfig_ValidateRejectsModelWithEmptyOwnerOrName(t *testing.T) {
	cases := []string{"/gpt-4o-mini", "openai/", "openai/gpt-4o-mini/extra"}

	for _, model := range cases {
		t.Run(model, func(t *testing.T) {
			err := (&Config{
				Name:          "test-agent",
				Model:         model,
				ModelProvider: "openai",
				ModelKey:      "test-key",
				RPCAddress:    "127.0.0.1",
				RPCPort:       50051,
				LogLevel:      "info",
			}).Validate()

			require.EqualError(t, err, "model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
		})
	}
}

func TestConfig_ValidateRejectsUnsupportedProvider(t *testing.T) {
	openRouterDefault := defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode)
	err := (&Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "anthropic",
		ModelKey:      "test-key",
		ModelBaseURL:  openRouterDefault,
		LogLevel:      "info",
	}).Validate()
	require.EqualError(t, err, "model provider must be one of: openai, openrouter")
}

func TestConfig_ValidateRejectsUnknownOpenRouterModel(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{}, nil
	})

	err := (&Config{
		Name:          "test-agent",
		Model:         "openai/gpt-unknown",
		ModelProvider: "openrouter",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
	}).Validate()

	require.EqualError(t, err, `model.name: model "openai/gpt-unknown" is not available on openrouter`)
}

func TestConfig_ValidateRejectsInvalidSummaryModelSlug(t *testing.T) {
	err := (&Config{
		Name:          "test-agent",
		Model:         defaultModel,
		SummaryModel:  "gpt-4o-mini",
		ModelProvider: "openai",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
	}).Validate()

	require.EqualError(t, err, "summary model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
}

func TestConfig_ValidateRejectsUnknownSummaryModel(t *testing.T) {
	stubModelMetadataResolver(t, func(_ context.Context, cfg *Config, _ ModelAuth) (ModelMetadata, error) {
		if cfg.Model == defaultModel {
			return ModelMetadata{Exists: true}, nil
		}

		return ModelMetadata{}, nil
	})

	err := (&Config{
		Name:          "test-agent",
		Model:         defaultModel,
		SummaryModel:  "openai/gpt-unknown-summary",
		ModelProvider: "openrouter",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
	}).Validate()

	require.EqualError(t, err, `model.summaryModel: model "openai/gpt-unknown-summary" is not available on openrouter`)
}

func TestConfig_SummaryModelEffective(t *testing.T) {
	t.Run("inherits_main_model_when_empty", func(t *testing.T) {
		cfg := &Config{Model: defaultModel}
		require.Equal(t, defaultModel, cfg.SummaryModelEffective())
	})

	t.Run("uses_summary_when_set", func(t *testing.T) {
		cfg := &Config{Model: defaultModel, SummaryModel: "anthropic/claude-3.5-haiku"}
		require.Equal(t, "anthropic/claude-3.5-haiku", cfg.SummaryModelEffective())
	})
}

func TestConfig_SummaryProviderEffective(t *testing.T) {
	cfg := &Config{ModelProvider: "openrouter"}
	require.Equal(t, "openrouter", cfg.SummaryProviderEffective())

	cfg.SummaryProvider = "openai"
	require.Equal(t, "openai", cfg.SummaryProviderEffective())
}

func TestConfig_SummaryModelAPIModeEffective(t *testing.T) {
	cfg := &Config{ModelAPIMode: "responses"}
	cfg.Normalize()
	require.Equal(t, "responses", cfg.SummaryModelAPIModeEffective())

	cfg.SummaryModelAPIMode = DefaultModelAPIMode
	cfg.Normalize()
	require.Equal(t, DefaultModelAPIMode, cfg.SummaryModelAPIModeEffective())
}

func TestConfig_ResolveSummaryModelAuth_UsesSummaryAPIModeForDefaultBaseURL(t *testing.T) {
	cfg := &Config{
		Name:                "test-agent",
		Model:               defaultModel,
		ModelProvider:       "openrouter",
		ModelKey:            "k",
		ModelAPIMode:        DefaultModelAPIMode,
		SummaryModelAPIMode: "responses",
	}
	cfg.Normalize()

	auth, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)
	require.Equal(t, "https://openrouter.ai/api/v1/responses", auth.BaseURL)
}

func TestConfig_ResolveSummaryModelAuthMatchesMainWhenUnset(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openrouter",
		ModelKey:      "k",
	}

	main, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	sum, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)
	require.True(t, ModelAuthEqual(main, sum))
}

func TestConfig_ResolveSummaryModelAuthUsesOpenAIWhenSummaryProviderDiffers(t *testing.T) {
	cfg := &Config{
		Name:                "test-agent",
		Model:               defaultModel,
		ModelProvider:       "openrouter",
		ModelKey:            "k",
		SummaryProvider:     "openai",
		SummaryModelBaseURL: "https://api.example/v1",
	}
	cfg.Normalize()

	auth, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)
	require.Equal(t, "openai", auth.Provider)
	require.Equal(t, "https://api.example/v1", auth.BaseURL)
	require.Equal(t, "k", auth.APIKey)
}

func TestConfig_ValidateRejectsInvalidSummaryProvider(t *testing.T) {
	err := (&Config{
		Name:            "test-agent",
		Model:           defaultModel,
		ModelProvider:   "openrouter",
		SummaryProvider: "anthropic",
		ModelKey:        "test-key",
		RPCAddress:      "127.0.0.1",
		RPCPort:         50051,
		LogLevel:        "info",
	}).Validate()

	require.EqualError(t, err, "summary model provider must be one of: openai, openrouter")
}

func TestConfig_ValidateRejectsInvalidSummaryModelAPIMode(t *testing.T) {
	err := (&Config{
		Name:                "test-agent",
		Model:               defaultModel,
		ModelProvider:       "openrouter",
		ModelKey:            "test-key",
		SummaryModelAPIMode: "invalid",
		RPCAddress:          "127.0.0.1",
		RPCPort:             50051,
		LogLevel:            "info",
	}).Validate()

	require.EqualError(t, err, "summary model api mode must be one of: completions, responses; "+
		"use --model.summary-api-mode")
}

func TestConfig_ValidateAcceptsSummaryModelAPIModeResponses(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: defaultContextLength}, nil
	})

	err := (&Config{
		Name:                "test-agent",
		Model:               defaultModel,
		ModelProvider:       "openrouter",
		ModelKey:            "test-key",
		SummaryModelAPIMode: "responses",
		RPCAddress:          "127.0.0.1",
		RPCPort:             50051,
		LogLevel:            "info",
		VerifyModel:         new(false),
	}).Validate()

	require.NoError(t, err)
}

func TestConfig_ValidateAcceptsSummaryModelAPIModeCompletions(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: defaultContextLength}, nil
	})

	err := (&Config{
		Name:                "test-agent",
		Model:               defaultModel,
		ModelProvider:       "openrouter",
		ModelKey:            "test-key",
		SummaryModelAPIMode: DefaultModelAPIMode,
		RPCAddress:          "127.0.0.1",
		RPCPort:             50051,
		LogLevel:            "info",
		VerifyModel:         new(false),
	}).Validate()

	require.NoError(t, err)
}

func TestConfig_ModelAuthEqual(t *testing.T) {
	require.True(t, ModelAuthEqual(
		ModelAuth{Provider: "openai", BaseURL: "http://a", APIKey: "k"},
		ModelAuth{Provider: "openai", BaseURL: "http://a", APIKey: "k"},
	))
	require.False(t, ModelAuthEqual(
		ModelAuth{Provider: "openai", BaseURL: "http://a", APIKey: "k"},
		ModelAuth{Provider: "openrouter", BaseURL: "http://a", APIKey: "k"},
	))
}

func TestConfig_ValidateReturnsOpenRouterLookupFailure(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{}, errors.New(`failed to verify openrouter model "openai/gpt-4o-mini": lookup failed`)
	})

	err := (&Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openrouter",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
	}).Validate()

	require.EqualError(t, err, `model.name: failed to verify openrouter model "openai/gpt-4o-mini": lookup failed`)
}

func TestConfig_ValidateRejectsUnknownOpenAIModel(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{}, nil
	})

	err := (&Config{
		Name:          "test-agent",
		Model:         "openai/gpt-unknown",
		ModelProvider: "openai",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
	}).Validate()

	require.EqualError(t, err, `model.name: model "openai/gpt-unknown" is not available on openai`)
}

func TestConfig_ValidateRejectsInvalidLogLevel(t *testing.T) {
	err := (&Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openai",
		ModelKey:      "test-key",
		LogLevel:      "trace",
	}).Validate()
	require.EqualError(t, err, "log level must be one of debug, info, warn, or error; use --log.level")
}

func TestConfig_ValidateAllowsEmptyProviderAndLogLevel(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	err := (&Config{
		Name:     "test-agent",
		Model:    defaultModel,
		ModelKey: "test-key",
	}).Validate()
	require.NoError(t, err)
}

func TestConfig_ValidateRejectsEmptyRPCAddress(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openai",
		ModelKey:      "test-key",
		RPCAddress:    "   ",
		RPCPort:       50051,
		LogLevel:      "info",
	}

	require.EqualError(t, cfg.Validate(), "rpc address is required; set RPC_ADDRESS, provide it in config, or use --rpc.address")
}

func TestConfig_ValidateRejectsInvalidRPCPort(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openai",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       -1,
		LogLevel:      "info",
	}

	require.EqualError(t, cfg.Validate(), "rpc port must be greater than zero; set RPC_PORT, provide it in config, or use --rpc.port")
}

func TestConfig_ValidateRejectsInvalidMaxIterations(t *testing.T) {
	cfg := &Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openai",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		MaxIterations: -1,
		LogLevel:      "info",
	}

	require.EqualError(t, cfg.Validate(), "max iterations must be greater than zero; set MAX_ITERATIONS, provide it in config, or use --max-iterations")
}

func TestConfig_ValidateRejectsCompactionThresholdsAboveOrEqualOne(t *testing.T) {
	err := (&Config{
		Name:                     "test-agent",
		Model:                    defaultModel,
		ModelProvider:            "openai",
		ModelKey:                 "test-key",
		RPCAddress:               "127.0.0.1",
		RPCPort:                  50051,
		MaxIterations:            1,
		LogLevel:                 "info",
		StorageBackend:           "memory",
		SessionDefaultIdleExpiry: time.Hour,
		SessionArchiveRetention:  24 * time.Hour,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 1,
		CompactionWarnPercent:    1,
	}).Validate()

	require.EqualError(t, err, "compaction trigger percent must be greater than zero and less than one")

	err = (&Config{
		Name:                     "test-agent",
		Model:                    defaultModel,
		ModelProvider:            "openai",
		ModelKey:                 "test-key",
		RPCAddress:               "127.0.0.1",
		RPCPort:                  50051,
		MaxIterations:            1,
		LogLevel:                 "info",
		StorageBackend:           "memory",
		SessionDefaultIdleExpiry: time.Hour,
		SessionArchiveRetention:  24 * time.Hour,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 0.9,
		CompactionWarnPercent:    1,
	}).Validate()

	require.EqualError(t, err, "compaction warn percent must be greater than zero and less than one")
}

func TestConfig_NormalizeDefaultsProviderWhenEmpty(t *testing.T) {
	cfg := &Config{
		Model:    defaultModel,
		LogLevel: "info",
	}
	cfg.Normalize()
	require.Equal(t, defaultModelProvider, cfg.ModelProvider)
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.ModelBaseURL)
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
	require.Equal(t, defaultModelProvider, cfg.ModelProvider)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, boolValue(cfg.CapFilesystem))
	require.True(t, boolValue(cfg.CapNetwork))
	require.True(t, boolValue(cfg.CapExec))
	require.True(t, boolValue(cfg.CapMemory))
	require.False(t, boolValue(cfg.CapBrowser))
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.ModelBaseURL)
	require.Equal(t, "127.0.0.1", cfg.RPCAddress)
	require.Equal(t, 50051, cfg.RPCPort)
	require.Equal(t, defaultMaxIterations, cfg.MaxIterations)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, DefaultWebMaxCharPerResult, cfg.WebMaxCharPerResult)
	require.Equal(t, DefaultWebMaxExtractCharPerResult, cfg.WebMaxExtractCharPerResult)
	require.True(t, boolValueDefault(cfg.VerifyModel, true))
}

func TestConfig_NormalizePreservesExplicitFalseCapabilities(t *testing.T) {
	cfg := &Config{
		CapFilesystem: new(false),
		CapNetwork:    new(false),
		CapExec:       new(false),
		CapMemory:     new(false),
		CapBrowser:    new(false),
	}

	cfg.Normalize()

	require.False(t, boolValue(cfg.CapFilesystem))
	require.False(t, boolValue(cfg.CapNetwork))
	require.False(t, boolValue(cfg.CapExec))
	require.False(t, boolValue(cfg.CapMemory))
	require.False(t, boolValue(cfg.CapBrowser))
}

func TestConfig_NormalizeDefaultsUnsetCapabilitiesIndividually(t *testing.T) {
	cfg := &Config{CapFilesystem: new(false)}

	cfg.Normalize()

	require.False(t, boolValue(cfg.CapFilesystem))
	require.True(t, boolValue(cfg.CapNetwork))
	require.True(t, boolValue(cfg.CapExec))
	require.True(t, boolValue(cfg.CapMemory))
	require.False(t, boolValue(cfg.CapBrowser))
}

func TestConfig_NormalizeUsesMappedBaseURLWhenProviderWasExplicitlySet(t *testing.T) {
	cfg := &Config{
		Model:         defaultModel,
		ModelProvider: defaultModelProvider,
		LogLevel:      "info",
	}
	cfg.Normalize()
	require.Equal(t, defaultModelProvider, cfg.ModelProvider)
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.ModelBaseURL)
}

func TestConfig_NormalizeKeepsOpenaiProvider(t *testing.T) {
	cfg := &Config{
		Model:         defaultModel,
		ModelProvider: "openai",
		ModelKey:      "test-key",
		LogLevel:      "info",
	}
	cfg.Normalize()
	require.Equal(t, "openai", cfg.ModelProvider)
	require.Equal(t, "", cfg.ModelBaseURL)
}

func TestConfig_NormalizeDefaultBaseURLDependsOnAPIMode(t *testing.T) {
	t.Run("openai uses sdk default for completions and responses", func(t *testing.T) {
		for _, mode := range []string{DefaultModelAPIMode, "responses"} {
			cfg := &Config{ModelProvider: "openai", ModelAPIMode: mode}
			cfg.Normalize()
			require.Empty(t, cfg.ModelBaseURL, mode)
		}
	})

	t.Run("openrouter defaults differ by api mode", func(t *testing.T) {
		cfgChat := &Config{ModelProvider: "openrouter", ModelAPIMode: DefaultModelAPIMode}
		cfgChat.Normalize()
		require.Equal(t, "https://openrouter.ai/api/v1", cfgChat.ModelBaseURL)

		cfgResp := &Config{ModelProvider: "openrouter", ModelAPIMode: "responses"}
		cfgResp.Normalize()
		require.Equal(t, "https://openrouter.ai/api/v1/responses", cfgResp.ModelBaseURL)
	})

	t.Run("unknown api mode does not fall back to default base url", func(t *testing.T) {
		cfg := &Config{ModelProvider: "openrouter", ModelAPIMode: "future-mode"}
		cfg.Normalize()
		require.Empty(t, cfg.ModelBaseURL)
	})
}

func TestConfig_NormalizeTrimsAndLowercasesFields(t *testing.T) {
	cfg := &Config{
		Name:          "  Test Agent  ",
		Model:         "  test-model  ",
		ModelProvider: " OpenRouter ",
		ModelKey:      "  test-key  ",
		ModelBaseURL:  "  https://example.com/v1  ",
		LogLevel:      " WARN ",
	}
	cfg.Normalize()
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "test-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelProvider)
	require.Equal(t, "test-key", cfg.ModelKey)
	require.Equal(t, "https://example.com/v1", cfg.ModelBaseURL)
	require.Equal(t, "warn", cfg.LogLevel)
}

func TestConfig_VerifyModelEnabledUsesFallbacks(t *testing.T) {
	var cfg *Config
	require.True(t, cfg.VerifyModelEnabled())

	cfg = &Config{}
	require.True(t, cfg.VerifyModelEnabled())

	cfg.VerifyModel = new(false)
	require.False(t, cfg.VerifyModelEnabled())
}

func TestHelpers_SplitAndDedupeCSVAndBools(t *testing.T) {
	require.Nil(t, splitAndTrimCSV(""))
	require.Equal(t, []string{"a", "b"}, splitAndTrimCSV(" a, ,b ,,"))

	require.Nil(t, dedupeAndTrim(nil))
	require.Equal(t, []string{"a", "b"}, dedupeAndTrim([]string{" a ", "", "b", "a"}))

	require.False(t, boolValue(nil))
	require.True(t, boolValue(new(true)))
	require.True(t, boolValueDefault(nil, true))
	require.False(t, boolValueDefault(new(false), true))
}

func TestResolvePathsFromBase_HandlesEmptyAndAbsolute(t *testing.T) {
	require.Nil(t, resolvePathsFromBase(nil, "/tmp"))
	require.Equal(t, []string{"a", "b"}, resolvePathsFromBase([]string{"a", "b"}, ""))

	abs := filepath.Join(string(os.PathSeparator), "tmp", "x")
	require.Equal(t, []string{abs, filepath.Join("/base", "rel")},
		resolvePathsFromBase([]string{abs, "rel"}, "/base"))
}

func TestDefaultFSRootsAndNormalizeFSRootsFallbackWhenGetwdFails(t *testing.T) {
	originalGetwd := getwd
	t.Cleanup(func() {
		getwd = originalGetwd
	})

	getwd = func() (string, error) {
		return "", errors.New("cwd missing")
	}

	require.Equal(t, []string{"."}, defaultFSRoots())
	require.Equal(t, []string{"."}, normalizeFSRoots([]string{"."}))
}

func TestNormalizeFSRoots_PreservesAbsoluteRoots(t *testing.T) {
	abs := filepath.Join(string(os.PathSeparator), "tmp", "workspace")
	require.Equal(t, []string{abs}, normalizeFSRoots([]string{abs}))
}

func TestResolveModelMetadataFromProvider_NilConfig(t *testing.T) {
	meta, err := resolveModelMetadataFromProvider(context.Background(), nil, ModelAuth{})
	require.NoError(t, err)
	require.Equal(t, ModelMetadata{}, meta)
}

func TestResolveModelAuth_CoversDefaultBranchAndNilReceiver(t *testing.T) {
	var cfg *Config
	_, err := cfg.ResolveModelAuth()
	require.EqualError(t, err, "config is required")

	cfg = &Config{
		ModelProvider: "custom",
		ModelKey:      "key",
	}
	auth, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "key", auth.APIKey)
}

func TestApplyEnvOverrides_CoversRemainingBranches(t *testing.T) {
	clearEnvKeys(t,
		"MODEL_CONTEXT_LENGTH", "MODEL_VERIFY_MODEL", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
		"AGENT_STORAGE_BACKEND", "AGENT_SESSION_DEFAULT_IDLE_EXPIRY", "AGENT_SESSION_ARCHIVE_RETENTION",
		"AGENT_COMPACTION_ENABLED", "AGENT_COMPACTION_TRIGGER_PERCENT", "AGENT_COMPACTION_WARN_PERCENT",
		"FIRECRAWL_API_KEY", "FIRECRAWL_API_URL", "PARALLEL_API_KEY", "TAVILY_API_KEY", "EXA_API_KEY",
	)

	cfg := &Config{}
	applyEnvOverrides(nil)

	t.Setenv("MODEL_CONTEXT_LENGTH", "64000")
	t.Setenv("MODEL_VERIFY_MODEL", "false")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
	t.Setenv("AGENT_STORAGE_BACKEND", "memory")
	t.Setenv("AGENT_SESSION_DEFAULT_IDLE_EXPIRY", "2h")
	t.Setenv("AGENT_SESSION_ARCHIVE_RETENTION", "48h")
	t.Setenv("AGENT_COMPACTION_ENABLED", "false")
	t.Setenv("AGENT_COMPACTION_TRIGGER_PERCENT", "0.5")
	t.Setenv("AGENT_COMPACTION_WARN_PERCENT", "0.8")

	applyEnvOverrides(cfg)

	require.Equal(t, 64000, cfg.ContextLength)
	require.False(t, boolValue(cfg.VerifyModel))
	require.Equal(t, "openai-key", cfg.OpenAIAPIKey)
	require.Equal(t, "openrouter-key", cfg.OpenRouterAPIKey)
	require.Equal(t, "memory", cfg.StorageBackend)
	require.Equal(t, 2*time.Hour, cfg.SessionDefaultIdleExpiry)
	require.Equal(t, 48*time.Hour, cfg.SessionArchiveRetention)
	require.False(t, boolValue(cfg.CompactionEnabled))
	require.Equal(t, 0.5, cfg.CompactionTriggerPercent)
	require.Equal(t, 0.8, cfg.CompactionWarnPercent)
}

func TestApplyEnvOverrides_WebProviderSpecificFallback(t *testing.T) {
	clearEnvKeys(t,
		"WEB_PROVIDER", "WEB_API_KEY", "WEB_BASE_URL",
		"FIRECRAWL_API_KEY", "FIRECRAWL_API_URL", "PARALLEL_API_KEY", "TAVILY_API_KEY", "EXA_API_KEY",
	)

	cfg := &Config{}
	t.Setenv("FIRECRAWL_API_URL", "http://localhost:3002")

	applyEnvOverrides(cfg)

	require.Equal(t, "firecrawl", cfg.WebProvider)
	require.Equal(t, "", cfg.WebAPIKey)
	require.Equal(t, "http://localhost:3002", cfg.WebBaseURL)

	cfg = &Config{}
	t.Setenv("WEB_PROVIDER", "exa")
	t.Setenv("EXA_API_KEY", "exa-key")

	applyEnvOverrides(cfg)

	require.Equal(t, "exa", cfg.WebProvider)
	require.Equal(t, "exa-key", cfg.WebAPIKey)
}

func TestApplyEnvOverrides_SummaryModelAndRelatedEnv(t *testing.T) {
	clearEnvKeys(t,
		"MODEL_SUMMARY", "MODEL_SUMMARY_PROVIDER", "MODEL_SUMMARY_BASE_URL",
		"MODEL_API_MODE", "MODEL_SUMMARY_API_MODE",
	)

	cfg := &Config{}
	t.Setenv("MODEL_SUMMARY", "openai/gpt-4o-mini")
	t.Setenv("MODEL_SUMMARY_PROVIDER", "openai")
	t.Setenv("MODEL_SUMMARY_BASE_URL", "https://example.com/v1")
	t.Setenv("MODEL_API_MODE", "responses")
	t.Setenv("MODEL_SUMMARY_API_MODE", "responses")

	applyEnvOverrides(cfg)

	require.Equal(t, "openai/gpt-4o-mini", cfg.SummaryModel)
	require.Equal(t, "openai", cfg.SummaryProvider)
	require.Equal(t, "https://example.com/v1", cfg.SummaryModelBaseURL)
	require.Equal(t, "responses", cfg.ModelAPIMode)
	require.Equal(t, "responses", cfg.SummaryModelAPIMode)
}

func TestNormalizeFields_NilReceiver_NoPanic(t *testing.T) {
	var cfg *Config
	cfg.normalizeFields()
}

func TestDefaultBaseURLForProvider_DefaultsEmptyAPIMode(t *testing.T) {
	require.Equal(t, "https://openrouter.ai/api/v1", defaultBaseURLForProvider("openrouter", ""))
	require.Equal(t, "https://openrouter.ai/api/v1", defaultBaseURLForProvider("openrouter", "   "))
}

func TestDefaultBaseURLForProvider_ReturnsEmptyForUnknownMode(t *testing.T) {
	require.Empty(t, defaultBaseURLForProvider("openrouter", "not-a-mode"))
}

func TestConfig_NilReceiver_StreamAndSummaryHelpers(t *testing.T) {
	var cfg *Config

	require.True(t, cfg.StreamEnabled())
	require.Equal(t, "", cfg.SummaryModelEffective())
	require.Equal(t, "", cfg.SummaryProviderEffective())
	require.Equal(t, "", cfg.SummaryModelAPIModeEffective())

	_, err := cfg.ResolveSummaryModelAuth()
	require.EqualError(t, err, "config is required")
}

func TestConfig_ResolveSummaryModelAuth_FailsWhenSummaryProviderHasNoKey(t *testing.T) {
	cfg := &Config{
		Name:                "test-agent",
		Model:               defaultModel,
		ModelProvider:       "openrouter",
		OpenRouterAPIKey:    "router-only",
		SummaryProvider:     "openai",
		SummaryModelBaseURL: "https://api.openai.com/v1",
	}
	cfg.Normalize()

	_, err := cfg.ResolveSummaryModelAuth()
	require.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
}

func TestConfig_Validate_ReturnsSummaryAuthErrorWhenOpenAIKeyMissing(t *testing.T) {
	err := (&Config{
		Name:             "test-agent",
		Model:            defaultModel,
		ModelProvider:    "openrouter",
		OpenRouterAPIKey: "router-only",
		SummaryProvider:  "openai",
		RPCAddress:       "127.0.0.1",
		RPCPort:          50051,
		LogLevel:         "info",
	}).Validate()

	require.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
}

func TestResolveModelMetadataForSlug_EmptySlug(t *testing.T) {
	meta, err := resolveModelMetadataForSlug(context.Background(), ModelAuth{Provider: "openai"}, "")
	require.NoError(t, err)
	require.Equal(t, ModelMetadata{}, meta)
}

func TestResolveModelMetadataForSlug_UnsupportedProvider(t *testing.T) {
	_, err := resolveModelMetadataForSlug(context.Background(), ModelAuth{Provider: "other"}, "openai/gpt-4o-mini")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported model provider")
}

func TestApplyProviderModelMetadata_CoversEarlyReturns(t *testing.T) {
	original := resolveModelMeta
	t.Cleanup(func() {
		resolveModelMeta = original
	})

	applyProviderModelMetadata(context.Background(), nil, 0)

	cfg := &Config{VerifyModel: new(false)}
	applyProviderModelMetadata(context.Background(), cfg, 0)

	cfg = &Config{VerifyModel: new(true)}
	applyProviderModelMetadata(context.Background(), cfg, 0)

	resolveModelMeta = func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{}, errors.New("boom")
	}
	cfg = &Config{
		Model:         defaultModel,
		VerifyModel:   new(true),
		ModelProvider: "openai",
		ModelKey:      "test-key",
	}
	cfg.Normalize()
	applyProviderModelMetadata(context.Background(), cfg, 0)

	resolveModelMeta = func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	}
	applyProviderModelMetadata(context.Background(), cfg, 0)
	require.Equal(t, defaultContextLength, cfg.ContextLength)
}

func TestFetchOpenRouterModelMetadata_CoversRemainingBranches(t *testing.T) {
	t.Run("empty_base_url", func(t *testing.T) {
		meta, err := fetchOpenRouterModelMetadata(context.Background(), "", "", "")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("invalid_base_url", func(t *testing.T) {
		meta, err := fetchOpenRouterModelMetadata(context.Background(), "://bad", "openai/gpt-4o-mini", "")
		require.Error(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("network_error", func(t *testing.T) {
		originalHTTPClient := httpClient
		t.Cleanup(func() {
			httpClient = originalHTTPClient
		})

		httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		})}
		meta, err := fetchOpenRouterModelMetadata(context.Background(), "", "openai/gpt-4o-mini", "")
		require.EqualError(t, err, `Get "https://openrouter.ai/api/v1/models": network down`)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("non_200_status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		t.Cleanup(server.Close)

		_, err := fetchOpenRouterModelMetadata(context.Background(), server.URL, "openai/gpt-4o-mini", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "openrouter models lookup returned 418 I'm a teapot")
	})

	t.Run("invalid_json_response", func(t *testing.T) {
		badJSONServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{`))
		}))
		t.Cleanup(badJSONServer.Close)
		meta, err := fetchOpenRouterModelMetadata(context.Background(), badJSONServer.URL, "openai/gpt-4o-mini", "")
		require.Error(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("read_error", func(t *testing.T) {
		originalHTTPClient := httpClient
		t.Cleanup(func() {
			httpClient = originalHTTPClient
		})

		httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(errReader{}),
				Header:     make(http.Header),
			}, nil
		})}
		meta, err := fetchOpenRouterModelMetadata(context.Background(), "", "openai/gpt-4o-mini", "")
		require.EqualError(t, err, "forced read error")
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("model_not_found", func(t *testing.T) {
		notFoundServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"id":"openai/other","context_length":1}]}`))
		}))
		t.Cleanup(notFoundServer.Close)
		meta, err := fetchOpenRouterModelMetadata(context.Background(), notFoundServer.URL, "openai/gpt-4o-mini", "")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})
}

func TestFetchOpenAIModelMetadata_CoversRemainingBranches(t *testing.T) {
	t.Run("empty_model", func(t *testing.T) {
		meta, err := fetchOpenAIModelMetadata(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("transport_error", func(t *testing.T) {
		originalHTTPClient := httpClient
		t.Cleanup(func() {
			httpClient = originalHTTPClient
		})

		httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport failed")
		})}
		meta, err := fetchOpenAIModelMetadataCandidate(context.Background(), "gpt-4o-mini")
		require.EqualError(t, err, `Get "https://developers.openai.com/api/docs/models/gpt-4o-mini": transport failed`)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("bad_base_url", func(t *testing.T) {
		originalModelDocsBaseURL := modelDocsBaseURL
		t.Cleanup(func() {
			modelDocsBaseURL = originalModelDocsBaseURL
		})

		modelDocsBaseURL = "://bad"
		meta, err := fetchOpenAIModelMetadataCandidate(context.Background(), "gpt-4o-mini")
		require.Error(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("various_http_responses", func(t *testing.T) {
		originalHTTPClient := httpClient
		originalModelDocsBaseURL := modelDocsBaseURL
		t.Cleanup(func() {
			httpClient = originalHTTPClient
			modelDocsBaseURL = originalModelDocsBaseURL
		})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/docs/models/missing":
				w.WriteHeader(http.StatusNotFound)
			case "/api/docs/models/status":
				w.WriteHeader(http.StatusBadGateway)
			case "/api/docs/models/no-window":
				_, _ = w.Write([]byte(`<html>exists</html>`))
			case "/api/docs/models/comment-window":
				_, _ = w.Write([]byte(`<div>128,000<!-- --> context window</div></div><div c`))
			case "/api/docs/models/bad-window":
				_, _ = w.Write([]byte(`<html>123,456 context window</html>`))
			case "/api/docs/models/fallback-2025-04-14":
				w.WriteHeader(http.StatusNotFound)
			case "/api/docs/models/fallback":
				_, _ = w.Write([]byte(`<html><p>8,192 context window</p></html>`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)
		httpClient = server.Client()
		modelDocsBaseURL = server.URL + "/api/docs/models"

		meta, err := fetchOpenAIModelMetadataCandidate(context.Background(), "missing")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)

		meta, err = fetchOpenAIModelMetadataCandidate(context.Background(), "status")
		require.Error(t, err)
		require.Contains(t, err.Error(), "openai model docs lookup returned 502 Bad Gateway")

		meta, err = fetchOpenAIModelMetadataCandidate(context.Background(), "no-window")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)

		meta, err = fetchOpenAIModelMetadataCandidate(context.Background(), "comment-window")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{Exists: true, ContextLength: 128000}, meta)

		meta, err = fetchOpenAIModelMetadata(context.Background(), "openai/fallback-2025-04-14")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{Exists: true, ContextLength: 8192}, meta)

		meta, err = fetchOpenAIModelMetadata(context.Background(), "openai/absent")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)

		meta, err = fetchOpenAIModelMetadataCandidate(context.Background(), "bad-window")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{Exists: true, ContextLength: 123456}, meta)
	})

	t.Run("read_error", func(t *testing.T) {
		originalHTTPClient := httpClient
		originalModelDocsBaseURL := modelDocsBaseURL
		t.Cleanup(func() {
			httpClient = originalHTTPClient
			modelDocsBaseURL = originalModelDocsBaseURL
		})

		httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(errReader{}),
				Header:     make(http.Header),
			}, nil
		})}
		modelDocsBaseURL = "https://developers.openai.com/api/docs/models"

		meta, err := fetchOpenAIModelMetadataCandidate(context.Background(), "gpt-4o-mini")
		require.EqualError(t, err, "forced read error")
		require.Equal(t, ModelMetadata{}, meta)
	})
}

func TestOpenAIModelCandidatesAndSnapshotTrim(t *testing.T) {
	require.Nil(t, openAIModelDocCandidates(""))
	require.False(t, isValidModelSlug(""))
	require.Equal(t, []string{"gpt-4.1-2025-04-14", "gpt-4.1"},
		openAIModelDocCandidates("openai/gpt-4.1-2025-04-14"))
	require.Equal(t, "gpt-4.1", trimOpenAISnapshotSuffix("gpt-4.1-2025-04-14"))
	require.Equal(t, "gpt-4.1-preview", trimOpenAISnapshotSuffix("gpt-4.1-preview"))
	require.Equal(t, "gpt-4.1-2025-4-14", trimOpenAISnapshotSuffix("gpt-4.1-2025-4-14"))
	require.Equal(t, "gpt-4.1-202x-04-14", trimOpenAISnapshotSuffix("gpt-4.1-202x-04-14"))
	require.Equal(t, "gpt-4.1-2025-0x-14", trimOpenAISnapshotSuffix("gpt-4.1-2025-0x-14"))
	require.Equal(t, "gpt-4.1-2025-04-1x", trimOpenAISnapshotSuffix("gpt-4.1-2025-04-1x"))
}

func TestFetchOpenAIModelMetadata_ReturnsCandidateError(t *testing.T) {
	originalModelDocsBaseURL := modelDocsBaseURL
	t.Cleanup(func() {
		modelDocsBaseURL = originalModelDocsBaseURL
	})

	modelDocsBaseURL = "://bad"

	meta, err := fetchOpenAIModelMetadata(context.Background(), "openai/gpt-4o-mini")
	require.Error(t, err)
	require.Equal(t, ModelMetadata{}, meta)
}

func TestFetchOpenAIModelMetadataCandidate_ReturnsContextParseError(t *testing.T) {
	originalHTTPClient := httpClient
	originalModelDocsBaseURL := modelDocsBaseURL
	t.Cleanup(func() {
		httpClient = originalHTTPClient
		modelDocsBaseURL = originalModelDocsBaseURL
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><p>999999999999999999999999999999 context window</p></html>`))
	}))
	t.Cleanup(server.Close)

	httpClient = server.Client()
	modelDocsBaseURL = server.URL + "/api/docs/models"

	meta, err := fetchOpenAIModelMetadataCandidate(context.Background(), "overflow")
	require.Error(t, err)
	require.Equal(t, ModelMetadata{}, meta)
}

func TestFetchOpenAIModelMetadataCandidate_EmptyModel(t *testing.T) {
	meta, err := fetchOpenAIModelMetadataCandidate(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, ModelMetadata{}, meta)
}

func TestNormalizeRulePaths_EmptyInput(t *testing.T) {
	require.Empty(t, normalizeRulePaths(nil))
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("forced read error")
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
  provider: openai
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
  provider: openai
  key: config-key
  apiMode: completions
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
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openai",
		ModelAPIMode:  "invalid",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
	}).Validate()
	require.EqualError(t, err, "model api mode must be one of: completions, responses; use --model.api-mode")
}

func TestConfig_ValidateAllowsResponsesModeWithOpenRouter(t *testing.T) {
	err := (&Config{
		Name:          "test-agent",
		Model:         defaultModel,
		ModelProvider: "openrouter",
		ModelAPIMode:  "responses",
		ModelKey:      "test-key",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
		VerifyModel:   new(false),
	}).Validate()
	require.NoError(t, err)
}

func TestLoad_UsesDebugTraceSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t, "DEBUG_TRACES", "DEBUG_TRACE_DIR")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  provider: openai
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
  provider: openai
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
	require.Equal(t, datadir.DebugTraceDir(), cfg.DebugTraceDir)
}

func TestConfig_NormalizeDefaultsDebugTraceDirFromHandHome(t *testing.T) {
	clearEnvKeys(t, "HAND_HOME")
	t.Setenv("HAND_HOME", "/tmp/hand-home")
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, "/tmp/hand-home/traces", cfg.DebugTraceDir)
}

func TestLoad_UsesFilesystemRootsAndExecRulesFromConfig(t *testing.T) {
	clearEnvKeys(t, "AGENT_FS_ROOTS", "AGENT_EXEC_ALLOW", "AGENT_EXEC_ASK", "AGENT_EXEC_DENY")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  provider: openai
  key: config-key
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
fs:
  roots:
    - .
    - ./nested
exec:
  allow:
    - git status
  ask:
    - git push
  deny:
    - git reset --hard
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(dir),
		filepath.Join(dir, "nested"),
	}, cfg.FSRoots)
	require.Equal(t, []string{"git status"}, cfg.ExecAllow)
	require.Equal(t, []string{"git push"}, cfg.ExecAsk)
	require.Equal(t, []string{"git reset --hard"}, cfg.ExecDeny)
}

func TestLoad_UsesFilesystemRootsAndExecRulesFromEnv(t *testing.T) {
	clearEnvKeys(t, "AGENT_FS_ROOTS", "AGENT_EXEC_ALLOW", "AGENT_EXEC_ASK", "AGENT_EXEC_DENY")
	dir := t.TempDir()
	t.Chdir(dir)
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("AGENT_FS_ROOTS=.,./nested\nAGENT_EXEC_ALLOW=git status\nAGENT_EXEC_ASK=git push\nAGENT_EXEC_DENY=git reset --hard\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: config-model
  provider: openai
  key: config-key
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(dir),
		filepath.Join(dir, "nested"),
	}, cfg.FSRoots)
	require.Equal(t, []string{"git status"}, cfg.ExecAllow)
	require.Equal(t, []string{"git push"}, cfg.ExecAsk)
	require.Equal(t, []string{"git reset --hard"}, cfg.ExecDeny)
}

func TestLoad_UsesSessionSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t, "AGENT_STORAGE_BACKEND", "AGENT_SESSION_DEFAULT_IDLE_EXPIRY", "AGENT_SESSION_ARCHIVE_RETENTION")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
storage:
  backend: memory
session:
  defaultIdleExpiry: 2h
  archiveRetention: 168h
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.Equal(t, "memory", cfg.StorageBackend)
	require.Equal(t, 2*time.Hour, cfg.SessionDefaultIdleExpiry)
	require.Equal(t, 168*time.Hour, cfg.SessionArchiveRetention)
}

func TestConfig_NormalizeDefaultsSessionSettings(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, "sqlite", cfg.StorageBackend)
	require.Equal(t, 24*time.Hour, cfg.SessionDefaultIdleExpiry)
	require.Equal(t, 30*24*time.Hour, cfg.SessionArchiveRetention)
}

func TestConfig_ValidateRejectsInvalidSessionSettings(t *testing.T) {
	cfg := &Config{
		Name:                     "daemon",
		Model:                    "openai/model",
		ModelProvider:            "openrouter",
		ModelKey:                 "key",
		ModelBaseURL:             "https://example.com",
		ModelAPIMode:             DefaultModelAPIMode,
		RPCAddress:               "127.0.0.1",
		RPCPort:                  50051,
		MaxIterations:            1,
		LogLevel:                 "info",
		StorageBackend:           "bogus",
		SessionDefaultIdleExpiry: 0,
		SessionArchiveRetention:  0,
	}

	err := cfg.Validate()
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func TestConfig_NormalizeDefaultsFilesystemRootsToCWD(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, []string{dir}, cfg.FSRoots)
}

func TestLoad_UsesCompactionSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t, "MODEL_CONTEXT_LENGTH", "AGENT_COMPACTION_ENABLED", "AGENT_COMPACTION_TRIGGER_PERCENT", "AGENT_COMPACTION_WARN_PERCENT")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
model:
  contextLength: 64000
compaction:
  enabled: false
  triggerPercent: 0.7
  warnPercent: 0.9
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 64000, cfg.ContextLength)
	require.False(t, boolValue(cfg.CompactionEnabled))
	require.Equal(t, 0.7, cfg.CompactionTriggerPercent)
	require.Equal(t, 0.9, cfg.CompactionWarnPercent)
}

func TestConfig_NormalizeDefaultsCompactionSettings(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, defaultContextLength, cfg.ContextLength)
	require.True(t, boolValue(cfg.CompactionEnabled))
	require.Equal(t, 0.85, cfg.CompactionTriggerPercent)
	require.Equal(t, 0.95, cfg.CompactionWarnPercent)
}

func TestConfig_ValidateRejectsInvalidCompactionSettings(t *testing.T) {
	cfg := &Config{
		Name:                     "daemon",
		Model:                    "openai/model",
		ContextLength:            128000,
		ModelProvider:            "openrouter",
		ModelKey:                 "key",
		ModelBaseURL:             "https://example.com",
		ModelAPIMode:             DefaultModelAPIMode,
		RPCAddress:               "127.0.0.1",
		RPCPort:                  50051,
		MaxIterations:            1,
		LogLevel:                 "info",
		StorageBackend:           "memory",
		SessionDefaultIdleExpiry: time.Hour,
		SessionArchiveRetention:  24 * time.Hour,
		CompactionEnabled:        new(true),
		CompactionTriggerPercent: 0.96,
		CompactionWarnPercent:    0.95,
	}

	err := cfg.Validate()
	require.EqualError(t, err, "compaction warn percent must be greater than or equal to compaction trigger percent")
}
