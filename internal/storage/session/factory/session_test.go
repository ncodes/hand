package factory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage/retrieval"
	storage "github.com/wandxy/hand/internal/storage/session"
	storagememory "github.com/wandxy/hand/internal/storage/session/memory"
	storagesqlite "github.com/wandxy/hand/internal/storage/session/sqlite"
)

func TestOpenSessionStore_ValidatesConfigAndBackend(t *testing.T) {
	store, err := OpenSessionStore(nil)
	require.Nil(t, store)
	require.EqualError(t, err, "config is required")

	store, err = OpenSessionStore(&config.Config{StorageBackend: "bogus"})
	require.Nil(t, store)
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func TestOpenSessionStore_ReturnsMemoryStore(t *testing.T) {
	store, err := OpenSessionStore(&config.Config{StorageBackend: "memory"})

	require.NoError(t, err)
	require.IsType(t, &storagememory.SessionStore{}, store)
}

func TestOpenSessionStore_IgnoresIncompleteVectorConfigWhenDisabled(t *testing.T) {
	store, err := OpenSessionStore(&config.Config{
		StorageBackend:                "memory",
		SessionVectorEnabled:          false,
		ModelEmbeddingProvider:        "",
		ModelEmbeddingModel:           "",
		SessionVectorRebuildBatchSize: -1,
	})

	require.NoError(t, err)
	require.IsType(t, &storagememory.SessionStore{}, store)
}

func TestOpenSessionStore_ReturnsSQLiteStore(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	store, err := OpenSessionStore(&config.Config{StorageBackend: "sqlite"})

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

	store, err := OpenSessionStore(&config.Config{StorageBackend: "sqlite"})

	require.Nil(t, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create sqlite db directory")
}

func TestOpenSessionStore_ConfiguresSQLiteVectorStore(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	provider := &factoryTestEmbeddingProvider{}
	vectorStore := &factoryTestVectorStore{}
	withSessionVectorHooks(t, provider, vectorStore, nil)

	rerankEnabled := false
	store, err := OpenSessionStore(&config.Config{
		StorageBackend:                "sqlite",
		SessionVectorEnabled:          true,
		ModelEmbeddingProvider:        "openai",
		ModelEmbeddingModel:           "text-embedding-test",
		SessionVectorRequired:         true,
		SessionVectorRebuildBatchSize: 7,
		SessionVectorEnableRerank:     &rerankEnabled,
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

func TestOpenSessionStore_ReturnsMemoryVectorIntegrationError(t *testing.T) {
	vectorStore := &factoryTestVectorStore{}
	withSessionVectorHooks(t, &factoryTestEmbeddingProvider{}, nil, vectorStore)

	store, err := OpenSessionStore(&config.Config{
		StorageBackend:       "memory",
		SessionVectorEnabled: true,
		ModelProvider:        "openai",
		ModelEmbeddingModel:  "text-embedding-test",
	})

	require.Nil(t, store)
	require.EqualError(t, err, "memory vector integration is not implemented")
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
				StorageBackend:         "sqlite",
				SessionVectorEnabled:   true,
				ModelEmbeddingProvider: "openai",
			},
			err: "embedding model is required",
		},
		{
			name: "negative batch size",
			cfg: config.Config{
				StorageBackend:                "sqlite",
				SessionVectorEnabled:          true,
				ModelEmbeddingProvider:        "openai",
				ModelEmbeddingModel:           "text-embedding-test",
				SessionVectorRebuildBatchSize: -1,
			},
			err: "vector rebuild batch size must be non-negative",
		},
		{
			name: "missing api key",
			cfg: config.Config{
				StorageBackend:       "sqlite",
				SessionVectorEnabled: true,
				ModelProvider:        "openai",
				ModelEmbeddingModel:  "text-embedding-test",
			},
			err: "embedding API key is required",
		},
		{
			name: "unsupported provider",
			cfg: config.Config{
				StorageBackend:         "sqlite",
				SessionVectorEnabled:   true,
				ModelEmbeddingProvider: "unsupported",
				ModelEmbeddingModel:    "text-embedding-test",
			},
			err: "embedding provider must be one of: openai, openrouter",
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
			StorageBackend:         "sqlite",
			SessionVectorEnabled:   true,
			ModelEmbeddingProvider: "openai",
			ModelEmbeddingModel:    "text-embedding-test",
			ModelKey:               "key",
		})

		require.Nil(t, store)
		require.EqualError(t, err, "sqlite vector store is required")
	})

	t.Run("memory vector store is required", func(t *testing.T) {
		originalMemoryStore := newMemorySessionVectorStore
		t.Cleanup(func() {
			newMemorySessionVectorStore = originalMemoryStore
		})

		newMemorySessionVectorStore = func() retrieval.VectorStore {
			return nil
		}

		store, err := OpenSessionStore(&config.Config{
			StorageBackend:         "memory",
			SessionVectorEnabled:   true,
			ModelEmbeddingProvider: "openai",
			ModelEmbeddingModel:    "text-embedding-test",
		})

		require.Nil(t, store)
		require.EqualError(t, err, "memory vector store is required")
	})
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
