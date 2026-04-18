package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
)

func TestNewLiveClients(t *testing.T) {
	originalFactory := newLiveModelClient
	t.Cleanup(func() {
		newLiveModelClient = originalFactory
	})

	t.Run("requires config", func(t *testing.T) {
		modelClient, summaryClient, err := NewLiveClients(nil)
		require.Error(t, err)
		assert.Nil(t, modelClient)
		assert.Nil(t, summaryClient)
		assert.EqualError(t, err, "live harness config is required")
	})

	t.Run("reuses main client when auth matches", func(t *testing.T) {
		retries := 3
		var calls []struct {
			key  string
			opts int
		}
		newLiveModelClient = func(key string, opts ...option.RequestOption) (*models.OpenAIClient, error) {
			calls = append(calls, struct {
				key  string
				opts int
			}{key: key, opts: len(opts)})
			return &models.OpenAIClient{}, nil
		}

		cfg := &config.Config{
			ModelProvider:   "openrouter",
			ModelKey:        "router-key",
			ModelBaseURL:    "https://router.example/v1",
			ModelMaxRetries: &retries,
		}

		modelClient, summaryClient, err := NewLiveClients(cfg)
		require.NoError(t, err)
		require.NotNil(t, modelClient)
		require.NotNil(t, summaryClient)
		assert.Same(t, modelClient, summaryClient)
		require.Len(t, calls, 1)
		assert.Equal(t, "router-key", calls[0].key)
		assert.Equal(t, 2, calls[0].opts)
	})

	t.Run("builds distinct summary client when auth differs", func(t *testing.T) {
		retries := 1
		var keys []string
		newLiveModelClient = func(key string, _ ...option.RequestOption) (*models.OpenAIClient, error) {
			keys = append(keys, key)
			return &models.OpenAIClient{}, nil
		}

		cfg := &config.Config{
			ModelProvider:       "openrouter",
			ModelKey:            "router-key",
			SummaryProvider:     "openai",
			OpenAIAPIKey:        "openai-key",
			SummaryModelBaseURL: "https://openai.example/v1",
			ModelMaxRetries:     &retries,
		}

		modelClient, summaryClient, err := NewLiveClients(cfg)
		require.NoError(t, err)
		require.NotNil(t, modelClient)
		require.NotNil(t, summaryClient)
		assert.NotSame(t, modelClient, summaryClient)
		assert.Equal(t, []string{"router-key", "openai-key"}, keys)
	})

	t.Run("returns factory errors", func(t *testing.T) {
		newLiveModelClient = func(string, ...option.RequestOption) (*models.OpenAIClient, error) {
			return nil, errors.New("client failed")
		}

		cfg := &config.Config{
			ModelProvider: "openrouter",
			ModelKey:      "router-key",
		}

		modelClient, summaryClient, err := NewLiveClients(cfg)
		require.Error(t, err)
		assert.Nil(t, modelClient)
		assert.Nil(t, summaryClient)
		assert.EqualError(t, err, "client failed")
	})

	t.Run("returns main auth error", func(t *testing.T) {
		newLiveModelClient = originalFactory

		modelClient, summaryClient, err := NewLiveClients(&config.Config{})
		require.Error(t, err)
		assert.Nil(t, modelClient)
		assert.Nil(t, summaryClient)
		assert.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	})

	t.Run("returns summary auth error", func(t *testing.T) {
		newLiveModelClient = originalFactory

		cfg := &config.Config{
			ModelProvider:    "openrouter",
			OpenRouterAPIKey: "router-key",
			SummaryProvider:  "openai",
		}

		modelClient, summaryClient, err := NewLiveClients(cfg)
		require.Error(t, err)
		assert.Nil(t, modelClient)
		assert.Nil(t, summaryClient)
		assert.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	})

	t.Run("returns summary client factory error", func(t *testing.T) {
		newLiveModelClient = func(key string, _ ...option.RequestOption) (*models.OpenAIClient, error) {
			if key == "openai-key" {
				return nil, errors.New("summary client failed")
			}
			return &models.OpenAIClient{}, nil
		}

		retries := 1
		cfg := &config.Config{
			ModelProvider:       "openrouter",
			ModelKey:            "router-key",
			SummaryProvider:     "openai",
			OpenAIAPIKey:        "openai-key",
			SummaryModelBaseURL: "https://openai.example/v1",
			ModelMaxRetries:     &retries,
		}

		modelClient, summaryClient, err := NewLiveClients(cfg)
		require.Error(t, err)
		assert.Nil(t, modelClient)
		assert.Nil(t, summaryClient)
		assert.EqualError(t, err, "summary client failed")
	})
}

func TestNewLiveHarnessAndRPCHarness(t *testing.T) {
	originalFactory := newLiveModelClient
	originalLoad := loadLiveConfig
	originalNewHarness := newLiveHarness
	originalNewRPCHarness := newLiveRPCHarness
	t.Cleanup(func() {
		newLiveModelClient = originalFactory
		loadLiveConfig = originalLoad
		newLiveHarness = originalNewHarness
		newLiveRPCHarness = originalNewRPCHarness
	})

	newLiveModelClient = func(string, ...option.RequestOption) (*models.OpenAIClient, error) {
		return &models.OpenAIClient{}, nil
	}

	writeConfig := func(t *testing.T) (string, string) {
		t.Helper()

		dir := t.TempDir()
		envPath := filepath.Join(dir, ".env")
		configPath := filepath.Join(dir, "config.yaml")
		require.NoError(t, os.WriteFile(envPath, []byte("MODEL_KEY=test-key\n"), 0o600))
		require.NoError(t, os.WriteFile(configPath, []byte(`
name: live-test
model:
  name: openai/gpt-4o-mini
  provider: openrouter
  verifyModel: false
rpc:
  address: 127.0.0.1
  port: 50051
storage:
  backend: sqlite
`), 0o600))
		return envPath, configPath
	}

	t.Run("new live harness loads config", func(t *testing.T) {
		envPath, configPath := writeConfig(t)

		h, err := NewLiveHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), envPath, configPath)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, h.Close())
		})

		require.NotNil(t, h.Config())
		assert.Equal(t, "live-test", h.Config().Name)
	})

	t.Run("new live rpc harness loads config", func(t *testing.T) {
		envPath, configPath := writeConfig(t)

		h, err := NewLiveRPCHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), envPath, configPath)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, h.Close())
		})

		require.NotNil(t, h.Config())
		assert.Equal(t, "live-test", h.Config().Name)
	})

	t.Run("returns config load error", func(t *testing.T) {
		loadLiveConfig = func(string, string) (*config.Config, error) {
			return nil, errors.New("load failed")
		}

		_, err := NewLiveHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), "", "config.yaml")
		require.Error(t, err)
		assert.EqualError(t, err, "load failed")
	})

	t.Run("returns live client error", func(t *testing.T) {
		loadLiveConfig = func(string, string) (*config.Config, error) {
			return &config.Config{ModelProvider: "openrouter"}, nil
		}

		_, err := NewLiveHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), "", "config.yaml")
		require.Error(t, err)
	})

	t.Run("returns harness construction error", func(t *testing.T) {
		loadLiveConfig = func(string, string) (*config.Config, error) {
			return &config.Config{
				ModelProvider: "openrouter",
				ModelKey:      "router-key",
			}, nil
		}
		newLiveHarness = func(context.Context, HarnessOptions) (*Harness, error) {
			return nil, errors.New("harness failed")
		}

		_, err := NewLiveHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), "", "config.yaml")
		require.Error(t, err)
		assert.EqualError(t, err, "harness failed")
	})

	t.Run("returns rpc harness construction error", func(t *testing.T) {
		loadLiveConfig = func(string, string) (*config.Config, error) {
			return &config.Config{
				ModelProvider: "openrouter",
				ModelKey:      "router-key",
			}, nil
		}
		newLiveRPCHarness = func(context.Context, HarnessOptions) (*RPCHarness, error) {
			return nil, errors.New("rpc harness failed")
		}

		_, err := NewLiveRPCHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), "", "config.yaml")
		require.Error(t, err)
		assert.EqualError(t, err, "rpc harness failed")
	})

	t.Run("returns rpc config load error", func(t *testing.T) {
		loadLiveConfig = func(string, string) (*config.Config, error) {
			return nil, errors.New("load failed")
		}

		_, err := NewLiveRPCHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), "", "config.yaml")
		require.Error(t, err)
		assert.EqualError(t, err, "load failed")
	})

	t.Run("returns rpc live client error", func(t *testing.T) {
		loadLiveConfig = func(string, string) (*config.Config, error) {
			return &config.Config{ModelProvider: "openrouter"}, nil
		}

		_, err := NewLiveRPCHarness(context.Background(), filepath.Join(t.TempDir(), "hand-home"), "", "config.yaml")
		require.Error(t, err)
	})
}

func TestLiveClientOptions(t *testing.T) {
	opts := liveClientOptions("", 2)
	require.Len(t, opts, 1)

	opts = liveClientOptions(" https://example.com/v1 ", 2)
	require.Len(t, opts, 2)
}
