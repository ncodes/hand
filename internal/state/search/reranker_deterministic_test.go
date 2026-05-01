package search

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeterministicReranker_Name(t *testing.T) {
	require.Equal(t, RerankerDeterministic, DeterministicReranker{}.Name())
}

func TestDeterministicReranker_CombinesScoresAndRecency(t *testing.T) {
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
	require.Equal(t, RerankerDeterministic, result.Reranker)
	require.Equal(t, []string{"candidate-best", "candidate-vector", "candidate-older"}, rerankIDs(result))
	require.Greater(t, result.Items[0].Score, result.Items[1].Score)
	require.Greater(t, result.Items[1].Score, result.Items[2].Score)
}

func TestDeterministicReranker_UsesDefaultWeightsAndIDTieBreak(t *testing.T) {
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

func TestDeterministicReranker_SupportsMemoryCandidatesAndRecencyOnlyWeights(t *testing.T) {
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

func TestDeterministicReranker_SupportsLowerIsBetterLexicalScores(t *testing.T) {
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

func TestDeterministicReranker_RejectsInvalidCandidate(t *testing.T) {
	invalid := testSessionCandidate("candidate-a", 0, 0, 0, time.Time{})
	invalid.Text = ""

	_, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: []Candidate{invalid},
	})

	require.EqualError(t, err, "candidate text is required")
}

func TestDeterministicReranker_RejectsInvalidOptions(t *testing.T) {
	candidates := []Candidate{testSessionCandidate("candidate-a", 0, 0, 0, time.Time{})}

	_, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: candidates,
		Options:    RerankOptions{MaxCandidates: -1},
	})
	require.EqualError(t, err, "max candidates must be greater than or equal to zero")

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
}

func TestDeterministicReranker_HandlesEmptyCandidates(t *testing.T) {
	result, err := DeterministicReranker{}.Rerank(context.Background(), RerankRequest{})

	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestDeterministicReranker_HandlesNegativeAndNaNWeights(t *testing.T) {
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
}

func TestDeterministicReranker_RecencyOnlyWithZeroTime(t *testing.T) {
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
}
