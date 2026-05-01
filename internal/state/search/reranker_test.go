package search

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeReranker struct {
	result RerankResult
	err    error
	name   string
}

func (f fakeReranker) Name() string {
	if f.name != "" {
		return f.name
	}

	return RerankerDeterministic
}

func (f fakeReranker) Rerank(context.Context, RerankRequest) (RerankResult, error) {
	if f.err != nil {
		return RerankResult{}, f.err
	}

	return f.result, nil
}

func TestRerankWithFallback(t *testing.T) {
	candidates := []Candidate{
		testSessionCandidate("candidate-a", 0.8, 0.4, 0.8, time.Time{}),
		testSessionCandidate("candidate-b", 0.2, 0.9, 0.2, time.Time{}),
	}

	t.Run("uses primary valid result", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			result: RerankResult{Items: []RerankItem{{CandidateID: "candidate-b", Score: 7}}},
		}, NoopReranker{}, RerankRequest{Candidates: candidates})

		require.NoError(t, err)
		require.Equal(t, RerankerDeterministic, result.Reranker)
		require.Equal(t, []RerankItem{{CandidateID: "candidate-b", Score: 7}}, result.Items)
	})

	t.Run("falls back when primary errors", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			err: context.DeadlineExceeded,
		}, NoopReranker{}, RerankRequest{Candidates: candidates})

		require.NoError(t, err)
		require.Equal(t, RerankerNoop, result.Reranker)
		require.Equal(t, []string{"candidate-a", "candidate-b"}, rerankIDs(result))
	})

	t.Run("falls back when primary returns malformed result", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			result: RerankResult{Items: []RerankItem{{CandidateID: "missing", Score: 1}}},
		}, NoopReranker{}, RerankRequest{Candidates: candidates})

		require.NoError(t, err)
		require.Equal(t, RerankerNoop, result.Reranker)
		require.Equal(t, []string{"candidate-a", "candidate-b"}, rerankIDs(result))
	})

	t.Run("uses deterministic fallback when fallback is nil", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			err: errors.New("failed"),
		}, nil, RerankRequest{Candidates: candidates})

		require.NoError(t, err)
		require.Equal(t, RerankerDeterministic, result.Reranker)
		require.Equal(t, []string{"candidate-a", "candidate-b"}, rerankIDs(result))
	})

	t.Run("nil primary uses fallback", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), nil, NoopReranker{}, RerankRequest{
			Candidates: candidates,
			Options:    RerankOptions{MaxCandidates: 1},
		})

		require.NoError(t, err)
		require.Equal(t, RerankerNoop, result.Reranker)
		require.Equal(t, []string{"candidate-a"}, rerankIDs(result))
	})

	t.Run("rejects invalid primary reranker", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{name: "test"}, NoopReranker{}, RerankRequest{
			Candidates: candidates,
		})

		require.EqualError(t, err, "reranker must be one of: noop, deterministic, llm")
		require.Empty(t, result.Items)
	})

	t.Run("rejects invalid fallback reranker", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{}, fakeReranker{name: "test"}, RerankRequest{
			Candidates: candidates,
		})

		require.EqualError(t, err, "reranker must be one of: noop, deterministic, llm")
		require.Empty(t, result.Items)
	})

	t.Run("nil primary rejects invalid fallback reranker", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), nil, fakeReranker{name: "test"}, RerankRequest{
			Candidates: candidates,
		})

		require.EqualError(t, err, "reranker must be one of: noop, deterministic, llm")
		require.Empty(t, result.Items)
	})
}

func TestRerankerNames(t *testing.T) {
	require.Equal(t, RerankerLLM, LLMReranker{}.Name())
}

func TestValidateReranker(t *testing.T) {
	require.NoError(t, ValidateReranker(nil))
	require.NoError(t, ValidateReranker(NoopReranker{}))
	require.NoError(t, ValidateReranker(DeterministicReranker{}))
	require.NoError(t, ValidateReranker(LLMReranker{}))
	require.EqualError(t, ValidateReranker(fakeReranker{name: "test"}), "reranker must be one of: noop, deterministic, llm")
}

func TestValidateRerankResult(t *testing.T) {
	candidates := []Candidate{
		testSessionCandidate("candidate-a", 0, 0, 0, time.Time{}),
	}

	require.NoError(t, ValidateRerankResult(candidates, RerankResult{
		Items: []RerankItem{{CandidateID: "candidate-a", Score: 1}},
	}))

	tests := []struct {
		result     RerankResult
		candidates []Candidate
		name       string
		want       string
	}{
		{
			name:       "items for empty candidates",
			candidates: nil,
			result:     RerankResult{Items: []RerankItem{{CandidateID: "candidate-a"}}},
			want:       "rerank result must be empty when candidates are empty",
		},
		{
			name:       "empty result",
			candidates: candidates,
			result:     RerankResult{},
			want:       "rerank result is empty",
		},
		{
			name:       "too many items",
			candidates: candidates,
			result: RerankResult{Items: []RerankItem{
				{CandidateID: "candidate-a"},
				{CandidateID: "candidate-a"},
			}},
			want: "rerank result cannot contain more items than candidates",
		},
		{
			name:       "missing candidate id",
			candidates: candidates,
			result:     RerankResult{Items: []RerankItem{{CandidateID: "   "}}},
			want:       "rerank item candidate id is required",
		},
		{
			name:       "untrimmed candidate id",
			candidates: candidates,
			result:     RerankResult{Items: []RerankItem{{CandidateID: " candidate-a "}}},
			want:       "rerank item candidate id must be trimmed",
		},
		{
			name:       "non-finite score",
			candidates: candidates,
			result:     RerankResult{Items: []RerankItem{{CandidateID: "candidate-a", Score: math.NaN()}}},
			want:       "rerank item score must be finite",
		},
		{
			name:       "unknown id",
			candidates: candidates,
			result:     RerankResult{Items: []RerankItem{{CandidateID: "candidate-missing"}}},
			want:       `rerank item candidate id "candidate-missing" is unknown`,
		},
		{
			name: "duplicated id",
			candidates: append(candidates,
				testSessionCandidate("candidate-b", 0, 0, 0, time.Time{}),
			),
			result: RerankResult{Items: []RerankItem{
				{CandidateID: "candidate-a"},
				{CandidateID: "candidate-a"},
			}},
			want: `rerank item candidate id "candidate-a" is duplicated`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRerankResult(tt.candidates, tt.result)
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestRerankerDefensiveBranches(t *testing.T) {
	candidates := []Candidate{testSessionCandidate("candidate-a", 0, 0, 0, time.Time{})}

	t.Run("RerankWithFallback with fallback and negative MaxCandidates", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			result: RerankResult{Items: []RerankItem{{CandidateID: "candidate-a"}}},
		}, NoopReranker{}, RerankRequest{
			Candidates: candidates,
			Options:    RerankOptions{MaxCandidates: -1},
		})
		require.EqualError(t, err, "max candidates must be greater than or equal to zero")
		require.Empty(t, result.Items)
	})

	t.Run("ValidateRerankResult allows empty input and result", func(t *testing.T) {
		require.NoError(t, ValidateRerankResult(nil, RerankResult{}))
	})
}

func testSessionCandidate(id string, lexicalScore float64, vectorScore float64, fusedScore float64, updatedAt time.Time) Candidate {
	return Candidate{
		ID:           id,
		SourceKind:   SourceKindSessionMessage,
		SessionID:    "ses_test",
		MessageID:    1,
		Text:         "session text",
		LexicalScore: lexicalScore,
		VectorScore:  vectorScore,
		FusedScore:   fusedScore,
		CreatedAt:    updatedAt.Add(-time.Minute),
		UpdatedAt:    updatedAt,
	}
}

func testMemoryCandidate(id string, lexicalScore float64, vectorScore float64, fusedScore float64, updatedAt time.Time) Candidate {
	return Candidate{
		ID:           id,
		SourceKind:   SourceKindMemoryItem,
		MemoryID:     id,
		Text:         "memory text",
		LexicalScore: lexicalScore,
		VectorScore:  vectorScore,
		FusedScore:   fusedScore,
		CreatedAt:    updatedAt.Add(-time.Minute),
		UpdatedAt:    updatedAt,
	}
}

func rerankIDs(result RerankResult) []string {
	ids := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		ids = append(ids, item.CandidateID)
	}

	return ids
}
