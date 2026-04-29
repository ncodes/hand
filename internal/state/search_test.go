package state

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state/retrieval"
)

func TestSessionIDAndMessageOrderHelpers(t *testing.T) {
	sessionID, err := NewSessionID()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(sessionID, SessionIDPrefix))

	archiveID, err := NewArchiveID()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(archiveID, ArchiveIDPrefix))

	order, err := NormalizeMessageQueryOrder("")
	require.NoError(t, err)
	require.Equal(t, MessageOrderAsc, order)

	order, err = NormalizeMessageQueryOrder(" DESC ")
	require.NoError(t, err)
	require.Equal(t, MessageOrderDesc, order)

	_, err = NormalizeMessageQueryOrder("sideways")
	require.EqualError(t, err, "message order must be asc or desc")
}

func TestMessageIndexRowsFromMessage(t *testing.T) {
	now := time.Now().UTC()

	require.Nil(t, MessageIndexRowsFromMessage("ses_a", handmsg.Message{Role: handmsg.RoleUser}))
	require.Nil(t, MessageIndexRowsFromMessage("ses_a", handmsg.Message{Role: handmsg.RoleTool, Name: "process"}))
	require.Nil(t, MessageIndexRowsFromMessage("ses_a", handmsg.Message{
		Role:      handmsg.RoleAssistant,
		ToolCalls: []handmsg.ToolCall{{}},
	}))

	rows := MessageIndexRowsFromMessage("ses_a", handmsg.Message{
		ID:        3,
		Role:      handmsg.RoleUser,
		Content:   "user body",
		CreatedAt: now,
	})
	require.Len(t, rows, 1)
	require.Equal(t, "user body", rows[0].Body)

	rows = MessageIndexRowsFromMessage(" ses_a ", handmsg.Message{
		ID:        1,
		Role:      handmsg.RoleAssistant,
		Content:   "assistant body",
		CreatedAt: now,
		ToolCalls: []handmsg.ToolCall{{
			ID:    "call-1",
			Name:  "Search Files",
			Input: `{"pattern":"needle"}`,
		}},
	})
	require.Len(t, rows, 2)
	require.Equal(t, "ses_a", rows[0].SessionID)
	require.Equal(t, "assistant body", rows[0].Body)
	require.Equal(t, "search files", rows[1].ToolName)

	rows = MessageIndexRowsFromMessage("ses_a", handmsg.Message{
		ID:      2,
		Role:    handmsg.RoleTool,
		Name:    "Plan Tool",
		Content: "tool body",
	})
	require.Len(t, rows, 1)
	require.Equal(t, "plan tool", rows[0].ToolName)
}

func TestVectorInputAndSourceHelpers(t *testing.T) {
	now := time.Now().UTC()
	rows := []MessageIndexRow{{
		CreatedAt: now,
		UpdatedAt: now,
		MessageID: 1,
		SessionID: "ses_a",
		Role:      string(handmsg.RoleUser),
		Body:      "first",
	}, {
		CreatedAt: now,
		UpdatedAt: now,
		MessageID: 1,
		SessionID: "ses_a",
		Role:      string(handmsg.RoleUser),
		ToolName:  "process",
		Body:      "second",
	}}

	inputs := VectorInputsFromIndexRows(rows)
	require.Len(t, inputs, 2)
	require.Equal(t, SourceIDForMessage("ses_a", 1)+":row:1", inputs[0].ID)
	require.Equal(t, SourceIDForMessage("ses_a", 1)+":row:2", inputs[1].ID)
	require.Equal(t, "process", inputs[1].ToolName)
	require.Nil(t, VectorInputsFromIndexRows(nil))

	row, ok := MessageIndexRowForVectorRecord(rows, inputs[1].ID)
	require.True(t, ok)
	require.Equal(t, "second", row.Body)
	_, ok = MessageIndexRowForVectorRecord(rows, SourceIDForMessage("ses_a", 1))
	require.False(t, ok)
	_, ok = MessageIndexRowForVectorRecord(rows, SourceIDForMessage("ses_a", 1)+":row:3")
	require.False(t, ok)
	_, ok = MessageIndexRowForVectorRecord(nil, inputs[0].ID)
	require.False(t, ok)

	sessionID, messageID, ok := MessageRefFromSourceID(SourceIDForMessage("ses_a", 2))
	require.True(t, ok)
	require.Equal(t, "ses_a", sessionID)
	require.Equal(t, uint(2), messageID)
	_, _, ok = MessageRefFromSourceID("bad")
	require.False(t, ok)
	_, _, ok = MessageRefFromSourceID(string(retrieval.SourceKindSessionMessage) + ":ses_a:")
	require.False(t, ok)
	_, _, ok = MessageRefFromSourceID(SourceIDForMessage("ses_a", 0))
	require.False(t, ok)

	require.Equal(t, []string{SourceIDForMessage("ses_a", 1)}, SourceIDsFromMessages("ses_a", []handmsg.Message{{ID: 1}}))
	require.Nil(t, SourceIDsFromMessages("ses_a", nil))
}

func TestSearchSharedRankingAndFilters(t *testing.T) {
	require.Equal(t, []string{"one", "two"}, UniqueStrings([]string{" one ", "", "two", "one"}))
	require.Nil(t, UniqueStrings(nil))
	require.Equal(t, "search files", NormalizeMatchValue(" Search   Files "))

	require.False(t, MessageIndexRowMatchesSearchOptions(MessageIndexRow{ToolName: "process"}, SearchMessageOptions{ToolName: "search_files"}))
	require.True(t, MessageIndexRowMatchesSearchOptions(MessageIndexRow{ToolName: "process"}, SearchMessageOptions{ToolName: " process "}))

	require.Equal(t, DefaultHybridRetrievalCandidateLimit, HybridRetrievalCandidateLimit(SearchMessageOptions{}))
	require.Equal(t, 120, HybridRetrievalCandidateLimit(SearchMessageOptions{
		MaxSessions:           12,
		MaxMessagesPerSession: 10,
	}))
	require.Equal(t, MaxHybridRetrievalCandidateLimit, HybridRetrievalCandidateLimit(SearchMessageOptions{
		MaxSessions:           MaxHybridRetrievalCandidateLimit,
		MaxMessagesPerSession: MaxHybridRetrievalCandidateLimit,
	}))

	require.Equal(t, float64(0), FusedCandidateScore(false, 0, false, 0))
	require.Greater(t, FusedCandidateScore(true, 1, true, 2), float64(0))
	require.Equal(t, 9.0, CandidateRankingScore(true, 9, 1))
	require.Equal(t, 1.0, CandidateRankingScore(false, 9, 1))

	now := time.Now().UTC()
	older := now.Add(-time.Minute)
	require.Equal(t, -1, CompareCandidateOrder(2, 1, now, now, "a", "a", 1, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 2, now, now, "a", "a", 1, 1))
	require.Equal(t, -1, CompareCandidateOrder(1, 1, now, older, "a", "a", 1, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 1, older, now, "a", "a", 1, 1))
	require.Equal(t, -1, CompareCandidateOrder(1, 1, now, now, "a", "b", 1, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 1, now, now, "b", "a", 1, 1))
	require.Equal(t, -1, CompareCandidateOrder(1, 1, now, now, "a", "a", 2, 1))
	require.Equal(t, 1, CompareCandidateOrder(1, 1, now, now, "a", "a", 1, 2))
	require.Equal(t, 0, CompareCandidateOrder(1, 1, now, now, "a", "a", 1, 1))
}

func TestVectorRepairResult_Add(t *testing.T) {
	var nilResult *VectorRepairResult
	nilResult.Add(VectorRepairResult{SessionsScanned: 1})

	result := VectorRepairResult{
		SessionsScanned: 1,
		MessagesScanned: 2,
		RowsScanned:     3,
		MissingRows:     4,
		StaleRows:       5,
		UnchangedRows:   6,
		RebuiltRows:     7,
		DeletedSources:  8,
		Batches:         9,
	}
	result.Add(VectorRepairResult{
		SessionsScanned: 10,
		MessagesScanned: 20,
		RowsScanned:     30,
		MissingRows:     40,
		StaleRows:       50,
		UnchangedRows:   60,
		RebuiltRows:     70,
		DeletedSources:  80,
		Batches:         90,
	})

	require.Equal(t, VectorRepairResult{
		SessionsScanned: 11,
		MessagesScanned: 22,
		RowsScanned:     33,
		MissingRows:     44,
		StaleRows:       55,
		UnchangedRows:   66,
		RebuiltRows:     77,
		DeletedSources:  88,
		Batches:         99,
	}, result)
}

func TestSearchCandidateSet_MergeAndSorted(t *testing.T) {
	candidates := SearchCandidateSet[string, *testSearchCandidate]{
		"lexical": {
			match: &CandidateMatch{
				SessionID:   "ses_a",
				LexicalRank: 1,
				HasLexical:  true,
			},
			id: "lexical",
		},
		"empty": {id: "empty"},
		"keep": {
			id: "keep",
			match: &CandidateMatch{
				MatchedText: "lexical text",
			},
		},
	}

	candidates.Merge([]*testSearchCandidate{
		nil,
		{id: "ignored"},
		{
			id: "lexical",
			match: &CandidateMatch{
				MatchedText:     "vector text",
				MatchedToolName: "tool",
				VectorScore:     0.7,
				VectorRank:      2,
			},
		},
		{
			id: "vector",
			match: &CandidateMatch{
				SessionID:  "ses_b",
				VectorRank: 1,
				HasVector:  true,
			},
		},
		{
			id: "empty",
			match: &CandidateMatch{
				VectorRank: 3,
			},
		},
		{
			id: "keep",
			match: &CandidateMatch{
				MatchedText: "vector replacement",
				VectorRank:  4,
			},
		},
	}, func(candidate *testSearchCandidate) string {
		if candidate == nil {
			return ""
		}
		return candidate.id
	})

	require.Len(t, candidates, 4)
	require.True(t, candidates["lexical"].match.HasVector)
	require.Equal(t, 0.7, candidates["lexical"].match.VectorScore)
	require.Equal(t, "vector text", candidates["lexical"].match.MatchedText)
	require.Equal(t, "lexical text", candidates["keep"].match.MatchedText)
	require.Contains(t, candidates, "vector")
	require.NotContains(t, candidates, "ignored")

	items := candidates.Sorted(func(left *testSearchCandidate, right *testSearchCandidate) bool {
		return left.id < right.id
	})
	require.Len(t, items, 3)
	require.Equal(t, "keep", items[0].id)
	require.Greater(t, candidates["lexical"].match.FusedScore, 0.0)
	require.Greater(t, candidates["vector"].match.FusedScore, 0.0)
}

func TestVectorRecordLister(t *testing.T) {
	lister, err := VectorRecordLister(&testVectorStore{})
	require.NoError(t, err)
	require.NotNil(t, lister)

	lister, err = VectorRecordLister(testVectorStoreWithoutList{})
	require.Nil(t, lister)
	require.EqualError(t, err, "vector store record listing is required")
}

func TestDirtyVectorSources(t *testing.T) {
	dirtySources, result, err := DirtyVectorSources(context.Background(), nil, "model", nil, false)
	require.Nil(t, dirtySources)
	require.Equal(t, VectorRepairResult{}, result)
	require.EqualError(t, err, "vector store record listing is required")

	store := &testVectorStore{}
	dirtySources, result, err = DirtyVectorSources(context.Background(), store, "model", nil, false)
	require.NoError(t, err)
	require.Nil(t, dirtySources)
	require.Equal(t, VectorRepairResult{}, result)

	store.listErr = errors.New("list failed")
	input := VectorInput{ID: "vec-1", SourceID: "source-1", Text: "one"}
	dirtySources, result, err = DirtyVectorSources(context.Background(), store, " model ", []VectorInput{input}, false)
	require.Nil(t, dirtySources)
	require.Equal(t, VectorRepairResult{}, result)
	require.EqualError(t, err, "list failed")
	require.Equal(t, "model", store.lastListReq.EmbeddingModel)

	store.listErr = nil
	store.records = []retrieval.VectorRecord{
		{ID: "vec-1", SourceID: "source-1", ContentHash: retrieval.VectorContentHash("one")},
		{ID: "vec-2", SourceID: "source-2", ContentHash: retrieval.VectorContentHash("old")},
		{ID: "extra", SourceID: "source-extra", ContentHash: retrieval.VectorContentHash("extra")},
	}
	inputs := []VectorInput{
		{ID: "vec-1", SourceID: "source-1", Text: "one"},
		{ID: "vec-2", SourceID: "source-2", Text: "two"},
		{ID: "missing", SourceID: "source-missing", Text: "missing"},
	}
	dirtySources, result, err = DirtyVectorSources(context.Background(), store, "model", inputs, false)
	require.NoError(t, err)
	require.Equal(t, []string{"source-2", "source-extra", "source-missing"}, dirtySources)
	require.Equal(t, VectorRepairResult{
		MissingRows:   1,
		StaleRows:     2,
		UnchangedRows: 1,
	}, result)
	require.Equal(t, retrieval.SourceKindSessionMessage, store.lastListReq.Filter.SourceKind)
	require.Equal(t, []string{"source-1", "source-2", "source-missing"}, store.lastListReq.Filter.SourceIDs)

	dirtySources, result, err = DirtyVectorSources(context.Background(), store, "model", inputs[:1], true)
	require.NoError(t, err)
	require.Equal(t, []string{"source-1", "source-2", "source-extra"}, dirtySources)
	require.Equal(t, VectorRepairResult{
		StaleRows:     2,
		UnchangedRows: 1,
	}, result)
}

func TestMessagesBySourceID(t *testing.T) {
	messages := []handmsg.Message{
		{ID: 1, Content: "one"},
		{ID: 2, Content: "two"},
	}

	require.Nil(t, MessagesBySourceID("ses_a", messages, nil))
	require.Nil(t, MessagesBySourceID("ses_a", messages, []string{" "}))
	require.Equal(t, []handmsg.Message{{ID: 2, Content: "two"}}, MessagesBySourceID(
		"ses_a",
		messages,
		[]string{SourceIDForMessage("ses_a", 2)},
	))
}

type testSearchCandidate struct {
	match *CandidateMatch
	id    string
}

func (c *testSearchCandidate) CandidateMatchRef() *CandidateMatch {
	if c == nil {
		return nil
	}
	return c.match
}

type testVectorStore struct {
	lastListReq retrieval.VectorListRequest
	records     []retrieval.VectorRecord
	listErr     error
}

func (s *testVectorStore) Upsert(context.Context, []retrieval.VectorRecord) error {
	return nil
}

func (s *testVectorStore) Delete(context.Context, retrieval.VectorDeleteRequest) error {
	return nil
}

func (s *testVectorStore) Search(
	context.Context,
	retrieval.VectorSearchRequest,
) (retrieval.VectorSearchResult, error) {
	return retrieval.VectorSearchResult{}, nil
}

func (s *testVectorStore) Metadata(context.Context) (retrieval.VectorStoreMetadata, error) {
	return retrieval.VectorStoreMetadata{}, nil
}

func (s *testVectorStore) List(
	_ context.Context,
	req retrieval.VectorListRequest,
) (retrieval.VectorListResult, error) {
	s.lastListReq = req
	if s.listErr != nil {
		return retrieval.VectorListResult{}, s.listErr
	}
	return retrieval.VectorListResult{Records: s.records}, nil
}

type testVectorStoreWithoutList struct{}

func (testVectorStoreWithoutList) Upsert(context.Context, []retrieval.VectorRecord) error {
	return nil
}

func (testVectorStoreWithoutList) Delete(context.Context, retrieval.VectorDeleteRequest) error {
	return nil
}

func (testVectorStoreWithoutList) Search(
	context.Context,
	retrieval.VectorSearchRequest,
) (retrieval.VectorSearchResult, error) {
	return retrieval.VectorSearchResult{}, nil
}

func (testVectorStoreWithoutList) Metadata(context.Context) (retrieval.VectorStoreMetadata, error) {
	return retrieval.VectorStoreMetadata{}, nil
}
