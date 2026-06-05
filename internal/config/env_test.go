package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/internal/constants"
)

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

func TestLoad_UsesEnvOverConfigFile(t *testing.T) {
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
		"HAND_MEMORY_EPISODIC_ENABLED", "HAND_MEMORY_EPISODIC_INTERVAL",
		"HAND_MEMORY_EPISODIC_IDLE_AFTER", "HAND_MEMORY_EPISODIC_MIN_MESSAGES",
		"HAND_MEMORY_EPISODIC_WINDOW_SIZE", "HAND_MEMORY_EPISODIC_MAX_WINDOWS",
		"HAND_MEMORY_EPISODIC_MAX_WINDOW_CHARS", "HAND_MEMORY_EPISODIC_MAX_WINDOW_TOKENS",
		"HAND_MEMORY_EPISODIC_MAX_RETRIES", "HAND_MEMORY_REFLECTION_ENABLED",
		"HAND_MEMORY_REFLECTION_INTERVAL", "HAND_MEMORY_REFLECTION_LIMIT",
		"HAND_MEMORY_REFLECTION_RELATED_LIMIT", "HAND_MEMORY_PROMOTION_ENABLED",
		"HAND_MEMORY_PROMOTION_INTERVAL", "HAND_MEMORY_PROMOTION_LIMIT")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_NAME=env-agent
HAND_MODEL=env-model
HAND_MODEL_PROVIDER=openrouter
OPENROUTER_API_KEY=env-key
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
HAND_MEMORY_PROVIDER=default-memory
HAND_MEMORY_BACKEND=sqlite
HAND_MEMORY_PINNED_ENABLED=false
HAND_MEMORY_PINNED_MAX_CHARS=3000
HAND_MEMORY_PINNED_MAX_ITEM_CHARS=600
HAND_MEMORY_REFLECTION_ENABLED=true
HAND_MEMORY_REFLECTION_INTERVAL=5m
HAND_MEMORY_REFLECTION_LIMIT=9
HAND_MEMORY_REFLECTION_RELATED_LIMIT=4
HAND_MEMORY_PROMOTION_ENABLED=true
HAND_MEMORY_PROMOTION_INTERVAL=3m
HAND_MEMORY_PROMOTION_LIMIT=8
`), 0o600))
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
	auth, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "config-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig}, auth.CredentialSource)
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
	require.True(t, getBoolValue(cfg.Cap.Filesystem))
	require.True(t, getBoolValue(cfg.Cap.Network))
	require.True(t, getBoolValue(cfg.Cap.Exec))
	require.True(t, getBoolValue(cfg.Cap.Memory))
	require.False(t, getBoolValue(cfg.Cap.Browser))
	require.False(t, cfg.MemoryEnabled())
	require.Equal(t, "default-memory", cfg.Memory.Provider)
	require.Equal(t, "sqlite", cfg.Memory.Backend)
	require.False(t, getBoolValue(cfg.Memory.Pinned.Enabled))
	require.Equal(t, 3000, cfg.Memory.Pinned.MaxChars)
	require.Equal(t, 600, cfg.Memory.Pinned.MaxItemChars)
	require.True(t, getBoolValue(cfg.Memory.Reflection.Enabled))
	require.Equal(t, 5*time.Minute, cfg.Memory.Reflection.Interval)
	require.Equal(t, 9, cfg.Memory.Reflection.Limit)
	require.Equal(t, 4, cfg.Memory.Reflection.RelatedLimit)
	require.True(t, getBoolValue(cfg.Memory.Promotion.Enabled))
	require.Equal(t, 3*time.Minute, cfg.Memory.Promotion.Interval)
	require.Equal(t, 8, cfg.Memory.Promotion.Limit)
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

func TestLoad_UsesGatewayConfigFromConfigAndEnv(t *testing.T) {
	clearEnvKeys(t,
		"HAND_GATEWAY_ENABLED",
		"HAND_GATEWAY_ADDRESS",
		"HAND_GATEWAY_PORT",
		"HAND_GATEWAY_AUTH_TOKEN",
		"HAND_GATEWAY_TELEGRAM_ENABLED",
		"HAND_GATEWAY_TELEGRAM_MODE",
		"HAND_GATEWAY_TELEGRAM_BOT_TOKEN",
		"HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET",
		"HAND_GATEWAY_SLACK_ENABLED",
		"HAND_GATEWAY_SLACK_MODE",
		"HAND_GATEWAY_SLACK_BOT_TOKEN",
		"HAND_GATEWAY_SLACK_APP_TOKEN",
		"HAND_GATEWAY_SLACK_SIGNING_SECRET",
	)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(strings.Join([]string{
		"HAND_GATEWAY_ENABLED=true",
		"HAND_GATEWAY_ADDRESS=127.0.0.2",
		"HAND_GATEWAY_PORT=7200",
		"HAND_GATEWAY_AUTH_TOKEN=HAND_GATEWAY_AUTH_TOKEN",
		"HAND_GATEWAY_TELEGRAM_ENABLED=true",
		"HAND_GATEWAY_TELEGRAM_MODE=webhook",
		"HAND_GATEWAY_TELEGRAM_BOT_TOKEN=HAND_GATEWAY_TELEGRAM_BOT_TOKEN",
		"HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET=HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET",
		"HAND_GATEWAY_SLACK_ENABLED=true",
		"HAND_GATEWAY_SLACK_MODE=http",
		"HAND_GATEWAY_SLACK_BOT_TOKEN=HAND_GATEWAY_SLACK_BOT_TOKEN",
		"HAND_GATEWAY_SLACK_APP_TOKEN=HAND_GATEWAY_SLACK_APP_TOKEN",
		"HAND_GATEWAY_SLACK_SIGNING_SECRET=HAND_GATEWAY_SLACK_SIGNING_SECRET",
		"",
	}, "\n")), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
gateway:
  enabled: false
  address: 127.0.0.1
  port: 7100
  authToken: CONFIG_GATEWAY_TOKEN
  telegram:
    enabled: false
    mode: polling
    botToken: CONFIG_HAND_GATEWAY_TELEGRAM_BOT_TOKEN
  slack:
    enabled: false
    mode: socket
    botToken: CONFIG_HAND_GATEWAY_SLACK_BOT_TOKEN
    appToken: CONFIG_HAND_GATEWAY_SLACK_APP_TOKEN
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.True(t, cfg.Gateway.Enabled)
	require.Equal(t, "127.0.0.2", cfg.Gateway.Address)
	require.Equal(t, 7200, cfg.Gateway.Port)
	require.Equal(t, "HAND_GATEWAY_AUTH_TOKEN", cfg.Gateway.AuthToken)
	require.True(t, cfg.Gateway.Telegram.Enabled)
	require.Equal(t, GatewayTelegramModeWebhook, cfg.Gateway.Telegram.Mode)
	require.Equal(t, "HAND_GATEWAY_TELEGRAM_BOT_TOKEN", cfg.Gateway.Telegram.BotToken)
	require.Equal(t, "HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET", cfg.Gateway.Telegram.WebhookSecret)
	require.True(t, cfg.Gateway.Slack.Enabled)
	require.Equal(t, GatewaySlackModeHTTP, cfg.Gateway.Slack.Mode)
	require.Equal(t, "HAND_GATEWAY_SLACK_BOT_TOKEN", cfg.Gateway.Slack.BotToken)
	require.Equal(t, "HAND_GATEWAY_SLACK_APP_TOKEN", cfg.Gateway.Slack.AppToken)
	require.Equal(t, "HAND_GATEWAY_SLACK_SIGNING_SECRET", cfg.Gateway.Slack.SigningSecret)
}

func TestLoad_UsesGatewayConfigFromConfigFile(t *testing.T) {
	clearEnvKeys(t,
		"HAND_GATEWAY_ENABLED",
		"HAND_GATEWAY_ADDRESS",
		"HAND_GATEWAY_PORT",
		"HAND_GATEWAY_AUTH_TOKEN",
		"HAND_GATEWAY_TELEGRAM_ENABLED",
		"HAND_GATEWAY_TELEGRAM_MODE",
		"HAND_GATEWAY_TELEGRAM_BOT_TOKEN",
		"HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET",
		"HAND_GATEWAY_SLACK_ENABLED",
		"HAND_GATEWAY_SLACK_MODE",
		"HAND_GATEWAY_SLACK_BOT_TOKEN",
		"HAND_GATEWAY_SLACK_APP_TOKEN",
		"HAND_GATEWAY_SLACK_SIGNING_SECRET",
	)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
gateway:
  enabled: true
  address: 127.0.0.3
  port: 7300
  authToken: CONFIG_GATEWAY_TOKEN
  telegram:
    enabled: true
    mode: webhook
    botToken: CONFIG_HAND_GATEWAY_TELEGRAM_BOT_TOKEN
    webhookSecret: CONFIG_HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET
  slack:
    enabled: true
    mode: http
    botToken: CONFIG_HAND_GATEWAY_SLACK_BOT_TOKEN
    appToken: CONFIG_HAND_GATEWAY_SLACK_APP_TOKEN
    signingSecret: CONFIG_HAND_GATEWAY_SLACK_SIGNING_SECRET
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.True(t, cfg.Gateway.Enabled)
	require.Equal(t, "127.0.0.3", cfg.Gateway.Address)
	require.Equal(t, 7300, cfg.Gateway.Port)
	require.Equal(t, "CONFIG_GATEWAY_TOKEN", cfg.Gateway.AuthToken)
	require.True(t, cfg.Gateway.Telegram.Enabled)
	require.Equal(t, GatewayTelegramModeWebhook, cfg.Gateway.Telegram.Mode)
	require.Equal(t, "CONFIG_HAND_GATEWAY_TELEGRAM_BOT_TOKEN", cfg.Gateway.Telegram.BotToken)
	require.Equal(t, "CONFIG_HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET", cfg.Gateway.Telegram.WebhookSecret)
	require.True(t, cfg.Gateway.Slack.Enabled)
	require.Equal(t, GatewaySlackModeHTTP, cfg.Gateway.Slack.Mode)
	require.Equal(t, "CONFIG_HAND_GATEWAY_SLACK_BOT_TOKEN", cfg.Gateway.Slack.BotToken)
	require.Equal(t, "CONFIG_HAND_GATEWAY_SLACK_APP_TOKEN", cfg.Gateway.Slack.AppToken)
	require.Equal(t, "CONFIG_HAND_GATEWAY_SLACK_SIGNING_SECRET", cfg.Gateway.Slack.SigningSecret)
}

func TestLoad_UsesGatewayCredentialEnvVars(t *testing.T) {
	clearEnvKeys(t,
		"HAND_GATEWAY_AUTH_TOKEN",
		"HAND_GATEWAY_TELEGRAM_BOT_TOKEN",
		"HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET",
		"HAND_GATEWAY_SLACK_BOT_TOKEN",
		"HAND_GATEWAY_SLACK_APP_TOKEN",
		"HAND_GATEWAY_SLACK_SIGNING_SECRET",
	)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(strings.Join([]string{
		"HAND_GATEWAY_AUTH_TOKEN=generic-token",
		"HAND_GATEWAY_TELEGRAM_BOT_TOKEN=telegram-token",
		"HAND_GATEWAY_TELEGRAM_WEBHOOK_SECRET=telegram-secret",
		"HAND_GATEWAY_SLACK_BOT_TOKEN=slack-bot-token",
		"HAND_GATEWAY_SLACK_APP_TOKEN=slack-app-token",
		"HAND_GATEWAY_SLACK_SIGNING_SECRET=slack-signing-secret",
		"",
	}, "\n")), 0o600))

	cfg, err := Load(envPath, "")

	require.NoError(t, err)
	require.Equal(t, "generic-token", cfg.Gateway.AuthToken)
	require.Equal(t, "telegram-token", cfg.Gateway.Telegram.BotToken)
	require.Equal(t, "telegram-secret", cfg.Gateway.Telegram.WebhookSecret)
	require.Equal(t, "slack-bot-token", cfg.Gateway.Slack.BotToken)
	require.Equal(t, "slack-app-token", cfg.Gateway.Slack.AppToken)
	require.Equal(t, "slack-signing-secret", cfg.Gateway.Slack.SigningSecret)
}

func TestLoad_UsesSafetyConfigFromConfigAndEnv(t *testing.T) {
	clearEnvKeys(t, "HAND_SAFETY_INPUT", "HAND_SAFETY_OUTPUT", "HAND_SAFETY_PII")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(strings.Join([]string{
		"HAND_SAFETY_INPUT=true",
		"HAND_SAFETY_OUTPUT=false",
		"HAND_SAFETY_PII=true",
		"",
	}, "\n")), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
safety:
  input: false
  output: true
  pii: false
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.True(t, cfg.InputSafetyEnabled())
	require.False(t, cfg.OutputSafetyEnabled())
	require.True(t, cfg.OutputPIIRedactionEnabled())
}

func TestLoad_UsesTUIConfigFromConfigAndEnv(t *testing.T) {
	clearEnvKeys(t, "HAND_TUI_THINKING_COMPOSER")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_TUI_THINKING_COMPOSER=true\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
tui:
  thinkingComposer: false
`), 0o600))

	cfg, err := Load(envPath, configPath)

	require.NoError(t, err)
	require.True(t, cfg.TUIThinkingComposerEnabled())
}

func TestLoad_IgnoresInvalidMaxIterationsEnvOverride(t *testing.T) {
	clearEnvKeys(t, "HAND_SESSION_MAX_ITERATIONS", "HAND_MODEL_API")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_SESSION_MAX_ITERATIONS=invalid\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
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

func TestApplyEnvOverrides_IgnoresInvalidWebCacheTTL(t *testing.T) {
	clearEnvKeys(t, "HAND_WEB_CACHE_TTL")
	t.Setenv("HAND_WEB_CACHE_TTL", "not-a-duration")

	cfg := &Config{}
	applyEnvOverrides(cfg)
	cfg.Normalize()

	require.Equal(t, constants.DefaultWebCacheTTL, cfg.Web.CacheTTL)
}

func TestApplyEnvOverrides_CoversRemainingBranches(t *testing.T) {
	clearEnvKeys(t,
		"HAND_MODEL_CONTEXT_LENGTH", "HAND_MODEL_MAX_RETRIES", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
		"HAND_STORAGE_BACKEND", "HAND_SESSION_DEFAULT_IDLE_EXPIRY", "HAND_SESSION_ARCHIVE_RETENTION",
		"HAND_SEARCH_VECTOR_ENABLED", "HAND_MODEL_EMBEDDING_PROVIDER",
		"HAND_MODEL_EMBEDDING_MODEL", "HAND_SEARCH_VECTOR_REQUIRED",
		"HAND_SEARCH_VECTOR_REBUILD_BATCH_SIZE", "HAND_SEARCH_ENABLE_RERANK", "HAND_RERANKER_ENABLED",
		"HAND_RERANKER_TYPE", "HAND_RERANKER_MODEL", "HAND_RERANKER_MAX_CANDIDATES",
		"HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS", "HAND_RERANKER_MAX_OUTPUT_TOKENS", "HAND_RERANKER_OVERRIDES",
		"HAND_COMPACTION_ENABLED", "HAND_COMPACTION_TRIGGER_PERCENT", "HAND_COMPACTION_WARN_PERCENT",
		"HAND_MEMORY_ENABLED", "HAND_MEMORY_PROVIDER", "HAND_MEMORY_BACKEND",
		"HAND_MEMORY_PINNED_ENABLED", "HAND_MEMORY_PINNED_MAX_CHARS", "HAND_MEMORY_PINNED_MAX_ITEM_CHARS",
		"HAND_MEMORY_EPISODIC_ENABLED", "HAND_MEMORY_EPISODIC_INTERVAL",
		"HAND_MEMORY_EPISODIC_IDLE_AFTER", "HAND_MEMORY_EPISODIC_MIN_MESSAGES",
		"HAND_MEMORY_EPISODIC_WINDOW_SIZE", "HAND_MEMORY_EPISODIC_MAX_WINDOWS",
		"HAND_MEMORY_EPISODIC_MAX_WINDOW_CHARS", "HAND_MEMORY_EPISODIC_MAX_WINDOW_TOKENS",
		"HAND_MEMORY_EPISODIC_MAX_RETRIES",
		"HAND_TUI_THINKING_COMPOSER",
		"HAND_FIRECRAWL_API_KEY", "HAND_FIRECRAWL_API_URL", "HAND_PARALLEL_API_KEY", "HAND_TAVILY_API_KEY", "HAND_EXA_API_KEY",
	)

	cfg := &Config{}
	applyEnvOverrides(nil)

	t.Setenv("HAND_MODEL_CONTEXT_LENGTH", "64000")
	t.Setenv("HAND_MODEL_MAX_RETRIES", "0")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
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
	t.Setenv("HAND_RERANKER_TYPE", constants.RerankerLLM)
	t.Setenv("HAND_RERANKER_MODEL", "gpt-4o-mini")
	t.Setenv("HAND_RERANKER_MAX_CANDIDATES", "12")
	t.Setenv("HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS", "700")
	t.Setenv("HAND_RERANKER_MAX_OUTPUT_TOKENS", "256")
	t.Setenv("HAND_RERANKER_OVERRIDES", `{"memory_reflection":{"type":"llm","model":"gpt-4o-mini","maxCandidates":7,"maxCandidateTextChars":500,"maxOutputTokens":96}}`)
	t.Setenv("HAND_COMPACTION_ENABLED", "false")
	t.Setenv("HAND_COMPACTION_TRIGGER_PERCENT", "0.5")
	t.Setenv("HAND_COMPACTION_WARN_PERCENT", "0.8")
	t.Setenv("HAND_MEMORY_ENABLED", "true")
	t.Setenv("HAND_MEMORY_PROVIDER", " Default-Memory ")
	t.Setenv("HAND_MEMORY_BACKEND", " SQLite ")
	t.Setenv("HAND_MEMORY_PINNED_ENABLED", "false")
	t.Setenv("HAND_MEMORY_RETRIEVAL_ENABLED", "false")
	t.Setenv("HAND_MEMORY_FLUSH_ENABLED", "true")
	t.Setenv("HAND_MEMORY_FLUSH_MAX_CALLS", "3")
	t.Setenv("HAND_MEMORY_FLUSH_MAX_OUTPUT_TOKENS", "256")
	t.Setenv("HAND_MEMORY_FLUSH_TIMEOUT", "4s")
	t.Setenv("HAND_MEMORY_PINNED_MAX_CHARS", "3200")
	t.Setenv("HAND_MEMORY_PINNED_MAX_ITEM_CHARS", "700")
	t.Setenv("HAND_MEMORY_EPISODIC_ENABLED", "true")
	t.Setenv("HAND_MEMORY_EPISODIC_INTERVAL", "20m")
	t.Setenv("HAND_MEMORY_EPISODIC_IDLE_AFTER", "10m")
	t.Setenv("HAND_MEMORY_EPISODIC_MIN_MESSAGES", "5")
	t.Setenv("HAND_MEMORY_EPISODIC_WINDOW_SIZE", "10")
	t.Setenv("HAND_MEMORY_EPISODIC_MAX_WINDOWS", "3")
	t.Setenv("HAND_MEMORY_EPISODIC_MAX_WINDOW_CHARS", "4000")
	t.Setenv("HAND_MEMORY_EPISODIC_MAX_WINDOW_TOKENS", "1000")
	t.Setenv("HAND_MEMORY_EPISODIC_MAX_RETRIES", "2")
	t.Setenv("HAND_MEMORY_WRITE_ENABLED", "false")
	t.Setenv("HAND_TUI_THINKING_COMPOSER", "false")

	applyEnvOverrides(cfg)

	require.Equal(t, 64000, cfg.Models.Main.ContextLength)
	require.Equal(t, 0, cfg.ModelMaxRetriesEffective())
	require.Equal(t, "memory", cfg.Storage.Backend)
	require.False(t, cfg.TUIThinkingComposerEnabled())
	require.Equal(t, 2*time.Hour, cfg.Session.DefaultIdleExpiry)
	require.Equal(t, 48*time.Hour, cfg.Session.ArchiveRetention)
	require.True(t, cfg.Search.Vector.Enabled)
	require.Equal(t, "test", cfg.Models.Embedding.Provider)
	require.Equal(t, "text-embedding-test", cfg.Models.Embedding.Name)
	require.True(t, cfg.Search.Vector.Required)
	require.Equal(t, 32, cfg.Search.Vector.RebuildBatchSize)
	require.False(t, getBoolValueDefault(cfg.Search.EnableRerank, true))
	require.False(t, getBoolValueDefault(cfg.Reranker.Enabled, true))
	require.Equal(t, constants.RerankerLLM, cfg.Reranker.Type)
	require.Equal(t, "gpt-4o-mini", cfg.Reranker.Model)
	require.Equal(t, 12, cfg.Reranker.MaxCandidates)
	require.Equal(t, 700, cfg.Reranker.MaxCandidateTextChars)
	require.Equal(t, 256, cfg.Reranker.MaxOutputTokens)
	require.Equal(t, RerankerOverrideConfig{
		Type:                  constants.RerankerLLM,
		Model:                 "gpt-4o-mini",
		MaxCandidates:         testIntPtr(7),
		MaxCandidateTextChars: testIntPtr(500),
		MaxOutputTokens:       testIntPtr(96),
	}, cfg.Reranker.Overrides["memory_reflection"])
	require.False(t, getBoolValue(cfg.Compaction.Enabled))
	require.Equal(t, 0.5, cfg.Compaction.TriggerPercent)
	require.Equal(t, 0.8, cfg.Compaction.WarnPercent)
	require.True(t, cfg.MemoryEnabled())
	require.Equal(t, "default-memory", cfg.Memory.Provider)
	require.Equal(t, "sqlite", cfg.Memory.Backend)
	require.False(t, getBoolValue(cfg.Memory.Pinned.Enabled))
	require.False(t, getBoolValue(cfg.Memory.Retrieval.Enabled))
	require.True(t, getBoolValue(cfg.Memory.Flush.Enabled))
	require.Equal(t, 3, cfg.Memory.Flush.MaxCalls)
	require.Equal(t, int64(256), cfg.Memory.Flush.MaxOutputTokens)
	require.Equal(t, 4*time.Second, cfg.Memory.Flush.Timeout)
	require.Equal(t, 3200, cfg.Memory.Pinned.MaxChars)
	require.Equal(t, 700, cfg.Memory.Pinned.MaxItemChars)
	require.True(t, getBoolValue(cfg.Memory.Episodic.Enabled))
	require.Equal(t, 20*time.Minute, cfg.Memory.Episodic.Interval)
	require.Equal(t, 10*time.Minute, cfg.Memory.Episodic.IdleAfter)
	require.Equal(t, 5, cfg.Memory.Episodic.MinMessages)
	require.Equal(t, 10, cfg.Memory.Episodic.WindowSize)
	require.Equal(t, 3, cfg.Memory.Episodic.MaxWindows)
	require.Equal(t, 4000, cfg.Memory.Episodic.MaxWindowChars)
	require.Equal(t, 1000, cfg.Memory.Episodic.MaxWindowTokens)
	require.Equal(t, 2, cfg.Memory.Episodic.MaxRetries)
	require.False(t, getBoolValue(cfg.Memory.Write.Enabled))
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
		"HAND_MODEL_API", "HAND_MODEL_SUMMARY_API",
	)

	cfg := &Config{}
	t.Setenv("HAND_MODEL_SUMMARY", "gpt-4o-mini")
	t.Setenv("HAND_MODEL_SUMMARY_PROVIDER", "openai")
	t.Setenv("HAND_MODEL_SUMMARY_BASE_URL", "https://example.com/v1")
	t.Setenv("HAND_MODEL_API", "openai-responses")
	t.Setenv("HAND_MODEL_SUMMARY_API", "openai-responses")

	applyEnvOverrides(cfg)

	require.Equal(t, "gpt-4o-mini", cfg.Models.Summary.Name)
	require.Equal(t, "openai", cfg.Models.Summary.Provider)
	require.Equal(t, "https://example.com/v1", cfg.Models.Summary.BaseURL)
	require.Equal(t, "openai-responses", cfg.Models.Main.API)
	require.Equal(t, "openai-responses", cfg.Models.Summary.API)
}

func TestLoad_UsesModelAPIFromEnvOverride(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL_API")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("HAND_MODEL_API=openai-responses\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: config-model
    provider: openai
    api: openai-completions
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load(envPath, configPath)
	require.NoError(t, err)
	require.Equal(t, "openai-responses", cfg.Models.Main.API)
}
