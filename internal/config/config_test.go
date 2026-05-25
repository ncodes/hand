package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/datadir"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
	"github.com/wandxy/hand/internal/profile"
)

func stubModelProviderToken(t *testing.T, fn func(string) (StoredModelCredential, error)) {
	t.Helper()

	original := loadModelProviderToken
	loadModelProviderToken = fn
	t.Cleanup(func() {
		loadModelProviderToken = original
	})
}

func stubProviderDefaultBaseURL(t *testing.T, provider string, apiID string, value string) {
	t.Helper()

	api, ok := getModelAPI(apiID)
	require.True(t, ok)
	originalRegistry := modelRegistry
	modelRegistry = registryWithProviderBaseURL(t, originalRegistry, provider, api.ID, value)
	t.Cleanup(func() {
		modelRegistry = originalRegistry
	})
}

func stubModelRegistry(t *testing.T, registry *modelprovider.Registry) {
	t.Helper()

	originalRegistry := modelRegistry
	modelRegistry = registry
	t.Cleanup(func() {
		modelRegistry = originalRegistry
	})
}

func registryWithProviderBaseURL(
	t *testing.T,
	registry *modelprovider.Registry,
	provider string,
	api string,
	value string,
) *modelprovider.Registry {
	t.Helper()

	apis := []modelprovider.APIDefinition{
		{ID: modelprovider.APIOpenAICompletions},
		{ID: modelprovider.APIOpenAIResponses},
		{ID: modelprovider.APIOpenAIEmbeddings},
	}
	providers := make([]modelprovider.ProviderDefinition, 0, len(registry.GetProviderIDs()))
	matched := false
	for _, providerID := range registry.GetProviderIDs() {
		definition, ok := registry.GetProvider(providerID)
		require.True(t, ok)
		if providerID == provider {
			matched = true
			definition.BaseURLs[api] = value
		}
		providers = append(providers, definition)
	}
	require.True(t, matched)

	return modelprovider.NewRegistry(apis, providers, nil)
}

func registryWithGenerationModel(providerID string, modelID string, contextWindow int) *modelprovider.Registry {
	return modelprovider.NewRegistry(
		[]modelprovider.APIDefinition{
			{ID: modelprovider.APIOpenAICompletions},
			{ID: modelprovider.APIOpenAIResponses},
			{ID: modelprovider.APIOpenAIEmbeddings},
		},
		[]modelprovider.ProviderDefinition{
			{
				ID:             constants.ModelProviderOpenRouter,
				DefaultAPI:     modelprovider.APIOpenAIResponses,
				SupportsModels: true,
				BaseURLs: map[string]string{
					modelprovider.APIOpenAICompletions: constants.DefaultOpenRouterBaseURL,
					modelprovider.APIOpenAIResponses:   constants.DefaultOpenRouterResponsesBaseURL,
					modelprovider.APIOpenAIEmbeddings:  constants.DefaultOpenRouterEmbeddingsBaseURL,
				},
			},
			{
				ID:             constants.ModelProviderOpenAI,
				DefaultAPI:     modelprovider.APIOpenAIResponses,
				SupportsModels: true,
				BaseURLs: map[string]string{
					modelprovider.APIOpenAICompletions: constants.DefaultOpenAIBaseURL,
					modelprovider.APIOpenAIResponses:   constants.DefaultOpenAIBaseURL,
					modelprovider.APIOpenAIEmbeddings:  constants.DefaultOpenAIEmbeddingsBaseURL,
				},
			},
		},
		[]modelprovider.ModelDefinition{
			{
				ID:            modelID,
				Provider:      providerID,
				API:           modelprovider.APIOpenAIResponses,
				Input:         []modelprovider.InputKind{modelprovider.InputText},
				ContextWindow: contextWindow,
			},
		},
	)
}

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
HAND_PLATFORM=desktop
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
	require.Equal(t, "desktop", os.Getenv("HAND_PLATFORM"))
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

func TestNewDefaultConfig_ReturnsIndependentConfig(t *testing.T) {
	first := NewDefaultConfig()
	second := NewDefaultConfig()

	require.Equal(t, DefaultConfig.Models.Main.Name, first.Models.Main.Name)
	require.Equal(t, DefaultConfig.Models.Main.Provider, first.Models.Main.Provider)
	require.Empty(t, first.Web.Provider)
	require.Equal(t, DefaultConfig.RPC.Address, first.RPC.Address)
	require.Equal(t, DefaultConfig.RPC.Port, first.RPC.Port)
	require.True(t, first.FS.NoProfileAccess)
	require.True(t, first.InputSafetyEnabled())
	require.True(t, first.OutputSafetyEnabled())
	require.False(t, first.OutputPIIRedactionEnabled())
	require.NotEmpty(t, first.FS.Roots)
	require.Equal(t, constants.RerankerDeterministic, first.Reranker.Type)
	require.Equal(t, constants.RerankerDeterministic, first.RerankerEffective())
	require.Equal(t, constants.DefaultProfileRerankerMaxCandidates, first.Reranker.MaxCandidates)
	require.Equal(t, constants.DefaultProfileRerankerMaxCandidateTextChars, first.Reranker.MaxCandidateTextChars)
	require.Equal(t, constants.DefaultProfileRerankerMaxOutputTokens, first.Reranker.MaxOutputTokens)
	require.Equal(t, map[string]RerankerOverrideConfig{
		"memory_episodic_extraction": {Type: constants.RerankerLLM},
		"memory_promotion":           {Type: constants.RerankerLLM},
		"memory_reflection":          {Type: constants.RerankerLLM},
	}, first.Reranker.Overrides)

	*first.Safety.Input = false
	*first.Safety.Output = false
	*first.Safety.PII = true
	first.FS.Roots[0] = "mutated"
	first.Reranker.Overrides["memory_reflection"] = RerankerOverrideConfig{Type: constants.RerankerNoop}

	require.True(t, *second.Safety.Input)
	require.True(t, *second.Safety.Output)
	require.False(t, *second.Safety.PII)
	require.NotEqual(t, "mutated", second.FS.Roots[0])
	require.True(t, *DefaultConfig.TUI.ThinkingComposer)
	require.True(t, *DefaultConfig.Safety.Input)
	require.True(t, *DefaultConfig.Safety.Output)
	require.False(t, *DefaultConfig.Safety.PII)
	require.Equal(t, constants.RerankerLLM, second.Reranker.Overrides["memory_reflection"].Type)
	require.Equal(t, constants.RerankerLLM, DefaultConfig.Reranker.Overrides["memory_reflection"].Type)
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

func TestLoad_PersonalitiesParseNormalizeAndResolveSoulPaths(t *testing.T) {
	originalProfile := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(originalProfile)
	})

	profileHome := t.TempDir()
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: profileHome}))

	profileSoul := filepath.Join(profileHome, "personalities", "researcher", "SOUL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(profileSoul), 0o700))
	require.NoError(t, os.WriteFile(profileSoul, []byte("profile soul"), 0o600))

	configDir := t.TempDir()
	configSoul := filepath.Join(configDir, "personalities", "reviewer", "SOUL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(configSoul), 0o700))
	require.NoError(t, os.WriteFile(configSoul, []byte("config soul"), 0o600))

	configPath := filepath.Join(configDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
personalities:
  Researcher:
    soul: personalities/researcher/SOUL.md
    instruct: " Prefer evidence-backed answers. "
    state: ""
    memory:
      pinned: true
      retrieval: true
      write: false
      episodic: false
      reflection: true
      promotion: false
      flush: true
    tools:
      fs: true
      net: true
      exec: false
      mem: read
    model:
      name: openai/gpt-4o-mini
      provider: OpenRouter
      api: openai-responses
      baseUrl: " https://models.example "
      stream: false
    maxIterations: 7
  reviewer:
    soul: personalities/reviewer/SOUL.md
    state: readonly
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)

	researcher := cfg.Personalities["researcher"]
	require.Equal(t, profileSoul, researcher.Soul)
	require.Equal(t, "Prefer evidence-backed answers.", researcher.Instruct)
	require.Equal(t, personalityStateShared, researcher.State)
	require.True(t, getBoolValue(researcher.Memory.Pinned))
	require.True(t, getBoolValue(researcher.Memory.Retrieval))
	require.False(t, getBoolValue(researcher.Memory.Write))
	require.False(t, getBoolValue(researcher.Memory.Episodic))
	require.True(t, getBoolValue(researcher.Memory.Reflection))
	require.False(t, getBoolValue(researcher.Memory.Promotion))
	require.True(t, getBoolValue(researcher.Memory.Flush))
	require.True(t, getBoolValue(researcher.Tools.Filesystem))
	require.True(t, getBoolValue(researcher.Tools.Network))
	require.False(t, getBoolValue(researcher.Tools.Exec))
	require.Equal(t, personalityToolMemoryRead, researcher.Tools.Memory)
	require.Equal(t, "openai/gpt-4o-mini", researcher.Model.Name)
	require.Equal(t, "openrouter", researcher.Model.Provider)
	require.Equal(t, "openai-responses", researcher.Model.API)
	require.Equal(t, "https://models.example", researcher.Model.BaseURL)
	require.False(t, getBoolValueDefault(researcher.Model.Stream, true))
	require.Equal(t, 7, researcher.MaxIterations)

	reviewer := cfg.Personalities["reviewer"]
	require.Equal(t, configSoul, reviewer.Soul)
	require.Equal(t, personalityStateReadonly, reviewer.State)
}

func TestResolvePersonalitySoulPath_LeavesEmptyAndAbsolutePaths(t *testing.T) {
	absolutePath := filepath.Join(string(os.PathSeparator), "profiles", "researcher", "SOUL.md")

	require.Empty(t, resolvePersonalitySoulPath("", t.TempDir()))
	require.Equal(t, absolutePath, resolvePersonalitySoulPath(absolutePath, t.TempDir()))
}

func TestCloneConfig_ClonesPersonalityPointers(t *testing.T) {
	cfg := Config{
		Personalities: map[string]PersonalityConfig{
			"researcher": {
				Memory: PersonalityMemoryConfig{
					Pinned: new(true),
				},
				Tools: PersonalityToolsConfig{
					Filesystem: new(true),
				},
				Model: MainModelConfig{
					Stream: new(false),
				},
			},
		},
	}

	cloned := cloneConfig(cfg)
	*cloned.Personalities["researcher"].Memory.Pinned = false
	*cloned.Personalities["researcher"].Tools.Filesystem = false
	*cloned.Personalities["researcher"].Model.Stream = true

	require.True(t, *cfg.Personalities["researcher"].Memory.Pinned)
	require.True(t, *cfg.Personalities["researcher"].Tools.Filesystem)
	require.False(t, *cfg.Personalities["researcher"].Model.Stream)
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
platform: desktop
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
	require.Equal(t, "desktop", cfg.Platform)
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

func TestConfig_SafetyDefaultsAndValidation(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.True(t, cfg.InputSafetyEnabled())
	require.True(t, cfg.OutputSafetyEnabled())
	require.False(t, cfg.OutputPIIRedactionEnabled())

	cfg.Safety.Input = new(false)
	cfg.Safety.Output = new(false)
	require.False(t, cfg.InputSafetyEnabled())
	require.False(t, cfg.OutputSafetyEnabled())

	cfg.Safety.PII = new(true)
	require.True(t, cfg.OutputPIIRedactionEnabled())
}

func TestConfig_StreamEnabledDefaultsToTrue(t *testing.T) {
	require.True(t, (&Config{}).StreamEnabled())
	require.False(t, (&Config{Models: ModelsConfig{Main: MainModelConfig{Stream: new(false)}}}).StreamEnabled())
}

func TestLoad_UsesRegistryModelMetadataWhenContextLengthIsUnset(t *testing.T) {
	stubModelRegistry(t, registryWithGenerationModel(constants.ModelProviderOpenRouter, "openai/test-chat-small", 8191))

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: openai/test-chat-small
    provider: openrouter
    contextLength: 0
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 8191, cfg.Models.Main.ContextLength)
}

func TestLoad_UsesRegistryModelMetadataWhenConfiguredContextLengthIsTooLarge(t *testing.T) {
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
    contextLength: 999999
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 8191, cfg.Models.Main.ContextLength)
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

func TestLoad_LeavesFreeFormModelContextLengthAtDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: openai/unregistered-model
    provider: openrouter
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, constants.DefaultContextLength, cfg.Models.Main.ContextLength)
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

func TestConfig_ValidateRequiresKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{Name: constants.DefaultModel}},
		Log:    LogConfig{Level: "info"},
	}
	require.EqualError(t, cfg.Validate(), "model API key is required; set a provider API key, provider env var, or role apiKey")
	require.Equal(t, constants.DefaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, getDefaultBaseURLForProvider(constants.DefaultModelProvider, modelprovider.APIOpenAIResponses), cfg.Models.Main.BaseURL)
}

func TestConfig_ValidateNilConfig(t *testing.T) {
	var cfg *Config
	require.EqualError(t, cfg.Validate(), "config is required")
}

func TestConfig_ResolveModelAuthUsesOpenRouterSpecificKey(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "openrouter-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
		},
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openrouter", auth.Provider)
	require.Equal(t, "openrouter-key", auth.APIKey)
	require.Equal(t, getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAIResponses), auth.BaseURL)
}

func TestConfig_ResolveModelAuthUsesOpenAISpecificKey(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openai": {APIKey: "openai-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openai"},
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
			Providers: map[string]ProviderModelConfig{"openai": {APIKey: "openai-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openai"},
		},
	}

	auth, err := cfg.ResolveModelAuth()

	require.NoError(t, err)
	require.Equal(t, "openai", auth.Provider)
	require.Equal(t, "openai-key", auth.APIKey)
	require.Equal(t, "https://api.openai.com/v1", auth.BaseURL)
}

func TestConfig_ResolveModelAuthUsesCredentialResolverOrder(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "registry-env-key")
	t.Setenv("CUSTOM_OPENROUTER_KEY", "provider-env-key")
	expiresAt := time.Now().Add(time.Hour)
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, "openrouter", provider)
		return StoredModelCredential{Token: "store-key", ExpiresAt: &expiresAt}, nil
	})

	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{
				"openrouter": {
					APIKey:    "provider-config-key",
					APIKeyEnv: []string{"CUSTOM_OPENROUTER_KEY"},
				},
			},
			Main: MainModelConfig{
				Name:     constants.DefaultModel,
				Provider: "openrouter",
				APIKey:   "role-key",
			},
		},
	}

	auth, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "role-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig}, auth.CredentialSource)

	cfg.Models.Main.APIKey = ""
	auth, err = cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "provider-config-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{
		Kind: ModelCredentialSourceProviderConfig,
		Name: "openrouter",
	}, auth.CredentialSource)

	cfg.Models.Providers["openrouter"] = ProviderModelConfig{APIKeyEnv: []string{"CUSTOM_OPENROUTER_KEY"}}
	auth, err = cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "provider-env-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{
		Kind: ModelCredentialSourceProviderEnv,
		Name: "CUSTOM_OPENROUTER_KEY",
	}, auth.CredentialSource)

	cfg.Models.Providers["openrouter"] = ProviderModelConfig{}
	auth, err = cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "registry-env-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{
		Kind: ModelCredentialSourceProviderEnv,
		Name: "OPENROUTER_API_KEY",
	}, auth.CredentialSource)

	t.Setenv("OPENROUTER_API_KEY", "")
	auth, err = cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "store-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{
		Kind:      ModelCredentialSourceTokenStore,
		Name:      "openrouter",
		HasExpiry: true,
	}, auth.CredentialSource)
}

func TestConfig_ResolveSummaryModelAuthUsesMainRoleKeyWhenRouteMatches(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Main: MainModelConfig{
				Name:     constants.DefaultModel,
				Provider: "openrouter",
				APIKey:   "main-role-key",
			},
		},
	}

	main, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	summary, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)

	require.True(t, ModelAuthEqual(main, summary))
	require.Equal(t, "main-role-key", summary.APIKey)
	require.Equal(t, ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig}, summary.CredentialSource)
}

func TestConfig_ResolveSummaryModelAuthUsesSummaryRoleKeyWhenSet(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Main:    MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter", APIKey: "main-role-key"},
			Summary: SummaryModelConfig{APIKey: "summary-role-key"},
		},
	}

	auth, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)
	require.Equal(t, "summary-role-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig}, auth.CredentialSource)
}

func TestConfig_ResolveModelAuthUsesProviderTokenStore(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	stubModelProviderToken(t, func(provider string) (StoredModelCredential, error) {
		require.Equal(t, "openai", provider)
		return StoredModelCredential{Token: "stored-token"}, nil
	})

	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{Name: constants.DefaultModel, Provider: "openai"}},
	}

	auth, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "stored-token", auth.APIKey)
	require.Equal(t, ModelCredentialSource{
		Kind: ModelCredentialSourceTokenStore,
		Name: "openai",
	}, auth.CredentialSource)
}

func TestConfig_ResolveModelAuthReturnsProviderTokenStoreError(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	stubModelProviderToken(t, func(string) (StoredModelCredential, error) {
		return StoredModelCredential{}, errors.New("token store failed")
	})

	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{Name: constants.DefaultModel, Provider: "openai"}},
	}

	_, err := cfg.ResolveModelAuth()
	require.EqualError(t, err, "token store failed")
}

func TestConfig_ResolveEmbeddingModelAuth(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "router-key"}},
			Main:      MainModelConfig{Provider: "openrouter"},
			Embedding: EmbeddingModelConfig{Provider: "openrouter"},
		},
	}

	auth, err := cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider:         "openrouter",
		API:              modelprovider.APIOpenAIEmbeddings,
		APIKey:           "router-key",
		BaseURL:          "https://openrouter.ai/api/v1/embeddings",
		CredentialSource: ModelCredentialSource{Kind: ModelCredentialSourceProviderConfig, Name: "openrouter"},
	}, auth)

	cfg = &Config{
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "router-key"}},
			Main:      MainModelConfig{Provider: "openrouter", API: modelprovider.APIOpenAIResponses},
			Embedding: EmbeddingModelConfig{Provider: "openrouter"},
		},
	}

	auth, err = cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAIEmbeddings), auth.BaseURL)

	cfg = &Config{
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "router-key"}},
			Main:      MainModelConfig{Provider: "openrouter"},
		},
	}

	auth, err = cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider:         "openrouter",
		API:              modelprovider.APIOpenAIEmbeddings,
		APIKey:           "router-key",
		BaseURL:          "https://openrouter.ai/api/v1/embeddings",
		CredentialSource: ModelCredentialSource{Kind: ModelCredentialSourceProviderConfig, Name: "openrouter"},
	}, auth)

	cfg = &Config{
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openai": {APIKey: "openai-key"}},
			Main:      MainModelConfig{Provider: "openrouter"},
			Embedding: EmbeddingModelConfig{Provider: "openai"},
		},
	}

	auth, err = cfg.ResolveEmbeddingModelAuth()

	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider:         "openai",
		API:              modelprovider.APIOpenAIEmbeddings,
		APIKey:           "openai-key",
		BaseURL:          "https://api.openai.com/v1/embeddings",
		CredentialSource: ModelCredentialSource{Kind: ModelCredentialSourceProviderConfig, Name: "openai"},
	}, auth)

	cfg = &Config{
		Models: ModelsConfig{
			Embedding: EmbeddingModelConfig{
				Provider: "openai",
				APIKey:   "embedding-role-key",
			},
		},
	}
	auth, err = cfg.ResolveEmbeddingModelAuth()
	require.NoError(t, err)
	require.Equal(t, "embedding-role-key", auth.APIKey)
	require.Equal(t, ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig}, auth.CredentialSource)

	t.Setenv("OPENAI_API_KEY", "")
	_, err = (&Config{Models: ModelsConfig{Embedding: EmbeddingModelConfig{Provider: "openai"}}}).ResolveEmbeddingModelAuth()
	require.EqualError(t, err, "embedding API key is required")

	_, err = (&Config{
		Models: ModelsConfig{Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}}, Embedding: EmbeddingModelConfig{Provider: "test"}},
	}).ResolveEmbeddingModelAuth()
	require.EqualError(t, err, "embedding provider must be one of: anthropic, github-copilot, openai, openrouter")
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
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "openrouter-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
		},
		Log: LogConfig{Level: "info"},
	}

	require.NoError(t, cfg.Validate())
}

func TestConfig_ValidateNormalizesFields(t *testing.T) {
	cfg := &Config{
		Name: "  Test Agent  ",
		Models: ModelsConfig{
			Main: MainModelConfig{Name: "  openai/test-model  ", Provider: " OpenRouter ", APIKey: "  test-key  "},
		},
		Log: LogConfig{Level: " WARN "},
	}

	require.NoError(t, cfg.Validate())
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "openai/test-model", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "test-key", cfg.Models.Main.APIKey)
	require.Equal(t, getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAIResponses), cfg.Models.Main.BaseURL)
	require.Equal(t, "warn", cfg.Log.Level)
}

func TestConfig_ValidateRequiresName(t *testing.T) {
	err := (&Config{
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel}},
		Log:    LogConfig{Level: "info"},
	}).Validate()
	require.EqualError(t, err, "name is required; set HAND_NAME, provide it in config, or use --name")
}

func TestConfig_ValidatePersonalityNames(t *testing.T) {
	err := (&Config{
		Personalities: map[string]PersonalityConfig{
			"work/team": {},
		},
	}).Validate()
	require.EqualError(t, err, `invalid personality name "work/team": must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,63}`)

	err = (&Config{
		Personalities: map[string]PersonalityConfig{
			"Researcher": {},
			"researcher": {},
		},
	}).Validate()
	require.EqualError(t, err, `duplicate personality name "researcher" conflicts with "Researcher"`)
}

func TestConfig_ValidateAcceptsValidPersonalitySettings(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
		},
		Personalities: map[string]PersonalityConfig{
			"Researcher": {
				State: personalityStateIsolated,
				Tools: PersonalityToolsConfig{
					Memory: personalityToolMemoryWrite,
				},
				Model: MainModelConfig{
					Name:     "openai/gpt-4o-mini",
					Provider: "OpenAI",
					API:      modelprovider.APIOpenAIResponses,
				},
				MaxIterations: 3,
			},
			"reviewer": {
				State: personalityStateReadonly,
			},
		},
	}

	require.NoError(t, cfg.Validate())
	require.Contains(t, cfg.Personalities, "researcher")
	require.Equal(t, "openai", cfg.Personalities["researcher"].Model.Provider)
	require.Equal(t, modelprovider.APIOpenAIResponses, cfg.Personalities["researcher"].Model.API)
	require.Equal(t, personalityStateReadonly, cfg.Personalities["reviewer"].State)
}

func TestConfig_ValidatePersonalitySettings(t *testing.T) {
	cases := []struct {
		name          string
		personality   PersonalityConfig
		expectedError string
	}{
		{
			name:          "invalid state",
			personality:   PersonalityConfig{State: "solo"},
			expectedError: "personalities.researcher.state must be one of: shared, isolated, readonly",
		},
		{
			name:          "invalid memory tool mode",
			personality:   PersonalityConfig{Tools: PersonalityToolsConfig{Memory: "admin"}},
			expectedError: "personalities.researcher.tools.mem must be one of: none, read, write",
		},
		{
			name:          "invalid max iterations",
			personality:   PersonalityConfig{MaxIterations: -1},
			expectedError: "personalities.researcher.maxIterations must be non-negative",
		},
		{
			name:          "invalid model name",
			personality:   PersonalityConfig{Model: MainModelConfig{Name: "gpt-4o-mini"}},
			expectedError: "personalities.researcher.model.name must use the format <owner>/<name>",
		},
		{
			name:          "invalid model provider",
			personality:   PersonalityConfig{Model: MainModelConfig{Provider: "other"}},
			expectedError: "personalities.researcher.model.provider must be one of: anthropic, github-copilot, openai, openrouter",
		},
		{
			name:          "invalid model api mode",
			personality:   PersonalityConfig{Model: MainModelConfig{API: "other"}},
			expectedError: "personalities.researcher.model.api must be one of: anthropic-messages, openai-completions, openai-responses",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (&Config{
				Personalities: map[string]PersonalityConfig{
					"researcher": tc.personality,
				},
			}).Validate()

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

func TestConfig_ValidateDefaultsModelWhenEmpty(t *testing.T) {
	cfg := &Config{Name: "test-agent", Models: ModelsConfig{Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}}}, Log: LogConfig{Level: "info"}}
	require.NoError(t, cfg.Validate())
	require.Equal(t, constants.DefaultModel, cfg.Models.Main.Name)
}

func TestConfig_ValidateRejectsModelWithoutOwnerPrefix(t *testing.T) {
	err := (&Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: "gpt-4o-mini", Provider: "openai"}},
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
				Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: model, Provider: "openai"}},
				RPC:    RPCConfig{Address: "127.0.0.1", Port: 50051},
				Log:    LogConfig{Level: "info"},
			}).Validate()

			require.EqualError(t, err, "model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
		})
	}
}

func TestConfig_ValidateRejectsUnsupportedProvider(t *testing.T) {
	openRouterDefault := getDefaultBaseURLForProvider(constants.DefaultModelProvider, modelprovider.APIOpenAIResponses)
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "missing", BaseURL: openRouterDefault},
		},
		Log: LogConfig{Level: "info"},
	}).Validate()
	require.EqualError(t, err, "model provider must be one of: anthropic, github-copilot, openai, openrouter")
}

func TestConfig_ValidateRejectsProviderAPIIncompatibilityWithoutNetwork(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Main: MainModelConfig{
				APIKey:   "test-key",
				Name:     constants.DefaultModel,
				Provider: "anthropic",
				API:      modelprovider.APIOpenAIResponses,
				BaseURL:  "https://api.example/v1",
			},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `model API "openai-responses" is not supported by provider "anthropic"`)
}

func TestConfig_ValidateRejectsFreeFormModelWhenProviderRequiresKnownModels(t *testing.T) {
	stubModelRegistry(t, modelprovider.NewRegistry(
		[]modelprovider.APIDefinition{{ID: modelprovider.APIOpenAIResponses}},
		[]modelprovider.ProviderDefinition{
			{
				ID:                 "strict",
				DefaultAPI:         modelprovider.APIOpenAIResponses,
				SupportsModels:     true,
				RequiresKnownModel: true,
				BaseURLs: map[string]string{
					modelprovider.APIOpenAIResponses: "https://strict.example/v1",
				},
			},
		},
		[]modelprovider.ModelDefinition{
			{
				ID:       "known/model",
				Provider: "strict",
				API:      modelprovider.APIOpenAIResponses,
			},
		},
	))

	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Main: MainModelConfig{
				APIKey:   "test-key",
				Name:     "unknown/model",
				Provider: "strict",
			},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `models.main.name "unknown/model" is not registered for provider "strict"`)
}

func TestConfig_ValidateRejectsFreeFormModelWhenProviderDoesNotSupportModels(t *testing.T) {
	stubModelRegistry(t, modelprovider.NewRegistry(
		[]modelprovider.APIDefinition{{ID: modelprovider.APIOpenAIResponses}},
		[]modelprovider.ProviderDefinition{
			{
				ID:         "fixed",
				DefaultAPI: modelprovider.APIOpenAIResponses,
				BaseURLs: map[string]string{
					modelprovider.APIOpenAIResponses: "https://fixed.example/v1",
				},
			},
		},
		[]modelprovider.ModelDefinition{
			{
				ID:       "known/model",
				Provider: "fixed",
				API:      modelprovider.APIOpenAIResponses,
			},
		},
	))

	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Main: MainModelConfig{
				APIKey:   "test-key",
				Name:     "unknown/model",
				Provider: "fixed",
			},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `models.main.name "unknown/model" is not registered for provider "fixed"`)
}

func TestConfig_ValidateAllowsFreeFormModelForProviderThatSupportsModels(t *testing.T) {
	err := (&Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: "openai/gpt-unknown", Provider: "openrouter"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:    LogConfig{Level: "info"},
	}).Validate()

	require.NoError(t, err)
}

func TestConfig_ValidateRejectsKnownModelWithIncompatibleRole(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openai": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultProfileEmbeddingModel, Provider: "openai"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, `models.main.name "openai/text-embedding-3-small" is not compatible with this model role`)
}

func TestConfig_ValidateRejectsInvalidSummaryModelSlug(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openai": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openai"},
			Summary:   SummaryModelConfig{Name: "gpt-4o-mini"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "summary model must use the format <owner>/<name>; for example openai/gpt-4o-mini")
}

func TestConfig_ValidateAllowsFreeFormSummaryModelForProviderThatSupportsModels(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
			Summary:   SummaryModelConfig{Name: "openai/gpt-unknown-summary"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.NoError(t, err)
}

func TestConfig_SummaryModelEffective(t *testing.T) {
	t.Run("inherits_main_model_when_empty", func(t *testing.T) {
		cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{Name: constants.DefaultModel}}}
		require.Equal(t, constants.DefaultModel, cfg.SummaryModelEffective())
	})

	t.Run("uses_summary_when_set", func(t *testing.T) {
		cfg := &Config{
			Models: ModelsConfig{
				Main:    MainModelConfig{Name: constants.DefaultModel},
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

func TestConfig_SummaryModelAPIEffective(t *testing.T) {
	cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{API: modelprovider.APIOpenAIResponses}}}
	cfg.Normalize()
	require.Equal(t, modelprovider.APIOpenAIResponses, cfg.SummaryModelAPIEffective())

	cfg.Models.Summary.API = modelprovider.APIOpenAICompletions
	cfg.Normalize()
	require.Equal(t, modelprovider.APIOpenAICompletions, cfg.SummaryModelAPIEffective())
}

func TestConfig_ResolveSummaryModelAuth_UsesSummaryAPIForDefaultBaseURL(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "k"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter", API: modelprovider.APIOpenAICompletions},
			Summary:   SummaryModelConfig{API: modelprovider.APIOpenAIResponses},
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
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "k", Name: constants.DefaultModel, Provider: "openrouter"}},
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
			Providers: map[string]ProviderModelConfig{
				"openrouter": {APIKey: "k"},
				"openai":     {APIKey: "k"},
			},
			Main:    MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
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
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
			Summary:   SummaryModelConfig{Provider: "missing"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "summary model provider must be one of: anthropic, github-copilot, openai, openrouter")
}

func TestConfig_ValidateRejectsInvalidSummaryModelAPI(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
			Summary:   SummaryModelConfig{API: "invalid"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "summary model API must be one of: anthropic-messages, openai-completions, openai-responses")
}

func TestConfig_ValidateAcceptsSummaryModelAPIResponses(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
			Summary:   SummaryModelConfig{API: modelprovider.APIOpenAIResponses},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.NoError(t, err)
}

func TestConfig_ValidateAcceptsSummaryModelAPICompletions(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
			Summary:   SummaryModelConfig{API: modelprovider.APIOpenAICompletions},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.NoError(t, err)
}

func TestConfig_ModelAuthEqual(t *testing.T) {
	require.True(t, ModelAuthEqual(
		ModelAuth{Provider: "openai", API: modelprovider.APIOpenAIResponses, BaseURL: "http://a", APIKey: "k"},
		ModelAuth{Provider: "openai", API: modelprovider.APIOpenAIResponses, BaseURL: "http://a", APIKey: "k"},
	))
	require.False(t, ModelAuthEqual(
		ModelAuth{Provider: "openai", API: modelprovider.APIOpenAIResponses, BaseURL: "http://a", APIKey: "k"},
		ModelAuth{Provider: "openrouter", API: modelprovider.APIOpenAIResponses, BaseURL: "http://a", APIKey: "k"},
	))
	require.False(t, ModelAuthEqual(
		ModelAuth{Provider: "openai", API: modelprovider.APIOpenAIResponses, BaseURL: "http://a", APIKey: "k"},
		ModelAuth{Provider: "openai", API: modelprovider.APIOpenAICompletions, BaseURL: "http://a", APIKey: "k"},
	))
}

func TestConfig_ValidateRejectsInvalidLogLevel(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openai": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openai"},
		},
		Log: LogConfig{Level: "trace"},
	}).Validate()
	require.EqualError(t, err, "log level must be one of debug, info, warn, or error; use --log.level")
}

func TestConfig_ValidateAllowsEmptyProviderAndLogLevel(t *testing.T) {
	err := (&Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel}},
	}).Validate()
	require.NoError(t, err)
}

func TestConfig_ValidateRejectsEmptyRPCAddress(t *testing.T) {
	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel, Provider: "openai"}},
		RPC:    RPCConfig{Address: "   ", Port: 50051},
		Log:    LogConfig{Level: "info"},
	}

	require.EqualError(t, cfg.Validate(), "rpc address is required; set HAND_RPC_ADDRESS, provide it in config, or use --rpc.address")
}

func TestConfig_ValidateRejectsInvalidRPCPort(t *testing.T) {
	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel, Provider: "openai"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: -1},
		Log:    LogConfig{Level: "info"},
	}

	require.EqualError(t, cfg.Validate(), "rpc port must be non-negative; set HAND_RPC_PORT, provide it in config, or use --rpc.port")
}

func TestConfig_ValidateAllowsZeroRPCPortForDynamicBind(t *testing.T) {
	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel, Provider: "openai"}},
		RPC:    RPCConfig{Address: "127.0.0.1", Port: 0},
		Log:    LogConfig{Level: "info"},
	}

	require.NoError(t, cfg.Validate())
}

func TestConfig_ValidateRejectsInvalidMaxIterations(t *testing.T) {
	cfg := &Config{
		Name:    "test-agent",
		Models:  ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel, Provider: "openai"}},
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
			Providers:  map[string]ProviderModelConfig{"openai": {APIKey: "test-key"}},
			MaxRetries: &retries,
			Main:       MainModelConfig{Name: constants.DefaultModel, Provider: "openai"},
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
		Models:     ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel, Provider: "openai"}},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "memory"},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 1, WarnPercent: 1},
	}).Validate()

	require.EqualError(t, err, "compaction trigger percent must be greater than zero and less than one")

	err = (&Config{
		Name:       "test-agent",
		Models:     ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel, Provider: "openai"}},
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
		Models: ModelsConfig{Main: MainModelConfig{Name: constants.DefaultModel}},
		Log:    LogConfig{Level: "info"},
	}
	cfg.Normalize()
	require.Equal(t, constants.DefaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, getDefaultBaseURLForProvider(constants.DefaultModelProvider, modelprovider.APIOpenAIResponses), cfg.Models.Main.BaseURL)
}

func TestConfig_NormalizeIgnoresNilReceiver(t *testing.T) {
	var cfg *Config
	cfg.Normalize()
}

func TestConfig_NormalizeDefaultsModelAndLogLevel(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Empty(t, cfg.Name)
	require.Equal(t, constants.DefaultModel, cfg.Models.Main.Name)
	require.Equal(t, constants.DefaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, "cli", cfg.Platform)
	require.True(t, getBoolValue(cfg.Cap.Filesystem))
	require.True(t, getBoolValue(cfg.Cap.Network))
	require.True(t, getBoolValue(cfg.Cap.Exec))
	require.True(t, getBoolValue(cfg.Cap.Memory))
	require.False(t, getBoolValue(cfg.Cap.Browser))
	require.Equal(t, getDefaultBaseURLForProvider(constants.DefaultModelProvider, modelprovider.APIOpenAIResponses), cfg.Models.Main.BaseURL)
	require.Equal(t, "127.0.0.1", cfg.RPC.Address)
	require.Equal(t, 50051, cfg.RPC.Port)
	require.Equal(t, constants.DefaultMaxIterations, cfg.Session.MaxIterations)
	require.Equal(t, "info", cfg.Log.Level)
	require.Equal(t, constants.DefaultWebMaxCharPerResult, cfg.Web.MaxCharPerResult)
	require.Equal(t, constants.DefaultWebMaxExtractCharPerResult, cfg.Web.MaxExtractCharPerResult)
	require.Equal(t, constants.DefaultWebMaxExtractResponseBytes, cfg.Web.MaxExtractResponseBytes)
	require.Equal(t, constants.DefaultWebCacheTTL, cfg.Web.CacheTTL)
	require.False(t, cfg.Web.BlockedDomainsEnabled)
	require.Empty(t, cfg.Web.BlockedDomains)
	require.Empty(t, cfg.Web.BlockedDomainFiles)
	require.Empty(t, cfg.Web.NativeAllowedHosts)
	require.Empty(t, cfg.Web.NativeBlockedHosts)
	require.Empty(t, cfg.Web.NativeAllowedHostFiles)
	require.Empty(t, cfg.Web.NativeBlockedHostFiles)
	require.Equal(t, constants.DefaultWebExtractMinSummarizeChars, cfg.Web.ExtractMinSummarizeChars)
	require.Equal(t, constants.DefaultWebExtractMaxSummaryChars, cfg.Web.ExtractMaxSummaryChars)
	require.Equal(t, constants.DefaultWebExtractMaxSummaryChunkChars, cfg.Web.ExtractMaxSummaryChunkChars)
	require.Less(t, cfg.Web.ExtractMaxSummaryChunkChars, cfg.Web.MaxExtractCharPerResult)
	require.Equal(t, constants.DefaultWebExtractRefusalThresholdChars, cfg.Web.ExtractRefusalThresholdChars)
}

func TestConfig_NormalizeDisablesNegativeWebCacheTTL(t *testing.T) {
	cfg := &Config{Web: WebConfig{CacheTTL: -time.Second}}
	cfg.Normalize()
	require.Equal(t, constants.DefaultWebCacheTTL, cfg.Web.CacheTTL)
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

	require.Equal(t, constants.DefaultWebCacheTTL, cfg.Web.CacheTTL)
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

	require.False(t, getBoolValue(cfg.Cap.Filesystem))
	require.False(t, getBoolValue(cfg.Cap.Network))
	require.False(t, getBoolValue(cfg.Cap.Exec))
	require.False(t, getBoolValue(cfg.Cap.Memory))
	require.False(t, getBoolValue(cfg.Cap.Browser))
}

func TestConfig_NormalizeDefaultsUnsetCapabilitiesIndividually(t *testing.T) {
	cfg := &Config{Cap: CapConfig{Filesystem: new(false)}}

	cfg.Normalize()

	require.False(t, getBoolValue(cfg.Cap.Filesystem))
	require.True(t, getBoolValue(cfg.Cap.Network))
	require.True(t, getBoolValue(cfg.Cap.Exec))
	require.True(t, getBoolValue(cfg.Cap.Memory))
	require.False(t, getBoolValue(cfg.Cap.Browser))
}

func TestConfig_NormalizeUsesMappedBaseURLWhenProviderWasExplicitlySet(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{Main: MainModelConfig{Name: constants.DefaultModel, Provider: constants.DefaultModelProvider}},
		Log:    LogConfig{Level: "info"},
	}
	cfg.Normalize()
	require.Equal(t, constants.DefaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, getDefaultBaseURLForProvider(constants.DefaultModelProvider, modelprovider.APIOpenAIResponses), cfg.Models.Main.BaseURL)
}

func TestConfig_NormalizeKeepsOpenaiProvider(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "test-key", Name: constants.DefaultModel, Provider: "openai"}},
		Log:    LogConfig{Level: "info"},
	}
	cfg.Normalize()
	require.Equal(t, "openai", cfg.Models.Main.Provider)
	require.Equal(t, "https://api.openai.com/v1", cfg.Models.Main.BaseURL)
}

func TestConfig_NormalizeRemapsInheritedProviderDefaultBaseURL(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Models.Main.Provider = "openai"

	cfg.Normalize()

	require.Equal(t, "openai", cfg.Models.Main.Provider)
	require.Equal(t, "https://api.openai.com/v1", cfg.Models.Main.BaseURL)
}

func TestConfig_NormalizeDefaultBaseURLDependsOnAPI(t *testing.T) {
	t.Run("openai uses api root for completions and responses", func(t *testing.T) {
		for _, mode := range []string{modelprovider.APIOpenAICompletions, modelprovider.APIOpenAIResponses} {
			cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openai", API: mode}}}
			cfg.Normalize()
			require.Equal(t, "https://api.openai.com/v1", cfg.Models.Main.BaseURL, mode)
		}
	})

	t.Run("openrouter defaults differ by api mode", func(t *testing.T) {
		cfgChat := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openrouter", API: modelprovider.APIOpenAICompletions}}}
		cfgChat.Normalize()
		require.Equal(t, "https://openrouter.ai/api/v1", cfgChat.Models.Main.BaseURL)

		cfgResp := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openrouter", API: modelprovider.APIOpenAIResponses}}}
		cfgResp.Normalize()
		require.Equal(t, "https://openrouter.ai/api/v1/responses", cfgResp.Models.Main.BaseURL)
	})

	t.Run("unknown api mode does not fall back to default base url", func(t *testing.T) {
		cfg := &Config{Models: ModelsConfig{Main: MainModelConfig{Provider: "openrouter", API: "future-mode"}}}
		cfg.Normalize()
		require.Empty(t, cfg.Models.Main.BaseURL)
	})
}

func TestConfig_NormalizeTrimsAndLowercasesFields(t *testing.T) {
	cfg := &Config{
		Name: "  Test Agent  ",
		Models: ModelsConfig{
			Main: MainModelConfig{
				Name:     "  test-model  ",
				Provider: " OpenRouter ",
				APIKey:   "  test-key  ",
				BaseURL:  "  https://example.com/v1  ",
			},
		},
		Log: LogConfig{Level: " WARN "},
	}
	cfg.Normalize()
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, "test-model", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "test-key", cfg.Models.Main.APIKey)
	require.Equal(t, "https://example.com/v1", cfg.Models.Main.BaseURL)
	require.Equal(t, "warn", cfg.Log.Level)
}

func TestHelpers_SplitAndDedupeCSVAndBools(t *testing.T) {
	require.Nil(t, splitAndTrimCSV(""))
	require.Equal(t, []string{"a", "b"}, splitAndTrimCSV(" a, ,b ,,"))

	require.Nil(t, dedupeAndTrim(nil))
	require.Equal(t, []string{"a", "b"}, dedupeAndTrim([]string{" a ", "", "b", "a"}))

	require.False(t, getBoolValue(nil))
	require.True(t, getBoolValue(new(true)))
	require.True(t, getBoolValueDefault(nil, true))
	require.False(t, getBoolValueDefault(new(false), true))
}

func TestResolvePathsFromBase_HandlesEmptyAndAbsolute(t *testing.T) {
	require.Nil(t, getPathsFromBase(nil, "/tmp"))
	require.Equal(t, []string{"a", "b"}, getPathsFromBase([]string{"a", "b"}, ""))

	abs := filepath.Join(string(os.PathSeparator), "tmp", "x")
	require.Equal(t, []string{abs, filepath.Join("/base", "rel")},
		getPathsFromBase([]string{abs, "rel"}, "/base"))
}

func TestDefaultFSRootsAndNormalizeFSRootsFallbackWhenGetwdFails(t *testing.T) {
	originalGetwd := getwd
	t.Cleanup(func() {
		getwd = originalGetwd
	})

	getwd = func() (string, error) {
		return "", errors.New("cwd missing")
	}

	require.Equal(t, []string{"."}, getDefaultFSRoots())
	require.Equal(t, []string{"."}, normalizeFSRoots([]string{"."}))
}

func TestNormalizeFSRoots_PreservesAbsoluteRoots(t *testing.T) {
	abs := filepath.Join(string(os.PathSeparator), "tmp", "workspace")
	require.Equal(t, []string{abs}, normalizeFSRoots([]string{abs}))
}

func TestResolveModelAuth_CoversDefaultBranchAndNilReceiver(t *testing.T) {
	var cfg *Config
	_, err := cfg.ResolveModelAuth()
	require.EqualError(t, err, "config is required")

	cfg = &Config{
		Models: ModelsConfig{Main: MainModelConfig{APIKey: "key", Provider: "custom"}},
	}
	auth, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, "key", auth.APIKey)
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
	t.Setenv("HAND_RERANKER_MODEL", "openai/gpt-4o-mini")
	t.Setenv("HAND_RERANKER_MAX_CANDIDATES", "12")
	t.Setenv("HAND_RERANKER_MAX_CANDIDATE_TEXT_CHARS", "700")
	t.Setenv("HAND_RERANKER_MAX_OUTPUT_TOKENS", "256")
	t.Setenv("HAND_RERANKER_OVERRIDES", `{"memory_reflection":{"type":"llm","model":"openai/gpt-4o-mini","maxCandidates":7,"maxCandidateTextChars":500,"maxOutputTokens":96}}`)
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
	require.Equal(t, "openai/gpt-4o-mini", cfg.Reranker.Model)
	require.Equal(t, 12, cfg.Reranker.MaxCandidates)
	require.Equal(t, 700, cfg.Reranker.MaxCandidateTextChars)
	require.Equal(t, 256, cfg.Reranker.MaxOutputTokens)
	require.Equal(t, RerankerOverrideConfig{
		Type:                  constants.RerankerLLM,
		Model:                 "openai/gpt-4o-mini",
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

func TestConfig_MemoryDefaultsAndNormalize(t *testing.T) {
	var cfg *Config
	require.False(t, cfg.MemoryEnabled())

	cfg = &Config{Memory: MemoryConfig{Provider: " Default-Memory ", Backend: " SQLite "}}
	cfg.Normalize()
	require.True(t, cfg.MemoryEnabled())
	require.Equal(t, "default-memory", cfg.Memory.Provider)
	require.Equal(t, "sqlite", cfg.Memory.Backend)
	require.True(t, getBoolValue(cfg.Memory.Pinned.Enabled))
	require.True(t, getBoolValue(cfg.Memory.Retrieval.Enabled))
	require.True(t, getBoolValue(cfg.Memory.Flush.Enabled))
	require.Equal(t, 2, cfg.Memory.Flush.MaxCalls)
	require.Equal(t, int64(512), cfg.Memory.Flush.MaxOutputTokens)
	require.Equal(t, 10*time.Second, cfg.Memory.Flush.Timeout)
	require.False(t, getBoolValue(cfg.Memory.Episodic.Enabled))
	require.False(t, getBoolValue(cfg.Memory.Reflection.Enabled))
	require.True(t, getBoolValue(cfg.Memory.Promotion.Enabled))
	require.True(t, getBoolValue(cfg.Memory.Write.Enabled))
	require.True(t, cfg.MemoryRetrievalEnabled())
	require.True(t, cfg.MemoryFlushEnabled())
	require.True(t, cfg.MemoryWriteEnabled())

	cfg = &Config{Memory: MemoryConfig{Enabled: new(false)}}
	cfg.Normalize()
	require.False(t, cfg.MemoryEnabled())
	require.Equal(t, "default-memory", cfg.Memory.Provider)

	cfg = &Config{Memory: MemoryConfig{
		Reflection: ReflectionMemoryConfig{
			Enabled:      new(true),
			Interval:     time.Minute,
			Limit:        6,
			RelatedLimit: 2,
		},
		Promotion: PromotionMemoryConfig{
			Enabled:  new(true),
			Interval: time.Minute,
			Limit:    7,
		},
		Write: WriteMemoryConfig{
			Enabled: new(true),
		},
	}}
	cfg.Normalize()
	require.True(t, getBoolValue(cfg.Memory.Reflection.Enabled))
	require.Equal(t, time.Minute, cfg.Memory.Reflection.Interval)
	require.Equal(t, 6, cfg.Memory.Reflection.Limit)
	require.Equal(t, 2, cfg.Memory.Reflection.RelatedLimit)
	require.True(t, getBoolValue(cfg.Memory.Promotion.Enabled))
	require.Equal(t, time.Minute, cfg.Memory.Promotion.Interval)
	require.Equal(t, 7, cfg.Memory.Promotion.Limit)
	require.True(t, getBoolValue(cfg.Memory.Write.Enabled))

	cfg = &Config{Memory: MemoryConfig{Pinned: PinnedMemoryConfig{MaxChars: 120, MaxItemChars: 60}}}
	cfg.Normalize()
	require.Equal(t, 120, cfg.Memory.Pinned.MaxChars)
	require.Equal(t, 60, cfg.Memory.Pinned.MaxItemChars)
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
	t.Setenv("HAND_MODEL_SUMMARY", "openai/gpt-4o-mini")
	t.Setenv("HAND_MODEL_SUMMARY_PROVIDER", "openai")
	t.Setenv("HAND_MODEL_SUMMARY_BASE_URL", "https://example.com/v1")
	t.Setenv("HAND_MODEL_API", "openai-responses")
	t.Setenv("HAND_MODEL_SUMMARY_API", "openai-responses")

	applyEnvOverrides(cfg)

	require.Equal(t, "openai/gpt-4o-mini", cfg.Models.Summary.Name)
	require.Equal(t, "openai", cfg.Models.Summary.Provider)
	require.Equal(t, "https://example.com/v1", cfg.Models.Summary.BaseURL)
	require.Equal(t, "openai-responses", cfg.Models.Main.API)
	require.Equal(t, "openai-responses", cfg.Models.Summary.API)
}

func TestNormalizeFields_NilReceiver_NoPanic(t *testing.T) {
	var cfg *Config
	cfg.normalizeFields()
}

func TestDefaultBaseURLForProvider_DefaultsEmptyAPI(t *testing.T) {
	require.Equal(t, "https://openrouter.ai/api/v1/responses", getDefaultBaseURLForProvider("openrouter", ""))
	require.Equal(t, "https://openrouter.ai/api/v1/responses", getDefaultBaseURLForProvider("openrouter", "   "))
	require.Equal(t, "https://api.openai.com/v1", getDefaultBaseURLForProvider("openai", modelprovider.APIOpenAICompletions))
	require.Equal(t, "https://api.openai.com/v1", getDefaultBaseURLForProvider("openai", modelprovider.APIOpenAIResponses))
	require.Equal(t, "https://openrouter.ai/api/v1/embeddings", getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAIEmbeddings))
	require.Equal(t, "https://api.openai.com/v1/embeddings", getDefaultBaseURLForProvider("openai", modelprovider.APIOpenAIEmbeddings))
}

func TestModelProviders_CoverDayOneProviderBaseURLs(t *testing.T) {
	require.True(t, hasModelProvider("openai"))
	require.True(t, hasModelProvider("openrouter"))
	require.True(t, hasModelProvider("anthropic"))
	require.True(t, hasModelProvider("github-copilot"))
	require.Equal(t, "anthropic, github-copilot, openai, openrouter", getModelProviderList())
	openai, ok := modelRegistry.GetProvider("openai")
	require.True(t, ok)
	require.Equal(t, "openai", openai.ID)
	openrouter, ok := modelRegistry.GetProvider("openrouter")
	require.True(t, ok)
	require.Equal(t, "openrouter", openrouter.ID)
	anthropic, ok := modelRegistry.GetProvider("anthropic")
	require.True(t, ok)
	require.Equal(t, []string{"ANTHROPIC_API_KEY"}, anthropic.APIKeyEnv)
	copilot, ok := modelRegistry.GetProvider("github-copilot")
	require.True(t, ok)
	require.Equal(t, []string{"COPILOT_GITHUB_TOKEN"}, copilot.APIKeyEnv)

	require.Equal(t, constants.DefaultOpenAIBaseURL, getDefaultBaseURLForProvider("openai", modelprovider.APIOpenAICompletions))
	require.Equal(t, constants.DefaultOpenAIBaseURL, getDefaultBaseURLForProvider("openai", modelprovider.APIOpenAIResponses))
	require.Equal(t, constants.DefaultOpenAIEmbeddingsBaseURL, getDefaultBaseURLForProvider("openai", modelprovider.APIOpenAIEmbeddings))
	require.Equal(t, constants.DefaultAnthropicBaseURL, getDefaultBaseURLForProvider("anthropic", modelprovider.APIAnthropicMessages))
	require.Equal(t, constants.DefaultOpenRouterBaseURL, getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAICompletions))
	require.Equal(t, constants.DefaultOpenRouterResponsesBaseURL, getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAIResponses))
	require.Equal(t, constants.DefaultOpenRouterEmbeddingsBaseURL, getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAIEmbeddings))
}

func TestConfig_ModelSlotsResolveProviderBaseURLsThroughRegistry(t *testing.T) {
	stubProviderDefaultBaseURL(t, "openrouter", modelprovider.APIOpenAICompletions, "https://registry.openrouter.example/v1")
	stubProviderDefaultBaseURL(t, "openai", modelprovider.APIOpenAIResponses, "https://registry.openai.example/v1")
	stubProviderDefaultBaseURL(t, "openrouter", modelprovider.APIOpenAIEmbeddings, "https://registry.openrouter.example/v1/embeddings")

	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Main: MainModelConfig{
				Name:     constants.DefaultModel,
				Provider: "openrouter",
				API:      modelprovider.APIOpenAICompletions,
				APIKey:   "router-key",
			},
			Summary: SummaryModelConfig{
				Provider: "openai",
				API:      modelprovider.APIOpenAIResponses,
				APIKey:   "openai-key",
			},
			Embedding: EmbeddingModelConfig{
				Name:     constants.DefaultProfileEmbeddingModel,
				Provider: "openrouter",
				APIKey:   "embedding-key",
			},
		},
	}

	mainAuth, err := cfg.ResolveModelAuth()
	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider:         "openrouter",
		API:              modelprovider.APIOpenAICompletions,
		APIKey:           "router-key",
		BaseURL:          "https://registry.openrouter.example/v1",
		CredentialSource: ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig},
	}, mainAuth)

	summaryAuth, err := cfg.ResolveSummaryModelAuth()
	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider:         "openai",
		API:              modelprovider.APIOpenAIResponses,
		APIKey:           "openai-key",
		BaseURL:          "https://registry.openai.example/v1",
		CredentialSource: ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig},
	}, summaryAuth)

	embeddingAuth, err := cfg.ResolveEmbeddingModelAuth()
	require.NoError(t, err)
	require.Equal(t, ModelAuth{
		Provider:         "openrouter",
		API:              modelprovider.APIOpenAIEmbeddings,
		APIKey:           "embedding-key",
		BaseURL:          "https://registry.openrouter.example/v1/embeddings",
		CredentialSource: ModelCredentialSource{Kind: ModelCredentialSourceRoleConfig},
	}, embeddingAuth)
}

func TestDefaultBaseURLForProvider_ReturnsEmptyForUnknownMode(t *testing.T) {
	require.Empty(t, getDefaultBaseURLForProvider("openrouter", "not-a-mode"))
}

func TestConfig_NilReceiver_StreamAndSummaryHelpers(t *testing.T) {
	var cfg *Config

	require.True(t, cfg.StreamEnabled())
	require.Equal(t, "", cfg.SummaryModelEffective())
	require.Equal(t, "", cfg.SummaryProviderEffective())
	require.Equal(t, "", cfg.SummaryModelAPIEffective())

	_, err := cfg.ResolveSummaryModelAuth()
	require.EqualError(t, err, "config is required")
}

func TestConfig_ResolveSummaryModelAuth_FailsWhenSummaryProviderHasNoKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "router-only"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
			Summary:   SummaryModelConfig{Provider: "openai", BaseURL: "https://api.openai.com/v1"},
		},
	}
	cfg.Normalize()

	_, err := cfg.ResolveSummaryModelAuth()
	require.EqualError(t, err, "model API key is required; set a provider API key, provider env var, or role apiKey")
}

func TestConfig_Validate_ReturnsSummaryAuthErrorWhenOpenAIKeyMissing(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "router-only"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter"},
			Summary:   SummaryModelConfig{Provider: "openai"},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()

	require.EqualError(t, err, "model API key is required; set a provider API key, provider env var, or role apiKey")
}

func TestNormalizeRulePaths_EmptyInput(t *testing.T) {
	require.Empty(t, normalizeRulePaths(nil))
}

func clearEnvKeys(t *testing.T, keys ...string) {
	t.Helper()
	keys = append(keys, "OPENAI_API_KEY", "OPENROUTER_API_KEY", "ANTHROPIC_API_KEY", "COPILOT_GITHUB_TOKEN")
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

func TestLoad_UsesModelAPIFromConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_MODEL_API")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: config-model
    provider: openai
    api: openai-responses
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, "openai-responses", cfg.Models.Main.API)
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

func TestConfig_ValidateRejectsInvalidAPI(t *testing.T) {
	for _, mode := range []string{"invalid", "embeddings"} {
		t.Run(mode, func(t *testing.T) {
			err := (&Config{
				Name: "test-agent",
				Models: ModelsConfig{
					Providers: map[string]ProviderModelConfig{"openai": {APIKey: "test-key"}},
					Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openai", API: mode},
				},
				RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
				Log: LogConfig{Level: "info"},
			}).Validate()
			require.EqualError(t, err, "model API must be one of: anthropic-messages, openai-completions, openai-responses")
		})
	}
}

func TestConfig_ValidateAllowsResponsesModeWithOpenRouter(t *testing.T) {
	err := (&Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "test-key"}},
			Main:      MainModelConfig{Name: constants.DefaultModel, Provider: "openrouter", API: modelprovider.APIOpenAIResponses},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}).Validate()
	require.NoError(t, err)
}

func TestConfig_ValidateAllowsAnthropicMessagesModel(t *testing.T) {
	cfg := &Config{
		Name: "test-agent",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"anthropic": {APIKey: "test-key"}},
			Main: MainModelConfig{
				Name:     "anthropic/claude-sonnet-4-5",
				Provider: "anthropic",
			},
			Summary: SummaryModelConfig{
				Name: "anthropic/claude-3-haiku-20240307",
			},
		},
		RPC: RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: LogConfig{Level: "info"},
	}

	err := cfg.Validate()

	require.NoError(t, err)
	require.Equal(t, modelprovider.APIAnthropicMessages, cfg.Models.Main.API)
	require.Equal(t, constants.DefaultAnthropicBaseURL, cfg.Models.Main.BaseURL)
	require.Equal(t, 200000, cfg.Models.Main.ContextLength)
}

func TestLoad_UsesDebugTraceSettingsFromConfig(t *testing.T) {
	clearEnvKeys(t,
		"HAND_TRACE_ENABLED",
		"HAND_TRACE_DISK_ENABLED",
		"HAND_TRACE_DISK_DIR",
		"HAND_TRACE_DATABASE_ENABLED",
		"HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION",
	)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: config-model
    provider: openai
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
debug:
  requests: false
trace:
  enabled: true
  disk:
    enabled: false
    dir: /tmp/explicit-hand-traces
  database:
    enabled: false
    maxEventsPerSession: 123
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.True(t, cfg.Trace.Enabled)
	require.False(t, *cfg.Trace.Disk.Enabled)
	require.Equal(t, "/tmp/explicit-hand-traces", cfg.Trace.Disk.Dir)
	require.False(t, *cfg.Trace.Database.Enabled)
	require.Equal(t, 123, cfg.Trace.Database.MaxEventsPerSession)
}

func TestLoad_UsesDebugTraceSettingsFromEnvOverride(t *testing.T) {
	clearEnvKeys(t,
		"HAND_TRACE_ENABLED",
		"HAND_TRACE_DISK_ENABLED",
		"HAND_TRACE_DISK_DIR",
		"HAND_TRACE_DATABASE_ENABLED",
		"HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION",
	)

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte(`
HAND_TRACE_ENABLED=true
HAND_TRACE_DISK_ENABLED=false
HAND_TRACE_DISK_DIR=/tmp/env-disk-traces
HAND_TRACE_DATABASE_ENABLED=false
HAND_TRACE_DATABASE_MAX_EVENTS_PER_SESSION=77
`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: config-model
    provider: openai
rpc:
  address: 127.0.0.1
  port: 50051
log:
  level: info
debug:
  requests: false
trace:
  enabled: false
`), 0o600))

	cfg, err := Load(envPath, configPath)
	require.NoError(t, err)
	require.True(t, cfg.Trace.Enabled)
	require.False(t, *cfg.Trace.Disk.Enabled)
	require.Equal(t, "/tmp/env-disk-traces", cfg.Trace.Disk.Dir)
	require.False(t, *cfg.Trace.Database.Enabled)
	require.Equal(t, 77, cfg.Trace.Database.MaxEventsPerSession)
}

func TestConfig_NormalizeDefaultsDebugTraceSinks(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.True(t, *cfg.Trace.Disk.Enabled)
	require.Equal(t, datadir.DebugTraceDir(), cfg.Trace.Disk.Dir)
	require.True(t, *cfg.Trace.Database.Enabled)
	require.Equal(t, constants.DefaultTraceMaxEventsPerSession, cfg.Trace.Database.MaxEventsPerSession)
}

func TestConfig_NormalizeDefaultsDebugTraceDiskDirFromActiveProfile(t *testing.T) {
	setProfileHome(t, "/tmp/hand-home")
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, "/tmp/hand-home/traces", cfg.Trace.Disk.Dir)
}

func TestConfig_NormalizeKeepsExplicitTraceDiskDir(t *testing.T) {
	cfg := &Config{
		Trace: TraceConfig{
			Disk: TraceDiskConfig{Dir: "/tmp/disk-traces"},
		},
	}

	cfg.Normalize()

	require.Equal(t, "/tmp/disk-traces", cfg.Trace.Disk.Dir)
}

func TestLoad_UsesFilesystemRootsAndExecRulesFromConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_FS_ROOTS", "HAND_EXEC_ALLOW", "HAND_EXEC_ASK", "HAND_EXEC_DENY")
	configDir := t.TempDir()
	workingDir := t.TempDir()
	t.Chdir(workingDir)
	configPath := filepath.Join(configDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
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
		filepath.Join(workingDir),
		filepath.Join(workingDir, "nested"),
	}, cfg.FS.Roots)
	require.Equal(t, []string{"git status"}, cfg.Exec.Allow)
	require.Equal(t, []string{"git push"}, cfg.Exec.Ask)
	require.Equal(t, []string{"git reset --hard"}, cfg.Exec.Deny)
}

func TestLoad_DefaultsNoProfileAccessToTrueWhenOmitted(t *testing.T) {
	clearEnvKeys(t, "HAND_CONFIG")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.True(t, cfg.FS.NoProfileAccess)
}

func TestLoad_AllowsNoProfileAccessOverrideFromConfig(t *testing.T) {
	clearEnvKeys(t, "HAND_CONFIG")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
fs:
  noProfileAccess: false
`), 0o600))

	cfg, err := Load("", configPath)

	require.NoError(t, err)
	require.False(t, cfg.FS.NoProfileAccess)
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
  providers:
    openrouter:
      apiKey: config-key
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
  overrides:
    memory_reflection:
      type: llm
      model: openai/gpt-4o-mini
      maxCandidates: 7
      maxCandidateTextChars: 500
      maxOutputTokens: 96
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
	require.False(t, getBoolValueDefault(cfg.Search.EnableRerank, true))
	require.False(t, getBoolValueDefault(cfg.Reranker.Enabled, true))
	require.Equal(t, constants.RerankerLLM, cfg.Reranker.Type)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Reranker.Model)
	require.Equal(t, 11, cfg.Reranker.MaxCandidates)
	require.Equal(t, 600, cfg.Reranker.MaxCandidateTextChars)
	require.Equal(t, 128, cfg.Reranker.MaxOutputTokens)
	require.Equal(t, RerankerOverrideConfig{
		Type:                  constants.RerankerLLM,
		Model:                 "openai/gpt-4o-mini",
		MaxCandidates:         testIntPtr(7),
		MaxCandidateTextChars: testIntPtr(500),
		MaxOutputTokens:       testIntPtr(96),
	}, cfg.Reranker.Overrides["memory_reflection"])
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
	require.Equal(t, constants.RerankerDeterministic, cfg.RerankerEffective())
}

func TestConfig_RerankerEffectiveDefaults(t *testing.T) {
	require.Equal(t, constants.RerankerDeterministic, (*Config)(nil).RerankerEffective())
	require.Equal(t, "", (*Config)(nil).RerankerModelEffective())
	require.Empty(t, (*Config)(nil).RerankerOverrideEffective(RerankerOverrideConfig{}))

	cfg := &Config{
		Models: ModelsConfig{
			Main: MainModelConfig{Name: "openai/main"},
		},
		Reranker: RerankerConfig{Model: "openai/reranker"},
	}

	require.Equal(t, "openai/reranker", cfg.RerankerModelEffective())

	cfg.Reranker.Model = ""
	cfg.Models.Summary.Name = "openai/summary"
	require.Equal(t, "openai/summary", cfg.RerankerModelEffective())
}

func TestConfig_RerankerOverrideEffectiveInheritsGlobalValues(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{
			Main:    MainModelConfig{Name: "main-model"},
			Summary: SummaryModelConfig{Name: "summary-model"},
		},
		Reranker: RerankerConfig{
			Type:                  constants.RerankerLLM,
			Model:                 "global-reranker",
			MaxCandidates:         20,
			MaxCandidateTextChars: 1200,
			MaxOutputTokens:       64,
		},
	}

	effective := cfg.RerankerOverrideEffective(RerankerOverrideConfig{})

	require.Equal(t, constants.RerankerLLM, effective.Type)
	require.Equal(t, "global-reranker", effective.Model)
	require.Equal(t, 20, effective.MaxCandidates)
	require.True(t, effective.MaxCandidatesSet)
	require.Equal(t, 1200, effective.MaxCandidateTextChars)
	require.True(t, effective.MaxCandidateTextCharsSet)
	require.Equal(t, 64, effective.MaxOutputTokens)

	effective = cfg.RerankerOverrideEffective(RerankerOverrideConfig{
		Type:                  constants.RerankerNoop,
		Model:                 "override-reranker",
		MaxCandidates:         testIntPtr(0),
		MaxCandidateTextChars: testIntPtr(0),
		MaxOutputTokens:       testIntPtr(0),
	})

	require.Equal(t, constants.RerankerNoop, effective.Type)
	require.Equal(t, "override-reranker", effective.Model)
	require.Zero(t, effective.MaxCandidates)
	require.True(t, effective.MaxCandidatesSet)
	require.Zero(t, effective.MaxCandidateTextChars)
	require.True(t, effective.MaxCandidateTextCharsSet)
	require.Zero(t, effective.MaxOutputTokens)

	cfg.Reranker.MaxCandidates = 0
	cfg.Reranker.MaxCandidateTextChars = 0
	effective = cfg.RerankerOverrideEffective(RerankerOverrideConfig{})

	require.Zero(t, effective.MaxCandidates)
	require.False(t, effective.MaxCandidatesSet)
	require.Zero(t, effective.MaxCandidateTextChars)
	require.False(t, effective.MaxCandidateTextCharsSet)
}

func TestNormalizeRerankerOverrides_CleansKeysAndValues(t *testing.T) {
	require.Nil(t, cloneRerankerOverrides(nil))
	require.Nil(t, normalizeRerankerOverrides(map[string]RerankerOverrideConfig{
		" ": {Type: constants.RerankerLLM},
	}))

	overrides := map[string]RerankerOverrideConfig{
		" Memory_Reflection ": {
			Type:          " LLM ",
			Model:         " openai/gpt-4o-mini ",
			MaxCandidates: testIntPtr(7),
		},
	}
	normalized := normalizeRerankerOverrides(overrides)

	require.Equal(t, RerankerOverrideConfig{
		Type:          constants.RerankerLLM,
		Model:         "openai/gpt-4o-mini",
		MaxCandidates: testIntPtr(7),
	}, normalized["memory_reflection"])
	require.NotSame(t, &overrides, &normalized)

	cloned := cloneRerankerOverrides(normalized)
	*cloned["memory_reflection"].MaxCandidates = 9
	require.Equal(t, 7, *normalized["memory_reflection"].MaxCandidates)
	cloned["memory_reflection"] = RerankerOverrideConfig{Type: constants.RerankerNoop}
	require.Equal(t, constants.RerankerLLM, normalized["memory_reflection"].Type)
}

func TestValidateRerankerOverride_RejectsInvalidValues(t *testing.T) {
	cfg := &Config{}

	require.NoError(t, cfg.validateRerankerSettings())
	cfg.Reranker.Type = constants.RerankerLLM
	cfg.Reranker.Model = "openai/gpt-4o-mini"
	require.NoError(t, cfg.validateRerankerSettings())
	require.NoError(t, cfg.validateRerankerOverride("memory_reflection", RerankerOverrideConfig{
		Type:  constants.RerankerLLM,
		Model: "openai/gpt-4o-mini",
	}))
	require.NoError(t, cfg.validateRerankerOverride("memory_reflection", RerankerOverrideConfig{}))
	require.EqualError(
		t,
		cfg.validateRerankerOverride("", RerankerOverrideConfig{Type: constants.RerankerDeterministic}),
		"reranker override use case is required",
	)
	require.EqualError(
		t,
		cfg.validateRerankerOverride("memory_reflection", RerankerOverrideConfig{
			Type:          constants.RerankerDeterministic,
			MaxCandidates: testIntPtr(-1),
		}),
		`reranker override "memory_reflection" max candidates must be non-negative`,
	)
	require.EqualError(
		t,
		cfg.validateRerankerOverride("memory_reflection", RerankerOverrideConfig{
			Type:                  constants.RerankerDeterministic,
			MaxCandidateTextChars: testIntPtr(-1),
		}),
		`reranker override "memory_reflection" max candidate text chars must be non-negative`,
	)
	require.EqualError(
		t,
		cfg.validateRerankerOverride("memory_reflection", RerankerOverrideConfig{
			Type:            constants.RerankerDeterministic,
			MaxOutputTokens: testIntPtr(-1),
		}),
		`reranker override "memory_reflection" max output tokens must be non-negative`,
	)
}

func TestConfig_ValidateRejectsInvalidSessionSettings(t *testing.T) {
	cfg := &Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", API: modelprovider.APIOpenAICompletions},
		},
		RPC:     RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session: SessionConfig{MaxIterations: 1},
		Log:     LogConfig{Level: "info"},
		Storage: StorageConfig{Backend: "bogus"},
	}

	err := cfg.Validate()
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func TestConfig_ValidateRejectsInvalidMemoryBackend(t *testing.T) {
	cfg := &Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", API: modelprovider.APIOpenAICompletions},
		},
		RPC:     RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session: SessionConfig{MaxIterations: 1},
		Log:     LogConfig{Level: "info"},
		Storage: StorageConfig{Backend: "sqlite"},
		Memory:  MemoryConfig{Backend: "bogus"},
	}

	err := cfg.Validate()
	require.EqualError(t, err, "memory backend must be one of: memory, sqlite")
}

func TestConfig_ValidateRejectsInvalidSessionVectorSettings(t *testing.T) {
	valid := Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", API: modelprovider.APIOpenAICompletions},
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
			err: "embedding provider must be one of: anthropic, github-copilot, openai, openrouter",
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
			name: "unsupported reranker override",
			mutate: func(cfg *Config) {
				cfg.Reranker.Overrides = map[string]RerankerOverrideConfig{
					"memory_reflection": {Type: "magic"},
				}
			},
			err: `reranker override "memory_reflection": reranker type must be one of: deterministic, noop, llm`,
		},
		{
			name: "negative reranker override max candidates",
			mutate: func(cfg *Config) {
				cfg.Reranker.Overrides = map[string]RerankerOverrideConfig{
					"memory_reflection": {Type: constants.RerankerDeterministic, MaxCandidates: testIntPtr(-1)},
				}
			},
			err: `reranker override "memory_reflection" max candidates must be non-negative`,
		},
		{
			name: "missing api key",
			mutate: func(cfg *Config) {
				t.Setenv("OPENAI_API_KEY", "")
				cfg.Models.Providers = nil
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

func TestConfig_ValidateAcceptsRegistryEmbeddingModelWithoutContextRequirement(t *testing.T) {
	cfg := Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", API: modelprovider.APIOpenAICompletions},
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
}

func TestConfig_ValidateRejectsKnownEmbeddingModelWithIncompatibleRole(t *testing.T) {
	cfg := Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main:      MainModelConfig{Name: constants.DefaultProfileModel, Provider: "openrouter"},
			Embedding: EmbeddingModelConfig{Name: constants.DefaultProfileModel, Provider: "openrouter"},
		},
		RPC:        RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session:    SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:        LogConfig{Level: "info"},
		Storage:    StorageConfig{Backend: "sqlite"},
		Search:     SearchConfig{Vector: SearchVectorConfig{Enabled: true}},
		Compaction: CompactionConfig{Enabled: new(true), TriggerPercent: 0.85, WarnPercent: 0.95},
	}

	err := cfg.Validate()

	require.EqualError(t, err, `models.embedding.name "minimax/minimax-m2.7" is not compatible with this model role`)
}

func TestConfig_ValidateAllowsFreeFormEmbeddingModelForProviderThatSupportsModels(t *testing.T) {
	cfg := Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main:      MainModelConfig{Name: "openai/model", Provider: "openrouter", BaseURL: "https://example.com", API: modelprovider.APIOpenAICompletions},
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

	require.NoError(t, err)
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
  recentSessionTail: 3
`), 0o600))

	cfg, err := Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, 64000, cfg.Models.Main.ContextLength)
	require.False(t, getBoolValue(cfg.Compaction.Enabled))
	require.Equal(t, 0.7, cfg.Compaction.TriggerPercent)
	require.Equal(t, 0.9, cfg.Compaction.WarnPercent)
	require.Equal(t, 3, cfg.CompactionRecentSessionTailEffective())
}

func TestConfig_NormalizeDefaultsCompactionSettings(t *testing.T) {
	cfg := &Config{}
	cfg.Normalize()
	require.Equal(t, constants.DefaultContextLength, cfg.Models.Main.ContextLength)
	require.True(t, getBoolValue(cfg.Compaction.Enabled))
	require.Equal(t, 0.85, cfg.Compaction.TriggerPercent)
	require.Equal(t, 0.95, cfg.Compaction.WarnPercent)
	require.Equal(t, 8, cfg.CompactionRecentSessionTailEffective())
}

func TestConfig_ValidateRejectsInvalidCompactionSettings(t *testing.T) {
	cfg := &Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main:      MainModelConfig{Name: "openai/model", ContextLength: 128000, Provider: "openrouter", BaseURL: "https://example.com", API: modelprovider.APIOpenAICompletions},
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

func TestConfig_ValidateRejectsInvalidCompactionRecentSessionTail(t *testing.T) {
	invalidTail := -1
	cfg := &Config{
		Name: "daemon",
		Models: ModelsConfig{
			Providers: map[string]ProviderModelConfig{"openrouter": {APIKey: "key"}},
			Main: MainModelConfig{
				Name:          "openai/model",
				ContextLength: 128000,
				Provider:      "openrouter",
				BaseURL:       "https://example.com",
				API:           modelprovider.APIOpenAICompletions,
			},
		},
		RPC:     RPCConfig{Address: "127.0.0.1", Port: 50051},
		Session: SessionConfig{MaxIterations: 1, DefaultIdleExpiry: time.Hour, ArchiveRetention: 24 * time.Hour},
		Log:     LogConfig{Level: "info"},
		Storage: StorageConfig{Backend: "memory"},
		Compaction: CompactionConfig{
			Enabled:           new(true),
			TriggerPercent:    0.85,
			WarnPercent:       0.95,
			RecentSessionTail: &invalidTail,
		},
	}

	err := cfg.Validate()
	require.EqualError(t, err, "compaction recent session tail must be greater than or equal to zero")
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
				require.Truef(
					t,
					strings.HasPrefix(match[1], "HAND_") || hasNativeProviderEnvKey(match[1]),
					"env key %q must use HAND_ prefix or be a provider-native credential key",
					match[1],
				)
			}
		})
	}
}

func hasNativeProviderEnvKey(key string) bool {
	switch key {
	case "OPENAI_API_KEY", "OPENROUTER_API_KEY", "ANTHROPIC_API_KEY", "COPILOT_GITHUB_TOKEN":
		return true
	default:
		return false
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

			rootKeys := []string{"name", "platform", "search", "reranker", "trace"}
			if !file.optional {
				rootKeys = append(rootKeys, "memory")
			}
			requireYAMLKeys(t, content, "", rootKeys)
			requireYAMLKeys(t, content, "models", []string{
				"maxRetries",
				"providers",
				"main",
				"summary",
				"embedding",
			})
			requireYAMLKeys(t, content, "main", []string{
				"name",
				"provider",
				"api",
				"baseUrl",
				"stream",
				"contextLength",
			})
			requireYAMLKeys(t, content, "summary", []string{"name", "provider", "api", "baseUrl"})
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
				"overrides",
			})
			requireYAMLKeys(t, content, "compaction", []string{"enabled", "triggerPercent", "warnPercent"})
			requireYAMLKeys(t, content, "cap", []string{"fs", "net", "exec", "mem", "browser"})
			requireYAMLKeys(t, content, "log", []string{"level", "noColor"})
			requireYAMLKeys(t, content, "debug", []string{"requests"})
			requireYAMLKeys(t, content, "trace", []string{"enabled", "disk", "database"})
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

	content := readTextFile(t, "env.go")
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

func setProfileHome(t *testing.T, home string) {
	t.Helper()

	original := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(original)
	})
	profile.SetActive(profile.Profile{Name: "test", HomeDir: home})
}

func testIntPtr(value int) *int {
	return &value
}
