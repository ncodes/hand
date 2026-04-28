package factory

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
	"github.com/wandxy/hand/internal/storage/retrieval"
	storage "github.com/wandxy/hand/internal/storage/session"
	storagememory "github.com/wandxy/hand/internal/storage/session/memory"
	storagesqlite "github.com/wandxy/hand/internal/storage/session/sqlite"
)

func TestOpenSessionStore_ValidatesConfigAndBackend(t *testing.T) {
	store, err := OpenSessionStore(nil)
	require.Nil(t, store)
	require.EqualError(t, err, "config is required")

	store, err = OpenSessionStore(factoryStorageConfig("bogus"))
	require.Nil(t, store)
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func factoryStorageConfig(backend string) *config.Config {
	return &config.Config{Storage: config.StorageConfig{Backend: backend}}
}

func TestOpenSessionStore_ReturnsMemoryStore(t *testing.T) {
	store, err := OpenSessionStore(factoryStorageConfig("memory"))

	require.NoError(t, err)
	require.IsType(t, &storagememory.SessionStore{}, store)
}

func TestOpenSessionStore_IgnoresIncompleteVectorConfigWhenDisabled(t *testing.T) {
	store, err := OpenSessionStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{RebuildBatchSize: -1}},
	})

	require.NoError(t, err)
	require.IsType(t, &storagememory.SessionStore{}, store)
}

func TestOpenSessionStore_ReturnsSQLiteStore(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	store, err := OpenSessionStore(factoryStorageConfig("sqlite"))

	require.NoError(t, err)
	require.IsType(t, &storagesqlite.SessionStore{}, store)
	require.FileExists(t, datadir.StateDBPath())
	require.Equal(t, filepath.Join(homeDir, "data", "state.db"), datadir.StateDBPath())
}

func TestOpenSessionStore_DefaultsToSQLite(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	store, err := OpenSessionStore(&config.Config{})

	require.NoError(t, err)
	require.IsType(t, &storagesqlite.SessionStore{}, store)
	require.FileExists(t, datadir.StateDBPath())
	require.Equal(t, filepath.Join(homeDir, "data", "state.db"), datadir.StateDBPath())
}

func TestOpenSessionStore_ReturnsSQLiteOpenError(t *testing.T) {
	homePath := filepath.Join(t.TempDir(), "hand-home")
	require.NoError(t, os.WriteFile(homePath, []byte("not-a-directory"), 0o600))
	t.Setenv("HAND_HOME", homePath)

	store, err := OpenSessionStore(factoryStorageConfig("sqlite"))

	require.Nil(t, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create sqlite db directory")
}

func TestOpenSessionStore_ReturnsSQLiteStoreInitializationError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	originalStore := newSQLiteSessionStoreFromDB
	t.Cleanup(func() {
		newSQLiteSessionStoreFromDB = originalStore
	})
	newSQLiteSessionStoreFromDB = func(*gorm.DB) (*storagesqlite.SessionStore, error) {
		return nil, errors.New("session store init failed")
	}

	store, err := OpenSessionStore(factoryStorageConfig("sqlite"))

	require.Nil(t, store)
	require.EqualError(t, err, "session store init failed")
}

func TestOpenSessionStore_ConfiguresSQLiteVectorStore(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	provider := &factoryTestEmbeddingProvider{}
	vectorStore := &factoryTestVectorStore{}
	withSessionVectorHooks(t, provider, vectorStore, nil)

	rerankEnabled := false
	store, err := OpenSessionStore(&config.Config{
		Storage: config.StorageConfig{Backend: "sqlite"},
		Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
		Search: config.SearchConfig{
			EnableRerank: &rerankEnabled,
			Vector:       config.SearchVectorConfig{Enabled: true, Required: true, RebuildBatchSize: 7},
		},
	})
	require.NoError(t, err)
	require.IsType(t, &storagesqlite.SessionStore{}, store)

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

func TestOpenSessionStore_ReturnsRerankerConstructionError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	provider := &factoryTestEmbeddingProvider{}
	vectorStore := &factoryTestVectorStore{}
	withSessionVectorHooks(t, provider, vectorStore, nil)

	store, err := OpenSessionStoreWithRerankerClient(&config.Config{
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

func TestOpenSessionStore_ReturnsSQLiteVectorConfigurationError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	originalProvider := newSessionEmbeddingProvider
	originalSQLiteStore := newSQLiteSessionVectorStore
	t.Cleanup(func() {
		newSessionEmbeddingProvider = originalProvider
		newSQLiteSessionVectorStore = originalSQLiteStore
	})
	newSessionEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
		return nil, nil
	}
	newSQLiteSessionVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
		return &factoryTestVectorStore{}, nil
	}

	store, err := OpenSessionStore(&config.Config{
		Storage: config.StorageConfig{Backend: "sqlite"},
		Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.Nil(t, store)
	require.EqualError(t, err, "embedding provider is required")
}

func TestOpenSessionStore_ConfiguresMemoryVectorStore(t *testing.T) {
	provider := &factoryTestEmbeddingProvider{}
	vectorStore := &factoryTestVectorStore{}
	withSessionVectorHooks(t, provider, nil, vectorStore)

	store, err := OpenSessionStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Models: config.ModelsConfig{
			Main:      config.MainModelConfig{Provider: "openai"},
			Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test"},
		},
		Search: config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.NoError(t, err)
	require.IsType(t, &storagememory.SessionStore{}, store)

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

func TestOpenSessionStore_ReturnsMemoryVectorConfigError(t *testing.T) {
	store, err := OpenSessionStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.Nil(t, store)
	require.EqualError(t, err, "embedding model is required")
}

func TestOpenSessionStore_ReturnsMemoryVectorConfigurationError(t *testing.T) {
	originalProvider := newSessionEmbeddingProvider
	t.Cleanup(func() {
		newSessionEmbeddingProvider = originalProvider
	})
	newSessionEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
		return nil, nil
	}

	store, err := OpenSessionStore(&config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
		Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
	})

	require.Nil(t, store)
	require.EqualError(t, err, "embedding provider is required")
}

func TestOpenSessionStore_ValidatesVectorConfig(t *testing.T) {
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

			store, err := OpenSessionStore(&tt.cfg)

			require.Nil(t, store)
			require.EqualError(t, err, tt.err)
		})
	}
}

func TestOpenSessionStore_ValidatesVectorStoreFactories(t *testing.T) {
	t.Run("sqlite vector store error", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HAND_HOME", homeDir)

		originalProvider := newSessionEmbeddingProvider
		originalSQLiteStore := newSQLiteSessionVectorStore
		t.Cleanup(func() {
			newSessionEmbeddingProvider = originalProvider
			newSQLiteSessionVectorStore = originalSQLiteStore
		})

		newSessionEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return &factoryTestEmbeddingProvider{}, nil
		}
		newSQLiteSessionVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
			return nil, errors.New("vector factory failed")
		}

		store, err := OpenSessionStore(&config.Config{
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

		originalProvider := newSessionEmbeddingProvider
		originalSQLiteStore := newSQLiteSessionVectorStore
		t.Cleanup(func() {
			newSessionEmbeddingProvider = originalProvider
			newSQLiteSessionVectorStore = originalSQLiteStore
		})

		newSessionEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return &factoryTestEmbeddingProvider{}, nil
		}
		newSQLiteSessionVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
			return nil, nil
		}

		store, err := OpenSessionStore(&config.Config{
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
		originalProvider := newSessionEmbeddingProvider
		originalMemoryStore := newMemorySessionVectorStore
		t.Cleanup(func() {
			newSessionEmbeddingProvider = originalProvider
			newMemorySessionVectorStore = originalMemoryStore
		})

		newSessionEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return &factoryTestEmbeddingProvider{}, nil
		}
		newMemorySessionVectorStore = func() retrieval.VectorStore {
			return nil
		}

		store, err := OpenSessionStore(&config.Config{
			Storage: config.StorageConfig{Backend: "memory"},
			Models:  config.ModelsConfig{Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"}},
			Search:  config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
		})

		require.Nil(t, store)
		require.EqualError(t, err, "memory vector store is required")
	})
}

func TestSessionReranker_SelectsConfiguredReranker(t *testing.T) {
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
			client:   &factoryTestModelClient{},
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
			reranker, err := sessionReranker(&tt.cfg, tt.client)
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

func TestFactoryDefaultVectorStores(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vectors.db")), &gorm.Config{})
	require.NoError(t, err)

	sqliteStore, err := newSQLiteSessionVectorStore(db)
	require.NoError(t, err)
	require.NotNil(t, sqliteStore)
	require.NotNil(t, newMemorySessionVectorStore())
}

func TestDefaultSessionEmbeddingProviderReturnsProvider(t *testing.T) {
	provider, err := defaultSessionEmbeddingProvider(&config.Config{
		Models: config.ModelsConfig{
			Key:       "key",
			Embedding: config.EmbeddingModelConfig{Name: "text-embedding-test", Provider: "openai"},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestSessionRerankEnabledDefaults(t *testing.T) {
	require.True(t, sessionRerankEnabled(nil))

	var cfg config.Config
	require.Nil(t, sessionSearchRerankEnabledOption(nil))
	require.Nil(t, sessionSearchRerankEnabledOption(&cfg))
}

func withSessionVectorHooks(
	t *testing.T,
	provider retrieval.Embedder,
	sqliteStore retrieval.VectorStore,
	memoryStore retrieval.VectorStore,
) {
	t.Helper()

	originalProvider := newSessionEmbeddingProvider
	originalSQLiteStore := newSQLiteSessionVectorStore
	originalMemoryStore := newMemorySessionVectorStore
	t.Cleanup(func() {
		newSessionEmbeddingProvider = originalProvider
		newSQLiteSessionVectorStore = originalSQLiteStore
		newMemorySessionVectorStore = originalMemoryStore
	})

	if provider != nil {
		newSessionEmbeddingProvider = func(*config.Config) (retrieval.Embedder, error) {
			return provider, nil
		}
	}
	if sqliteStore != nil {
		newSQLiteSessionVectorStore = func(*gorm.DB) (retrieval.VectorStore, error) {
			return sqliteStore, nil
		}
	}
	if memoryStore != nil {
		newMemorySessionVectorStore = func() retrieval.VectorStore {
			return memoryStore
		}
	}
}

type factoryTestEmbeddingProvider struct {
	requests []retrieval.EmbeddingRequest
}

type factoryTestModelClient struct{}

func (*factoryTestModelClient) Complete(context.Context, models.Request) (*models.Response, error) {
	return &models.Response{OutputText: `{"items":[]}`}, nil
}

func (*factoryTestModelClient) CompleteStream(
	context.Context,
	models.Request,
	func(models.StreamDelta),
) (*models.Response, error) {
	return &models.Response{OutputText: `{"items":[]}`}, nil
}

func (p *factoryTestEmbeddingProvider) Embed(
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

type factoryTestVectorStore struct {
	upserts [][]retrieval.VectorRecord
}

func (s *factoryTestVectorStore) Upsert(_ context.Context, records []retrieval.VectorRecord) error {
	cloned := make([]retrieval.VectorRecord, len(records))
	copy(cloned, records)
	s.upserts = append(s.upserts, cloned)
	return nil
}

func (s *factoryTestVectorStore) Delete(context.Context, retrieval.VectorDeleteRequest) error {
	return nil
}

func (s *factoryTestVectorStore) Search(
	context.Context,
	retrieval.VectorSearchRequest,
) (retrieval.VectorSearchResult, error) {
	return retrieval.VectorSearchResult{}, nil
}

func (s *factoryTestVectorStore) Metadata(context.Context) (retrieval.VectorStoreMetadata, error) {
	return retrieval.VectorStoreMetadata{}, nil
}
