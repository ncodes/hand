package vector

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state/retrieval"
)

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
