package search

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNoopReranker_Name(t *testing.T) {
	require.Equal(t, RerankerNoop, NoopReranker{}.Name())
}

func TestNoopReranker_PreservesOrderAndBoundsCandidates(t *testing.T) {
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
	require.Equal(t, RerankerNoop, result.Reranker)
	require.Equal(t, []RerankItem{
		{CandidateID: "candidate-b", Score: 0.2},
		{CandidateID: "candidate-a", Score: 0.9},
	}, result.Items)
}

func TestNoopReranker_RejectsInvalidCandidate(t *testing.T) {
	invalid := testSessionCandidate("candidate-a", 0, 0, 0, time.Time{})
	invalid.Text = ""

	_, err := NoopReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: []Candidate{invalid},
	})

	require.EqualError(t, err, "candidate text is required")
}

func TestNoopReranker_RejectsNegativeMaxCandidates(t *testing.T) {
	_, err := NoopReranker{}.Rerank(context.Background(), RerankRequest{
		Candidates: []Candidate{testSessionCandidate("candidate-a", 0, 0, 0, time.Time{})},
		Options:    RerankOptions{MaxCandidates: -1},
	})

	require.EqualError(t, err, "max candidates must be greater than or equal to zero")
}
