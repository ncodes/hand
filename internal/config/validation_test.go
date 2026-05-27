package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/internal/constants"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

func TestConfig_ValidateRequiresKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	cfg := &Config{
		Name:   "test-agent",
		Models: ModelsConfig{Main: MainModelConfig{Name: constants.DefaultModel}},
		Log:    LogConfig{Level: "info"},
	}
	require.ErrorContains(t, cfg.Validate(), "hand auth login openrouter")
	require.Equal(t, constants.DefaultModelProvider, cfg.Models.Main.Provider)
	require.Equal(t, getDefaultBaseURLForProvider(constants.DefaultModelProvider, modelprovider.APIOpenAIResponses), cfg.Models.Main.BaseURL)
}

func TestConfig_ValidateNilConfig(t *testing.T) {
	var cfg *Config
	require.EqualError(t, cfg.Validate(), "config is required")
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

func TestConfig_ValidateAcceptsProviderNativeSummaryModelID(t *testing.T) {
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

	require.NoError(t, err)
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

func TestConfig_Validate_ReturnsSummaryAuthErrorWhenOpenAIKeyMissing(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	stubModelProviderToken(t, func(string) (StoredModelCredential, error) {
		return StoredModelCredential{}, nil
	})

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

	require.ErrorContains(t, err, "hand auth login openai")
}
