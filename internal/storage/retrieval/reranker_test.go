package retrieval

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
}

func (f fakeReranker) Rerank(context.Context, RerankRequest) (RerankResult, error) {
	if f.err != nil {
		return RerankResult{}, f.err
	}

	return f.result, nil
}

func TestNoopRerankerPreservesOrderAndBoundsCandidates(t *testing.T) {
	candidates := []Candidate{
		testSessionCandidate("candidate-b", 0, 0, 0.2, time.Time{}),
		testSessionCandidate("candidate-a", 0, 0, 0.9, time.Time{}),
		testMemoryCandidate("candidate-c", 0, 0, 0.4, time.Time{}),
	}

	result, err := NoopReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options:    RerankOptions{MaxCandidates: 2},
	})

	require.NoError(t, err)
	require.Equal(t, []RerankItem{
		{CandidateID: "candidate-b", Score: 0.2},
		{CandidateID: "candidate-a", Score: 0.9},
	}, result.Items)
}

func TestDeterministicRerankerCombinesScoresAndRecency(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		testSessionCandidate("candidate-older", 0.2, 0.3, 0.1, now),
		testSessionCandidate("candidate-best", 0.9, 0.8, 0.7, now.Add(time.Second)),
		testMemoryCandidate("candidate-vector", 0.3, 1.0, 0.5, now.Add(2*time.Second)),
	}

	result, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options: RerankOptions{
			LexicalWeight: 0.5,
			VectorWeight:  0.3,
			FusedWeight:   0.2,
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"candidate-best", "candidate-vector", "candidate-older"}, rerankIDs(result))
	require.Greater(t, result.Items[0].Score, result.Items[1].Score)
	require.Greater(t, result.Items[1].Score, result.Items[2].Score)
}

func TestDeterministicRerankerUsesDefaultWeightsAndIDTieBreak(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		testSessionCandidate("candidate-b", 0.5, 0.5, 0.5, now),
		testSessionCandidate("candidate-a", 0.5, 0.5, 0.5, now),
	}

	result, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
	})

	require.NoError(t, err)
	require.Equal(t, []string{"candidate-a", "candidate-b"}, rerankIDs(result))
	require.Equal(t, result.Items[0].Score, result.Items[1].Score)
}

func TestDeterministicRerankerSupportsMemoryCandidatesAndRecencyOnlyWeights(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		testMemoryCandidate("memory-old", 0, 0, 0, now),
		testMemoryCandidate("memory-new", 0, 0, 0, now.Add(time.Hour)),
	}

	result, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Query:      "memory",
		Caller:     "memory_search",
		TraceID:    "trace-1",
		SourceKind: SourceKindMemoryItem,
		Candidates: candidates,
		Options:    RerankOptions{RecencyWeight: 1},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"memory-new", "memory-old"}, rerankIDs(result))
}

func TestDeterministicRerankerSupportsLowerIsBetterLexicalScores(t *testing.T) {
	candidates := []Candidate{
		testSessionCandidate("candidate-bm25-best", -3, 0, 0, time.Time{}),
		testSessionCandidate("candidate-bm25-weak", -1, 0, 0, time.Time{}),
	}

	result, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options: RerankOptions{
			LexicalDirection: ScoreLowerIsBetter,
			LexicalWeight:    1,
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"candidate-bm25-best", "candidate-bm25-weak"}, rerankIDs(result))
	require.Greater(t, result.Items[0].Score, result.Items[1].Score)
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
		require.Equal(t, []RerankItem{{CandidateID: "candidate-b", Score: 7}}, result.Items)
	})

	t.Run("falls back when primary errors", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			err: context.DeadlineExceeded,
		}, NoopReranker{}, RerankRequest{Candidates: candidates})

		require.NoError(t, err)
		require.Equal(t, []string{"candidate-a", "candidate-b"}, rerankIDs(result))
	})

	t.Run("falls back when primary returns malformed result", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			result: RerankResult{Items: []RerankItem{{CandidateID: "missing", Score: 1}}},
		}, NoopReranker{}, RerankRequest{Candidates: candidates})

		require.NoError(t, err)
		require.Equal(t, []string{"candidate-a", "candidate-b"}, rerankIDs(result))
	})

	t.Run("uses deterministic fallback when fallback is nil", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), fakeReranker{
			err: errors.New("failed"),
		}, nil, RerankRequest{Candidates: candidates})

		require.NoError(t, err)
		require.Equal(t, []string{"candidate-a", "candidate-b"}, rerankIDs(result))
	})

	t.Run("nil primary uses fallback", func(t *testing.T) {
		result, err := RerankWithFallback(context.Background(), nil, NoopReranker{}, RerankRequest{
			Candidates: candidates,
			Options:    RerankOptions{MaxCandidates: 1},
		})

		require.NoError(t, err)
		require.Equal(t, []string{"candidate-a"}, rerankIDs(result))
	})
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

func TestRerankerValidationErrors(t *testing.T) {
	candidates := []Candidate{testSessionCandidate("candidate-a", 0, 0, 0, time.Time{})}

	_, err := NoopReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options:    RerankOptions{MaxCandidates: -1},
	})
	require.EqualError(t, err, "max candidates must be greater than or equal to zero")

	invalid := candidates[0]
	invalid.Text = ""
	_, err = DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: []Candidate{invalid},
	})
	require.EqualError(t, err, "candidate text is required")

	_, err = DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options:    RerankOptions{LexicalDirection: ScoreDirection(99)},
	})
	require.EqualError(t, err, "score direction is not supported")

	_, err = DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options:    RerankOptions{VectorDirection: ScoreDirection(99)},
	})
	require.EqualError(t, err, "score direction is not supported")

	_, err = DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options:    RerankOptions{FusedDirection: ScoreDirection(99)},
	})
	require.EqualError(t, err, "score direction is not supported")

	empty, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{})
	require.NoError(t, err)
	require.Empty(t, empty.Items)
}

func TestRerankerDefensiveBranches(t *testing.T) {
	candidates := []Candidate{testSessionCandidate("candidate-a", 0, 0, 0, time.Time{})}

	t.Run("NoopReranker rejects invalid candidate", func(t *testing.T) {
		invalid := candidates[0]
		invalid.Text = ""
		_, err := NoopReranker{}.Rerank(context.Background(), RerankRequest{
			Candidates: []Candidate{invalid},
		})
		require.EqualError(t, err, "candidate text is required")
	})

	t.Run("DeterministicReranker with negative MaxCandidates", func(t *testing.T) {
		_, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
			Candidates: candidates,
			Options:    RerankOptions{MaxCandidates: -1},
		})
		require.EqualError(t, err, "max candidates must be greater than or equal to zero")
	})

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

	t.Run("DeterministicReranker handles negative and NaN weights", func(t *testing.T) {
		result, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
			Candidates: []Candidate{testSessionCandidate("candidate-negative", -1, -1, -1, time.Time{})},
			Options: RerankOptions{
				LexicalWeight: -1,
				VectorWeight:  math.NaN(),
				FusedWeight:   0,
				RecencyWeight: 0,
			},
		})
		require.NoError(t, err)
		require.Equal(t, []RerankItem{{CandidateID: "candidate-negative", Score: 1}}, result.Items)
	})

	t.Run("DeterministicReranker recency only with zero time", func(t *testing.T) {
		result, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
			Candidates: []Candidate{{
				ID:           "candidate-zero-time",
				SourceKind:   SourceKindSessionMessage,
				SessionID:    "ses_test",
				MessageID:    1,
				Text:         "session text",
				LexicalScore: 1,
				VectorScore:  1,
				FusedScore:   1,
			}},
			Options: RerankOptions{RecencyWeight: 1},
		})
		require.NoError(t, err)
		require.Equal(t, []RerankItem{{CandidateID: "candidate-zero-time", Score: 1}}, result.Items)
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
