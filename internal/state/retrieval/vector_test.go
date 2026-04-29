package retrieval

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateEmbeddingRequest(t *testing.T) {
	valid := EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{{
			ID:         "candidate-a",
			Text:       "hello",
			SourceKind: SourceKindSessionMessage,
		}},
	}
	require.NoError(t, ValidateEmbeddingRequest(valid))
	require.NoError(t, ValidateEmbeddingRequest(EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{{
			ID:   "query",
			Text: "hello",
		}},
	}))

	tests := []struct {
		req  EmbeddingRequest
		name string
		want string
	}{
		{name: "missing model", req: EmbeddingRequest{Inputs: valid.Inputs}, want: "embedding model is required"},
		{name: "missing inputs", req: EmbeddingRequest{Model: "model"}, want: "embedding inputs are required"},
		{
			name: "missing input id",
			req: EmbeddingRequest{
				Model: "model",
				Inputs: []EmbeddingInput{{
					Text:       "hello",
					SourceKind: SourceKindSessionMessage,
				}},
			},
			want: "embedding input id is required",
		},
		{
			name: "untrimmed input id",
			req: EmbeddingRequest{
				Model: "model",
				Inputs: []EmbeddingInput{{
					ID:   " candidate-a ",
					Text: "hello",
				}},
			},
			want: "embedding input id must be trimmed",
		},
		{
			name: "duplicate input id",
			req: EmbeddingRequest{
				Model: "model",
				Inputs: []EmbeddingInput{
					{ID: "candidate-a", Text: "hello"},
					{ID: "candidate-a", Text: "world"},
				},
			},
			want: `embedding input id "candidate-a" is duplicated`,
		},
		{
			name: "unsupported source kind",
			req: EmbeddingRequest{
				Model: "model",
				Inputs: []EmbeddingInput{{
					ID:         "candidate-a",
					Text:       "hello",
					SourceKind: SourceKind("unknown"),
				}},
			},
			want: `embedding input source kind "unknown" is not supported`,
		},
		{
			name: "missing input text",
			req: EmbeddingRequest{
				Model: "model",
				Inputs: []EmbeddingInput{{
					ID:         "candidate-a",
					SourceKind: SourceKindSessionMessage,
				}},
			},
			want: "embedding input text is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmbeddingRequest(tt.req)
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestValidateEmbeddingResult(t *testing.T) {
	req := EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{{
			ID:         "candidate-a",
			Text:       "hello",
			SourceKind: SourceKindSessionMessage,
		}},
	}
	valid := EmbeddingResult{
		Model:      "text-embedding-test",
		Dimensions: 3,
		Items: []Embedding{{
			ID:          "candidate-a",
			Vector:      []float64{1, 2, 3},
			ContentHash: VectorContentHash("hello"),
		}},
	}
	require.NoError(t, ValidateEmbeddingResult(req, valid))

	tests := []struct {
		mutate func(EmbeddingResult) EmbeddingResult
		name   string
		want   string
	}{
		{name: "missing model", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Model = ""
			return result
		}, want: "embedding result model is required"},
		{name: "wrong model", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Model = "other-model"
			return result
		}, want: "embedding result model must match request model"},
		{name: "missing dimensions", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Dimensions = 0
			return result
		}, want: "embedding result dimensions must be greater than zero"},
		{name: "wrong count", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items = nil
			return result
		}, want: "embedding result count must match input count"},
		{name: "missing id", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items[0].ID = ""
			return result
		}, want: "embedding id is required"},
		{name: "untrimmed id", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items[0].ID = " candidate-a "
			return result
		}, want: "embedding id must be trimmed"},
		{name: "unknown id", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items[0].ID = "missing"
			return result
		}, want: `embedding id "missing" is unknown`},
		{name: "wrong vector dimensions", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items[0].Vector = []float64{1}
			return result
		}, want: "embedding vector dimensions do not match result dimensions"},
		{name: "non finite vector value", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items[0].Vector = []float64{1, math.NaN(), 3}
			return result
		}, want: "embedding vector value must be finite"},
		{name: "missing content hash", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items[0].ContentHash = ""
			return result
		}, want: "embedding content hash is required"},
		{name: "wrong content hash", mutate: func(result EmbeddingResult) EmbeddingResult {
			result.Items[0].ContentHash = VectorContentHash("other")
			return result
		}, want: "embedding content hash must match input text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmbeddingResult(req, tt.mutate(cloneEmbeddingResult(valid)))
			require.EqualError(t, err, tt.want)
		})
	}

	duplicateReq := EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{
			{ID: "candidate-a", Text: "hello"},
			{ID: "candidate-b", Text: "world"},
		},
	}
	duplicateResult := EmbeddingResult{
		Model:      "text-embedding-test",
		Dimensions: 3,
		Items: []Embedding{
			{ID: "candidate-a", Vector: []float64{1, 2, 3}, ContentHash: VectorContentHash("hello")},
			{ID: "candidate-a", Vector: []float64{4, 5, 6}, ContentHash: VectorContentHash("hello")},
		},
	}
	err := ValidateEmbeddingResult(duplicateReq, duplicateResult)
	require.EqualError(t, err, `embedding id "candidate-a" is duplicated`)

	err = ValidateEmbeddingResult(EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{
			{ID: "candidate-a", Text: "hello"},
			{ID: "candidate-a", Text: "world"},
		},
	}, duplicateResult)
	require.EqualError(t, err, `embedding input id "candidate-a" is duplicated`)
}

func TestValidateVectorRecord(t *testing.T) {
	valid := testVectorRecord("vec-a", SourceKindSessionMessage, "msg-a", []float64{1, 0, 0})
	require.NoError(t, ValidateVectorRecord(valid))

	tests := []struct {
		mutate func(VectorRecord) VectorRecord
		name   string
		want   string
	}{
		{name: "missing id", mutate: func(record VectorRecord) VectorRecord {
			record.ID = ""
			return record
		}, want: "vector id is required"},
		{name: "missing source kind", mutate: func(record VectorRecord) VectorRecord {
			record.SourceKind = ""
			return record
		}, want: "vector source kind is required"},
		{name: "unsupported source kind", mutate: func(record VectorRecord) VectorRecord {
			record.SourceKind = SourceKind("unknown")
			return record
		}, want: `vector source kind "unknown" is not supported`},
		{name: "missing source id", mutate: func(record VectorRecord) VectorRecord {
			record.SourceID = ""
			return record
		}, want: "vector source id is required"},
		{name: "missing model", mutate: func(record VectorRecord) VectorRecord {
			record.EmbeddingModel = ""
			return record
		}, want: "vector embedding model is required"},
		{name: "missing dimensions", mutate: func(record VectorRecord) VectorRecord {
			record.Dimensions = 0
			return record
		}, want: "vector dimensions must be greater than zero"},
		{name: "wrong vector length", mutate: func(record VectorRecord) VectorRecord {
			record.Vector = []float64{1}
			return record
		}, want: "vector length must match dimensions"},
		{name: "non finite value", mutate: func(record VectorRecord) VectorRecord {
			record.Vector = []float64{1, math.Inf(1), 0}
			return record
		}, want: "vector value must be finite"},
		{name: "missing content hash", mutate: func(record VectorRecord) VectorRecord {
			record.ContentHash = ""
			return record
		}, want: "vector content hash is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVectorRecord(tt.mutate(valid))
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestValidateVectorSearchRequest(t *testing.T) {
	valid := VectorSearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    []float64{1, 0, 0},
		Limit:          10,
	}
	require.NoError(t, ValidateVectorSearchRequest(valid))

	tests := []struct {
		mutate func(VectorSearchRequest) VectorSearchRequest
		name   string
		want   string
	}{
		{name: "missing model", mutate: func(req VectorSearchRequest) VectorSearchRequest {
			req.EmbeddingModel = ""
			return req
		}, want: "vector search embedding model is required"},
		{name: "unsupported source kind", mutate: func(req VectorSearchRequest) VectorSearchRequest {
			req.Filter.SourceKind = SourceKind("unknown")
			return req
		}, want: `vector search filter source kind "unknown" is not supported`},
		{name: "missing limit", mutate: func(req VectorSearchRequest) VectorSearchRequest {
			req.Limit = 0
			return req
		}, want: "vector search limit must be greater than zero"},
		{name: "missing dimensions", mutate: func(req VectorSearchRequest) VectorSearchRequest {
			req.Dimensions = 0
			return req
		}, want: "vector search dimensions must be greater than zero"},
		{name: "wrong query length", mutate: func(req VectorSearchRequest) VectorSearchRequest {
			req.QueryVector = []float64{1}
			return req
		}, want: "vector search query length must match dimensions"},
		{name: "non finite query value", mutate: func(req VectorSearchRequest) VectorSearchRequest {
			req.QueryVector = []float64{1, math.Inf(-1), 0}
			return req
		}, want: "vector search query value must be finite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVectorSearchRequest(tt.mutate(valid))
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestValidateVectorDeleteRequest(t *testing.T) {
	require.NoError(t, ValidateVectorDeleteRequest(VectorDeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{"msg-a"},
	}))
	require.NoError(t, ValidateVectorDeleteRequest(VectorDeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{"msg-a", "msg-b"},
	}))

	err := ValidateVectorDeleteRequest(VectorDeleteRequest{})
	require.EqualError(t, err, "source kind is required")

	err = ValidateVectorDeleteRequest(VectorDeleteRequest{SourceKind: SourceKind("unknown")})
	require.EqualError(t, err, `source kind "unknown" is not supported`)

	err = ValidateVectorDeleteRequest(VectorDeleteRequest{SourceKind: SourceKindSessionMessage})
	require.EqualError(t, err, "source id is required")

	err = ValidateVectorDeleteRequest(VectorDeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{" msg-a "},
	})
	require.EqualError(t, err, "source id must be trimmed")

	err = ValidateVectorDeleteRequest(VectorDeleteRequest{
		SourceKind: SourceKindSessionMessage,
		SourceIDs:  []string{""},
	})
	require.EqualError(t, err, "source id is required")
}

func TestVectorContentHashAndStaleDetection(t *testing.T) {
	hash := VectorContentHash("same text")

	require.Len(t, hash, 64)
	require.Equal(t, hash, VectorContentHash("same text"))
	require.NotEqual(t, hash, VectorContentHash("other text"))
	require.False(t, IsVectorRecordStale(VectorRecord{ContentHash: hash}, "same text"))
	require.True(t, IsVectorRecordStale(VectorRecord{ContentHash: hash}, "other text"))
	require.True(t, IsVectorRecordStale(VectorRecord{ContentHash: " " + hash + " "}, "same text"))
}

func TestFakeEmbeddingProviderEmbedsDeterministically(t *testing.T) {
	provider := fakeEmbeddingProvider{dimensions: 4}
	req := EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{
			{ID: "one", Text: "alpha", SourceKind: SourceKindSessionMessage},
			{ID: "two", Text: "alpha", SourceKind: SourceKindMemoryItem},
		},
	}

	result, err := provider.Embed(context.Background(), req)

	require.NoError(t, err)
	require.NoError(t, ValidateEmbeddingResult(req, result))
	require.Equal(t, "text-embedding-test", result.Model)
	require.Equal(t, 4, result.Dimensions)
	require.Equal(t, result.Items[0].Vector, result.Items[1].Vector)
	require.Equal(t, VectorContentHash("alpha"), result.Items[0].ContentHash)
}

func TestFakeEmbeddingProviderRejectsInvalidRequests(t *testing.T) {
	provider := fakeEmbeddingProvider{dimensions: 3}

	_, err := provider.Embed(context.Background(), EmbeddingRequest{})
	require.EqualError(t, err, "embedding model is required")

	_, err = (fakeEmbeddingProvider{}).Embed(context.Background(), EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []EmbeddingInput{{
			ID:         "one",
			Text:       "alpha",
			SourceKind: SourceKindSessionMessage,
		}},
	})
	require.EqualError(t, err, "embedding dimensions must be greater than zero")
}
