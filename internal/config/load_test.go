package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/internal/constants"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

func TestPreloadEnvFile_LoadsValues(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
		"HAND_MODEL_BASE_URL", "HAND_MODEL_API", "HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS", "HAND_LOG_LEVEL",
		"HAND_LOG_NO_COLOR", "HAND_DEBUG_REQUESTS", "HAND_RULES_FILES", "HAND_SESSION_INSTRUCT", "HAND_PLATFORM", "HAND_CAP_FS", "HAND_CAP_NET",
		"HAND_CAP_EXEC", "HAND_CAP_MEM", "HAND_CAP_BROWSER", "HAND_MEMORY_ENABLED", "HAND_MEMORY_PROVIDER", "HAND_MEMORY_BACKEND",
		"HAND_MEMORY_PINNED_ENABLED", "HAND_MEMORY_PINNED_MAX_CHARS", "HAND_MEMORY_PINNED_MAX_ITEM_CHARS")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_NAME=env-agent
HAND_MODEL=env-model
HAND_MODEL_PROVIDER=openrouter
OPENAI_API_KEY=openai-env-key
OPENROUTER_API_KEY=openrouter-env-key
HAND_MODEL_BASE_URL=https://env.example/v1
HAND_RPC_ADDRESS=0.0.0.0
HAND_RPC_PORT=6000
HAND_SESSION_MAX_ITERATIONS=45
HAND_LOG_LEVEL=warn
HAND_LOG_NO_COLOR=true
HAND_DEBUG_REQUESTS=true
HAND_RULES_FILES=hand.md,custom.md
HAND_SESSION_INSTRUCT=be terse
HAND_PLATFORM=cli
HAND_CAP_FS=false
HAND_CAP_NET=false
HAND_CAP_EXEC=false
HAND_CAP_MEM=false
HAND_CAP_BROWSER=true
HAND_MEMORY_ENABLED=true
HAND_MEMORY_PROVIDER=default-memory
HAND_MEMORY_BACKEND=memory
HAND_MEMORY_PINNED_ENABLED=false
HAND_MEMORY_PINNED_MAX_CHARS=2000
HAND_MEMORY_PINNED_MAX_ITEM_CHARS=500
`), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "env-agent", os.Getenv("HAND_NAME"))
	require.Equal(t, "env-model", os.Getenv("HAND_MODEL"))
	require.Equal(t, "openrouter", os.Getenv("HAND_MODEL_PROVIDER"))
	require.Equal(t, "openai-env-key", os.Getenv("OPENAI_API_KEY"))
	require.Equal(t, "openrouter-env-key", os.Getenv("OPENROUTER_API_KEY"))
	require.Equal(t, "https://env.example/v1", os.Getenv("HAND_MODEL_BASE_URL"))
	require.Equal(t, "0.0.0.0", os.Getenv("HAND_RPC_ADDRESS"))
	require.Equal(t, "6000", os.Getenv("HAND_RPC_PORT"))
	require.Equal(t, "45", os.Getenv("HAND_SESSION_MAX_ITERATIONS"))
	require.Equal(t, "warn", os.Getenv("HAND_LOG_LEVEL"))
	require.Equal(t, "true", os.Getenv("HAND_LOG_NO_COLOR"))
	require.Equal(t, "true", os.Getenv("HAND_DEBUG_REQUESTS"))
	require.Equal(t, "hand.md,custom.md", os.Getenv("HAND_RULES_FILES"))
	require.Equal(t, "be terse", os.Getenv("HAND_SESSION_INSTRUCT"))
	require.Equal(t, "cli", os.Getenv("HAND_PLATFORM"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_FS"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_NET"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_EXEC"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_MEM"))
	require.Equal(t, "true", os.Getenv("HAND_CAP_BROWSER"))
	require.Equal(t, "true", os.Getenv("HAND_MEMORY_ENABLED"))
	require.Equal(t, "default-memory", os.Getenv("HAND_MEMORY_PROVIDER"))
	require.Equal(t, "memory", os.Getenv("HAND_MEMORY_BACKEND"))
	require.Equal(t, "false", os.Getenv("HAND_MEMORY_PINNED_ENABLED"))
	require.Equal(t, "2000", os.Getenv("HAND_MEMORY_PINNED_MAX_CHARS"))
	require.Equal(t, "500", os.Getenv("HAND_MEMORY_PINNED_MAX_ITEM_CHARS"))
}

func TestPreloadEnvFile_DoesNotOverrideShellEnv(t *testing.T) {
	clearEnvKeys(t, "OPENROUTER_API_KEY")
	t.Setenv("OPENROUTER_API_KEY", "shell-key")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("OPENROUTER_API_KEY=env-key\n"), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "shell-key", os.Getenv("OPENROUTER_API_KEY"))
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

func TestConfig_ToYAMLAndSaveYAML(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Name = "alpha"
	path := filepath.Join(t.TempDir(), "profile", "config.yaml")

	require.NoError(t, SaveYAML(path, cfg))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	loaded, err := loadConfigFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha", loaded.Name)
	require.Equal(t, cfg.Models.Main.Name, loaded.Models.Main.Name)
	require.True(t, getBoolValue(loaded.Safety.Input))
	require.True(t, getBoolValue(loaded.Safety.Output))
	require.False(t, getBoolValue(loaded.Safety.PII))
	require.Equal(t, map[string]RerankerOverrideConfig{
		"memory_episodic_extraction": {Type: constants.RerankerLLM},
		"memory_promotion":           {Type: constants.RerankerLLM},
		"memory_reflection":          {Type: constants.RerankerLLM},
	}, loaded.Reranker.Overrides)
}

func TestSaveYAML_RefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: existing\n"), 0o600))

	cfg := NewDefaultConfig()
	cfg.Name = "alpha"
	err := SaveYAML(path, cfg)

	require.EqualError(t, err, "config file already exists: "+path)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "name: existing\n", string(data))
}

func TestSaveYAML_ReturnsValidationErrors(t *testing.T) {
	err := SaveYAML("", NewDefaultConfig())
	require.EqualError(t, err, "config path is required")

	err = SaveYAML(filepath.Join(t.TempDir(), "config.yaml"), nil)
	require.EqualError(t, err, "config is required")
}

func TestLoad_UsesConfigFileValues(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "OPENAI_API_KEY",
		"OPENROUTER_API_KEY",
		"HAND_MODEL_BASE_URL", "HAND_MODEL_API", "HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR",
		"HAND_MODEL_MAX_RETRIES",
		"HAND_WEB_PROVIDER", "HAND_WEB_API_KEY", "HAND_WEB_BASE_URL", "HAND_WEB_MAX_CHAR_PER_RESULT",
		"HAND_WEB_MAX_EXTRACT_CHAR_PER_RESULT", "HAND_WEB_MAX_EXTRACT_RESPONSE_BYTES",
		"HAND_WEB_CACHE_TTL", "HAND_WEB_BLOCKED_DOMAINS_ENABLED", "HAND_WEB_BLOCKED_DOMAINS",
		"HAND_WEB_BLOCKED_DOMAIN_FILES", "HAND_WEB_NATIVE_ALLOWED_HOSTS", "HAND_WEB_NATIVE_BLOCKED_HOSTS",
		"HAND_WEB_NATIVE_ALLOWED_HOST_FILES", "HAND_WEB_NATIVE_BLOCKED_HOST_FILES",
		"HAND_WEB_EXTRACT_MIN_SUMMARIZE_CHARS", "HAND_WEB_EXTRACT_MAX_SUMMARY_CHARS",
		"HAND_WEB_EXTRACT_MAX_SUMMARY_CHUNK_CHARS", "HAND_WEB_EXTRACT_REFUSAL_THRESHOLD_CHARS",
		"HAND_DEBUG_REQUESTS", "HAND_RULES_FILES", "HAND_SESSION_INSTRUCT", "HAND_PLATFORM", "HAND_CAP_FS",
		"HAND_CAP_NET", "HAND_CAP_EXEC", "HAND_CAP_MEM", "HAND_CAP_BROWSER", "HAND_MEMORY_ENABLED", "HAND_MEMORY_PROVIDER",
		"HAND_MEMORY_BACKEND",
		"HAND_MEMORY_PINNED_ENABLED", "HAND_MEMORY_PINNED_MAX_CHARS", "HAND_MEMORY_PINNED_MAX_ITEM_CHARS",
		"HAND_MEMORY_REFLECTION_ENABLED", "HAND_MEMORY_REFLECTION_INTERVAL",
		"HAND_MEMORY_REFLECTION_LIMIT", "HAND_MEMORY_REFLECTION_RELATED_LIMIT",
		"HAND_MEMORY_PROMOTION_ENABLED", "HAND_MEMORY_PROMOTION_INTERVAL",
		"HAND_MEMORY_PROMOTION_LIMIT")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  maxRetries: 4
  main:
    name: config-model
    provider: openrouter
    apiKey: config-key
    baseUrl: https://config.example/v1
rpc:
  address: 0.0.0.0
  port: 6000
session:
  maxIterations: 45
  instruct: be terse
cap:
  fs: false
  net: false
  exec: false
  mem: false
  browser: true
platform: cli
memory:
  enabled: true
  provider: default-memory
  backend: memory
  pinned:
    enabled: false
    maxChars: 2000
    maxItemChars: 500
  episodic:
    enabled: true
    interval: 30m
    idleAfter: 15m
    minMessages: 3
    windowSize: 12
    maxWindows: 4
    maxWindowChars: 5000
    maxWindowTokens: 1250
    maxRetries: 2
  reflection:
    enabled: true
    interval: 4m
    limit: 6
    relatedLimit: 2
  promotion:
    enabled: true
    interval: 2m
    limit: 7
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
  maxExtractResponseBytes: 2048
  cacheTTL: 15m
  blockedDomains:
    enabled: true
    domains:
      - blocked.example
    files:
      - blocked.txt
  native:
    allowedHosts:
      - allowed.example
    blockedHosts:
      - blocked.example
    allowedHostFiles:
      - allow.txt
    blockedHostFiles:
      - deny.txt
  extractMinSummarizeChars: 12000
  extractMaxSummaryChars: 3000
  extractMaxSummaryChunkChars: 60000
  extractRefusalThresholdChars: 180000
rules:
  files:
    - hand.md
    - custom.md
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.Equal(t, "config-agent", cfg.Name)
	require.Equal(t, "config-model", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "config-key", cfg.Models.Main.APIKey)
	require.Equal(t, "https://config.example/v1", cfg.Models.Main.BaseURL)
	require.Equal(t, 4, cfg.ModelMaxRetriesEffective())
	require.Equal(t, "0.0.0.0", cfg.RPC.Address)
	require.Equal(t, 6000, cfg.RPC.Port)
	require.Equal(t, 45, cfg.Session.MaxIterations)
	require.Equal(t, "error", cfg.Log.Level)
	require.True(t, cfg.Log.NoColor)
	require.True(t, cfg.Debug.Requests)
	require.Equal(t, "exa", cfg.Web.Provider)
	require.Equal(t, "web-key", cfg.Web.APIKey)
	require.Equal(t, "https://web.example", cfg.Web.BaseURL)
	require.Equal(t, 2400, cfg.Web.MaxCharPerResult)
	require.Equal(t, 9600, cfg.Web.MaxExtractCharPerResult)
	require.Equal(t, 2048, cfg.Web.MaxExtractResponseBytes)
	require.Equal(t, 15*time.Minute, cfg.Web.CacheTTL)
	require.True(t, cfg.Web.BlockedDomainsEnabled)
	require.Equal(t, []string{"blocked.example"}, cfg.Web.BlockedDomains)
	require.Equal(t, []string{filepath.Join(dir, "blocked.txt")}, cfg.Web.BlockedDomainFiles)
	require.Equal(t, []string{"allowed.example"}, cfg.Web.NativeAllowedHosts)
	require.Equal(t, []string{"blocked.example"}, cfg.Web.NativeBlockedHosts)
	require.Equal(t, []string{filepath.Join(dir, "allow.txt")}, cfg.Web.NativeAllowedHostFiles)
	require.Equal(t, []string{filepath.Join(dir, "deny.txt")}, cfg.Web.NativeBlockedHostFiles)
	require.Equal(t, 12000, cfg.Web.ExtractMinSummarizeChars)
	require.Equal(t, 3000, cfg.Web.ExtractMaxSummaryChars)
	require.Equal(t, 60000, cfg.Web.ExtractMaxSummaryChunkChars)
	require.Equal(t, 180000, cfg.Web.ExtractRefusalThresholdChars)
	require.Equal(t, []string{"hand.md", "custom.md"}, cfg.Rules.Files)
	require.Equal(t, "be terse", cfg.Session.Instruct)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, cfg.MemoryEnabled())
	require.Equal(t, "default-memory", cfg.Memory.Provider)
	require.Equal(t, "memory", cfg.Memory.Backend)
	require.False(t, getBoolValue(cfg.Memory.Pinned.Enabled))
	require.Equal(t, 2000, cfg.Memory.Pinned.MaxChars)
	require.Equal(t, 500, cfg.Memory.Pinned.MaxItemChars)
	require.True(t, getBoolValue(cfg.Memory.Episodic.Enabled))
	require.Equal(t, 30*time.Minute, cfg.Memory.Episodic.Interval)
	require.Equal(t, 15*time.Minute, cfg.Memory.Episodic.IdleAfter)
	require.Equal(t, 3, cfg.Memory.Episodic.MinMessages)
	require.Equal(t, 12, cfg.Memory.Episodic.WindowSize)
	require.Equal(t, 4, cfg.Memory.Episodic.MaxWindows)
	require.Equal(t, 5000, cfg.Memory.Episodic.MaxWindowChars)
	require.Equal(t, 1250, cfg.Memory.Episodic.MaxWindowTokens)
	require.Equal(t, 2, cfg.Memory.Episodic.MaxRetries)
	require.True(t, getBoolValue(cfg.Memory.Reflection.Enabled))
	require.Equal(t, 4*time.Minute, cfg.Memory.Reflection.Interval)
	require.Equal(t, 6, cfg.Memory.Reflection.Limit)
	require.Equal(t, 2, cfg.Memory.Reflection.RelatedLimit)
	require.True(t, getBoolValue(cfg.Memory.Promotion.Enabled))
	require.Equal(t, 2*time.Minute, cfg.Memory.Promotion.Interval)
	require.Equal(t, 7, cfg.Memory.Promotion.Limit)
	require.False(t, getBoolValue(cfg.Cap.Filesystem))
	require.False(t, getBoolValue(cfg.Cap.Network))
	require.False(t, getBoolValue(cfg.Cap.Exec))
	require.False(t, getBoolValue(cfg.Cap.Memory))
	require.True(t, getBoolValue(cfg.Cap.Browser))
}

func TestLoad_PreservesSmallerConfiguredContextLengthThanRegistryMetadata(t *testing.T) {
	stubModelRegistry(t, registryWithGenerationModel(constants.ModelProviderOpenAI, "openai/test-chat-small", 8191))

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openai:
      apiKey: config-key
  main:
    name: openai/test-chat-small
    provider: openai
    contextLength: 4000
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 4000, cfg.Models.Main.ContextLength)
}

func TestLoad_IgnoresMissingConfigFile(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "OPENAI_API_KEY",
		"OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL", "HAND_MODEL_API", "HAND_MODEL_MAX_RETRIES",
		"HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_DEBUG_REQUESTS")

	cfg, err := Load("", filepath.Join(t.TempDir(), "missing.yaml"))

	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, DefaultConfig.Models.Main.Name, cfg.Models.Main.Name)
	require.Equal(t, DefaultConfig.Models.Main.Provider, cfg.Models.Main.Provider)
	require.Equal(t, constants.DefaultRPCAddress, cfg.RPC.Address)
	require.Equal(t, constants.DefaultRPCPort, cfg.RPC.Port)
	require.Equal(t, DefaultConfig.Session.MaxIterations, cfg.Session.MaxIterations)
	require.Equal(t, DefaultConfig.Platform, cfg.Platform)
	require.True(t, getBoolValue(cfg.Cap.Filesystem))
	require.True(t, getBoolValue(cfg.Cap.Network))
	require.True(t, getBoolValue(cfg.Cap.Exec))
	require.True(t, getBoolValue(cfg.Cap.Memory))
	require.False(t, getBoolValue(cfg.Cap.Browser))
	require.Equal(t, DefaultConfig.Log.Level, cfg.Log.Level)
}

func TestLoad_DefaultsOmittedMainAPIToSelectedProvider(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL_API", "HAND_MODEL_PROVIDER", "HAND_MODEL_BASE_URL")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, nil, 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: anthropic-agent
models:
  providers:
    anthropic:
      apiKey: test-key
  main:
    name: claude-sonnet-4-5
    provider: anthropic
    stream: true
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.Equal(t, modelprovider.APIAnthropicMessages, cfg.Models.Main.API)
	require.Equal(t, constants.DefaultAnthropicBaseURL, cfg.Models.Main.BaseURL)
}

func TestLoad_PreservesExplicitMainAPIForValidation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
models:
  main:
    provider: anthropic
    api: openai-responses
`), 0o600))

	cfg, err := loadConfigFile(configPath)

	require.NoError(t, err)
	require.Equal(t, modelprovider.APIOpenAIResponses, cfg.Models.Main.API)
}

func TestLoadStrictRejectsMissingGatewayCredentials(t *testing.T) {
	clearEnvKeys(t, "OPENROUTER_API_KEY")

	configPath := writeLoadGatewayConfig(t, "")

	_, err := LoadStrict("", configPath)

	require.EqualError(t, err, "gateway telegram bot token is required when telegram gateway is enabled; "+
		"set HAND_GATEWAY_TELEGRAM_BOT_TOKEN, provide it in config, or use --gateway.telegram.bot-token")
}

func TestLoadRelaxedSkipsMissingGatewayCredentials(t *testing.T) {
	clearEnvKeys(t, "OPENROUTER_API_KEY")

	configPath := writeLoadGatewayConfig(t, "")

	cfg, err := LoadRelaxed("", configPath)

	require.NoError(t, err)
	require.True(t, cfg.Gateway.Telegram.Enabled)
	require.Empty(t, cfg.Gateway.Telegram.BotToken)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Models.Main.Name)
}

func TestLoad_ReturnsErrorForInvalidConfigFile(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "OPENAI_API_KEY",
		"OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL", "HAND_MODEL_API", "HAND_MODEL_MAX_RETRIES",
		"HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_DEBUG_REQUESTS")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: [\n"), 0o600))

	_, err := Load("", configPath)

	require.Error(t, err)
	require.Contains(t, err.Error(), `failed to parse config file`)
}

func writeLoadGatewayConfig(t *testing.T, telegramBotToken string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: test-agent
models:
  providers:
    openrouter:
      apiKey: router-key
  main:
    provider: openrouter
    name: openai/gpt-4o-mini
gateway:
  enabled: true
  telegram:
    enabled: true
    mode: polling
    botToken: "`+telegramBotToken+`"
search:
  vector:
    enabled: false
`), 0o600))

	return configPath
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
	require.Equal(t, constants.DefaultName, cfg.Name)
	require.Equal(t, DefaultConfig.Models.Main.Name, cfg.Models.Main.Name)
	require.Equal(t, DefaultConfig.Log.Level, cfg.Log.Level)
	require.False(t, cfg.Log.NoColor)
	require.Equal(t, DefaultConfig.Models.Main.Provider, cfg.Models.Main.Provider)
	require.Equal(t, DefaultConfig.Models.Main.BaseURL, cfg.Models.Main.BaseURL)
	require.Equal(t, constants.DefaultRPCAddress, cfg.RPC.Address)
	require.Equal(t, constants.DefaultRPCPort, cfg.RPC.Port)
	require.Equal(t, DefaultConfig.Session.MaxIterations, cfg.Session.MaxIterations)
	require.Equal(t, DefaultConfig.Platform, cfg.Platform)
	require.True(t, getBoolValue(cfg.Cap.Filesystem))
	require.True(t, getBoolValue(cfg.Cap.Network))
	require.True(t, getBoolValue(cfg.Cap.Exec))
	require.True(t, getBoolValue(cfg.Cap.Memory))
	require.False(t, getBoolValue(cfg.Cap.Browser))
}

func TestSet_StoresConfigGlobally(t *testing.T) {
	original := Get()
	t.Cleanup(func() {
		Set(original)
	})

	cfg := &Config{
		Name: "Test Agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openai": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: "test-model", Provider: "openai"},
		},
		Log: LogConfig{Level: "debug"},
	}
	Set(cfg)
	require.Same(t, cfg, Get())
}
