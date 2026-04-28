package memory

import (
	"context"
	"errors"
	"io"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage/retrieval"
	base "github.com/wandxy/hand/internal/storage/session"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestSessionStore_ConfigureVectorStore(t *testing.T) {
	t.Run("rejects nil store", func(t *testing.T) {
		var nilStore *SessionStore
		require.EqualError(t, nilStore.ConfigureVectorStore(base.VectorStoreOptions{}), "session store is required")
	})

	t.Run("clears vector config when options are empty", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{}))
		require.Nil(t, store.vectors)
	})

	t.Run("requires embedder", func(t *testing.T) {
		store := NewSessionStore()
		require.EqualError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "model",
		}), "vector store embedding provider is required")
	})

	t.Run("requires vector store", func(t *testing.T) {
		store := NewSessionStore()
		require.EqualError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			EmbeddingModel: "model",
		}), "vector store is required")
	})

	t.Run("requires embedding model", func(t *testing.T) {
		store := NewSessionStore()
		require.EqualError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:    semanticTestEmbedder{},
			VectorStore: &memoryTestVectorStore{},
		}), "vector store embedding model is required")
	})

	t.Run("rejects negative rerank candidate limit", func(t *testing.T) {
		store := NewSessionStore()
		require.EqualError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:            semanticTestEmbedder{},
			VectorStore:         &memoryTestVectorStore{},
			EmbeddingModel:      "model",
			RerankMaxCandidates: -1,
		}), "vector store rerank max candidates must be greater than or equal to zero")
	})

	t.Run("rejects invalid reranker", func(t *testing.T) {
		store := NewSessionStore()
		require.EqualError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			Reranker:       invalidNameReranker{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "model",
		}), "reranker must be one of: noop, deterministic, llm")
	})
}

func TestSessionStore_SearchMessages(t *testing.T) {
	t.Run("returns embedder errors", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       failingEmbedder{err: errors.New("embed failed")},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "model",
		}))

		results, err := store.SearchMessages(context.Background(), testSessionA, base.SearchMessageOptions{
			Query: "needle",
		})
		require.EqualError(t, err, "embed failed")
		require.Nil(t, results)
	})

	t.Run("returns embedding validation errors", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       malformedEmbedder{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "model",
		}))

		results, err := store.SearchMessages(context.Background(), testSessionA, base.SearchMessageOptions{
			Query: "needle",
		})
		require.EqualError(t, err, "embedding result count must match input count")
		require.Nil(t, results)
	})

	t.Run("returns vector store errors", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{searchErr: errors.New("search failed")},
			EmbeddingModel: "semantic-test",
		}))

		results, err := store.SearchMessages(context.Background(), testSessionA, base.SearchMessageOptions{
			Query: "needle",
		})
		require.EqualError(t, err, "search failed")
		require.Nil(t, results)
	})
}

func TestSessionStore_AppendMessages(t *testing.T) {
	t.Run("returns required vector upsert errors", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{upsertErr: errors.New("upsert failed")},
			EmbeddingModel: "semantic-test",
			Required:       true,
		}))

		err := store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "needle",
		}})
		require.EqualError(t, err, "upsert failed")
	})
}

func TestSessionStore_ClearMessages(t *testing.T) {
	t.Run("returns required vector delete errors", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{deleteErr: errors.New("delete failed")},
			EmbeddingModel: "semantic-test",
			Required:       true,
		}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "needle",
		}}))

		err := store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.EqualError(t, err, "delete failed")
	})
}

func TestFindMessageByID(t *testing.T) {
	t.Run("returns false when id is missing", func(t *testing.T) {
		_, ok := findMessageByID([]handmsg.Message{{ID: 1}}, 2)
		require.False(t, ok)
	})
}

func TestMessageMatchesSearchOptions(t *testing.T) {
	t.Run("rejects mismatched session id", func(t *testing.T) {
		require.False(t, messageMatchesSearchOptions(testSessionA, handmsg.Message{Role: handmsg.RoleUser}, testSessionB, base.SearchMessageOptions{}))
	})

	t.Run("rejects ignored session id", func(t *testing.T) {
		require.False(t, messageMatchesSearchOptions(testSessionA, handmsg.Message{Role: handmsg.RoleUser}, "", base.SearchMessageOptions{IgnoreSessionID: testSessionA}))
	})

	t.Run("rejects mismatched role", func(t *testing.T) {
		require.False(t, messageMatchesSearchOptions(testSessionA, handmsg.Message{Role: handmsg.RoleUser}, "", base.SearchMessageOptions{Role: handmsg.RoleAssistant}))
	})
}

func TestLexicalScore(t *testing.T) {
	t.Run("returns zero when query is missing", func(t *testing.T) {
		require.Equal(t, float64(0), lexicalScore("body", "missing"))
	})
}

func TestSearchRerankResultName(t *testing.T) {
	t.Run("uses fallback when result name is empty", func(t *testing.T) {
		require.Equal(t, "fallback", searchRerankResultName(retrieval.RerankResult{}, "fallback"))
	})
}

func TestSessionStore_RerankerName(t *testing.T) {
	t.Run("defaults to deterministic", func(t *testing.T) {
		require.Equal(t, retrieval.RerankerDeterministic, (*SessionStore)(nil).rerankerName())
	})
}

func TestSessionStore_DiagnosticsEnabled(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		require.False(t, (*SessionStore)(nil).diagnosticsEnabled())
	})
}

func TestSearchResultsFromCandidates(t *testing.T) {
	now := time.Now().UTC()
	older := now.Add(-time.Minute)

	t.Run("applies result limits", func(t *testing.T) {
		results := searchResultsFromCandidates([]*searchCandidate{{
			CandidateMatch: base.CandidateMatch{
				SessionID:   testSessionA,
				VectorRank:  1,
				HasVector:   true,
				VectorScore: 1,
			},
			Message: handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "body a", CreatedAt: now},
		}, {
			CandidateMatch: base.CandidateMatch{
				SessionID:   testSessionB,
				VectorRank:  2,
				HasVector:   true,
				VectorScore: 0.8,
			},
			Message: handmsg.Message{ID: 2, Role: handmsg.RoleUser, Content: "body b", CreatedAt: older},
		}}, base.SearchMessageOptions{
			MaxSessions:           1,
			MaxMessagesPerSession: 1,
		})
		require.Len(t, results, 1)
		require.Equal(t, testSessionA, results[0].SessionID)
		require.Equal(t, 1, results[0].MatchCount)
	})

	t.Run("result ordering prefers newest session match when scores tie", func(t *testing.T) {
		results := searchResultsFromCandidates([]*searchCandidate{{
			CandidateMatch: base.CandidateMatch{SessionID: testSessionB, FusedScore: 1},
			Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "newer", CreatedAt: now},
		}, {
			CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 1},
			Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "older", CreatedAt: older},
		}}, base.SearchMessageOptions{})
		require.Equal(t, testSessionB, results[0].SessionID)
	})

	t.Run("result ordering uses session id as final tie break", func(t *testing.T) {
		results := searchResultsFromCandidates([]*searchCandidate{{
			CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 1},
			Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "a", CreatedAt: now},
		}, {
			CandidateMatch: base.CandidateMatch{SessionID: testSessionB, FusedScore: 1},
			Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "b", CreatedAt: now},
		}}, base.SearchMessageOptions{})
		require.Equal(t, testSessionA, results[0].SessionID)
	})

	t.Run("message limit preserves total match count", func(t *testing.T) {
		results := searchResultsFromCandidates([]*searchCandidate{{
			CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 2},
			Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "newer", CreatedAt: now},
		}, {
			CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 1},
			Message:        handmsg.Message{ID: 2, Role: handmsg.RoleUser, Content: "older", CreatedAt: older},
		}}, base.SearchMessageOptions{MaxMessagesPerSession: 1})
		require.Len(t, results[0].Messages, 1)
		require.Equal(t, 2, results[0].MatchCount)
	})

}

func TestCompareSearchCandidates(t *testing.T) {
	now := time.Now().UTC()
	older := now.Add(-time.Minute)

	t.Run("orders message ids descending", func(t *testing.T) {
		left := &searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 1, CreatedAt: now}}
		right := &searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 2, CreatedAt: now}}
		require.Greater(t, compareSearchCandidates(left, right), 0)
		require.Less(t, compareSearchCandidates(right, left), 0)
	})

	t.Run("covers score time session and equality ties", func(t *testing.T) {
		require.Equal(t, 0, compareSearchCandidates(
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
		))
		require.Less(t, compareSearchCandidates(
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 2}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 1}, Message: handmsg.Message{ID: 2, CreatedAt: now}},
		), 0)
		require.Greater(t, compareSearchCandidates(
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 1}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA, FusedScore: 2}, Message: handmsg.Message{ID: 2, CreatedAt: now}},
		), 0)
		require.Greater(t, compareSearchCandidates(
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 1, CreatedAt: older}},
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 2, CreatedAt: now}},
		), 0)
		require.Less(t, compareSearchCandidates(
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 2, CreatedAt: older}},
		), 0)
		require.Less(t, compareSearchCandidates(
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionB}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
		), 0)
		require.Greater(t, compareSearchCandidates(
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionB}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
			&searchCandidate{CandidateMatch: base.CandidateMatch{SessionID: testSessionA}, Message: handmsg.Message{ID: 1, CreatedAt: now}},
		), 0)
	})
}

func TestRetrievalCandidateFromSearchCandidate(t *testing.T) {
	t.Run("falls back to message content", func(t *testing.T) {
		candidate := retrievalCandidateFromSearchCandidate(&searchCandidate{
			CandidateMatch: base.CandidateMatch{SessionID: testSessionA},
			Message:        handmsg.Message{ID: 3, Role: handmsg.RoleUser, Content: "fallback text", CreatedAt: time.Now().UTC()},
		})
		require.Equal(t, "fallback text", candidate.Text)
	})
}

func TestSessionStore_SearchMessagesLexicalCandidates(t *testing.T) {
	now := time.Now().UTC()

	t.Run("session scoped lexical candidates respect limit", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "needle one", CreatedAt: now},
			{ID: 2, Role: handmsg.RoleUser, Content: "needle two", CreatedAt: now},
		}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
			{ID: 3, Role: handmsg.RoleUser, Content: "needle three", CreatedAt: now},
		}))

		candidates := store.searchMessagesLexicalCandidates(testSessionA, base.SearchMessageOptions{Query: "needle"}, "needle", 1)
		require.Len(t, candidates, 1)
		require.Contains(t, candidates, base.SourceIDForMessage(testSessionA, 2))
	})

	t.Run("cross session lexical candidates respect limit", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "needle one", CreatedAt: now},
			{ID: 2, Role: handmsg.RoleUser, Content: "needle two", CreatedAt: now},
		}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
			{ID: 3, Role: handmsg.RoleUser, Content: "needle three", CreatedAt: now},
		}))

		candidates := store.searchMessagesLexicalCandidates("", base.SearchMessageOptions{Query: "needle"}, "needle", 1)
		require.Len(t, candidates, 1)

		candidates = store.searchMessagesLexicalCandidates("", base.SearchMessageOptions{Query: "needle"}, "needle", 2)
		require.Len(t, candidates, 2)
	})

	t.Run("lexical candidates prefer newer messages", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{ID: 1, Role: handmsg.RoleUser, Content: "needle older", CreatedAt: now.Add(-time.Minute)},
			{ID: 2, Role: handmsg.RoleUser, Content: "needle newer", CreatedAt: now},
		}))

		candidates := store.searchMessagesLexicalCandidates(testSessionA, base.SearchMessageOptions{Query: "needle"}, "needle", 1)
		require.Len(t, candidates, 1)
		require.Contains(t, candidates, base.SourceIDForMessage(testSessionA, 2))
		require.NotContains(t, candidates, base.SourceIDForMessage(testSessionA, 1))
	})
}

func TestSessionStore_VectorMatchesToCandidates(t *testing.T) {
	now := time.Now().UTC()
	newStore := func(t *testing.T) *SessionStore {
		t.Helper()

		store := NewSessionStore()
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
			ID:        1,
			Role:      handmsg.RoleUser,
			Content:   "body",
			CreatedAt: now,
		}}))

		return store
	}

	t.Run("skips malformed source ids", func(t *testing.T) {
		candidates := newStore(t).vectorMatchesToCandidates(testSessionA, base.SearchMessageOptions{}, []retrieval.VectorSearchMatch{
			{Record: retrieval.VectorRecord{SourceID: "invalid"}},
		})
		require.Empty(t, candidates)
	})

	t.Run("skips missing messages", func(t *testing.T) {
		candidates := newStore(t).vectorMatchesToCandidates(testSessionA, base.SearchMessageOptions{}, []retrieval.VectorSearchMatch{{
			Record: retrieval.VectorRecord{
				ID:       retrieval.StableSessionMessageID(testSessionA, 99) + ":row:1",
				SourceID: retrieval.StableSessionMessageID(testSessionA, 99),
			},
		}})
		require.Empty(t, candidates)
	})

	t.Run("skips invalid vector row ids", func(t *testing.T) {
		candidates := newStore(t).vectorMatchesToCandidates(testSessionA, base.SearchMessageOptions{}, []retrieval.VectorSearchMatch{{
			Record: retrieval.VectorRecord{
				ID:       retrieval.StableSessionMessageID(testSessionA, 1) + ":row:99",
				SourceID: retrieval.StableSessionMessageID(testSessionA, 1),
			},
		}})
		require.Empty(t, candidates)
	})

	t.Run("deduplicates repeated message matches", func(t *testing.T) {
		candidates := newStore(t).vectorMatchesToCandidates(testSessionA, base.SearchMessageOptions{}, []retrieval.VectorSearchMatch{
			{Record: retrieval.VectorRecord{
				ID:       retrieval.StableSessionMessageID(testSessionA, 1) + ":row:1",
				SourceID: retrieval.StableSessionMessageID(testSessionA, 1),
			}},
			{Record: retrieval.VectorRecord{
				ID:       retrieval.StableSessionMessageID(testSessionA, 1) + ":row:1",
				SourceID: retrieval.StableSessionMessageID(testSessionA, 1),
			}},
		})
		require.Len(t, candidates, 1)
	})
}

func TestSessionStore_IndexVectors(t *testing.T) {
	t.Run("skips empty inputs", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       failingEmbedder{err: errors.New("should not embed")},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "model",
			Required:       true,
		}))

		require.NoError(t, store.indexVectors(context.Background(), testSessionA, []handmsg.Message{{
			Role: handmsg.RoleUser,
		}}))
		require.NoError(t, (*SessionStore)(nil).indexVectors(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "body"}}))
	})

	t.Run("returns embedder errors", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       failingEmbedder{err: errors.New("embed failed")},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "model",
			Required:       true,
		}))

		require.EqualError(t, store.indexVectors(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "body",
		}}), "embed failed")
	})

	t.Run("returns embedding validation errors", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       malformedEmbedder{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "model",
			Required:       true,
		}))

		require.EqualError(t, store.indexVectors(context.Background(), testSessionA, []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: "body",
		}}), "embedding result count must match input count")
	})
}

func TestSessionStore_DeleteVectorRows(t *testing.T) {
	t.Run("skips empty inputs", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "semantic-test",
			Required:       true,
		}))

		require.NoError(t, store.deleteVectorRows(context.Background(), nil))
		require.NoError(t, store.deleteVectorRows(context.Background(), []string{" ", ""}))
		require.NoError(t, (*SessionStore)(nil).deleteVectorRows(context.Background(), []string{"x"}))
	})
}

func TestSearchCandidateSet_Merge(t *testing.T) {
	t.Run("ignores nil candidates and adds vector only candidates", func(t *testing.T) {
		candidates := searchCandidateSet{}
		candidates.Merge([]*searchCandidate{nil, {
			CandidateMatch: base.CandidateMatch{SessionID: testSessionA},
			Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "vector"},
		}}, searchCandidateKey)
		require.Len(t, candidates, 1)
	})

	t.Run("adds vector evidence to lexical candidate", func(t *testing.T) {
		candidates := searchCandidateSet{
			base.SourceIDForMessage(testSessionA, 1): {
				CandidateMatch: base.CandidateMatch{SessionID: testSessionA},
				Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "lexical"},
			},
		}
		candidates.Merge([]*searchCandidate{{
			CandidateMatch: base.CandidateMatch{
				SessionID:       testSessionA,
				MatchedText:     "vector text",
				MatchedToolName: "tool",
				VectorRank:      1,
				HasVector:       true,
			},
			Message: handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "vector"},
		}}, searchCandidateKey)
		require.Equal(t, "vector text", candidates[base.SourceIDForMessage(testSessionA, 1)].MatchedText)
	})
}

func TestSessionStore_RerankSearchCandidates(t *testing.T) {
	t.Run("falls back when configured reranker fails", func(t *testing.T) {
		candidates := searchCandidateSet{
			base.SourceIDForMessage(testSessionA, 1): {
				CandidateMatch: base.CandidateMatch{SessionID: testSessionA},
				Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "lexical"},
			},
		}
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			Reranker:       failingReranker{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "semantic-test",
		}))

		items := store.rerankSearchCandidates(context.Background(), base.SearchMessageOptions{Query: "needle"}, candidates)
		require.Len(t, items, 1)
	})

	t.Run("defaults to deterministic and respects max candidates", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "semantic-test",
		}))
		store.vectors.Reranker = nil
		store.vectors.RerankMax = 1
		candidates := searchCandidateSet{
			base.SourceIDForMessage(testSessionA, 1): {
				CandidateMatch: base.CandidateMatch{SessionID: testSessionA},
				Message:        handmsg.Message{ID: 1, Role: handmsg.RoleUser, Content: "first"},
			},
			base.SourceIDForMessage(testSessionA, 2): {
				CandidateMatch: base.CandidateMatch{
					SessionID:  testSessionA,
					VectorRank: 2,
					HasVector:  true,
				},
				Message: handmsg.Message{ID: 2, Role: handmsg.RoleUser, Content: "second"},
			},
		}

		items := store.rerankSearchCandidates(context.Background(), base.SearchMessageOptions{Query: "needle"}, candidates)
		require.Len(t, items, 1)
	})

	t.Run("skips empty candidates", func(t *testing.T) {
		store := NewSessionStore()
		require.Nil(t, store.rerankSearchCandidates(context.Background(), base.SearchMessageOptions{}, searchCandidateSet{}))
	})

	t.Run("returns sorted items when fallback fails", func(t *testing.T) {
		store := NewSessionStore()
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "semantic-test",
		}))
		badCandidates := searchCandidateSet{
			base.SourceIDForMessage(testSessionA, 3): {
				CandidateMatch: base.CandidateMatch{
					SessionID:    testSessionA,
					LexicalScore: math.NaN(),
					LexicalRank:  1,
					HasLexical:   true,
				},
				Message: handmsg.Message{ID: 3, Role: handmsg.RoleUser, Content: "bad"},
			},
		}

		items := store.rerankSearchCandidates(context.Background(), base.SearchMessageOptions{Query: "needle"}, badCandidates)
		require.Len(t, items, 1)
	})
}

func TestSafeErrorKind(t *testing.T) {
	t.Run("classifies common errors", func(t *testing.T) {
		require.Equal(t, "", safeErrorKind(nil))
		require.Equal(t, "context_canceled", safeErrorKind(context.Canceled))
		require.Equal(t, "timeout", safeErrorKind(context.DeadlineExceeded))
		require.Equal(t, "validation_failed", safeErrorKind(errors.New("validation failed")))
		require.Equal(t, "not_found", safeErrorKind(errors.New("not found")))
		require.Equal(t, "missing_required_value", safeErrorKind(errors.New("name is required")))
		require.Equal(t, "timeout", safeErrorKind(errors.New("request timeout")))
		require.Equal(t, "operation_failed", safeErrorKind(errors.New("boom")))
	})
}

func TestSessionStore_LogSearchEvent(t *testing.T) {
	t.Run("records search option metadata", func(t *testing.T) {
		store := NewSessionStore()
		_ = store.logSearchEvent("test", testSessionA, base.SearchMessageOptions{
			IgnoreSessionID:       testSessionB,
			MaxMessagesPerSession: 2,
			MaxSessions:           3,
			Query:                 "needle",
			Role:                  handmsg.RoleUser,
			ToolName:              "search files",
		})
	})
}

func TestSessionStore_LogCandidateDiagnostics(t *testing.T) {
	t.Run("logs ranked candidates", func(t *testing.T) {
		store := NewSessionStore()
		store.logCandidateDiagnostics("ignored", nil)
		require.NoError(t, store.ConfigureVectorStore(base.VectorStoreOptions{
			Embedder:       semanticTestEmbedder{},
			VectorStore:    &memoryTestVectorStore{},
			EmbeddingModel: "semantic-test",
			Diagnostics:    true,
		}))

		store.logCandidateDiagnostics("candidate merged", []*searchCandidate{{
			CandidateMatch: base.CandidateMatch{
				SessionID:       testSessionA,
				MatchedToolName: "session_search",
				LexicalRank:     1,
				VectorRank:      2,
				HasRerank:       true,
			},
			Message: handmsg.Message{ID: 1},
		}})
	})
}

func TestLogSafeError(t *testing.T) {
	t.Run("preserves log events", func(t *testing.T) {
		store := NewSessionStore()
		require.NotNil(t, logSafeError(store.logVectorEvent("test"), nil))
		require.NotNil(t, logSafeError(store.logVectorEvent("test"), errors.New("boom")))
	})
}

type invalidNameReranker struct{}

func (invalidNameReranker) Name() string {
	return "invalid"
}

type failingReranker struct{}

func (failingReranker) Name() string {
	return retrieval.RerankerLLM
}

func (failingReranker) Rerank(
	context.Context,
	retrieval.RerankRequest,
) (retrieval.RerankResult, error) {
	return retrieval.RerankResult{}, errors.New("rerank failed")
}

func (invalidNameReranker) Rerank(
	context.Context,
	retrieval.RerankRequest,
) (retrieval.RerankResult, error) {
	return retrieval.RerankResult{}, nil
}

type failingEmbedder struct {
	err error
}

func (e failingEmbedder) Embed(
	context.Context,
	retrieval.EmbeddingRequest,
) (retrieval.EmbeddingResult, error) {
	return retrieval.EmbeddingResult{}, e.err
}

type malformedEmbedder struct{}

func (malformedEmbedder) Embed(
	context.Context,
	retrieval.EmbeddingRequest,
) (retrieval.EmbeddingResult, error) {
	return retrieval.EmbeddingResult{Model: "model", Dimensions: 2}, nil
}

type memoryTestVectorStore struct {
	upsertErr error
	deleteErr error
	searchErr error
}

func (s *memoryTestVectorStore) Upsert(context.Context, []retrieval.VectorRecord) error {
	return s.upsertErr
}

func (s *memoryTestVectorStore) Delete(context.Context, retrieval.VectorDeleteRequest) error {
	return s.deleteErr
}

func (s *memoryTestVectorStore) Search(
	context.Context,
	retrieval.VectorSearchRequest,
) (retrieval.VectorSearchResult, error) {
	return retrieval.VectorSearchResult{}, s.searchErr
}

func (s *memoryTestVectorStore) Metadata(context.Context) (retrieval.VectorStoreMetadata, error) {
	return retrieval.VectorStoreMetadata{}, nil
}
