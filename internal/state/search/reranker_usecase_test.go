package search

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUseCaseReranker_RoutesConfiguredUseCase(t *testing.T) {
	reranker := UseCaseReranker{
		Default: namedTestReranker{name: RerankerDeterministic, score: 0.1},
		Override: map[string]Reranker{
			"memory_reflection": namedTestReranker{name: RerankerLLM, score: 0.9},
		},
		Fallback: NoopReranker{},
	}

	result, err := reranker.Rerank(context.Background(), RerankRequest{
		Caller: "memory_reflection",
		Candidates: []Candidate{{
			ID:         "candidate-a",
			MemoryID:   "source-a",
			SourceKind: SourceKindMemoryItem,
			Text:       "candidate",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, RerankerLLM, result.Reranker)
	require.Equal(t, 0.9, result.Items[0].Score)
}

func TestUseCaseReranker_NameUsesDefaultOrDeterministicFallback(t *testing.T) {
	require.Equal(t, RerankerLLM, UseCaseReranker{
		Default: namedTestReranker{name: RerankerLLM},
	}.Name())
	require.Equal(t, RerankerDeterministic, UseCaseReranker{}.Name())
}

func TestUseCaseReranker_FallsBackToDefaultForUnknownUseCase(t *testing.T) {
	reranker := UseCaseReranker{
		Default: namedTestReranker{name: RerankerDeterministic, score: 0.1},
		Override: map[string]Reranker{
			"memory_reflection": namedTestReranker{name: RerankerLLM, score: 0.9},
		},
		Fallback: NoopReranker{},
	}

	result, err := reranker.Rerank(context.Background(), RerankRequest{
		Caller: "memory_tool_search",
		Candidates: []Candidate{{
			ID:         "candidate-a",
			MemoryID:   "source-a",
			SourceKind: SourceKindMemoryItem,
			Text:       "candidate",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, RerankerDeterministic, result.Reranker)
	require.Equal(t, 0.1, result.Items[0].Score)
}

func TestUseCaseReranker_UsesDeterministicFallbackForEmptyRouter(t *testing.T) {
	result, err := UseCaseReranker{}.Rerank(context.Background(), RerankRequest{
		Caller: "",
		Candidates: []Candidate{{
			ID:         "candidate-a",
			MemoryID:   "source-a",
			SourceKind: SourceKindMemoryItem,
			Text:       "candidate",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, RerankerDeterministic, result.Reranker)
	require.Len(t, result.Items, 1)
}

type namedTestReranker struct {
	name  string
	score float64
}

func (r namedTestReranker) Name() string {
	return r.name
}

func (r namedTestReranker) Rerank(_ context.Context, req RerankRequest) (RerankResult, error) {
	items := make([]RerankItem, 0, len(req.Candidates))
	for _, candidate := range req.Candidates {
		items = append(items, RerankItem{
			CandidateID: candidate.ID,
			Score:       r.score,
		})
	}

	return RerankResult{Reranker: r.name, Items: items}, nil
}
