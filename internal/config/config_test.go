package config

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/rerank"
)

func stubModelMetadataResolver(t *testing.T, fn func(context.Context, *Config, ModelAuth) (ModelMetadata, error)) {
	t.Helper()

	original := resolveModelMeta
	resolveModelMeta = fn
	t.Cleanup(func() {
		resolveModelMeta = original
	})
}

func stubProviderDefaultBaseURL(t *testing.T, provider string, mode string, value string) {
	t.Helper()

	original := providerDefaultBaseURLs[provider][mode]
	providerDefaultBaseURLs[provider][mode] = value
	t.Cleanup(func() {
		providerDefaultBaseURLs[provider][mode] = original
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestPreloadEnvFile_LoadsValues(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_OPENAI_API_KEY", "HAND_OPENROUTER_API_KEY",
		"HAND_MODEL_BASE_URL", "HAND_MODEL_API_MODE", "HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS", "HAND_LOG_LEVEL",
		"HAND_LOG_NO_COLOR", "HAND_DEBUG_REQUESTS", "HAND_RULES_FILES", "HAND_SESSION_INSTRUCT", "HAND_PLATFORM", "HAND_CAP_FS", "HAND_CAP_NET",
		"HAND_CAP_EXEC", "HAND_CAP_MEM", "HAND_CAP_BROWSER", "HAND_MEMORY_ENABLED", "HAND_MEMORY_PROVIDER")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_NAME=env-agent
HAND_MODEL=env-model
HAND_MODEL_PROVIDER=openrouter
HAND_MODEL_KEY=env-key
HAND_OPENAI_API_KEY=openai-env-key
HAND_OPENROUTER_API_KEY=openrouter-env-key
HAND_MODEL_BASE_URL=https://env.example/v1
HAND_RPC_ADDRESS=0.0.0.0
HAND_RPC_PORT=6000
HAND_SESSION_MAX_ITERATIONS=45
HAND_LOG_LEVEL=warn
HAND_LOG_NO_COLOR=true
HAND_DEBUG_REQUESTS=true
HAND_RULES_FILES=hand.md,custom.md
HAND_SESSION_INSTRUCT=be terse
HAND_PLATFORM=desktop
HAND_CAP_FS=false
HAND_CAP_NET=false
HAND_CAP_EXEC=false
HAND_CAP_MEM=false
HAND_CAP_BROWSER=true
HAND_MEMORY_ENABLED=true
HAND_MEMORY_PROVIDER=memory
`), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "env-agent", os.Getenv("HAND_NAME"))
	require.Equal(t, "env-model", os.Getenv("HAND_MODEL"))
	require.Equal(t, "openrouter", os.Getenv("HAND_MODEL_PROVIDER"))
	require.Equal(t, "env-key", os.Getenv("HAND_MODEL_KEY"))
	require.Equal(t, "openai-env-key", os.Getenv("HAND_OPENAI_API_KEY"))
	require.Equal(t, "openrouter-env-key", os.Getenv("HAND_OPENROUTER_API_KEY"))
	require.Equal(t, "https://env.example/v1", os.Getenv("HAND_MODEL_BASE_URL"))
	require.Equal(t, "0.0.0.0", os.Getenv("HAND_RPC_ADDRESS"))
	require.Equal(t, "6000", os.Getenv("HAND_RPC_PORT"))
	require.Equal(t, "45", os.Getenv("HAND_SESSION_MAX_ITERATIONS"))
	require.Equal(t, "warn", os.Getenv("HAND_LOG_LEVEL"))
	require.Equal(t, "true", os.Getenv("HAND_LOG_NO_COLOR"))
	require.Equal(t, "true", os.Getenv("HAND_DEBUG_REQUESTS"))
	require.Equal(t, "hand.md,custom.md", os.Getenv("HAND_RULES_FILES"))
	require.Equal(t, "be terse", os.Getenv("HAND_SESSION_INSTRUCT"))
	require.Equal(t, "desktop", os.Getenv("HAND_PLATFORM"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_FS"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_NET"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_EXEC"))
	require.Equal(t, "false", os.Getenv("HAND_CAP_MEM"))
	require.Equal(t, "true", os.Getenv("HAND_CAP_BROWSER"))
	require.Equal(t, "true", os.Getenv("HAND_MEMORY_ENABLED"))
	require.Equal(t, "memory", os.Getenv("HAND_MEMORY_PROVIDER"))
}

func TestPreloadEnvFile_DoesNotOverrideShellEnv(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL_KEY")
	t.Setenv("HAND_MODEL_KEY", "shell-key")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_MODEL_KEY=env-key\n"), 0o600))

	require.NoError(t, PreloadEnvFile(envPath))
	require.Equal(t, "shell-key", os.Getenv("HAND_MODEL_KEY"))
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
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_OPENAI_API_KEY",
		"HAND_OPENROUTER_API_KEY",
		"HAND_MODEL_BASE_URL", "HAND_MODEL_API_MODE", "HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS",
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
		"HAND_CAP_NET", "HAND_CAP_EXEC", "HAND_CAP_MEM", "HAND_CAP_BROWSER", "HAND_MEMORY_ENABLED", "HAND_MEMORY_PROVIDER")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  maxRetries: 4
  main:
    name: config-model
    provider: openrouter
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
platform: desktop
memory:
  enabled: true
  provider: memory
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
	require.Equal(t, "config-key", cfg.Models.Key)
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
	require.Equal(t, "desktop", cfg.Platform)
	require.True(t, cfg.MemoryEnabled())
	require.Equal(t, "memory", cfg.Memory.Provider)
	require.False(t, boolValue(cfg.Cap.Filesystem))
	require.False(t, boolValue(cfg.Cap.Network))
	require.False(t, boolValue(cfg.Cap.Exec))
	require.False(t, boolValue(cfg.Cap.Memory))
	require.True(t, boolValue(cfg.Cap.Browser))
}

func TestLoad_UsesEnvOverConfigFile(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_OPENAI_API_KEY",
		"HAND_OPENROUTER_API_KEY",
		"HAND_MODEL_BASE_URL", "HAND_MODEL_API_MODE", "HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS",
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
		"HAND_CAP_NET", "HAND_CAP_EXEC", "HAND_CAP_MEM", "HAND_CAP_BROWSER", "HAND_MEMORY_ENABLED", "HAND_MEMORY_PROVIDER")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_NAME=env-agent
HAND_MODEL=env-model
HAND_MODEL_PROVIDER=openrouter
HAND_MODEL_KEY=env-key
HAND_MODEL_BASE_URL=https://env.example/v1
HAND_MODEL_MAX_RETRIES=0
HAND_RPC_ADDRESS=127.0.0.1
HAND_RPC_PORT=7000
HAND_SESSION_MAX_ITERATIONS=55
HAND_LOG_LEVEL=warn
HAND_LOG_NO_COLOR=false
HAND_DEBUG_REQUESTS=false
HAND_WEB_PROVIDER=tavily
HAND_WEB_API_KEY=web-env-key
HAND_WEB_BASE_URL=https://env-web.example
HAND_WEB_MAX_CHAR_PER_RESULT=3100
HAND_WEB_MAX_EXTRACT_CHAR_PER_RESULT=12400
HAND_WEB_MAX_EXTRACT_RESPONSE_BYTES=4096
HAND_WEB_CACHE_TTL=30m
HAND_WEB_BLOCKED_DOMAINS_ENABLED=true
HAND_WEB_BLOCKED_DOMAINS=blocked.example,ads.example
HAND_WEB_BLOCKED_DOMAIN_FILES=blocked.txt,shared.txt
HAND_WEB_NATIVE_ALLOWED_HOSTS=allowed.example,docs.example
HAND_WEB_NATIVE_BLOCKED_HOSTS=blocked.example,raw.example
HAND_WEB_NATIVE_ALLOWED_HOST_FILES=allow.txt,safe.txt
HAND_WEB_NATIVE_BLOCKED_HOST_FILES=deny.txt,banned.txt
HAND_WEB_EXTRACT_MIN_SUMMARIZE_CHARS=13000
HAND_WEB_EXTRACT_MAX_SUMMARY_CHARS=3200
HAND_WEB_EXTRACT_MAX_SUMMARY_CHUNK_CHARS=70000
HAND_WEB_EXTRACT_REFUSAL_THRESHOLD_CHARS=190000
HAND_RULES_FILES=hand.md,custom.md
HAND_SESSION_INSTRUCT=be terse
HAND_PLATFORM=editor
HAND_CAP_FS=true
HAND_CAP_NET=true
HAND_CAP_EXEC=true
HAND_CAP_MEM=true
HAND_CAP_BROWSER=false
HAND_MEMORY_ENABLED=false
HAND_MEMORY_PROVIDER=noop
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  maxRetries: 4
  main:
    name: config-model
    provider: openrouter
    baseUrl: https://config.example/v1
rpc:
  address: 0.0.0.0
  port: 6000
session:
  maxIterations: 45
  instruct: be formal
web:
  provider: firecrawl
  apiKey: config-web-key
  baseUrl: https://config-web.example
  maxCharPerResult: 1800
  maxExtractCharPerResult: 7200
  maxExtractResponseBytes: 2048
  cacheTTL: 15m
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
	require.Equal(t, "env-model", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "env-key", cfg.Models.Key)
	require.Equal(t, "https://env.example/v1", cfg.Models.Main.BaseURL)
	require.Equal(t, 0, cfg.ModelMaxRetriesEffective())
	require.Equal(t, "127.0.0.1", cfg.RPC.Address)
	require.Equal(t, 7000, cfg.RPC.Port)
	require.Equal(t, 55, cfg.Session.MaxIterations)
	require.Equal(t, "warn", cfg.Log.Level)
	require.False(t, cfg.Log.NoColor)
	require.False(t, cfg.Debug.Requests)
	require.Equal(t, "tavily", cfg.Web.Provider)
	require.Equal(t, "web-env-key", cfg.Web.APIKey)
	require.Equal(t, "https://env-web.example", cfg.Web.BaseURL)
	require.Equal(t, 3100, cfg.Web.MaxCharPerResult)
	require.Equal(t, 12400, cfg.Web.MaxExtractCharPerResult)
	require.Equal(t, 4096, cfg.Web.MaxExtractResponseBytes)
	require.Equal(t, 30*time.Minute, cfg.Web.CacheTTL)
	require.True(t, cfg.Web.BlockedDomainsEnabled)
	require.Equal(t, []string{"blocked.example", "ads.example"}, cfg.Web.BlockedDomains)
	require.Equal(t, []string{"blocked.txt", "shared.txt"}, cfg.Web.BlockedDomainFiles)
	require.Equal(t, []string{"allowed.example", "docs.example"}, cfg.Web.NativeAllowedHosts)
	require.Equal(t, []string{"blocked.example", "raw.example"}, cfg.Web.NativeBlockedHosts)
	require.Equal(t, []string{"allow.txt", "safe.txt"}, cfg.Web.NativeAllowedHostFiles)
	require.Equal(t, []string{"deny.txt", "banned.txt"}, cfg.Web.NativeBlockedHostFiles)
	require.Equal(t, 13000, cfg.Web.ExtractMinSummarizeChars)
	require.Equal(t, 3200, cfg.Web.ExtractMaxSummaryChars)
	require.Equal(t, 70000, cfg.Web.ExtractMaxSummaryChunkChars)
	require.Equal(t, 190000, cfg.Web.ExtractRefusalThresholdChars)
	require.Equal(t, []string{"hand.md", "custom.md"}, cfg.Rules.Files)
	require.Equal(t, "be terse", cfg.Session.Instruct)
	require.Equal(t, "editor", cfg.Platform)
	require.True(t, boolValue(cfg.Cap.Filesystem))
	require.True(t, boolValue(cfg.Cap.Network))
	require.True(t, boolValue(cfg.Cap.Exec))
	require.True(t, boolValue(cfg.Cap.Memory))
	require.False(t, boolValue(cfg.Cap.Browser))
	require.False(t, cfg.MemoryEnabled())
	require.Equal(t, "noop", cfg.Memory.Provider)
}

func TestLoad_UsesModelStreamFromConfigAndEnv(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL_STREAM")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_MODEL_STREAM=true\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
models:
  main:
    stream: false
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.True(t, cfg.StreamEnabled())
}

func TestConfig_StreamEnabledDefaultsToTrue(t *testing.T) {
	require.True(t, (&Config{}).StreamEnabled())
	require.False(t, (&Config{Models: ModelsConfig{Main: MainModelConfig{Stream: new(false)}}}).StreamEnabled())
}

func TestLoad_UsesOpenRouterModelMetadataWhenContextLengthIsUnset(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 222222}, nil
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
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 222222, cfg.Models.Main.ContextLength)
}

func TestLoad_UsesProviderMetadataWhenConfiguredContextLengthIsTooLarge(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 64000}, nil
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: gpt-4.1-nano
    provider: openai
    contextLength: 999999
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 64000, cfg.Models.Main.ContextLength)
}

func TestLoad_PreservesSmallerConfiguredContextLengthThanProviderMetadata(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 128000}, nil
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: gpt-4.1-nano
    provider: openai
    contextLength: 32000
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 32000, cfg.Models.Main.ContextLength)
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
models:
  key: config-key
  verify: false
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.False(t, called)
	require.Equal(t, defaultContextLength, cfg.Models.Main.ContextLength)
	require.False(t, boolValueDefault(cfg.Models.Verify, true))
}

func TestConfig_NormalizeLeavesRulesFilesEmptyWhenUnset(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Empty(t, cfg.Rules.Files)
}

func TestConfig_NormalizeNormalizesRulesFiles(t *testing.T) {
	cfg := &Config{Rules: RulesConfig{Files: []string{" ./Hand.md ", "custom.md", "Hand.md", ""}}}
	cfg.Normalize()
	require.Equal(t, []string{"Hand.md", "custom.md"}, cfg.Rules.Files)
}

func TestConfig_NormalizeTrimsInstruct(t *testing.T) {
	cfg := &Config{Session: SessionConfig{Instruct: "  be terse  "}}
	cfg.Normalize()
	require.Equal(t, "be terse", cfg.Session.Instruct)
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
	clearEnvKeys(t, "HAND_SESSION_MAX_ITERATIONS", "HAND_MODEL_API_MODE")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_SESSION_MAX_ITERATIONS=invalid\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: config-model
    provider: openrouter
rpc:
  address: 127.0.0.1
  port: 50051
session:
  maxIterations: 45
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.Equal(t, 45, cfg.Session.MaxIterations)
}

func TestLoad_IgnoresMissingConfigFile(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_OPENAI_API_KEY",
		"HAND_OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL", "HAND_MODEL_API_MODE", "HAND_MODEL_MAX_RETRIES",
		"HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_DEBUG_REQUESTS")

	cfg, err := Load("", filepath.Join(t.TempDir(), "missing.yaml"))

	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, defaultModel, cfg.Models.Main.Name)
	require.Equal(t, defaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, "127.0.0.1", cfg.RPC.Address)
	require.Equal(t, 50051, cfg.RPC.Port)
	require.Equal(t, defaultMaxIterations, cfg.Session.MaxIterations)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, boolValue(cfg.Cap.Filesystem))
	require.True(t, boolValue(cfg.Cap.Network))
	require.True(t, boolValue(cfg.Cap.Exec))
	require.True(t, boolValue(cfg.Cap.Memory))
	require.False(t, boolValue(cfg.Cap.Browser))
	require.Equal(t, "info", cfg.Log.Level)
}

func TestLoad_ReturnsErrorForInvalidConfigFile(t *testing.T) {
	clearEnvKeys(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "HAND_MODEL_KEY", "HAND_OPENAI_API_KEY",
		"HAND_OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL", "HAND_MODEL_API_MODE", "HAND_MODEL_MAX_RETRIES",
		"HAND_RPC_ADDRESS", "HAND_RPC_PORT", "HAND_SESSION_MAX_ITERATIONS", "HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_DEBUG_REQUESTS")

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
	require.Equal(t, defaultModel, cfg.Models.Main.Name)
	require.Equal(t, "info", cfg.Log.Level)
	require.Empty(t, cfg.Models.Main.Provider)
	require.Empty(t, cfg.Models.Main.BaseURL)
	require.Empty(t, cfg.RPC.Address)
	require.Zero(t, cfg.RPC.Port)
	require.Equal(t, defaultMaxIterations, cfg.Session.MaxIterations)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, boolValue(cfg.Cap.Filesystem))
	require.True(t, boolValue(cfg.Cap.Network))
	require.True(t, boolValue(cfg.Cap.Exec))
	require.True(t, boolValue(cfg.Cap.Memory))
	require.False(t, boolValue(cfg.Cap.Browser))
}

func TestSet_StoresConfigGlobally(t *testing.T) {
	original := Get()
	t.Cleanup(func() {
		Set(original)
	})

	cfg := &Config{
		Name: "Test Agent",
		Models: ModelsConfig{
			Key:  "test-key",
			Main: MainModelConfig{Name: "test-model", Provider: "openai"},
		},
		Log: LogConfig{Level: "debug"},
	}
	Set(cfg)
	require.Same(t, cfg, Get())
}

func TestConfig_ValidateRequiresKey(t *testing.T) {
	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{Name: defaultModel}},
		Log:    LogConfig{Level: "info"},
	}
	require.EqualError(t, cfg.Validate(), "model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
	require.Equal(t, defaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.Models.Main.BaseURL)
}

func TestConfig_ValidateNilConfig(t *testing.T) {
	var cfg *Config
	require.EqualError(t, cfg.Validate(), "config is required")
}

func TestConfig_ResolveModelAuthUsesOpenRouterSpecificKey(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			OpenRouterAPIKey: "openrouter-key",
			Main:             MainModelConfig{Name: defaultModel, Provider: "openrouter"},
		},
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openrouter", auth.Provider)
	require.Equal(t, "openrouter-key", auth.APIKey)
	require.Equal(t, defaultBaseURLForProvider("openrouter", DefaultModelAPIMode), auth.BaseURL)
}

func TestConfig_ResolveModelAuthUsesOpenAISpecificKey(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			OpenAIAPIKey: "openai-key",
			Main:         MainModelConfig{Name: defaultModel, Provider: "openai"},
		},
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openai", auth.Provider)
	require.Equal(t, "openai-key", auth.APIKey)
	require.Equal(t, "https://api.openai.com/v1", auth.BaseURL)
}

func TestConfig_ResolveModelAuthAcceptsOpenAIProviderAlias(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			OpenAIAPIKey: "openai-key",
			Main:         MainModelConfig{Name: defaultModel, Provider: "openai"},
		},
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openai", auth.Provider)
	require.Equal(t, "openai-key", auth.APIKey)
	require.Equal(t, "https://api.openai.com/v1", auth.BaseURL)
}

func TestConfig_ResolveModelAuthFallsBackToModelKey(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:  "generic-key",
			Main: MainModelConfig{Name: defaultModel, Provider: "openrouter"},
		},
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "generic-key", auth.APIKey)
}

func TestConfig_ResolveEmbeddingModelAuth(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{
			OpenRouterAPIKey: "router-key",
			Main:             MainModelConfig{Provider: "openrouter"},
			Embedding:        EmbeddingModelConfig{Provider: "openrouter"},
		},
	}

	auth, err := cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider: "openrouter",
		APIKey:   "router-key",
		BaseURL:  "https://openrouter.ai/api/v1/embeddings",
	}, auth)

	cfg = &Config{
		Models: ModelsConfig{
			OpenRouterAPIKey: "router-key",
			Main:             MainModelConfig{Provider: "openrouter", APIMode: "responses"},
			Embedding:        EmbeddingModelConfig{Provider: "openrouter"},
		},
	}

	auth, err = cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, defaultBaseURLForProvider("openrouter", "embeddings"), auth.BaseURL)

	cfg = &Config{
		Models: ModelsConfig{
			OpenRouterAPIKey: "router-key",
			Main:             MainModelConfig{Provider: "openrouter"},
		},
	}

	auth, err = cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider: "openrouter",
		APIKey:   "router-key",
		BaseURL:  "https://openrouter.ai/api/v1/embeddings",
	}, auth)

	cfg = &Config{
		Models: ModelsConfig{
			OpenAIAPIKey: "openai-key",
			Main:         MainModelConfig{Provider: "openrouter"},
			Embedding:    EmbeddingModelConfig{Provider: "openai"},
		},
	}

	auth, err = cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider: "openai",
		APIKey:   "openai-key",
		BaseURL:  "https://api.openai.com/v1/embeddings",
	}, auth)

	_, err = (&Config{Models: ModelsConfig{Embedding: EmbeddingModelConfig{Provider: "openai"}}}).ResolveEmbeddingModelAuth()
	require.EqualError(t, err, "embedding API key is required")

	_, err = (&Config{
		Models: ModelsConfig{Key: "key", Embedding: EmbeddingModelConfig{Provider: "test"}},
	}).ResolveEmbeddingModelAuth()
	require.EqualError(t, err, "embedding provider must be one of: openai, openrouter")
}

func TestConfig_ModelEmbeddingProviderEffective(t *testing.T) {
	var cfg *Config
	require.Empty(t, cfg.ModelEmbeddingProviderEffective())

	cfg = &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: " OpenRouter "}}}
	require.Equal(t, "openrouter", cfg.ModelEmbeddingProviderEffective())

	cfg = &Config{
		Models: ModelsConfig{
			Main:      MainModelConfig{Provider: "openrouter"},
			Embedding: EmbeddingModelConfig{Provider: " OpenAI "},
		},
	}
	require.Equal(t, "openai", cfg.ModelEmbeddingProviderEffective())
}

func TestConfig_ValidateAllowsProviderSpecificAuthWithoutModelKey(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			OpenRouterAPIKey: "openrouter-key",
			Main:             MainModelConfig{Name: defaultModel, Provider: "openrouter"},
		},
		Log: LogConfig{Level: "info"},
	}

	require.NoError(t, cfg.Validate())
}

func TestConfig_ValidateNormalizesFields(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	cfg := &Config{
		Name: "  Test Agent  ",
		Models: ModelsConfig{
			Key:  "  test-key  ",
			Main: MainModelConfig{Name: "  openai/test-model  ", Provider: " OpenRouter "},
		},
		Log: LogConfig{Level: " WARN "},
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "openai/test-model", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "test-key", cfg.Models.Key)
	require.Equal(t, defaultBaseURLForProvider("openrouter", DefaultModelAPIMode), cfg.Models.Main.BaseURL)
	require.Equal(t, "warn", cfg.Log.Level)
}

func TestConfig_ValidateRequiresName(t *testing.T) {
	err := (&Config{
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel}},
		Log:    LogConfig{Level: "info"},
	}).Validate()
	require.EqualError(t, err, "name is required; set HAND_NAME, provide it in config, or use --name")
}

func TestConfig_ValidateDefaultsModelWhenEmpty(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	cfg := &Config{Name: "test-agent", Models: ModelsConfig{Key: "test-key"}, Log: LogConfig{Level: "info"}}
	require.NoError(t, cfg.Validate())
	require.Equal(t, defaultModel, cfg.Models.Main.Name)
}

func TestConfig_ValidateRejectsModelWithoutOwnerPrefix(t *testing.T) {
	err := (&Config{
		Name:   "test-agent",
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: "gpt-4o-mini", Provider: "openai"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:    LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
}

func TestConfig_ValidateRejectsModelWithEmptyOwnerOrName(t *testing.T) {
	cases := []string{"/gpt-4o-mini", "openai/", "openai/gpt-4o-mini/extra"}

	for _, model := range cases {
		t.Run(model, func(t *testing.T) {
			err := (&Config{
				Name:   "test-agent",
				Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: model, Provider: "openai"}},
				RPC:    RPCConfig{Address: "127.0.0.1", Port: 50051},
				Log:    LogConfig{Level: "info"},
			}).Validate()

			require.EqualError(t, err, "model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
		})
	}
}

func TestConfig_ValidateRejectsUnsupportedProvider(t *testing.T) {
	openRouterDefault := defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode)
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:  "test-key",
			Main: MainModelConfig{Name: defaultModel, Provider: "anthropic", BaseURL: openRouterDefault},
		},
		Log: LogConfig{Level: "info"},
	}).Validate()
	require.EqualError(t, err, "model provider must be one of: openai, openrouter")
}

func TestConfig_ValidateRejectsUnknownOpenRouterModel(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{}, nil
	})

	err := (&Config{
		Name:   "test-agent",
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: "openai/gpt-unknown", Provider: "openrouter"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:    LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `models.main.name: model "openai/gpt-unknown" is not available on openrouter`)
}

func TestConfig_ValidateRejectsInvalidSummaryModelSlug(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:     "test-key",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openai"},
			Summary: SummaryModelConfig{Name: "gpt-4o-mini"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "summary model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
}

func TestConfig_ValidateRejectsUnknownSummaryModel(t *testing.T) {
	stubModelMetadataResolver(t, func(_ context.Context, cfg *Config, _ ModelAuth) (ModelMetadata, error) {
		if cfg.Models.Main.Name == defaultModel {
			return ModelMetadata{Exists: true}, nil
		}

		return ModelMetadata{}, nil
	})

	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:     "test-key",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary: SummaryModelConfig{Name: "openai/gpt-unknown-summary"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `models.summary.name: model "openai/gpt-unknown-summary" is not available on openrouter`)
}

func TestConfig_SummaryModelEffective(t *testing.T) {
	t.Run("inherits_main_model_when_empty", func(t *testing.T) {
		cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{Name: defaultModel}}}
		require.Equal(t, defaultModel, cfg.SummaryModelEffective())
	})

	t.Run("uses_summary_when_set", func(t *testing.T) {
		cfg := &Config{
			Models: ModelsConfig{
				Main:    MainModelConfig{Name: defaultModel},
				Summary: SummaryModelConfig{Name: "anthropic/claude-3.5-haiku"},
			},
		}
		require.Equal(t, "anthropic/claude-3.5-haiku", cfg.SummaryModelEffective())
	})
}

func TestConfig_SummaryProviderEffective(t *testing.T) {
	cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openrouter"}}}
	require.Equal(t, "openrouter", cfg.SummaryProviderEffective())

	cfg.Models.Summary.Provider = "openai"
	require.Equal(t, "openai", cfg.SummaryProviderEffective())
}

func TestConfig_SummaryModelAPIModeEffective(t *testing.T) {
	cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{APIMode: "responses"}}}
	cfg.Normalize()
	require.Equal(t, "responses", cfg.SummaryModelAPIModeEffective())

	cfg.Models.Summary.APIMode = DefaultModelAPIMode
	cfg.Normalize()
	require.Equal(t, DefaultModelAPIMode, cfg.SummaryModelAPIModeEffective())
}

func TestConfig_ResolveSummaryModelAuth_UsesSummaryAPIModeForDefaultBaseURL(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:     "k",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openrouter", APIMode: DefaultModelAPIMode},
			Summary: SummaryModelConfig{APIMode: "responses"},
		},
	}
	cfg.Normalize()

	auth, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)
	require.Equal(t, "https://openrouter.ai/api/v1/responses", auth.BaseURL)
}

func TestConfig_ResolveSummaryModelAuthMatchesMainWhenUnset(t *testing.T) {
	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Key: "k", Main: MainModelConfig{Name: defaultModel, Provider: "openrouter"}},
	}

	main, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	sum, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)
	require.True(t, ModelAuthEqual(main, sum))
}

func TestConfig_ResolveSummaryModelAuthUsesOpenAIWhenSummaryProviderDiffers(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:     "k",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary: SummaryModelConfig{Provider: "openai", BaseURL: "https://api.example/v1"},
		},
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
		Name: "test-agent",
		Models: ModelsConfig{
			Key:     "test-key",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary: SummaryModelConfig{Provider: "anthropic"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "summary model provider must be one of: openai, openrouter")
}

func TestConfig_ValidateRejectsInvalidSummaryModelAPIMode(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:     "test-key",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary: SummaryModelConfig{APIMode: "invalid"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "summary model api mode must be one of: completions, responses; "+
		"use --model.summary-api-mode")
}

func TestConfig_ValidateAcceptsSummaryModelAPIModeResponses(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: defaultContextLength}, nil
	})

	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Verify:  new(false),
			Key:     "test-key",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary: SummaryModelConfig{APIMode: "responses"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.NoError(t, err)
}

func TestConfig_ValidateAcceptsSummaryModelAPIModeCompletions(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: defaultContextLength}, nil
	})

	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Verify:  new(false),
			Key:     "test-key",
			Main:    MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary: SummaryModelConfig{APIMode: DefaultModelAPIMode},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
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
		Name:   "test-agent",
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel, Provider: "openrouter"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:    LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `models.main.name: failed to verify openrouter model "openai/gpt-4o-mini": lookup failed`)
}

func TestConfig_ValidateRejectsUnknownOpenAIModel(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{}, nil
	})

	err := (&Config{
		Name:   "test-agent",
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: "openai/gpt-unknown", Provider: "openai"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:    LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `models.main.name: model "openai/gpt-unknown" is not available on openai`)
}

func TestConfig_ValidateRejectsInvalidLogLevel(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Verify: new(false),
			Key:    "test-key",
			Main:   MainModelConfig{Name: defaultModel, Provider: "openai"},
		},
		Log: LogConfig{Level: "trace"},
	}).Validate()
	require.EqualError(t, err, "log level must be one of debug, info, warn, or error; use --log.level")
}

func TestConfig_ValidateAllowsEmptyProviderAndLogLevel(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	})

	err := (&Config{
		Name:   "test-agent",
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel}},
	}).Validate()
	require.NoError(t, err)
}

func TestConfig_ValidateRejectsEmptyRPCAddress(t *testing.T) {
	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel, Provider: "openai"}},
		RPC:    RPCConfig{Address: "   ", Port: 50051},
		Log:    LogConfig{Level: "info"},
	}

	require.EqualError(t, cfg.Validate(), "rpc address is required; set HAND_RPC_ADDRESS, provide it in config, or use --rpc.address")
}

func TestConfig_ValidateRejectsInvalidRPCPort(t *testing.T) {
	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel, Provider: "openai"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: -1},
		Log:    LogConfig{Level: "info"},
	}

	require.EqualError(t, cfg.Validate(), "rpc port must be greater than zero; set HAND_RPC_PORT, provide it in config, or use --rpc.port")
}

func TestConfig_ValidateRejectsInvalidMaxIterations(t *testing.T) {
	cfg := &Config{
		Name:    "test-agent",
		Models:  ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel, Provider: "openai"}},
		RPC:     RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session: SessionConfig{MaxIterations: -1},
		Log:     LogConfig{Level: "info"},
	}

	require.EqualError(t, cfg.Validate(), "max iterations must be greater than zero; set "+
		"HAND_SESSION_MAX_ITERATIONS, provide it in config, or use --max-iterations")
}

func TestConfig_ValidateRejectsNegativeModelMaxRetries(t *testing.T) {
	retries := -1
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:        "test-key",
			MaxRetries: &retries,
			Main:       MainModelConfig{Name: defaultModel, Provider: "openai"},
		},
		RPC:     RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session: SessionConfig{MaxIterations: 1},
		Log:     LogConfig{Level: "info"},
	}

	require.EqualError(t, cfg.Validate(), "model max retries must be greater than or equal to "+
		"zero; use --model.max-retries")
}

func TestConfig_ValidateRejectsCompactionThresholdsAboveOrEqualOne(t *testing.T) {
	err := (&Config{
		Name:       "test-agent",
		Models:     ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel, Provider: "openai"}},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "memory"},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 1, WarnPercent: 1},
	}).Validate()

	require.EqualError(t, err, "compaction trigger percent must be greater than zero and less than one")

	err = (&Config{
		Name:       "test-agent",
		Models:     ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel, Provider: "openai"}},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "memory"},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 0.9, WarnPercent: 1},
	}).Validate()

	require.EqualError(t, err, "compaction warn percent must be greater than zero and less than one")
}

func TestConfig_NormalizeDefaultsProviderWhenEmpty(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{Main: MainModelConfig{Name: defaultModel}},
		Log:    LogConfig{Level: "info"},
	}
	cfg.Normalize()
	require.Equal(t, defaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.Models.Main.BaseURL)
}

func TestConfig_NormalizeIgnoresNilReceiver(t *testing.T) {
	var cfg *Config
	cfg.Normalize()
}

func TestConfig_NormalizeDefaultsModelAndLogLevel(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Empty(t, cfg.Name)
	require.Equal(t, defaultModel, cfg.Models.Main.Name)
	require.Equal(t, defaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, boolValue(cfg.Cap.Filesystem))
	require.True(t, boolValue(cfg.Cap.Network))
	require.True(t, boolValue(cfg.Cap.Exec))
	require.True(t, boolValue(cfg.Cap.Memory))
	require.False(t, boolValue(cfg.Cap.Browser))
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.Models.Main.BaseURL)
	require.Equal(t, "127.0.0.1", cfg.RPC.Address)
	require.Equal(t, 50051, cfg.RPC.Port)
	require.Equal(t, defaultMaxIterations, cfg.Session.MaxIterations)
	require.Equal(t, "info", cfg.Log.Level)
	require.Equal(t, DefaultWebMaxCharPerResult, cfg.Web.MaxCharPerResult)
	require.Equal(t, DefaultWebMaxExtractCharPerResult, cfg.Web.MaxExtractCharPerResult)
	require.Equal(t, DefaultWebMaxExtractResponseBytes, cfg.Web.MaxExtractResponseBytes)
	require.Equal(t, DefaultWebCacheTTL, cfg.Web.CacheTTL)
	require.False(t, cfg.Web.BlockedDomainsEnabled)
	require.Empty(t, cfg.Web.BlockedDomains)
	require.Empty(t, cfg.Web.BlockedDomainFiles)
	require.Empty(t, cfg.Web.NativeAllowedHosts)
	require.Empty(t, cfg.Web.NativeBlockedHosts)
	require.Empty(t, cfg.Web.NativeAllowedHostFiles)
	require.Empty(t, cfg.Web.NativeBlockedHostFiles)
	require.Equal(t, DefaultWebExtractMinSummarizeChars, cfg.Web.ExtractMinSummarizeChars)
	require.Equal(t, DefaultWebExtractMaxSummaryChars, cfg.Web.ExtractMaxSummaryChars)
	require.Equal(t, DefaultWebExtractMaxSummaryChunkChars, cfg.Web.ExtractMaxSummaryChunkChars)
	require.Less(t, cfg.Web.ExtractMaxSummaryChunkChars, cfg.Web.MaxExtractCharPerResult)
	require.Equal(t, DefaultWebExtractRefusalThresholdChars, cfg.Web.ExtractRefusalThresholdChars)
	require.True(t, boolValueDefault(cfg.Models.Verify, true))
}

func TestConfig_NormalizeDisablesNegativeWebCacheTTL(t *testing.T) {
	cfg := &Config{Web: WebConfig{CacheTTL: -time.Second}}
	cfg.Normalize()
	require.Equal(t, DefaultWebCacheTTL, cfg.Web.CacheTTL)
}

func TestConfig_NormalizeTrimsWebBlockedDomains(t *testing.T) {
	cfg := &Config{
		Web: WebConfig{
			BlockedDomains:         []string{" blocked.example ", "blocked.example", ""},
			BlockedDomainFiles:     []string{" blocked.txt ", "blocked.txt", ""},
			NativeAllowedHosts:     []string{" allowed.example ", "allowed.example", ""},
			NativeBlockedHosts:     []string{" blocked.example ", "blocked.example", ""},
			NativeAllowedHostFiles: []string{" allow.txt ", "allow.txt", ""},
			NativeBlockedHostFiles: []string{" deny.txt ", "deny.txt", ""},
		},
	}

	cfg.Normalize()

	require.Equal(t, []string{"blocked.example"}, cfg.Web.BlockedDomains)
	require.Equal(t, []string{"blocked.txt"}, cfg.Web.BlockedDomainFiles)
	require.Equal(t, []string{"allowed.example"}, cfg.Web.NativeAllowedHosts)
	require.Equal(t, []string{"blocked.example"}, cfg.Web.NativeBlockedHosts)
	require.Equal(t, []string{"allow.txt"}, cfg.Web.NativeAllowedHostFiles)
	require.Equal(t, []string{"deny.txt"}, cfg.Web.NativeBlockedHostFiles)
}

func TestApplyEnvOverrides_IgnoresInvalidWebCacheTTL(t *testing.T) {
	clearEnvKeys(t, "HAND_WEB_CACHE_TTL")
	t.Setenv("HAND_WEB_CACHE_TTL", "not-a-duration")

	cfg := &Config{}
	applyEnvOverrides(cfg)
	cfg.Normalize()

	require.Equal(t, DefaultWebCacheTTL, cfg.Web.CacheTTL)
}

func TestConfig_NormalizePreservesExplicitFalseCapabilities(t *testing.T) {
	cfg := &Config{
		Cap: CapConfig{
			Filesystem: new(false),
			Network:    new(false),
			Exec:       new(false),
			Memory:     new(false),
			Browser:    new(false),
		},
	}

	cfg.Normalize()

	require.False(t, boolValue(cfg.Cap.Filesystem))
	require.False(t, boolValue(cfg.Cap.Network))
	require.False(t, boolValue(cfg.Cap.Exec))
	require.False(t, boolValue(cfg.Cap.Memory))
	require.False(t, boolValue(cfg.Cap.Browser))
}

func TestConfig_NormalizeDefaultsUnsetCapabilitiesIndividually(t *testing.T) {
	cfg := &Config{Cap: CapConfig{Filesystem: new(false)}}

	cfg.Normalize()

	require.False(t, boolValue(cfg.Cap.Filesystem))
	require.True(t, boolValue(cfg.Cap.Network))
	require.True(t, boolValue(cfg.Cap.Exec))
	require.True(t, boolValue(cfg.Cap.Memory))
	require.False(t, boolValue(cfg.Cap.Browser))
}

func TestConfig_NormalizeUsesMappedBaseURLWhenProviderWasExplicitlySet(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{Main: MainModelConfig{Name: defaultModel, Provider: defaultModelProvider}},
		Log:    LogConfig{Level: "info"},
	}
	cfg.Normalize()
	require.Equal(t, defaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, defaultBaseURLForProvider(defaultModelProvider, DefaultModelAPIMode), cfg.Models.Main.BaseURL)
}

func TestConfig_NormalizeKeepsOpenaiProvider(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{Key: "test-key", Main: MainModelConfig{Name: defaultModel, Provider: "openai"}},
		Log:    LogConfig{Level: "info"},
	}
	cfg.Normalize()
	require.Equal(t, "openai", cfg.Models.Main.Provider)
	require.Equal(t, "https://api.openai.com/v1", cfg.Models.Main.BaseURL)
}

func TestConfig_NormalizeDefaultBaseURLDependsOnAPIMode(t *testing.T) {
	t.Run("openai uses api root for completions and responses", func(t *testing.T) {
		for _, mode := range []string{DefaultModelAPIMode, "responses"} {
			cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openai", APIMode: mode}}}
			cfg.Normalize()
			require.Equal(t, "https://api.openai.com/v1", cfg.Models.Main.BaseURL, mode)
		}
	})

	t.Run("openrouter defaults differ by api mode", func(t *testing.T) {
		cfgChat := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openrouter", APIMode: DefaultModelAPIMode}}}
		cfgChat.Normalize()
		require.Equal(t, "https://openrouter.ai/api/v1", cfgChat.Models.Main.BaseURL)

		cfgResp := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openrouter", APIMode: "responses"}}}
		cfgResp.Normalize()
		require.Equal(t, "https://openrouter.ai/api/v1/responses", cfgResp.Models.Main.BaseURL)
	})

	t.Run("unknown api mode does not fall back to default base url", func(t *testing.T) {
		cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openrouter", APIMode: "future-mode"}}}
		cfg.Normalize()
		require.Empty(t, cfg.Models.Main.BaseURL)
	})
}

func TestConfig_NormalizeTrimsAndLowercasesFields(t *testing.T) {
	cfg := &Config{
		Name: "  Test Agent  ",
		Models: ModelsConfig{
			Key:  "  test-key  ",
			Main: MainModelConfig{Name: "  test-model  ", Provider: " OpenRouter ", BaseURL: "  https://example.com/v1  "},
		},
		Log: LogConfig{Level: " WARN "},
	}
	cfg.Normalize()
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "test-model", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "test-key", cfg.Models.Key)
	require.Equal(t, "https://example.com/v1", cfg.Models.Main.BaseURL)
	require.Equal(t, "warn", cfg.Log.Level)
}

func TestConfig_VerifyEnabledUsesFallbacks(t *testing.T) {
	var cfg *Config
	require.True(t, cfg.VerifyEnabled())

	cfg = &Config{}
	require.True(t, cfg.VerifyEnabled())

	cfg.Models.Verify = new(false)
	require.False(t, cfg.VerifyEnabled())
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
		Models: ModelsConfig{Key: "key", Main: MainModelConfig{Provider: "custom"}},
	}
	auth, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "key", auth.APIKey)
}

func TestApplyEnvOverrides_CoversRemainingBranches(t *testing.T) {
	clearEnvKeys(t,
		"HAND_MODEL_CONTEXT_LENGTH", "HAND_MODELS_VERIFY", "HAND_MODEL_MAX_RETRIES", "HAND_OPENAI_API_KEY", "HAND_OPENROUTER_API_KEY",
		"HAND_STORAGE_BACKEND", "HAND_SESSION_DEFAULT_IDLE_EXPIRY", "HAND_SESSION_ARCHIVE_RETENTION",
		"HAND_SEARCH_VECTOR_ENABLED", "HAND_MODEL_EMBEDDING_PROVIDER",
		"HAND_MODEL_EMBEDDING_MODEL", "HAND_SEARCH_VECTOR_REQUIRED",
		"HAND_SEARCH_VECTOR_REBUILD_BATCH_SIZE", "HAND_SEARCH_ENABLE_RERANK", "HAND_RERANKER_ENABLED",
		"HAND_RERANKER_TYPE", "HAND_RERANKER_MODEL", "HAND_RERANKER_MAX_CANDIDATES",
		"HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS", "HAND_RERANKER_MAX_OUTPUT_TOKENS",
		"HAND_COMPACTION_ENABLED", "HAND_COMPACTION_TRIGGER_PERCENT", "HAND_COMPACTION_WARN_PERCENT",
		"HAND_MEMORY_ENABLED", "HAND_MEMORY_PROVIDER",
		"HAND_FIRECRAWL_API_KEY", "HAND_FIRECRAWL_API_URL", "HAND_PARALLEL_API_KEY", "HAND_TAVILY_API_KEY", "HAND_EXA_API_KEY",
	)

	cfg := &Config{}
	applyEnvOverrides(nil)

	t.Setenv("HAND_MODEL_CONTEXT_LENGTH", "64000")
	t.Setenv("HAND_MODELS_VERIFY", "false")
	t.Setenv("HAND_MODEL_MAX_RETRIES", "0")
	t.Setenv("HAND_OPENAI_API_KEY", "openai-key")
	t.Setenv("HAND_OPENROUTER_API_KEY", "openrouter-key")
	t.Setenv("HAND_STORAGE_BACKEND", "memory")
	t.Setenv("HAND_SESSION_DEFAULT_IDLE_EXPIRY", "2h")
	t.Setenv("HAND_SESSION_ARCHIVE_RETENTION", "48h")
	t.Setenv("HAND_SEARCH_VECTOR_ENABLED", "true")
	t.Setenv("HAND_MODEL_EMBEDDING_PROVIDER", "test")
	t.Setenv("HAND_MODEL_EMBEDDING_MODEL", "text-embedding-test")
	t.Setenv("HAND_SEARCH_VECTOR_REQUIRED", "true")
	t.Setenv("HAND_SEARCH_VECTOR_REBUILD_BATCH_SIZE", "32")
	t.Setenv("HAND_SEARCH_ENABLE_RERANK", "false")
	t.Setenv("HAND_RERANKER_ENABLED", "false")
	t.Setenv("HAND_RERANKER_TYPE", rerank.LLM)
	t.Setenv("HAND_RERANKER_MODEL", "openai/gpt-4o-mini")
	t.Setenv("HAND_RERANKER_MAX_CANDIDATES", "12")
	t.Setenv("HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS", "700")
	t.Setenv("HAND_RERANKER_MAX_OUTPUT_TOKENS", "256")
	t.Setenv("HAND_COMPACTION_ENABLED", "false")
	t.Setenv("HAND_COMPACTION_TRIGGER_PERCENT", "0.5")
	t.Setenv("HAND_COMPACTION_WARN_PERCENT", "0.8")
	t.Setenv("HAND_MEMORY_ENABLED", "true")
	t.Setenv("HAND_MEMORY_PROVIDER", " Memory ")

	applyEnvOverrides(cfg)

	require.Equal(t, 64000, cfg.Models.Main.ContextLength)
	require.False(t, boolValue(cfg.Models.Verify))
	require.Equal(t, 0, cfg.ModelMaxRetriesEffective())
	require.Equal(t, "openai-key", cfg.Models.OpenAIAPIKey)
	require.Equal(t, "openrouter-key", cfg.Models.OpenRouterAPIKey)
	require.Equal(t, "memory", cfg.Storage.Backend)
	require.Equal(t, 2*time.Hour, cfg.Session.DefaultIdleExpiry)
	require.Equal(t, 48*time.Hour, cfg.Session.ArchiveRetention)
	require.True(t, cfg.Search.Vector.Enabled)
	require.Equal(t, "test", cfg.Models.Embedding.Provider)
	require.Equal(t, "text-embedding-test", cfg.Models.Embedding.Name)
	require.True(t, cfg.Search.Vector.Required)
	require.Equal(t, 32, cfg.Search.Vector.RebuildBatchSize)
	require.False(t, boolValueDefault(cfg.Search.EnableRerank, true))
	require.False(t, boolValueDefault(cfg.Reranker.Enabled, true))
	require.Equal(t, rerank.LLM, cfg.Reranker.Type)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Reranker.Model)
	require.Equal(t, 12, cfg.Reranker.MaxCandidates)
	require.Equal(t, 700, cfg.Reranker.MaxCandidateTextChars)
	require.Equal(t, 256, cfg.Reranker.MaxOutputTokens)
	require.False(t, boolValue(cfg.Compaction.Enabled))
	require.Equal(t, 0.5, cfg.Compaction.TriggerPercent)
	require.Equal(t, 0.8, cfg.Compaction.WarnPercent)
	require.True(t, cfg.MemoryEnabled())
	require.Equal(t, "memory", cfg.Memory.Provider)
}

func TestConfig_MemoryDefaultsAndNormalize(t *testing.T) {
	var cfg *Config
	require.False(t, cfg.MemoryEnabled())

	cfg = &Config{Memory: MemoryConfig{Provider: " Memory "}}
	cfg.Normalize()
	require.True(t, cfg.MemoryEnabled())
	require.Equal(t, "memory", cfg.Memory.Provider)

	cfg = &Config{Memory: MemoryConfig{Enabled: new(false)}}
	cfg.Normalize()
	require.False(t, cfg.MemoryEnabled())
	require.Equal(t, "noop", cfg.Memory.Provider)
}

func TestApplyEnvOverrides_WebProviderSpecificFallback(t *testing.T) {
	clearEnvKeys(t,
		"HAND_WEB_PROVIDER", "HAND_WEB_API_KEY", "HAND_WEB_BASE_URL",
		"HAND_FIRECRAWL_API_KEY", "HAND_FIRECRAWL_API_URL", "HAND_PARALLEL_API_KEY", "HAND_TAVILY_API_KEY", "HAND_EXA_API_KEY",
	)

	cfg := &Config{}
	t.Setenv("HAND_FIRECRAWL_API_URL", "http://localhost:3002")

	applyEnvOverrides(cfg)

	require.Equal(t, "firecrawl", cfg.Web.Provider)
	require.Equal(t, "", cfg.Web.APIKey)
	require.Equal(t, "http://localhost:3002", cfg.Web.BaseURL)

	cfg = &Config{}
	t.Setenv("HAND_WEB_PROVIDER", "exa")
	t.Setenv("HAND_EXA_API_KEY", "exa-key")

	applyEnvOverrides(cfg)

	require.Equal(t, "exa", cfg.Web.Provider)
	require.Equal(t, "exa-key", cfg.Web.APIKey)
}

func TestApplyEnvOverrides_SummaryModelAndRelatedEnv(t *testing.T) {
	clearEnvKeys(t,
		"HAND_MODEL_SUMMARY", "HAND_MODEL_SUMMARY_PROVIDER", "HAND_MODEL_SUMMARY_BASE_URL",
		"HAND_MODEL_API_MODE", "HAND_MODEL_SUMMARY_API_MODE",
	)

	cfg := &Config{}
	t.Setenv("HAND_MODEL_SUMMARY", "openai/gpt-4o-mini")
	t.Setenv("HAND_MODEL_SUMMARY_PROVIDER", "openai")
	t.Setenv("HAND_MODEL_SUMMARY_BASE_URL", "https://example.com/v1")
	t.Setenv("HAND_MODEL_API_MODE", "responses")
	t.Setenv("HAND_MODEL_SUMMARY_API_MODE", "responses")

	applyEnvOverrides(cfg)

	require.Equal(t, "openai/gpt-4o-mini", cfg.Models.Summary.Name)
	require.Equal(t, "openai", cfg.Models.Summary.Provider)
	require.Equal(t, "https://example.com/v1", cfg.Models.Summary.BaseURL)
	require.Equal(t, "responses", cfg.Models.Main.APIMode)
	require.Equal(t, "responses", cfg.Models.Summary.APIMode)
}

func TestNormalizeFields_NilReceiver_NoPanic(t *testing.T) {
	var cfg *Config
	cfg.normalizeFields()
}

func TestDefaultBaseURLForProvider_DefaultsEmptyAPIMode(t *testing.T) {
	require.Equal(t, "https://openrouter.ai/api/v1", defaultBaseURLForProvider("openrouter", ""))
	require.Equal(t, "https://openrouter.ai/api/v1", defaultBaseURLForProvider("openrouter", "   "))
	require.Equal(t, "https://api.openai.com/v1", defaultBaseURLForProvider("openai", DefaultModelAPIMode))
	require.Equal(t, "https://api.openai.com/v1", defaultBaseURLForProvider("openai", "responses"))
	require.Equal(t, "https://openrouter.ai/api/v1/embeddings", defaultBaseURLForProvider("openrouter", "embeddings"))
	require.Equal(t, "https://api.openai.com/v1/embeddings", defaultBaseURLForProvider("openai", "embeddings"))
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
		Name: "test-agent",
		Models: ModelsConfig{
			OpenRouterAPIKey: "router-only",
			Main:             MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary:          SummaryModelConfig{Provider: "openai", BaseURL: "https://api.openai.com/v1"},
		},
	}
	cfg.Normalize()

	_, err := cfg.ResolveSummaryModelAuth()
	require.EqualError(t, err, "model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
}

func TestConfig_Validate_ReturnsSummaryAuthErrorWhenOpenAIKeyMissing(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			OpenRouterAPIKey: "router-only",
			Main:             MainModelConfig{Name: defaultModel, Provider: "openrouter"},
			Summary:          SummaryModelConfig{Provider: "openai"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
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

	cfg := &Config{Models: ModelsConfig{Verify: new(false)}}
	applyProviderModelMetadata(context.Background(), cfg, 0)

	cfg = &Config{Models: ModelsConfig{Verify: new(true)}}
	applyProviderModelMetadata(context.Background(), cfg, 0)

	resolveModelMeta = func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{}, errors.New("boom")
	}
	cfg = &Config{
		Models: ModelsConfig{
			Verify: new(true),
			Key:    "test-key",
			Main:   MainModelConfig{Name: defaultModel, Provider: "openai"},
		},
	}
	cfg.Normalize()
	applyProviderModelMetadata(context.Background(), cfg, 0)

	resolveModelMeta = func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true}, nil
	}
	applyProviderModelMetadata(context.Background(), cfg, 0)
	require.Equal(t, defaultContextLength, cfg.Models.Main.ContextLength)
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

func TestFetchOpenRouterModelEndpoints_CoversResponses(t *testing.T) {
	t.Run("empty_model", func(t *testing.T) {
		meta, err := fetchOpenRouterModelEndpoints(context.Background(), "", "", "")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("validates_model_endpoint", func(t *testing.T) {
		var authorization string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			require.Equal(t, "/models/openai/text-embedding-ada-002/endpoints", r.URL.Path)
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		t.Cleanup(server.Close)

		meta, err := fetchOpenRouterModelEndpoints(
			context.Background(),
			server.URL,
			"openai/text-embedding-ada-002",
			"router-key",
		)

		require.NoError(t, err)
		require.Equal(t, ModelMetadata{Exists: true}, meta)
		require.Equal(t, "Bearer router-key", authorization)
	})

	t.Run("not_found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		meta, err := fetchOpenRouterModelEndpoints(context.Background(), server.URL, "openai/missing", "")

		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("non_200_status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(server.Close)

		_, err := fetchOpenRouterModelEndpoints(context.Background(), server.URL, "openai/model", "")

		require.Error(t, err)
		require.Contains(t, err.Error(), "openrouter model endpoints lookup returned 502 Bad Gateway")
	})

	t.Run("network_error", func(t *testing.T) {
		originalHTTPClient := httpClient
		t.Cleanup(func() {
			httpClient = originalHTTPClient
		})

		httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		})}

		meta, err := fetchOpenRouterModelEndpoints(context.Background(), "", "openai/model", "")

		require.EqualError(t, err, `Get "https://openrouter.ai/api/v1/models/openai/model/endpoints": network down`)
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
		meta, err := fetchOpenAIModelMetadataPage(context.Background(), "gpt-4o-mini", true)
		require.EqualError(t, err, `Get "https://developers.openai.com/api/docs/models/gpt-4o-mini": transport failed`)
		require.Equal(t, ModelMetadata{}, meta)
	})

	t.Run("bad_base_url", func(t *testing.T) {
		originalModelDocsBaseURL := modelDocsBaseURL
		t.Cleanup(func() {
			modelDocsBaseURL = originalModelDocsBaseURL
		})

		modelDocsBaseURL = "://bad"
		meta, err := fetchOpenAIModelMetadataPage(context.Background(), "gpt-4o-mini", true)
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
			case "/api/docs/models/page-not-found":
				_, _ = w.Write([]byte(`<html><head><title>Page not found | OpenAI API</title></head></html>`))
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

		meta, err := fetchOpenAIModelMetadataPage(context.Background(), "missing", true)
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)

		meta, err = fetchOpenAIModelMetadataPage(context.Background(), "status", true)
		require.Error(t, err)
		require.Contains(t, err.Error(), "openai model docs lookup returned 502 Bad Gateway")

		meta, err = fetchOpenAIModelMetadataPage(context.Background(), "no-window", true)
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)

		meta, err = fetchOpenAIModelMetadataPage(context.Background(), "no-window", false)
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{Exists: true}, meta)

		meta, err = fetchOpenAIModelMetadataPage(context.Background(), "page-not-found", false)
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)

		meta, err = fetchOpenAIModelMetadataPage(context.Background(), "comment-window", true)
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{Exists: true, ContextLength: 128000}, meta)

		meta, err = fetchOpenAIModelMetadata(context.Background(), "openai/fallback-2025-04-14")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{Exists: true, ContextLength: 8192}, meta)

		meta, err = fetchOpenAIModelMetadata(context.Background(), "openai/absent")
		require.NoError(t, err)
		require.Equal(t, ModelMetadata{}, meta)

		meta, err = fetchOpenAIModelMetadataPage(context.Background(), "bad-window", true)
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

		meta, err := fetchOpenAIModelMetadataPage(context.Background(), "gpt-4o-mini", true)
		require.EqualError(t, err, "forced read error")
		require.Equal(t, ModelMetadata{}, meta)
	})
}

func TestOpenAIModelCandidatesAndSnapshotTrim(t *testing.T) {
	require.Nil(t, openAIModelDocSlugs(""))
	require.False(t, isValidModelSlug(""))
	require.Equal(t, []string{"gpt-4.1-2025-04-14", "gpt-4.1"},
		openAIModelDocSlugs("openai/gpt-4.1-2025-04-14"))
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

	meta, err := fetchOpenAIModelMetadataPage(context.Background(), "overflow", true)
	require.Error(t, err)
	require.Equal(t, ModelMetadata{}, meta)
}

func TestFetchOpenAIModelMetadataCandidate_EmptyModel(t *testing.T) {
	meta, err := fetchOpenAIModelMetadataPage(context.Background(), "", true)
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
	clearEnvKeys(t, "HAND_MODEL_API_MODE")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: config-model
    provider: openai
    apiMode: responses
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, "responses", cfg.Models.Main.APIMode)
}

func TestLoad_UsesModelAPIModeFromEnvOverride(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL_API_MODE")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_MODEL_API_MODE=responses\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: config-model
    provider: openai
    apiMode: completions
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)
	require.NoError(t, err)
	require.Equal(t, "responses", cfg.Models.Main.APIMode)
}

func TestConfig_ValidateRejectsInvalidAPIMode(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Key:  "test-key",
			Main: MainModelConfig{Name: defaultModel, Provider: "openai", APIMode: "invalid"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()
	require.EqualError(t, err, "model api mode must be one of: completions, responses; use --model.api-mode")
}

func TestConfig_ValidateAllowsResponsesModeWithOpenRouter(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Verify: new(false),
			Key:    "test-key",
			Main:   MainModelConfig{Name: defaultModel, Provider: "openrouter", APIMode: "responses"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()
	require.NoError(t, err)
}

func TestLoad_UsesDebugTraceSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_DEBUG_TRACES", "HAND_DEBUG_TRACE_DIR")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: config-model
    provider: openai
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
	require.True(t, cfg.Debug.Traces)
	require.Equal(t, "/tmp/hand-traces", cfg.Debug.TraceDir)
}

func TestLoad_UsesDebugTraceSettingsFromEnvOverride(t *testing.T) {
	clearEnvKeys(t, "HAND_DEBUG_TRACES", "HAND_DEBUG_TRACE_DIR")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_DEBUG_TRACES=true\nHAND_DEBUG_TRACE_DIR=/tmp/env-traces\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: config-model
    provider: openai
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
	require.True(t, cfg.Debug.Traces)
	require.Equal(t, "/tmp/env-traces", cfg.Debug.TraceDir)
}

func TestConfig_NormalizeDefaultsDebugTraceDir(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, datadir.DebugTraceDir(), cfg.Debug.TraceDir)
}

func TestConfig_NormalizeDefaultsDebugTraceDirFromHandHome(t *testing.T) {
	clearEnvKeys(t, "HAND_HOME")
	t.Setenv("HAND_HOME", "/tmp/hand-home")
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, "/tmp/hand-home/traces", cfg.Debug.TraceDir)
}

func TestLoad_UsesFilesystemRootsAndExecRulesFromConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_FS_ROOTS", "HAND_EXEC_ALLOW", "HAND_EXEC_ASK", "HAND_EXEC_DENY")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: config-model
    provider: openai
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
	}, cfg.FS.Roots)
	require.Equal(t, []string{"git status"}, cfg.Exec.Allow)
	require.Equal(t, []string{"git push"}, cfg.Exec.Ask)
	require.Equal(t, []string{"git reset --hard"}, cfg.Exec.Deny)
}

func TestLoad_UsesFilesystemRootsAndExecRulesFromEnv(t *testing.T) {
	clearEnvKeys(t, "HAND_FS_ROOTS", "HAND_EXEC_ALLOW", "HAND_EXEC_ASK", "HAND_EXEC_DENY")
	dir := t.TempDir()
	t.Chdir(dir)
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_FS_ROOTS=.,./nested\nHAND_EXEC_ALLOW=git status\nHAND_EXEC_ASK=git push\nHAND_EXEC_DENY=git reset --hard\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  main:
    name: config-model
    provider: openai
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
	}, cfg.FS.Roots)
	require.Equal(t, []string{"git status"}, cfg.Exec.Allow)
	require.Equal(t, []string{"git push"}, cfg.Exec.Ask)
	require.Equal(t, []string{"git reset --hard"}, cfg.Exec.Deny)
}

func TestLoad_UsesSessionSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_STORAGE_BACKEND", "HAND_SESSION_DEFAULT_IDLE_EXPIRY", "HAND_SESSION_ARCHIVE_RETENTION",
		"HAND_SEARCH_VECTOR_ENABLED", "HAND_MODEL_EMBEDDING_PROVIDER",
		"HAND_MODEL_EMBEDDING_MODEL", "HAND_SEARCH_VECTOR_REQUIRED",
		"HAND_SEARCH_VECTOR_REBUILD_BATCH_SIZE", "HAND_SEARCH_ENABLE_RERANK", "HAND_RERANKER_ENABLED",
		"HAND_RERANKER_TYPE", "HAND_RERANKER_MODEL", "HAND_RERANKER_MAX_CANDIDATES",
		"HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS", "HAND_RERANKER_MAX_OUTPUT_TOKENS")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
models:
  embedding:
    provider: test
    name: text-embedding-test
storage:
  backend: memory
session:
  defaultIdleExpiry: 2h
  archiveRetention: 168h
search:
  vector:
    enabled: true
    required: true
    rebuildBatchSize: 25
  enableRerank: false
reranker:
  enabled: false
  type: llm
  model: openai/gpt-4o-mini
  maxCandidates: 11
  maxCandidateTextChars: 600
  maxOutputTokens: 128
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.Equal(t, "memory", cfg.Storage.Backend)
	require.Equal(t, 2*time.Hour, cfg.Session.DefaultIdleExpiry)
	require.Equal(t, 168*time.Hour, cfg.Session.ArchiveRetention)
	require.True(t, cfg.Search.Vector.Enabled)
	require.Equal(t, "test", cfg.Models.Embedding.Provider)
	require.Equal(t, "text-embedding-test", cfg.Models.Embedding.Name)
	require.True(t, cfg.Search.Vector.Required)
	require.Equal(t, 25, cfg.Search.Vector.RebuildBatchSize)
	require.False(t, boolValueDefault(cfg.Search.EnableRerank, true))
	require.False(t, boolValueDefault(cfg.Reranker.Enabled, true))
	require.Equal(t, rerank.LLM, cfg.Reranker.Type)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Reranker.Model)
	require.Equal(t, 11, cfg.Reranker.MaxCandidates)
	require.Equal(t, 600, cfg.Reranker.MaxCandidateTextChars)
	require.Equal(t, 128, cfg.Reranker.MaxOutputTokens)
}

func TestConfig_NormalizeDefaultsSessionSettings(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, "sqlite", cfg.Storage.Backend)
	require.Equal(t, 24*time.Hour, cfg.Session.DefaultIdleExpiry)
	require.Equal(t, 30*24*time.Hour, cfg.Session.ArchiveRetention)
	require.False(t, cfg.Search.Vector.Enabled)
	require.Empty(t, cfg.Models.Embedding.Provider)
	require.Empty(t, cfg.Models.Embedding.Name)
	require.False(t, cfg.Search.Vector.Required)
	require.Zero(t, cfg.Search.Vector.RebuildBatchSize)
	require.Nil(t, cfg.Search.EnableRerank)
	require.Nil(t, cfg.Reranker.Enabled)
	require.Empty(t, cfg.Reranker.Type)
	require.Equal(t, rerank.Deterministic, cfg.RerankerEffective())
}

func TestConfig_ValidateRejectsInvalidSessionSettings(t *testing.T) {
	cfg := &Config{
		Name: "daemon",
		Models: ModelsConfig{
			Verify: new(false),
			Key:    "key",
			Main:   MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", APIMode: DefaultModelAPIMode},
		},
		RPC:     RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session: SessionConfig{MaxIterations: 1},
		Log:     LogConfig{Level: "info"},
		Storage: StorageConfig{Backend: "bogus"},
	}

	err := cfg.Validate()
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func TestConfig_ValidateRejectsInvalidSessionVectorSettings(t *testing.T) {
	valid := Config{
		Name: "daemon",
		Models: ModelsConfig{
			Verify:    new(false),
			Key:       "key",
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", APIMode: DefaultModelAPIMode},
			Embedding: EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"},
		},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "sqlite"},
		Search:     SearchConfig{Vector: SearchVectorConfig{Enabled: true}},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 0.85, WarnPercent: 0.95},
	}

	tests := []struct {
		name   string
		mutate func(*Config)
		err    string
	}{
		{
			name: "missing model",
			mutate: func(cfg *Config) {
				cfg.Models.Embedding.Name = ""
			},
			err: "embedding model is required",
		},
		{
			name: "unsupported provider",
			mutate: func(cfg *Config) {
				cfg.Models.Embedding.Provider = "test"
			},
			err: "embedding provider must be one of: openai, openrouter",
		},
		{
			name: "negative batch size",
			mutate: func(cfg *Config) {
				cfg.Search.Vector.RebuildBatchSize = -1
			},
			err: "vector rebuild batch size must be non-negative",
		},
		{
			name: "unsupported reranker",
			mutate: func(cfg *Config) {
				cfg.Reranker.Type = "magic"
			},
			err: "reranker type must be one of: deterministic, noop, llm",
		},
		{
			name: "negative rerank max candidates",
			mutate: func(cfg *Config) {
				cfg.Reranker.MaxCandidates = -1
			},
			err: "reranker max candidates must be non-negative",
		},
		{
			name: "negative rerank max candidate text chars",
			mutate: func(cfg *Config) {
				cfg.Reranker.MaxCandidateTextChars = -1
			},
			err: "reranker max candidate text chars must be non-negative",
		},
		{
			name: "negative rerank max output tokens",
			mutate: func(cfg *Config) {
				cfg.Reranker.MaxOutputTokens = -1
			},
			err: "reranker max output tokens must be non-negative",
		},
		{
			name: "missing api key",
			mutate: func(cfg *Config) {
				cfg.Models.Key = ""
				cfg.Models.OpenAIAPIKey = ""
				cfg.Models.OpenRouterAPIKey = ""
			},
			err: "embedding API key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)

			err := cfg.Validate()

			require.EqualError(t, err, tt.err)
		})
	}
}

func TestConfig_ValidateVerifiesEmbeddingModelWithoutContextRequirement(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 128000}, nil
	})

	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		require.Equal(t, "/models/openai/text-embedding-3-small/endpoints", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(server.Close)
	stubProviderDefaultBaseURL(t, "openrouter", DefaultModelAPIMode, server.URL)

	cfg := Config{
		Name: "daemon",
		Models: ModelsConfig{
			Verify:    new(true),
			Key:       "key",
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", APIMode: DefaultModelAPIMode},
			Embedding: EmbeddingModelConfig{Name: "openai/text-embedding-3-small", Provider: "openrouter"},
		},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "sqlite"},
		Search:     SearchConfig{Vector: SearchVectorConfig{Enabled: true}},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 0.85, WarnPercent: 0.95},
	}

	err := cfg.Validate()

	require.NoError(t, err)
	require.Equal(t, "Bearer key", authorization)
}

func TestConfig_ValidateRejectsUnknownEmbeddingModel(t *testing.T) {
	stubModelMetadataResolver(t, func(context.Context, *Config, ModelAuth) (ModelMetadata, error) {
		return ModelMetadata{Exists: true, ContextLength: 128000}, nil
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	stubProviderDefaultBaseURL(t, "openrouter", DefaultModelAPIMode, server.URL)

	cfg := Config{
		Name: "daemon",
		Models: ModelsConfig{
			Verify:    new(true),
			Key:       "key",
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", APIMode: DefaultModelAPIMode},
			Embedding: EmbeddingModelConfig{Name: "openai/text-embedding-missing", Provider: "openrouter"},
		},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "sqlite"},
		Search:     SearchConfig{Vector: SearchVectorConfig{Enabled: true}},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 0.85, WarnPercent: 0.95},
	}

	err := cfg.Validate()

	require.EqualError(t, err, `models.embedding.name: model "openai/text-embedding-missing" is not available on openrouter`)
}

func TestConfig_NormalizeDefaultsFilesystemRootsToCWD(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, []string{dir}, cfg.FS.Roots)
}

func TestLoad_UsesCompactionSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL_CONTEXT_LENGTH", "HAND_COMPACTION_ENABLED", "HAND_COMPACTION_TRIGGER_PERCENT",
		"HAND_COMPACTION_WARN_PERCENT")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
models:
  main:
    contextLength: 64000
compaction:
  enabled: false
  triggerPercent: 0.7
  warnPercent: 0.9
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 64000, cfg.Models.Main.ContextLength)
	require.False(t, boolValue(cfg.Compaction.Enabled))
	require.Equal(t, 0.7, cfg.Compaction.TriggerPercent)
	require.Equal(t, 0.9, cfg.Compaction.WarnPercent)
}

func TestConfig_NormalizeDefaultsCompactionSettings(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, defaultContextLength, cfg.Models.Main.ContextLength)
	require.True(t, boolValue(cfg.Compaction.Enabled))
	require.Equal(t, 0.85, cfg.Compaction.TriggerPercent)
	require.Equal(t, 0.95, cfg.Compaction.WarnPercent)
}

func TestConfig_ValidateRejectsInvalidCompactionSettings(t *testing.T) {
	cfg := &Config{
		Name: "daemon",
		Models: ModelsConfig{
			Key:  "key",
			Main: MainModelConfig{Name: "openai/model", ContextLength: 128000, Provider: "openrouter", BaseURL: "https://example.com", APIMode: DefaultModelAPIMode},
		},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "memory"},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 0.96, WarnPercent: 0.95},
	}

	err := cfg.Validate()
	require.EqualError(t, err, "compaction warn percent must be greater than or equal to "+
		"compaction trigger percent")
}

func TestConfigExamples_EnvFilesListSupportedEnvironmentKeys(t *testing.T) {
	expected := supportedEnvironmentKeys(t)
	for _, file := range []struct {
		path     string
		optional bool
	}{
		{path: filepath.Join("..", "..", ".env"), optional: true},
		{path: filepath.Join("..", "..", "example.env")},
	} {
		t.Run(file.path, func(t *testing.T) {
			content, ok := readOptionalTextFile(t, file.path)
			if !ok && file.optional {
				t.Skip("local env file is not present")
			}
			require.True(t, ok)

			for _, key := range expected {
				require.Regexp(t, regexp.MustCompile(`(?m)^#?\s*`+regexp.QuoteMeta(key)+`=`), content, key)
			}
			for _, match := range regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]*)=`).FindAllStringSubmatch(content, -1) {
				require.Truef(t, strings.HasPrefix(match[1], "HAND_"), "env key %q must use HAND_ prefix", match[1])
			}
		})
	}
}

func TestConfigExamples_YAMLFilesListSupportedConfigPaths(t *testing.T) {
	for _, file := range []struct {
		path     string
		optional bool
	}{
		{path: filepath.Join("..", "..", "config.yaml"), optional: true},
		{path: filepath.Join("..", "..", "example.yaml")},
	} {
		t.Run(file.path, func(t *testing.T) {
			content, ok := readOptionalTextFile(t, file.path)
			if !ok && file.optional {
				t.Skip("local YAML config file is not present")
			}
			require.True(t, ok)

			rootKeys := []string{"name", "platform", "search", "reranker"}
			if !file.optional {
				rootKeys = append(rootKeys, "memory")
			}
			requireYAMLKeys(t, content, "", rootKeys)
			requireYAMLKeys(t, content, "models", []string{
				"verify",
				"maxRetries",
				"key",
				"openaiApiKey",
				"openrouterApiKey",
				"main",
				"summary",
				"embedding",
			})
			requireYAMLKeys(t, content, "main", []string{
				"name",
				"provider",
				"apiMode",
				"baseUrl",
				"stream",
				"contextLength",
			})
			requireYAMLKeys(t, content, "summary", []string{"name", "provider", "apiMode", "baseUrl"})
			requireYAMLKeys(t, content, "embedding", []string{"name", "provider"})
			requireYAMLKeys(t, content, "rpc", []string{"address", "port"})
			requireYAMLKeys(t, content, "fs", []string{"roots"})
			requireYAMLKeys(t, content, "exec", []string{"allow", "ask", "deny"})
			requireYAMLKeys(t, content, "storage", []string{"backend"})
			requireYAMLKeys(t, content, "session", []string{
				"maxIterations",
				"instruct",
				"defaultIdleExpiry",
				"archiveRetention",
			})
			requireYAMLKeys(t, content, "vector", []string{
				"enabled",
				"required",
				"rebuildBatchSize",
			})
			requireYAMLKeys(t, content, "search", []string{"enableRerank"})
			if !file.optional {
				requireYAMLKeys(t, content, "memory", []string{"enabled", "provider"})
			}
			requireYAMLKeys(t, content, "reranker", []string{
				"enabled",
				"type",
				"model",
				"maxCandidates",
				"maxCandidateTextChars",
				"maxOutputTokens",
			})
			requireYAMLKeys(t, content, "compaction", []string{"enabled", "triggerPercent", "warnPercent"})
			requireYAMLKeys(t, content, "cap", []string{"fs", "net", "exec", "mem", "browser"})
			requireYAMLKeys(t, content, "log", []string{"level", "noColor"})
			requireYAMLKeys(t, content, "debug", []string{"requests", "traces", "traceDir"})
			requireYAMLKeys(t, content, "web", []string{
				"provider",
				"apiKey",
				"baseUrl",
				"maxCharPerResult",
				"maxExtractCharPerResult",
				"maxExtractResponseBytes",
				"cacheTTL",
				"blockedDomains",
				"native",
				"enabled",
				"domains",
				"files",
				"extractMinSummarizeChars",
				"extractMaxSummaryChars",
				"extractMaxSummaryChunkChars",
				"extractRefusalThresholdChars",
			})
			requireYAMLKeys(t, content, "native", []string{
				"allowedHosts",
				"blockedHosts",
				"allowedHostFiles",
				"blockedHostFiles",
			})
			requireYAMLKeys(t, content, "rules", []string{"files"})
		})
	}
}

func supportedEnvironmentKeys(t *testing.T) []string {
	t.Helper()

	content := readTextFile(t, "config.go")
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`os\.Getenv\("([A-Z0-9_]+)"\)`),
		regexp.MustCompile(`parseOptionalBoolEnv\("([A-Z0-9_]+)"\)`),
	}
	seen := map[string]struct{}{}
	var keys []string
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatch(content, -1) {
			if _, ok := seen[match[1]]; ok {
				continue
			}
			seen[match[1]] = struct{}{}
			keys = append(keys, match[1])
		}
	}

	require.NotEmpty(t, keys)
	return keys
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

func readOptionalTextFile(t *testing.T, path string) (string, bool) {
	t.Helper()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	require.NoError(t, err)

	return string(data), true
}

func requireYAMLKeys(t *testing.T, content, section string, keys []string) {
	t.Helper()

	if section != "" {
		require.Regexp(t, regexp.MustCompile(`(?m)^#?\s*`+regexp.QuoteMeta(section)+`:`), content, section)
	}
	for _, key := range keys {
		var pattern string
		if section == "" {
			pattern = `(?m)^#?\s*` + regexp.QuoteMeta(key) + `:`
		} else {
			pattern = `(?m)^#?\s{2,}` + regexp.QuoteMeta(key) + `:`
		}
		require.Regexp(t, regexp.MustCompile(pattern), content, section+"."+key)
	}
}
