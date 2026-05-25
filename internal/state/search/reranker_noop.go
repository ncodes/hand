package search

import "context"

// NoopReranker reranks noop candidates.
type NoopReranker struct{}

func (NoopReranker) Name() string {
	return RerankerNoop
}

func (NoopReranker) Rerank(_ context.Context, req RerankRequest) (RerankResult, error) {
	name := NoopReranker{}.Name()
	rerankTraceLogEvent(req, name).Int("candidate_count", len(req.Candidates)).Msg("rerank started")

	candidates, err := limitCandidates(req.Candidates, req.Options.MaxCandidates)
	if err != nil {
		rerankTraceLogEvent(req, name).Err(err).Msg("rerank candidate bound failed")
		return RerankResult{}, err
	}

	items := make([]RerankItem, 0, len(candidates))
	for _, candidate := range candidates {
		if err := ValidateCandidate(candidate); err != nil {
			rerankTraceLogEvent(req, name).Err(err).Msg("rerank candidate validation failed")
			return RerankResult{}, err
		}
		items = append(items, RerankItem{
			CandidateID: candidate.ID,
			Score:       candidate.FusedScore,
		})
	}

	rerankTraceLogEvent(req, name).
		Int("bounded_candidate_count", len(candidates)).
		Int("result_count", len(items)).
		Msg("rerank completed")

	return RerankResult{Reranker: name, Items: items}, nil
}
