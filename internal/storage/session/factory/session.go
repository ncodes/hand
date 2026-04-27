package factory

import (
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	handdb "github.com/wandxy/hand/internal/db"
	"github.com/wandxy/hand/internal/storage/retrieval"
	storage "github.com/wandxy/hand/internal/storage/session"
	storagememory "github.com/wandxy/hand/internal/storage/session/memory"
	storagesqlite "github.com/wandxy/hand/internal/storage/session/sqlite"
	vectormemory "github.com/wandxy/hand/internal/storage/vector/memory"
	vectorsqlite "github.com/wandxy/hand/internal/storage/vector/sqlite"
	"gorm.io/gorm"
)

var (
	newSessionEmbeddingProvider = defaultSessionEmbeddingProvider
	newSQLiteSessionVectorStore = func(db *gorm.DB) (retrieval.VectorStore, error) {
		return vectorsqlite.NewStoreFromDB(db)
	}
	newMemorySessionVectorStore = func() retrieval.VectorStore {
		return vectormemory.NewStore()
	}
)

// OpenSessionStore opens a session store based on the configuration.
//
// It supports the following storage backends:
//
//   - sqlite: uses a SQLite database
//   - memory: uses a memory-based store
//
// The configuration is used to determine the storage backend to use.
//
// The function returns a SessionStore interface that can be used to store and retrieve sessions.
func OpenSessionStore(cfg *config.Config) (storage.SessionStore, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	cfg.Normalize()
	switch strings.TrimSpace(strings.ToLower(cfg.StorageBackend)) {
	case "", "sqlite":
		if err := validateSessionVectorConfig(cfg); err != nil {
			return nil, err
		}
		provider, err := sessionEmbeddingProvider(cfg)
		if err != nil {
			return nil, err
		}

		db, err := handdb.Open(cfg)
		if err != nil {
			return nil, err
		}

		store, err := storagesqlite.NewSessionStoreFromDB(db)
		if err != nil {
			return nil, err
		}
		if err := configureSQLiteSessionVectors(cfg, db, store, provider); err != nil {
			return nil, err
		}

		return store, nil
	case "memory":
		if err := validateSessionVectorConfig(cfg); err != nil {
			return nil, err
		}
		if cfg.SessionVectorEnabled {
			vectorStore := newMemorySessionVectorStore()
			if vectorStore == nil {
				return nil, errors.New("memory vector store is required")
			}

			return nil, errors.New("memory vector integration is not implemented")
		}

		return storagememory.NewSessionStore(), nil
	default:
		return nil, errors.New("storage backend must be one of: memory, sqlite")
	}
}

func configureSQLiteSessionVectors(
	cfg *config.Config,
	db *gorm.DB,
	store *storagesqlite.SessionStore,
	provider retrieval.Embedder,
) error {
	if !cfg.SessionVectorEnabled {
		return nil
	}
	if provider == nil {
		return errors.New("embedding provider is required")
	}

	vectorStore, err := newSQLiteSessionVectorStore(db)
	if err != nil {
		return err
	}
	if vectorStore == nil {
		return errors.New("sqlite vector store is required")
	}

	return store.ConfigureVectorStore(storagesqlite.VectorStoreOptions{
		Embedder:         provider,
		VectorStore:      vectorStore,
		EnableRerank:     cfg.SessionVectorEnableRerank,
		EmbeddingModel:   cfg.ModelEmbeddingModel,
		RebuildBatchSize: cfg.SessionVectorRebuildBatchSize,
		Required:         cfg.SessionVectorRequired,
	})
}

func validateSessionVectorConfig(cfg *config.Config) error {
	if !cfg.SessionVectorEnabled {
		return nil
	}
	switch cfg.ModelEmbeddingProviderEffective() {
	case "openai", "openrouter":
	default:
		return errors.New("embedding provider must be one of: openai, openrouter")
	}
	if cfg.ModelEmbeddingModel == "" {
		return errors.New("embedding model is required")
	}
	if cfg.SessionVectorRebuildBatchSize < 0 {
		return errors.New("vector rebuild batch size must be non-negative")
	}

	return nil
}

func sessionEmbeddingProvider(cfg *config.Config) (retrieval.Embedder, error) {
	if !cfg.SessionVectorEnabled {
		return nil, nil
	}

	return newSessionEmbeddingProvider(cfg)
}

func defaultSessionEmbeddingProvider(cfg *config.Config) (retrieval.Embedder, error) {
	auth, err := cfg.ResolveEmbeddingModelAuth()
	if err != nil {
		return nil, err
	}

	return retrieval.NewEmbeddingProvider(retrieval.EmbeddingProviderOptions{
		Provider:    auth.Provider,
		APIKey:      auth.APIKey,
		EndpointURL: auth.BaseURL,
	})
}
