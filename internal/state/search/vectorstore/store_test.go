package vectorstore

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRecord(t *testing.T) {
	valid := testRecord("vec-a", SourceKindSessionMessage, "msg-a", []float64{1, 0, 0})
	require.NoError(t, ValidateRecord(valid))

	tests := []struct {
		mutate func(Record) Record
		name   string
		want   string
	}{
		{name: "missing id", mutate: func(record Record) Record {
			record.ID = ""
			return record
		}, want: "vector id is required"},
		{name: "missing source kind", mutate: func(record Record) Record {
			record.SourceKind = ""
			return record
		}, want: "vector source kind is required"},
		{name: "unsupported source kind", mutate: func(record Record) Record {
			record.SourceKind = SourceKind("unknown")
			return record
		}, want: `vector source kind "unknown" is not supported`},
		{name: "missing source id", mutate: func(record Record) Record {
			record.SourceID = ""
			return record
		}, want: "vector source id is required"},
		{name: "missing model", mutate: func(record Record) Record {
			record.EmbeddingModel = ""
			return record
		}, want: "vector embedding model is required"},
		{name: "missing dimensions", mutate: func(record Record) Record {
			record.Dimensions = 0
			return record
		}, want: "vector dimensions must be greater than zero"},
		{name: "wrong vector length", mutate: func(record Record) Record {
			record.Vector = []float64{1}
			return record
		}, want: "vector length must match dimensions"},
		{name: "non finite value", mutate: func(record Record) Record {
			record.Vector = []float64{1, math.Inf(1), 0}
			return record
		}, want: "vector value must be finite"},
		{name: "missing content hash", mutate: func(record Record) Record {
			record.ContentHash = ""
			return record
		}, want: "vector content hash is required"},
		{name: "untrimmed tag", mutate: func(record Record) Record {
			record.Tags = []string{" tag "}
			return record
		}, want: "vector tag must be trimmed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRecord(tt.mutate(valid))
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestValidateSearchRequest(t *testing.T) {
	valid := SearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          10,
	}
	require.NoError(t, ValidateSearchRequest(valid))

	tests := []struct {
		mutate func(SearchRequest) SearchRequest
		name   string
		want   string
	}{
		{name: "missing model", mutate: func(req SearchRequest) SearchRequest {
			req.EmbeddingModel = ""
			return req
		}, want: "vector search embedding model is required"},
		{name: "unsupported source kind", mutate: func(req SearchRequest) SearchRequest {
			req.Filter.SourceKind = SourceKind("unknown")
			return req
		}, want: `vector search filter source kind "unknown" is not supported`},
		{name: "missing limit", mutate: func(req SearchRequest) SearchRequest {
			req.Limit = 0
			return req
		}, want: "vector search limit must be greater than zero"},
		{name: "missing dimensions", mutate: func(req SearchRequest) SearchRequest {
			req.Dimensions = 0
			return req
		}, want: "vector search dimensions must be greater than zero"},
		{name: "wrong query length", mutate: func(req SearchRequest) SearchRequest {
			req.QueryVector = []float64{1}
			return req
		}, want: "vector search query length must match dimensions"},
		{name: "non finite query value", mutate: func(req SearchRequest) SearchRequest {
			req.QueryVector = []float64{1, math.Inf(-1), 0}
			return req
		}, want: "vector search query value must be finite"},
		{name: "uppercase filter tag", mutate: func(req SearchRequest) SearchRequest {
			req.Filter.Tags = []string{"Tag"}
			return req
		}, want: "vector search filter tag must be lowercase"},
		{name: "empty filter tag group", mutate: func(req SearchRequest) SearchRequest {
			req.Filter.TagGroups = [][]string{{}}
			return req
		}, want: "vector search filter tag group is required"},
		{name: "untrimmed filter tag group tag", mutate: func(req SearchRequest) SearchRequest {
			req.Filter.TagGroups = [][]string{{" tag "}}
			return req
		}, want: "vector search filter tag group tag must be trimmed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSearchRequest(tt.mutate(valid))
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestValidateListRequest(t *testing.T) {
	require.NoError(t, ValidateListRequest(ListRequest{
		EmbeddingModel: "text-embedding-test",
		Filter: Filter{
			SourceKind: SourceKindSessionMessage,
			SourceIDs:  []string{"msg-a"},
		},
	}))

	err := ValidateListRequest(ListRequest{})
	require.EqualError(t, err, "vector list embedding model is required")

	err = ValidateListRequest(ListRequest{
		EmbeddingModel: "text-embedding-test",
		Filter:         Filter{SourceKind: SourceKind("unknown")},
	})
	require.EqualError(t, err, `vector list filter source kind "unknown" is not supported`)

	err = ValidateListRequest(ListRequest{
		EmbeddingModel: "text-embedding-test",
		Filter:         Filter{SourceIDs: []string{""}},
	})
	require.EqualError(t, err, "vector list filter source id is required")

	err = ValidateListRequest(ListRequest{
		EmbeddingModel: "text-embedding-test",
		Filter:         Filter{SourceIDs: []string{" msg-a "}},
	})
	require.EqualError(t, err, "vector list filter source id must be trimmed")

	err = ValidateListRequest(ListRequest{
		EmbeddingModel: "text-embedding-test",
		Filter:         Filter{Tags: []string{""}},
	})
	require.EqualError(t, err, "vector list filter tag is required")

	err = ValidateListRequest(ListRequest{
		EmbeddingModel: "text-embedding-test",
		Filter:         Filter{TagGroups: [][]string{{"Tag"}}},
	})
	require.EqualError(t, err, "vector list filter tag group tag must be lowercase")
}

func TestValidateDeleteRequest(t *testing.T) {
	require.NoError(t, ValidateDeleteRequest(DeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{"msg-a"},
	}))
	require.NoError(t, ValidateDeleteRequest(DeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{"msg-a", "msg-b"},
	}))

	err := ValidateDeleteRequest(DeleteRequest{})
	require.EqualError(t, err, "source kind is required")

	err = ValidateDeleteRequest(DeleteRequest{SourceKind: SourceKind("unknown")})
	require.EqualError(t, err, `source kind "unknown" is not supported`)

	err = ValidateDeleteRequest(DeleteRequest{SourceKind: SourceKindSessionMessage})
	require.EqualError(t, err, "source id is required")

	err = ValidateDeleteRequest(DeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{" msg-a "},
	})
	require.EqualError(t, err, "source id must be trimmed")

	err = ValidateDeleteRequest(DeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{""},
	})
	require.EqualError(t, err, "source id is required")
}

func TestContentHash(t *testing.T) {
	hash := ContentHash("same text")

	require.Len(t, hash, 64)
	require.Equal(t, hash, ContentHash("same text"))
	require.NotEqual(t, hash, ContentHash("other text"))
}

func TestIsRecordStale(t *testing.T) {
	hash := ContentHash("same text")

	require.False(t, IsRecordStale(Record{ContentHash: hash}, "same text"))
	require.True(t, IsRecordStale(Record{ContentHash: hash}, "other text"))
	require.True(t, IsRecordStale(Record{ContentHash: " " + hash + " "}, "same text"))
}

func TestNormalizeTags(t *testing.T) {
	require.Equal(t, []string{"alpha", "beta"}, NormalizeTags([]string{" Beta ", "", "alpha", "beta"}))
}

func TestNormalizeTagGroups(t *testing.T) {
	require.Equal(t, [][]string{
		{"alpha", "beta"},
		{"gamma"},
	}, NormalizeTagGroups([][]string{
		{" Beta ", "alpha"},
		{},
		{"gamma"},
		{"alpha", "beta"},
	}))
}

func testRecord(id string, sourceKind SourceKind, sourceID string, vector []float64) Record {
	return Record{
		ID:             id,
		SourceKind:     sourceKind,
		SourceID:       sourceID,
		EmbeddingModel: "text-embedding-test",
		Dimensions:     len(vector),
		Vector:         append([]float64(nil), vector...),
		ContentHash:    ContentHash(id),
	}
}
