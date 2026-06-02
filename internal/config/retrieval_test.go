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
  model: gpt-4o-mini
  maxCandidates: 11
  maxCandidateTextChars: 600
  maxOutputTokens: 128
  overrides:
    memory_reflection:
      type: llm
      model: gpt-4o-mini
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
	require.Equal(t, "gpt-4o-mini", cfg.Reranker.Model)
	require.Equal(t, 11, cfg.Reranker.MaxCandidates)
	require.Equal(t, 600, cfg.Reranker.MaxCandidateTextChars)
	require.Equal(t, 128, cfg.Reranker.MaxOutputTokens)
	require.Equal(t, RerankerOverrideConfig{
		Type:                  constants.RerankerLLM,
		Model:                 "gpt-4o-mini",
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
			Model:         " gpt-4o-mini ",
			MaxCandidates: testIntPtr(7),
		},
	}
	normalized := normalizeRerankerOverrides(overrides)

	require.Equal(t, RerankerOverrideConfig{
		Type:          constants.RerankerLLM,
		Model:         "gpt-4o-mini",
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
	cfg.Reranker.Model = "gpt-4o-mini"
	require.NoError(t, cfg.validateRerankerSettings())
	require.NoError(t, cfg.validateRerankerOverride("memory_reflection", RerankerOverrideConfig{
		Type:  constants.RerankerLLM,
		Model: "gpt-4o-mini",
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
			err: "embedding provider must be one of: anthropic, github-copilot, openai, openai-codex, openrouter",
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
			err: `embedding API key is required for provider "openai"; set a provider API key, provider env var, role apiKey, or run hand auth login openai`,
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
