package storememory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	base "github.com/wandxy/hand/internal/state"
	"github.com/wandxy/hand/internal/state/retrieval"
	vectormemory "github.com/wandxy/hand/internal/state/vector/memory"
)

func TestStore_RepairVectorStore(t *testing.T) {
	t.Run("returns nil store errors", func(t *testing.T) {
		var store *Store

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{})
		require.EqualError(t, err, "store is required")
		require.Zero(t, result)
	})

	t.Run("skips when vectors are disabled", func(t *testing.T) {
		store := NewStore()

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{})
		require.NoError(t, err)
		require.Zero(t, result)
	})

	t.Run("validates session id", func(t *testing.T) {
		store := NewStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectormemory.NewStore(),
			EmbeddingModel: "semantic-test",
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: "bad"})
		require.EqualError(t, err, "session id must be a valid ses_ nanoid")
		require.Zero(t, result)
	})

	t.Run("requires listable vector store", func(t *testing.T) {
		store := NewStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "semantic-test",
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{})
		require.EqualError(t, err, "vector store record listing is required")
		require.Zero(t, result)
	})

	t.Run("validates batch size", func(t *testing.T) {
		store := NewStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectormemory.NewStore(),
			EmbeddingModel: "semantic-test",
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{BatchSize: -1})
		require.EqualError(t, err, "vector repair batch size must be greater than or equal to zero")
		require.Zero(t, result)
	})

	t.Run("returns missing session errors", func(t *testing.T) {
		store := NewStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectormemory.NewStore(),
			EmbeddingModel: "semantic-test",
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.EqualError(t, err, "session not found")
		require.Zero(t, result)
	})

	t.Run("scans all sessions when session id is omitted", func(t *testing.T) {
		store := NewStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "first repair row",
		}}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "second repair row",
		}}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectormemory.NewStore(),
			EmbeddingModel: "semantic-test",
			Required:       true,
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{})
		require.NoError(t, err)
		require.Equal(t, 2, result.SessionsScanned)
		require.Equal(t, 2, result.MessagesScanned)
		require.Equal(t, 2, result.MissingRows)
		require.Equal(t, 2, result.RebuiltRows)
	})

	t.Run("rebuilds missing vector rows", func(t *testing.T) {
		store := NewStore()
		vectorStore := vectormemory.NewStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "retention renewal note",
		}}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectorStore,
			EmbeddingModel: "semantic-test",
			Required:       true,
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.NoError(t, err)
		require.Equal(t, 1, result.SessionsScanned)
		require.Equal(t, 1, result.MessagesScanned)
		require.Equal(t, 1, result.MissingRows)
		require.Equal(t, 1, result.RebuiltRows)
		require.Equal(t, 1, result.Batches)

		list, err := vectorStore.List(context.Background(), retrieval.VectorListRequest{
			EmbeddingModel: "semantic-test",
			Filter: retrieval.VectorFilter{
				SourceKind: retrieval.SourceKindSessionMessage,
				SessionID:  testSessionA,
			},
		})
		require.NoError(t, err)
		require.Len(t, list.Records, 1)
	})

	t.Run("rebuilds stale vector rows", func(t *testing.T) {
		store := newVectorMemoryStore(t, nil)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "original text",
		}}))
		store.mu.Lock()
		store.messages[testSessionA][0].Content = "changed text"
		store.mu.Unlock()

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.NoError(t, err)
		require.Equal(t, 1, result.StaleRows)
		require.Equal(t, 1, result.RebuiltRows)
	})

	t.Run("skips unchanged vector rows", func(t *testing.T) {
		store := newVectorMemoryStore(t, nil)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "unchanged text",
		}}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.NoError(t, err)
		require.Equal(t, 1, result.UnchangedRows)
		require.Zero(t, result.RebuiltRows)
		require.Zero(t, result.Batches)
	})

	t.Run("full rebuild refreshes unchanged vector rows", func(t *testing.T) {
		store := newVectorMemoryStore(t, nil)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "unchanged text",
		}}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{
			SessionID: testSessionA,
			Full:      true,
		})
		require.NoError(t, err)
		require.Equal(t, 1, result.UnchangedRows)
		require.Equal(t, 1, result.RebuiltRows)
		require.Equal(t, 1, result.Batches)
	})

	t.Run("honors batch size", func(t *testing.T) {
		embedder := &countingEmbedder{}
		store := NewStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "first"},
			{Role: handmsg.RoleUser, Content: "second"},
			{Role: handmsg.RoleUser, Content: "third"},
		}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       embedder,
			VectorStore:    vectormemory.NewStore(),
			EmbeddingModel: "semantic-test",
			Required:       true,
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{
			SessionID: testSessionA,
			BatchSize: 1,
		})
		require.NoError(t, err)
		require.Equal(t, 3, result.Batches)
		require.Equal(t, 3, embedder.Calls)
	})

	t.Run("returns required provider errors", func(t *testing.T) {
		store := NewStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "needs vectors",
		}}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       failingEmbedder{err: errors.New("embed failed")},
			VectorStore:    vectormemory.NewStore(),
			EmbeddingModel: "semantic-test",
			Required:       true,
		}))

		_, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.EqualError(t, err, "embed failed")
	})

	t.Run("continues after best effort provider errors", func(t *testing.T) {
		store := NewStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "needs vectors",
		}}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       failingEmbedder{err: errors.New("embed failed")},
			VectorStore:    vectormemory.NewStore(),
			EmbeddingModel: "semantic-test",
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.NoError(t, err)
		require.Equal(t, 1, result.MissingRows)
		require.Zero(t, result.RebuiltRows)
	})

	t.Run("keeps existing stale rows after best effort provider errors", func(t *testing.T) {
		vectorStore := vectormemory.NewStore()
		store := NewStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectorStore,
			EmbeddingModel: "semantic-test",
		}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "original text",
		}}))
		store.mu.Lock()
		store.messages[testSessionA][0].Content = "changed text"
		store.vectors.Provider = failingEmbedder{err: errors.New("embed failed")}
		store.mu.Unlock()

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.NoError(t, err)
		require.Equal(t, 1, result.StaleRows)
		require.Zero(t, result.RebuiltRows)

		list, err := vectorStore.List(context.Background(), retrieval.VectorListRequest{
			EmbeddingModel: "semantic-test",
			Filter: retrieval.VectorFilter{
				SourceKind: retrieval.SourceKindSessionMessage,
				SessionID:  testSessionA,
			},
		})
		require.NoError(t, err)
		require.Len(t, list.Records, 1)
	})

	t.Run("continues indexing after best effort delete errors", func(t *testing.T) {
		vectorStore := &memoryTestVectorStoreWithList{
			memoryTestVectorStore: memoryTestVectorStore{deleteErr: errors.New("delete failed")},
		}
		store := NewStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "needs vectors",
		}}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectorStore,
			EmbeddingModel: "semantic-test",
		}))

		result, err := store.RepairVectorStore(context.Background(), base.VectorRepairOptions{SessionID: testSessionA})
		require.NoError(t, err)
		require.Equal(t, 1, result.RebuiltRows)
		require.Len(t, vectorStore.upserts, 1)
		require.Len(t, vectorStore.deletes, 1)
	})
}

func TestStore_RepairVectorBatch(t *testing.T) {
	t.Run("skips empty batches", func(t *testing.T) {
		store := newVectorMemoryStore(t, nil)

		result, err := store.repairVectorBatch(context.Background(), vectormemory.NewStore(), testSessionA, nil, false)
		require.NoError(t, err)
		require.Zero(t, result)
	})

	t.Run("skips messages with no indexable rows", func(t *testing.T) {
		store := newVectorMemoryStore(t, nil)

		result, err := store.repairVectorBatch(context.Background(), vectormemory.NewStore(), testSessionA, []handmsg.Message{{
			Role: handmsg.RoleUser,
		}}, false)
		require.NoError(t, err)
		require.Equal(t, 1, result.MessagesScanned)
		require.Zero(t, result.RowsScanned)
		require.Zero(t, result.RebuiltRows)
	})

	t.Run("returns lister errors", func(t *testing.T) {
		store := newVectorMemoryStore(t, nil)

		result, err := store.repairVectorBatch(context.Background(), nil, testSessionA, []handmsg.Message{{
			ID:      1,
			Role:    handmsg.RoleUser,
			Content: "needs a list",
		}}, false)
		require.EqualError(t, err, "vector store record listing is required")
		require.Equal(t, 1, result.MessagesScanned)
		require.Equal(t, 1, result.RowsScanned)
	})

	t.Run("returns upsert errors", func(t *testing.T) {
		vectorStore := &memoryTestVectorStoreWithList{
			memoryTestVectorStore: memoryTestVectorStore{upsertErr: errors.New("upsert failed")},
		}
		store := NewStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    vectorStore,
			EmbeddingModel: "semantic-test",
		}))

		result, err := store.repairVectorBatch(context.Background(), vectorStore, testSessionA, []handmsg.Message{{
			ID:      1,
			Role:    handmsg.RoleUser,
			Content: "needs an upsert",
		}}, false)
		require.EqualError(t, err, "upsert failed")
		require.Equal(t, 1, result.MissingRows)
		require.Zero(t, result.RebuiltRows)
		require.Len(t, vectorStore.deletes, 1)
		require.Len(t, vectorStore.upserts, 1)
	})
}

type countingEmbedder struct {
	Calls int
}

func (e *countingEmbedder) Embed(
	ctx context.Context,
	req retrieval.EmbeddingRequest,
) (retrieval.EmbeddingResult, error) {
	e.Calls++
	return semanticTestEmbedder{}.Embed(ctx, req)
}

type memoryTestVectorStoreWithList struct {
	memoryTestVectorStore
	upserts [][]retrieval.VectorRecord
	deletes []retrieval.VectorDeleteRequest
}

func (s *memoryTestVectorStoreWithList) Upsert(_ context.Context, records []retrieval.VectorRecord) error {
	s.upserts = append(s.upserts, append([]retrieval.VectorRecord(nil), records...))
	return s.memoryTestVectorStore.Upsert(context.Background(), records)
}

func (s *memoryTestVectorStoreWithList) Delete(_ context.Context, req retrieval.VectorDeleteRequest) error {
	s.deletes = append(s.deletes, req)
	return s.memoryTestVectorStore.Delete(context.Background(), req)
}

func (s *memoryTestVectorStoreWithList) List(
	_ context.Context,
	req retrieval.VectorListRequest,
) (retrieval.VectorListResult, error) {
	if err := retrieval.ValidateVectorListRequest(req); err != nil {
		return retrieval.VectorListResult{}, err
	}
	return retrieval.VectorListResult{}, nil
}
