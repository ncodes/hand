package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/retrieval"
	storagememory "github.com/wandxy/hand/internal/state/storememory"
	storagesqlite "github.com/wandxy/hand/internal/state/storesqlite"
)

func TestOpenStore_ValidatesConfigAndBackend(t *testing.T) {
	store, err := OpenStore(nil)
	require.Nil(t, store)
	require.EqualError(t, err, "config is required")

	store, err = OpenStore(storeConfig("bogus"))
	require.Nil(t, store)
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func storeConfig(backend string) *config.Config {
	return &config.Config{Storage: config.StorageConfig{Backend: backend}}
}

func TestOpenStore_ReturnsMemoryStore(t *testing.T) {
	store, err := OpenStore(storeConfig("memory"))

	require.NoError(t, err)
	require.IsType(t, &storagememory.Store{}, store)
}

func TestOpenStore_IgnoresIncompleteVectorConfigWhenDisabled(t *testing.T) {
	store, err := OpenStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{RebuildBatchSize: -1}},
	})

	require.NoError(t, err)
	require.IsType(t, &storagememory.Store{}, store)
}

func TestOpenStore_ReturnsSQLiteStore(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	store, err := OpenStore(storeConfig("sqlite"))

	require.NoError(t, err)
	require.IsType(t, &storagesqlite.Store{}, store)
	require.FileExists(t, datadir.StateDBPath())
	require.Equal(t, filepath.Join(homeDir, "data", "state.db"), datadir.StateDBPath())
}

func TestOpenStore_DefaultsToSQLite(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	store, err := OpenStore(&config.Config{})

	require.NoError(t, err)
	require.IsType(t, &storagesqlite.Store{}, store)
	require.FileExists(t, datadir.StateDBPath())
	require.Equal(t, filepath.Join(homeDir, "data", "state.db"), datadir.StateDBPath())
}

func TestOpenStore_ReturnsSQLiteOpenError(t *testing.T) {
	homePath := filepath.Join(t.TempDir(), "hand-home")
	require.NoError(t, os.WriteFile(homePath, []byte("not-a-directory"), 0o600))
	t.Setenv("HAND_HOME", homePath)

	store, err := OpenStore(storeConfig("sqlite"))

	require.Nil(t, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create sqlite db directory")
}

func TestOpenStore_ReturnsSQLiteStoreInitializationError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	originalStore := newSQLiteStoreFromDB
	t.Cleanup(func() {
		newSQLiteStoreFromDB = originalStore
	})
	newSQLiteStoreFromDB = func(*gorm.DB) (*storagesqlite.Store, error) {
		return nil, errors.New("store init failed")
	}

	store, err := OpenStore(storeConfig("sqlite"))

	require.Nil(t, store)
	require.EqualError(t, err, "store init failed")
}

func TestOpenStore_ConfiguresSQLiteVectorStore(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	provider := &storeTestEmbeddingProvider{}
	vectorStore := &storeTestVectorStore{}
	withStoreVectorHooks(t, provider, vectorStore, nil)

	rerankEnabled := false
	store, err := OpenStore(&config.Config{
		Storage: config.StorageConfig{Backend: "sqlite"},
		Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
		Search: config.SearchConfig{
			EnableRerank: &rerankEnabled,
			Vector:       config.SearchVectorConfig{Enabled: true, Required: true, RebuildBatchSize: 7},
		},
	})
	require.NoError(t, err)
	require.IsType(t, &storagesqlite.Store{}, store)

	sessionID, err := storage.NewSessionID()
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), storage.Session{ID: sessionID}))
	require.NoError(t, store.AppendMessages(context.Background(), sessionID, []handmsg.Message{{
		Role:    handmsg.RoleUser,
		Content: "semantic indexing text",
	}}))

	require.Len(t, provider.requests, 1)
	require.Equal(t, "text-embedding-test", provider.requests[0].Model)
	require.Len(t, vectorStore.upserts, 1)
	require.Len(t, vectorStore.upserts[0], 1)
	require.Equal(t, "text-embedding-test", vectorStore.upserts[0][0].EmbeddingModel)
	require.Equal(t, retrieval.SourceKindSessionMessage, vectorStore.upserts[0][0].SourceKind)
}

func TestOpenStore_ReturnsRerankerConstructionError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	provider := &storeTestEmbeddingProvider{}
	vectorStore := &storeTestVectorStore{}
	withStoreVectorHooks(t, provider, vectorStore, nil)

	store, err := OpenStoreWithRerankerClient(&config.Config{
		Storage: config.StorageConfig{Backend: "sqlite"},
		Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
		Reranker: config.RerankerConfig{
			Type: retrieval.RerankerLLM,
		},
	}, nil)

	require.Nil(t, store)
	require.EqualError(t, err, "reranker model client is required")
}

func TestOpenStore_ReturnsSQLiteVectorConfigurationError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	originalProvider := newStoreEmbeddingProvider
	originalSQLiteStore := newSQLiteVectorStore
	t.Cleanup(func() {
		newStoreEmbeddingProvider = originalProvider
		newSQLiteVectorStore = originalSQLiteStore
	})
	newStoreEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
		return nil, nil
	}
	newSQLiteVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
		return &storeTestVectorStore{}, nil
	}

	store, err := OpenStore(&config.Config{
		Storage: config.StorageConfig{Backend: "sqlite"},
		Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.Nil(t, store)
	require.EqualError(t, err, "embedding provider is required")
}

func TestOpenStore_ConfiguresMemoryVectorStore(t *testing.T) {
	provider := &storeTestEmbeddingProvider{}
	vectorStore := &storeTestVectorStore{}
	withStoreVectorHooks(t, provider, nil, vectorStore)

	store, err := OpenStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Models: config.ModelsConfig{
			Main:      config.MainModelConfig{Provider: "openai"},
			Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test"},
		},
		Search: config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.NoError(t, err)
	require.IsType(t, &storagememory.Store{}, store)

	sessionID, err := storage.NewSessionID()
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), storage.Session{ID: sessionID}))
	require.NoError(t, store.AppendMessages(context.Background(), sessionID, []handmsg.Message{{
		Role:    handmsg.RoleUser,
		Content: "semantic indexing text",
	}}))

	require.Len(t, provider.requests, 1)
	require.Equal(t, "text-embedding-test", provider.requests[0].Model)
	require.Len(t, vectorStore.upserts, 1)
	require.Len(t, vectorStore.upserts[0], 1)
	require.Equal(t, "text-embedding-test", vectorStore.upserts[0][0].EmbeddingModel)
	require.Equal(t, retrieval.SourceKindSessionMessage, vectorStore.upserts[0][0].SourceKind)
}

func TestOpenStore_ReturnsMemoryVectorConfigError(t *testing.T) {
	store, err := OpenStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.Nil(t, store)
	require.EqualError(t, err, "embedding model is required")
}

func TestOpenStore_ReturnsMemoryVectorConfigurationError(t *testing.T) {
	originalProvider := newStoreEmbeddingProvider
	t.Cleanup(func() {
		newStoreEmbeddingProvider = originalProvider
	})
	newStoreEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
		return nil, nil
	}

	store, err := OpenStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.Nil(t, store)
	require.EqualError(t, err, "embedding provider is required")
}

func TestOpenStore_ValidatesVectorConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		err  string
	}{
		{
			name: "missing model",
			cfg: config.Config{
				Storage: config.StorageConfig{Backend: "sqlite"},
				Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Provider: "openai"}},
				Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
			},
			err: "embedding model is required",
		},
		{
			name: "negative batch size",
			cfg: config.Config{
				Storage: config.StorageConfig{Backend: "sqlite"},
				Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
				Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true, RebuildBatchSize: -1}},
			},
			err: "vector rebuild batch size must be non-negative",
		},
		{
			name: "missing api key",
			cfg: config.Config{
				Storage: config.StorageConfig{Backend: "sqlite"},
				Models: config.ModelsConfig{
					Main:      config.MainModelConfig{Provider: "openai"},
					Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test"},
				},
				Search: config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
			},
			err: "embedding API key is required",
		},
		{
			name: "unsupported provider",
			cfg: config.Config{
				Storage: config.StorageConfig{Backend: "sqlite"},
				Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "unsupported"}},
				Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
			},
			err: "embedding provider must be one of: openai, openrouter",
		},
		{
			name: "negative reranker max candidates",
			cfg: config.Config{
				Storage:  config.StorageConfig{Backend: "sqlite"},
				Models:   config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{MaxCandidates: -1},
			},
			err: "reranker max candidates must be non-negative",
		},
		{
			name: "negative reranker max candidate text chars",
			cfg: config.Config{
				Storage:  config.StorageConfig{Backend: "sqlite"},
				Models:   config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{MaxCandidateTextChars: -1},
			},
			err: "reranker max candidate text chars must be non-negative",
		},
		{
			name: "negative reranker max output tokens",
			cfg: config.Config{
				Storage:  config.StorageConfig{Backend: "sqlite"},
				Models:   config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{MaxOutputTokens: -1},
			},
			err: "reranker max output tokens must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HAND_HOME", homeDir)

			store, err := OpenStore(&tt.cfg)

			require.Nil(t, store)
			require.EqualError(t, err, tt.err)
		})
	}
}

func TestOpenStore_ValidatesVectorStoreFactories(t *testing.T) {
	t.Run("sqlite vector store error", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HAND_HOME", homeDir)

		originalProvider := newStoreEmbeddingProvider
		originalSQLiteStore := newSQLiteVectorStore
		t.Cleanup(func() {
			newStoreEmbeddingProvider = originalProvider
			newSQLiteVectorStore = originalSQLiteStore
		})

		newStoreEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return &storeTestEmbeddingProvider{}, nil
		}
		newSQLiteVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
			return nil, errors.New("vector factory failed")
		}

		store, err := OpenStore(&config.Config{
			Storage: config.StorageConfig{Backend: "sqlite"},
			Models: config.ModelsConfig{
				Key:       "key",
				Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"},
			},
			Search: config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
		})

		require.Nil(t, store)
		require.EqualError(t, err, "vector factory failed")
	})

	t.Run("sqlite vector store is required", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HAND_HOME", homeDir)

		originalProvider := newStoreEmbeddingProvider
		originalSQLiteStore := newSQLiteVectorStore
		t.Cleanup(func() {
			newStoreEmbeddingProvider = originalProvider
			newSQLiteVectorStore = originalSQLiteStore
		})

		newStoreEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return &storeTestEmbeddingProvider{}, nil
		}
		newSQLiteVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
			return nil, nil
		}

		store, err := OpenStore(&config.Config{
			Storage: config.StorageConfig{Backend: "sqlite"},
			Models: config.ModelsConfig{
				Key:       "key",
				Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"},
			},
			Search: config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
		})

		require.Nil(t, store)
		require.EqualError(t, err, "sqlite vector store is required")
	})

	t.Run("memory vector store is required", func(t *testing.T) {
		originalProvider := newStoreEmbeddingProvider
		originalMemoryStore := newMemoryVectorStore
		t.Cleanup(func() {
			newStoreEmbeddingProvider = originalProvider
			newMemoryVectorStore = originalMemoryStore
		})

		newStoreEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return &storeTestEmbeddingProvider{}, nil
		}
		newMemoryVectorStore = func() retrieval.VectorStore {
			return nil
		}

		store, err := OpenStore(&config.Config{
			Storage: config.StorageConfig{Backend: "memory"},
			Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
			Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
		})

		require.Nil(t, store)
		require.EqualError(t, err, "memory vector store is required")
	})
}

func TestStoreReranker_SelectsConfiguredReranker(t *testing.T) {
	disabled := false

	tests := []struct {
		name     string
		cfg      config.Config
		client   models.Client
		wantName string
		wantType any
		wantErr  string
		wantNil  bool
	}{
		{
			name:    "vector disabled",
			cfg:     config.Config{},
			wantNil: true,
		},
		{
			name: "deterministic default",
			cfg: config.Config{
				Search: config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
			},
			wantName: retrieval.RerankerDeterministic,
			wantType: retrieval.DeterministicReranker{},
		},
		{
			name: retrieval.RerankerNoop,
			cfg: config.Config{
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{Type: retrieval.RerankerNoop},
			},
			wantName: retrieval.RerankerNoop,
			wantType: retrieval.NoopReranker{},
		},
		{
			name: "globally disabled llm does not require client",
			cfg: config.Config{
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{Enabled: &disabled, Type: retrieval.RerankerLLM},
			},
			wantNil: true,
		},
		{
			name: "search disabled llm does not require client",
			cfg: config.Config{
				Search:   config.SearchConfig{EnableRerank: &disabled, Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{Type: retrieval.RerankerLLM},
			},
			wantNil: true,
		},
		{
			name: "llm requires client",
			cfg: config.Config{
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{Type: retrieval.RerankerLLM},
			},
			wantErr: "reranker model client is required",
		},
		{
			name: retrieval.RerankerLLM,
			cfg: config.Config{
				Models:   config.ModelsConfig{Main: config.MainModelConfig{Name: "openai/gpt-4o-mini", APIMode: config.DefaultModelAPIMode}},
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{Type: retrieval.RerankerLLM, Model: "openai/gpt-4o-mini", MaxCandidates: 3, MaxCandidateTextChars: 40, MaxOutputTokens: 50},
			},
			client:   &storeTestModelClient{},
			wantName: retrieval.RerankerLLM,
			wantType: retrieval.LLMReranker{},
		},
		{
			name: "invalid reranker",
			cfg: config.Config{
				Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
				Reranker: config.RerankerConfig{Type: "invalid"},
			},
			wantErr: "reranker type must be one of: deterministic, noop, llm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reranker, err := storeReranker(&tt.cfg, tt.client)
			if tt.wantErr != "" {
				require.Nil(t, reranker)
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.wantNil {
				require.Nil(t, reranker)
				return
			}
			require.IsType(t, tt.wantType, reranker)
			require.Equal(t, tt.wantName, reranker.Name())
		})
	}
}

func TestOpenStore_DefaultVectorStores(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vectors.db")), &gorm.Config{})
	require.NoError(t, err)

	sqliteStore, err := newSQLiteVectorStore(db)
	require.NoError(t, err)
	require.NotNil(t, sqliteStore)
	require.NotNil(t, newMemoryVectorStore())
}

func TestDefaultStoreEmbeddingProviderReturnsProvider(t *testing.T) {
	provider, err := defaultStoreEmbeddingProvider(&config.Config{
		Models: config.ModelsConfig{
			Key:       "key",
			Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestStoreRerankEnabledDefaults(t *testing.T) {
	require.True(t, storeRerankEnabled(nil))

	var cfg config.Config
	require.Nil(t, storeSearchRerankEnabledOption(nil))
	require.Nil(t, storeSearchRerankEnabledOption(&cfg))
}

func withStoreVectorHooks(
	t *testing.T,
	provider retrieval.Embedder,
	sqliteStore retrieval.VectorStore,
	memoryStore retrieval.VectorStore,
) {
	t.Helper()

	originalProvider := newStoreEmbeddingProvider
	originalSQLiteStore := newSQLiteVectorStore
	originalMemoryStore := newMemoryVectorStore
	t.Cleanup(func() {
		newStoreEmbeddingProvider = originalProvider
		newSQLiteVectorStore = originalSQLiteStore
		newMemoryVectorStore = originalMemoryStore
	})

	if provider != nil {
		newStoreEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return provider, nil
		}
	}
	if sqliteStore != nil {
		newSQLiteVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
			return sqliteStore, nil
		}
	}
	if memoryStore != nil {
		newMemoryVectorStore = func() retrieval.VectorStore {
			return memoryStore
		}
	}
}

type storeTestEmbeddingProvider struct {
	requests []retrieval.EmbeddingRequest
}

type storeTestModelClient struct{}

func (*storeTestModelClient) Complete(context.Context, models.Request) (*models.Response, error) {
	return &models.Response{OutputText: `{"items":[]}`}, nil
}

func (*storeTestModelClient) CompleteStream(
	context.Context,
	models.Request,
	func(models.StreamDelta),
) (*models.Response, error) {
	return &models.Response{OutputText: `{"items":[]}`}, nil
}

func (p *storeTestEmbeddingProvider) Embed(
	_ context.Context,
	req retrieval.EmbeddingRequest,
) (retrieval.EmbeddingResult, error) {
	p.requests = append(p.requests, req)

	items := make([]retrieval.Embedding, 0, len(req.Inputs))
	for idx, input := range req.Inputs {
		items = append(items, retrieval.Embedding{
			ID:          input.ID,
			ContentHash: retrieval.VectorContentHash(input.Text),
			Vector:      []float64{1, float64(idx + 1)},
		})
	}

	return retrieval.EmbeddingResult{
		Model:      req.Model,
		Items:      items,
		Dimensions: 2,
	}, nil
}

type storeTestVectorStore struct {
	upserts [][]retrieval.VectorRecord
}

func (s *storeTestVectorStore) Upsert(_ context.Context, records []retrieval.VectorRecord) error {
	cloned := make([]retrieval.VectorRecord, len(records))
	copy(cloned, records)
	s.upserts = append(s.upserts, cloned)
	return nil
}

func (s *storeTestVectorStore) Delete(context.Context, retrieval.VectorDeleteRequest) error {
	return nil
}

func (s *storeTestVectorStore) Search(
	context.Context,
	retrieval.VectorSearchRequest,
) (retrieval.VectorSearchResult, error) {
	return retrieval.VectorSearchResult{}, nil
}

func (s *storeTestVectorStore) Metadata(context.Context) (retrieval.VectorStoreMetadata, error) {
	return retrieval.VectorStoreMetadata{}, nil
}
