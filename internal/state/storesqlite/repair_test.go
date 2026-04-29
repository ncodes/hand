package storesqlite

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state/retrieval"
	statevector "github.com/wandxy/hand/internal/state/vector"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestSQLiteStore_RebuildVectorStoreContinuesAfterBestEffortDeleteError(t *testing.T) {
	store, _, vectorStore := sqliteRepairTestStore(t)

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	sqliteRepairSaveMessages(t, store, testSessionA, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "repair me", CreatedAt: now},
	})

	vectorStore.deleteErr = errors.New("delete failed")
	require.NoError(t, store.RebuildVectorStore(context.Background(), testSessionA))
	require.Len(t, vectorStore.deletes, 1)
	require.Len(t, vectorStore.upserts, 1)
}

func TestSQLiteStore_RebuildVectorStoreBatchesMessages(t *testing.T) {
	store, provider, vectorStore := sqliteRepairTestStore(t)
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		Embedder:         provider,
		VectorStore:      vectorStore,
		EmbeddingModel:   "text-embedding-test",
		RebuildBatchSize: 2,
	}))

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	sqliteRepairSaveMessages(t, store, testSessionA, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "first", CreatedAt: now},
		{ID: 2, Role: handmsg.RoleUser, Content: "second", CreatedAt: now.Add(time.Second)},
		{ID: 3, Role: handmsg.RoleUser, Content: "third", CreatedAt: now.Add(2 * time.Second)},
	})

	provider.requests = nil
	vectorStore.upserts = nil
	vectorStore.deletes = nil

	require.NoError(t, store.RebuildVectorStore(context.Background(), testSessionA))

	require.Len(t, provider.requests, 2)
	require.Len(t, provider.requests[0].Inputs, 2)
	require.Len(t, provider.requests[1].Inputs, 1)
	require.Len(t, vectorStore.upserts, 2)
	require.Len(t, vectorStore.upserts[0], 2)
	require.Len(t, vectorStore.upserts[1], 1)
	require.Len(t, vectorStore.deletes, 2)
	require.Equal(t, []string{
		sourceIDForMessage(testSessionA, 1),
		sourceIDForMessage(testSessionA, 2),
	}, vectorStore.deletes[0].SourceIDs)
	require.Equal(t, []string{
		sourceIDForMessage(testSessionA, 3),
	}, vectorStore.deletes[1].SourceIDs)
}

func TestSQLiteStore_RebuildVectorStoreValidationAndErrorPaths(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	var nilStore *Store
	require.EqualError(t, nilStore.RebuildVectorStore(context.Background(), testSessionA), "store is required")

	store, _, _ := sqliteRepairTestStore(t)
	require.EqualError(t, store.RebuildVectorStore(context.Background(), " "), "session id is required")
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.RebuildVectorStore(context.Background(), testSessionA))

	brokenStore, _, _ := sqliteRepairTestStore(t)
	require.NoError(t, brokenStore.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, brokenStore.db.Exec(`DROP TABLE session_messages`).Error)
	require.Error(t, brokenStore.RebuildVectorStore(context.Background(), testSessionA))

	brokenStore, _, _ = sqliteRepairTestStore(t)
	require.NoError(t, brokenStore.db.Exec(`DROP TABLE sessions`).Error)
	require.Error(t, brokenStore.RebuildVectorStore(context.Background(), testSessionA))

	deleteErr := errors.New("delete failed")
	requiredDeleteStore, _, vectorStore := sqliteRepairTestStore(t)
	require.NoError(t, requiredDeleteStore.ConfigureVectorStore(VectorStoreOptions{
		Embedder:       &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:    vectorStore,
		EmbeddingModel: "text-embedding-test",
		Required:       true,
	}))
	require.NoError(t, requiredDeleteStore.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	sqliteRepairSaveMessages(t, requiredDeleteStore, testSessionA, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "delete required", CreatedAt: now},
	})
	vectorStore.deleteErr = deleteErr
	require.ErrorIs(t, requiredDeleteStore.RebuildVectorStore(context.Background(), testSessionA), deleteErr)

	upsertErr := errors.New("upsert failed")
	upsertStore, _, _ := sqliteRepairTestStore(t)
	upsertVectorStore := &sqliteTestVectorStore{}
	require.NoError(t, upsertStore.ConfigureVectorStore(VectorStoreOptions{
		Embedder:       &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:    upsertVectorStore,
		EmbeddingModel: "text-embedding-test",
		Required:       true,
	}))
	require.NoError(t, upsertStore.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	sqliteRepairSaveMessages(t, upsertStore, testSessionA, []handmsg.Message{
		{ID: 1, Role: handmsg.RoleUser, Content: "upsert required", CreatedAt: now},
	})
	upsertVectorStore.upsertErr = upsertErr
	require.ErrorIs(t, upsertStore.RebuildVectorStore(context.Background(), testSessionA), upsertErr)
}

func TestSQLiteStore_RepairVectorStore(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	t.Run("returns zero result when vectors are not configured", func(t *testing.T) {
		store := sqliteRepairStoreWithoutVectors(t)
		result, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{})
		require.NoError(t, err)
		require.Equal(t, statevector.VectorRepairResult{}, result)
	})

	t.Run("rejects invalid session id", func(t *testing.T) {
		store, _, _ := sqliteRepairTestStore(t)
		_, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{SessionID: "invalid"})
		require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	})

	t.Run("rejects negative batch size", func(t *testing.T) {
		store, _, _ := sqliteRepairTestStore(t)
		_, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{BatchSize: -1})
		require.EqualError(t, err, "vector repair batch size must be greater than or equal to zero")
	})

	t.Run("requires vector record listing", func(t *testing.T) {
		store := sqliteRepairStoreWithoutVectors(t)
		require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
			Embedder:       &sqliteTestEmbeddingProvider{dimensions: 3},
			VectorStore:    repairSearchOnlyVectorStore{},
			EmbeddingModel: "text-embedding-test",
		}))

		_, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{})
		require.EqualError(t, err, "vector store record listing is required")
	})

	t.Run("returns session listing errors", func(t *testing.T) {
		store, _, _ := sqliteRepairTestStore(t)
		require.NoError(t, store.db.Exec(`DROP TABLE sessions`).Error)
		_, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{})
		require.Error(t, err)
	})

	t.Run("returns missing scoped session errors", func(t *testing.T) {
		store, _, _ := sqliteRepairTestStore(t)
		_, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{
			SessionID: testMissingSession,
		})
		require.EqualError(t, err, "session not found")
	})

	t.Run("repairs all sessions and reports missing rows", func(t *testing.T) {
		store, provider, vectorStore := sqliteRepairTestStore(t)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
		sqliteRepairSaveMessages(t, store, testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "alpha repair", CreatedAt: now},
		})
		sqliteRepairSaveMessages(t, store, testSessionB, []handmsg.Message{
			{ID: 2, Role: handmsg.RoleUser, Content: "beta repair", CreatedAt: now},
		})

		result, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{})

		require.NoError(t, err)
		require.Equal(t, 2, result.SessionsScanned)
		require.Equal(t, 2, result.MessagesScanned)
		require.Equal(t, 2, result.RowsScanned)
		require.Equal(t, 2, result.MissingRows)
		require.Equal(t, 2, result.RebuiltRows)
		require.Equal(t, 2, result.DeletedSources)
		require.Equal(t, 2, result.Batches)
		require.Len(t, provider.requests, 2)
		require.Len(t, vectorStore.upserts, 2)
	})

	t.Run("skips unchanged rows", func(t *testing.T) {
		store, provider, vectorStore := sqliteRepairTestStore(t)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		records := sqliteRepairSaveMessages(t, store, testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "already indexed", CreatedAt: now},
		})
		vectorRecords, err := store.vectorRecordsForMessages(context.Background(), records)
		require.NoError(t, err)
		require.NoError(t, vectorStore.Upsert(context.Background(), vectorRecords))
		provider.requests = nil
		vectorStore.upserts = nil

		result, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{SessionID: testSessionA})

		require.NoError(t, err)
		require.Equal(t, 1, result.SessionsScanned)
		require.Equal(t, 1, result.MessagesScanned)
		require.Equal(t, 1, result.RowsScanned)
		require.Equal(t, 1, result.UnchangedRows)
		require.Zero(t, result.RebuiltRows)
		require.Empty(t, provider.requests)
		require.Empty(t, vectorStore.upserts)
	})

	t.Run("full repair rebuilds unchanged rows", func(t *testing.T) {
		store, provider, vectorStore := sqliteRepairTestStore(t)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		records := sqliteRepairSaveMessages(t, store, testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "force rebuild", CreatedAt: now},
		})
		vectorRecords, err := store.vectorRecordsForMessages(context.Background(), records)
		require.NoError(t, err)
		require.NoError(t, vectorStore.Upsert(context.Background(), vectorRecords))
		provider.requests = nil
		vectorStore.upserts = nil
		vectorStore.deletes = nil

		result, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{
			SessionID: testSessionA,
			Full:      true,
		})

		require.NoError(t, err)
		require.Equal(t, 1, result.UnchangedRows)
		require.Equal(t, 1, result.RebuiltRows)
		require.Equal(t, 1, result.DeletedSources)
		require.Equal(t, 1, result.Batches)
		require.Len(t, provider.requests, 1)
		require.Len(t, vectorStore.deletes, 1)
		require.Len(t, vectorStore.upserts, 1)
	})

	t.Run("continues after best effort list errors", func(t *testing.T) {
		store, _, vectorStore := sqliteRepairTestStore(t)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		sqliteRepairSaveMessages(t, store, testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "list failure", CreatedAt: now},
		})
		vectorStore.listErr = errors.New("list failed")

		result, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{SessionID: testSessionA})

		require.NoError(t, err)
		require.Equal(t, 1, result.MessagesScanned)
		require.Equal(t, 1, result.RowsScanned)
		require.Empty(t, vectorStore.upserts)
	})

	t.Run("returns required list errors", func(t *testing.T) {
		store, _, vectorStore := sqliteRepairTestStore(t)
		require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
			Embedder:       &sqliteTestEmbeddingProvider{dimensions: 3},
			VectorStore:    vectorStore,
			EmbeddingModel: "text-embedding-test",
			Required:       true,
		}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		sqliteRepairSaveMessages(t, store, testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "required list failure", CreatedAt: now},
		})
		vectorStore.listErr = errors.New("list failed")

		_, err := store.RepairVectorStore(context.Background(), statevector.VectorRepairOptions{SessionID: testSessionA})

		require.EqualError(t, err, "list failed")
	})
}

func TestSQLiteStore_RepairVectorBatch(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	t.Run("skips empty records", func(t *testing.T) {
		store, _, vectorStore := sqliteRepairTestStore(t)

		result, err := store.repairVectorBatch(context.Background(), vectorStore, nil, false)

		require.NoError(t, err)
		require.Equal(t, statevector.VectorRepairResult{}, result)
	})

	t.Run("skips messages without indexable rows", func(t *testing.T) {
		store, _, vectorStore := sqliteRepairTestStore(t)
		records := messagesToMessageModelsWithOffset(testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, CreatedAt: now},
		}, 0)

		result, err := store.repairVectorBatch(context.Background(), vectorStore, records, false)

		require.NoError(t, err)
		require.Equal(t, 1, result.MessagesScanned)
		require.Zero(t, result.RowsScanned)
	})

	t.Run("returns embedding errors before deleting dirty rows", func(t *testing.T) {
		store, _, vectorStore := sqliteRepairTestStore(t)
		embedErr := errors.New("embed failed")
		require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
			Embedder:       &sqliteTestEmbeddingProvider{err: embedErr},
			VectorStore:    vectorStore,
			EmbeddingModel: "text-embedding-test",
		}))
		records := messagesToMessageModelsWithOffset(testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "embed failure", CreatedAt: now},
		}, 0)

		result, err := store.repairVectorBatch(context.Background(), vectorStore, records, false)

		require.ErrorIs(t, err, embedErr)
		require.Equal(t, 1, result.MissingRows)
		require.Empty(t, vectorStore.deletes)
		require.Empty(t, vectorStore.upserts)
	})
}

func TestMessageModelsBySourceID(t *testing.T) {
	records := []messageModel{
		{ID: 1, SessionID: testSessionA},
		{ID: 2, SessionID: testSessionA},
	}

	t.Run("returns nil without source ids", func(t *testing.T) {
		require.Nil(t, messageModelsBySourceID(records, []string{" ", ""}))
	})

	t.Run("selects matching records", func(t *testing.T) {
		selected := messageModelsBySourceID(records, []string{sourceIDForMessage(testSessionA, 2)})

		require.Len(t, selected, 1)
		require.Equal(t, uint(2), selected[0].ID)
	})
}

func sqliteRepairTestStore(
	t *testing.T,
) (*Store, *sqliteTestEmbeddingProvider, *sqliteTestVectorStore) {
	t.Helper()

	store := sqliteRepairStoreWithoutVectors(t)
	provider := &sqliteTestEmbeddingProvider{dimensions: 3}
	vectorStore := &sqliteTestVectorStore{}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		Embedder:       provider,
		VectorStore:    vectorStore,
		EmbeddingModel: "text-embedding-test",
	}))

	return store, provider, vectorStore
}

func sqliteRepairStoreWithoutVectors(t *testing.T) *Store {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "session.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&sessionModel{}, &messageModel{}))

	return &Store{db: db}
}

func sqliteRepairSaveMessages(
	t *testing.T,
	store *Store,
	sessionID string,
	messages []handmsg.Message,
) []messageModel {
	t.Helper()

	records := messagesToMessageModelsWithOffset(sessionID, messages, 0)
	require.NoError(t, store.db.Create(&records).Error)

	return records
}

type repairSearchOnlyVectorStore struct{}

func (repairSearchOnlyVectorStore) Upsert(context.Context, []retrieval.VectorRecord) error {
	return nil
}

func (repairSearchOnlyVectorStore) Delete(context.Context, retrieval.VectorDeleteRequest) error {
	return nil
}

func (repairSearchOnlyVectorStore) Search(
	context.Context,
	retrieval.VectorSearchRequest,
) (retrieval.VectorSearchResult, error) {
	return retrieval.VectorSearchResult{}, nil
}

func (repairSearchOnlyVectorStore) Metadata(context.Context) (retrieval.VectorStoreMetadata, error) {
	return retrieval.VectorStoreMetadata{}, nil
}
