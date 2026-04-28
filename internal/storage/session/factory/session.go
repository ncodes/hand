package factory

import (
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	handdb "github.com/wandxy/hand/internal/db"
	"github.com/wandxy/hand/internal/models"
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
	newSQLiteSessionStoreFromDB = storagesqlite.NewSessionStoreFromDB
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
	return OpenSessionStoreWithRerankerClient(cfg, nil)
}

func OpenSessionStoreWithRerankerClient(
	cfg *config.Config,
	rerankerClient models.Client,
) (storage.SessionStore, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	cfg.Normalize()
	switch strings.TrimSpace(strings.ToLower(cfg.Storage.Backend)) {
	case "", "sqlite":
		if err := validateSearchVectorConfig(cfg); err != nil {
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

		store, err := newSQLiteSessionStoreFromDB(db)
		if err != nil {
			return nil, err
		}
		reranker, err := sessionReranker(cfg, rerankerClient)
		if err != nil {
			return nil, err
		}
		if err := configureSQLiteSessionVectors(cfg, db, store, provider, reranker); err != nil {
			return nil, err
		}

		return store, nil
	case "memory":
		if err := validateSearchVectorConfig(cfg); err != nil {
			return nil, err
		}
		provider, err := sessionEmbeddingProvider(cfg)
		if err != nil {
			return nil, err
		}

		store := storagememory.NewSessionStore()
		reranker, err := sessionReranker(cfg, rerankerClient)
		if err != nil {
			return nil, err
		}
		if err := configureMemorySessionVectors(cfg, store, provider, reranker); err != nil {
			return nil, err
		}

		return store, nil
	default:
		return nil, errors.New("storage backend must be one of: memory, sqlite")
	}
}

func configureMemorySessionVectors(
	cfg *config.Config,
	store *storagememory.SessionStore,
	provider retrieval.Embedder,
	reranker retrieval.Reranker,
) error {
	if !cfg.Search.Vector.Enabled {
		return nil
	}
	if provider == nil {
		return errors.New("embedding provider is required")
	}

	vectorStore := newMemorySessionVectorStore()
	if vectorStore == nil {
		return errors.New("memory vector store is required")
	}

	return store.ConfigureVectorStore(storage.VectorStoreOptions{
		Embedder:            provider,
		Reranker:            reranker,
		VectorStore:         vectorStore,
		EnableRerank:        sessionSearchRerankEnabledOption(cfg),
		EmbeddingModel:      cfg.Models.Embedding.Name,
		RerankMaxCandidates: cfg.Reranker.MaxCandidates,
		Required:            cfg.Search.Vector.Required,
		Diagnostics:         cfg.Debug.Requests,
	})
}

func configureSQLiteSessionVectors(
	cfg *config.Config,
	db *gorm.DB,
	store *storagesqlite.SessionStore,
	provider retrieval.Embedder,
	reranker retrieval.Reranker,
) error {
	if !cfg.Search.Vector.Enabled {
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
		Embedder:            provider,
		Reranker:            reranker,
		VectorStore:         vectorStore,
		EnableRerank:        sessionSearchRerankEnabledOption(cfg),
		EmbeddingModel:      cfg.Models.Embedding.Name,
		RebuildBatchSize:    cfg.Search.Vector.RebuildBatchSize,
		RerankMaxCandidates: cfg.Reranker.MaxCandidates,
		Required:            cfg.Search.Vector.Required,
		Diagnostics:         cfg.Debug.Requests,
	})
}

func validateSearchVectorConfig(cfg *config.Config) error {
	if !cfg.Search.Vector.Enabled {
		return nil
	}

	switch cfg.ModelEmbeddingProviderEffective() {
	case "openai", "openrouter":
	default:
		return errors.New("embedding provider must be one of: openai, openrouter")
	}

	if cfg.Models.Embedding.Name == "" {
		return errors.New("embedding model is required")
	}

	if cfg.Search.Vector.RebuildBatchSize < 0 {
		return errors.New("vector rebuild batch size must be non-negative")
	}

	if cfg.Reranker.MaxCandidates < 0 {
		return errors.New("reranker max candidates must be non-negative")
	}

	if cfg.Reranker.MaxCandidateTextChars < 0 {
		return errors.New("reranker max candidate text chars must be non-negative")
	}

	if cfg.Reranker.MaxOutputTokens < 0 {
		return errors.New("reranker max output tokens must be non-negative")
	}

	return nil
}

func sessionEmbeddingProvider(cfg *config.Config) (retrieval.Embedder, error) {
	if !cfg.Search.Vector.Enabled {
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

func sessionReranker(
	cfg *config.Config,
	client models.Client,
) (retrieval.Reranker, error) {
	if !cfg.Search.Vector.Enabled {
		return nil, nil
	}

	if !sessionRerankEnabled(cfg) {
		return nil, nil
	}

	switch cfg.RerankerEffective() {
	case retrieval.RerankerDeterministic:
		return retrieval.DeterministicReranker{}, nil
	case retrieval.RerankerNoop:
		return retrieval.NoopReranker{}, nil
	case retrieval.RerankerLLM:
		if client == nil {
			return nil, errors.New("reranker model client is required")
		}

		return retrieval.NewLLMReranker(retrieval.LLMRerankerOptions{
			Fallback:              retrieval.DeterministicReranker{},
			Client:                client,
			Model:                 cfg.RerankerModelEffective(),
			APIMode:               cfg.SummaryModelAPIModeEffective(),
			MaxCandidates:         cfg.Reranker.MaxCandidates,
			MaxCandidateTextChars: cfg.Reranker.MaxCandidateTextChars,
			MaxOutputTokens:       int64(cfg.Reranker.MaxOutputTokens),
			Enabled:               true,
			DebugRequests:         cfg.Debug.Requests,
		}), nil
	default:
		return nil, errors.New("reranker type must be one of: deterministic, noop, llm")
	}
}

func sessionRerankEnabled(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.Reranker.Enabled != nil && !*cfg.Reranker.Enabled {
		return false
	}
	if cfg.Search.EnableRerank == nil {
		return true
	}

	return *cfg.Search.EnableRerank
}

func sessionSearchRerankEnabledOption(cfg *config.Config) *bool {
	if cfg == nil || (cfg.Reranker.Enabled == nil && cfg.Search.EnableRerank == nil) {
		return nil
	}

	enabled := sessionRerankEnabled(cfg)
	return &enabled
}
