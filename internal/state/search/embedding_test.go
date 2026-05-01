package search

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
