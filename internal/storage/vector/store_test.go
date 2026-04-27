package vector_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/storage/retrieval"
	vectormemory "github.com/wandxy/hand/internal/storage/vector/memory"
	vectorsqlite "github.com/wandxy/hand/internal/storage/vector/sqlite"
)

func TestVectorStoreContract(t *testing.T) {
	tests := []struct {
		newStore func(t *testing.T) retrieval.VectorStore
		name     string
	}{
		{
			name: "memory",
			newStore: func(t *testing.T) retrieval.VectorStore {
				t.Helper()

				return vectormemory.NewStore()
			},
		},
		{
			name: "sqlite",
			newStore: func(t *testing.T) retrieval.VectorStore {
				t.Helper()

				store, err := vectorsqlite.NewStore(filepath.Join(t.TempDir(), "vectors.db"))
				require.NoError(t, err)

				return store
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/upsert search delete and metadata", func(t *testing.T) {
			assertVectorStoreUpsertSearchDeleteAndMetadata(t, tt.newStore(t))
		})
		t.Run(tt.name+"/replace record and isolate vectors", func(t *testing.T) {
			assertVectorStoreReplaceAndCopyIsolation(t, tt.newStore(t))
		})
		t.Run(tt.name+"/filter model dimensions source and metadata", func(t *testing.T) {
			assertVectorStoreFiltersAndMetadata(t, tt.newStore(t))
		})
		t.Run(tt.name+"/validation errors", func(t *testing.T) {
			assertVectorStoreValidationErrors(t, tt.newStore(t))
		})
		t.Run(tt.name+"/empty search for missing index", func(t *testing.T) {
			assertVectorStoreSearchMissingIndex(t, tt.newStore(t))
		})
	}
}

func assertVectorStoreUpsertSearchDeleteAndMetadata(t *testing.T, store retrieval.VectorStore) {
	t.Helper()

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	records := []retrieval.VectorRecord{
		testRecord("vec-a", retrieval.SourceKindSessionMessage, "msg-a", []float64{1, 0, 0}, now),
		testRecord("vec-b", retrieval.SourceKindSessionMessage, "msg-b", []float64{0.8, 0.2, 0}, now.Add(time.Second)),
		testRecord("vec-c", retrieval.SourceKindMemoryItem, "mem-c", []float64{0, 1, 0}, now.Add(2*time.Second)),
	}
	records[0].SessionID = "ses-a"
	records[0].Role = "assistant"
	records[1].SessionID = "ses-b"
	records[1].Role = "assistant"
	records[1].ToolName = "process"

	require.NoError(t, store.Upsert(context.Background(), records))

	result, err := store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          2,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-a", "vec-b"}, matchIDs(result.Matches))
	require.Greater(t, result.Matches[0].Score, result.Matches[1].Score)
	require.Equal(t, []float64{1, 0, 0}, result.Matches[0].Record.Vector)
	require.Equal(t, now, result.Matches[0].Record.CreatedAt)

	result, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
			SourceIDs:  []string{"msg-b"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-b"}, matchIDs(result.Matches))

	result, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
			SessionID:  "ses-b",
			Role:       "assistant",
			ToolName:   "process",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-b"}, matchIDs(result.Matches))

	result, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind:      retrieval.SourceKindSessionMessage,
			IgnoreSessionID: "ses-a",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-b"}, matchIDs(result.Matches))

	metadata, err := store.Metadata(context.Background())
	require.NoError(t, err)
	require.Equal(t, retrieval.VectorStoreMetadata{Models: []retrieval.VectorModelMetadata{{
		Model:      "text-embedding-test",
		Dimensions: 3,
		Count:      3,
	}}}, metadata)

	require.NoError(t, store.Delete(context.Background(), retrieval.VectorDeleteRequest{
		SourceKind: retrieval.SourceKindSessionMessage,
		SourceIDs:  []string{"msg-b"},
	}))
	result, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-a"}, matchIDs(result.Matches))
}

func assertVectorStoreReplaceAndCopyIsolation(t *testing.T, store retrieval.VectorStore) {
	t.Helper()

	record := testRecord("vec-a", retrieval.SourceKindSessionMessage, "msg-a", []float64{1, 0}, time.Time{})
	require.NoError(t, store.Upsert(context.Background(), []retrieval.VectorRecord{record}))
	record.Vector[0] = 0

	updated := testRecord("vec-a", retrieval.SourceKindSessionMessage, "msg-a", []float64{0, 1}, time.Time{})
	require.NoError(t, store.Upsert(context.Background(), []retrieval.VectorRecord{updated}))
	updated.Vector[1] = 0

	result, err := store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     2,
		QueryVector:    []float64{0, 1},
		Limit:          1,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Matches, 1)
	require.Equal(t, []float64{0, 1}, result.Matches[0].Record.Vector)

	result.Matches[0].Record.Vector[1] = 0
	result, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     2,
		QueryVector:    []float64{0, 1},
		Limit:          1,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []float64{0, 1}, result.Matches[0].Record.Vector)
}

func assertVectorStoreFiltersAndMetadata(t *testing.T, store retrieval.VectorStore) {
	t.Helper()

	require.NoError(t, store.Upsert(context.Background(), []retrieval.VectorRecord{
		testRecord("vec-b", retrieval.SourceKindSessionMessage, "msg-b", []float64{1, 0}, time.Time{}),
		testRecord("vec-a", retrieval.SourceKindSessionMessage, "msg-a", []float64{1, 0}, time.Time{}),
		testRecordWithModel("vec-other-model", retrieval.SourceKindSessionMessage, "msg-c", "other-model", []float64{1, 0}),
		testRecord("vec-other-dim", retrieval.SourceKindSessionMessage, "msg-d", []float64{1, 0, 0}, time.Time{}),
		testRecord("vec-memory", retrieval.SourceKindMemoryItem, "mem-a", []float64{1, 0}, time.Time{}),
	}))

	result, err := store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     2,
		QueryVector:    []float64{1, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-a", "vec-b"}, matchIDs(result.Matches))

	result, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "other-model",
		Dimensions:     2,
		QueryVector:    []float64{1, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-other-model"}, matchIDs(result.Matches))

	result, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     2,
		QueryVector:    []float64{1, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindMemoryItem,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"vec-memory"}, matchIDs(result.Matches))

	metadata, err := store.Metadata(context.Background())
	require.NoError(t, err)
	require.Equal(t, retrieval.VectorStoreMetadata{Models: []retrieval.VectorModelMetadata{
		{Model: "other-model", Dimensions: 2, Count: 1},
		{Model: "text-embedding-test", Dimensions: 2, Count: 3},
		{Model: "text-embedding-test", Dimensions: 3, Count: 1},
	}}, metadata)
}

func assertVectorStoreValidationErrors(t *testing.T, store retrieval.VectorStore) {
	t.Helper()

	err := store.Upsert(context.Background(), []retrieval.VectorRecord{{}})
	require.EqualError(t, err, "vector id is required")

	err = store.Delete(context.Background(), retrieval.VectorDeleteRequest{})
	require.EqualError(t, err, "source kind is required")

	_, err = store.Search(context.Background(), retrieval.VectorSearchRequest{})
	require.EqualError(t, err, "vector search embedding model is required")

	_, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     1,
		QueryVector:    []float64{1},
		Limit:          1,
	})
	require.EqualError(t, err, "vector search filter source kind is required")

	_, err = store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     1,
		QueryVector:    []float64{1},
		Limit:          1,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
			SourceIDs:  []string{""},
		},
	})
	require.EqualError(t, err, "vector search filter source id is required")
}

func assertVectorStoreSearchMissingIndex(t *testing.T, store retrieval.VectorStore) {
	t.Helper()

	result, err := store.Search(context.Background(), retrieval.VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          10,
		Filter: retrieval.VectorFilter{
			SourceKind: retrieval.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Matches)
}

func testRecord(
	id string,
	sourceKind retrieval.SourceKind,
	sourceID string,
	vector []float64,
	at time.Time,
) retrieval.VectorRecord {
	return retrieval.VectorRecord{
		CreatedAt:      at,
		UpdatedAt:      at,
		ID:             id,
		SourceKind:     sourceKind,
		SourceID:       sourceID,
		SessionID:      "ses-test",
		Role:           "user",
		EmbeddingModel: "text-embedding-test",
		Dimensions:     len(vector),
		ContentHash:    retrieval.VectorContentHash(id),
		Vector:         append([]float64(nil), vector...),
	}
}

func testRecordWithModel(
	id string,
	sourceKind retrieval.SourceKind,
	sourceID string,
	model string,
	vector []float64,
) retrieval.VectorRecord {
	record := testRecord(id, sourceKind, sourceID, vector, time.Time{})
	record.EmbeddingModel = model

	return record
}

func matchIDs(matches []retrieval.VectorSearchMatch) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.Record.ID)
	}

	return ids
}
