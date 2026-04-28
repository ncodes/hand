package retrieval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/wandxy/hand/internal/rerank"
	"github.com/wandxy/hand/pkg/logutils"
)

var retrievalLog = logutils.InitLogger("storage.retrieval")

const (
	RerankerNoop          = rerank.Noop
	RerankerDeterministic = rerank.Deterministic
	RerankerLLM           = rerank.LLM
)

type Reranker interface {
	Name() string
	Rerank(context.Context, RerankRequest) (RerankResult, error)
}

type RerankRequest struct {
	Options    RerankOptions
	Query      string
	Caller     string
	TraceID    string
	SourceKind SourceKind
	Candidates []Candidate
}

type RerankOptions struct {
	LexicalDirection ScoreDirection
	VectorDirection  ScoreDirection
	FusedDirection   ScoreDirection
	MaxCandidates    int
	LexicalWeight    float64
	VectorWeight     float64
	FusedWeight      float64
	RecencyWeight    float64
}

const (
	defaultRerankLexicalWeight = 0.45
	defaultRerankVectorWeight  = 0.45
	defaultRerankFusedWeight   = 0.10
)

type RerankResult struct {
	Reranker string
	Items    []RerankItem
}

type RerankItem struct {
	CandidateID string
	Score       float64
}

type NoopReranker struct{}

func (NoopReranker) Name() string {
	return RerankerNoop
}

func (NoopReranker) Rerank(_ context.Context, req RerankRequest) (RerankResult, error) {
	name := NoopReranker{}.Name()
	rerankTraceLogEvent(req, name).Int("candidate_count", len(req.Candidates)).Msg("rerank started")

	candidates, err := boundedCandidates(req.Candidates, req.Options.MaxCandidates)
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

type DeterministicReranker struct{}

func (DeterministicReranker) Name() string {
	return RerankerDeterministic
}

func (DeterministicReranker) Rerank(_ context.Context, req RerankRequest) (RerankResult, error) {
	name := DeterministicReranker{}.Name()
	rerankTraceLogEvent(req, name).Int("candidate_count", len(req.Candidates)).Msg("rerank started")

	candidates, err := boundedCandidates(req.Candidates, req.Options.MaxCandidates)
	if err != nil {
		rerankTraceLogEvent(req, name).Err(err).Msg("rerank candidate bound failed")
		return RerankResult{}, err
	}
	if len(candidates) == 0 {
		rerankTraceLogEvent(req, name).Msg("rerank skipped without candidates")
		return RerankResult{Reranker: name}, nil
	}
	for _, candidate := range candidates {
		if err := ValidateCandidate(candidate); err != nil {
			rerankTraceLogEvent(req, name).Err(err).Msg("rerank candidate validation failed")
			return RerankResult{}, err
		}
	}

	weights, err := normalizeRerankWeights(req.Options)
	if err != nil {
		rerankTraceLogEvent(req, name).Err(err).Msg("rerank weight normalization failed")
		return RerankResult{}, err
	}
	lexicalScores := make([]float64, 0, len(candidates))
	vectorScores := make([]float64, 0, len(candidates))
	fusedScores := make([]float64, 0, len(candidates))
	recencyScores := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		lexicalScores = append(lexicalScores, candidate.LexicalScore)
		vectorScores = append(vectorScores, candidate.VectorScore)
		fusedScores = append(fusedScores, candidate.FusedScore)
		recencyScores = append(recencyScores, candidateRecencyScore(candidate))
	}

	normalizedLexical, _ := NormalizeScores(lexicalScores, weights.LexicalDirection)
	normalizedVector, _ := NormalizeScores(vectorScores, weights.VectorDirection)
	normalizedFused, _ := NormalizeScores(fusedScores, weights.FusedDirection)
	normalizedRecency, _ := NormalizeScores(recencyScores, ScoreHigherIsBetter)

	items := make([]RerankItem, 0, len(candidates))
	for idx, candidate := range candidates {
		score := weights.LexicalWeight*normalizedLexical[idx] +
			weights.VectorWeight*normalizedVector[idx] +
			weights.FusedWeight*normalizedFused[idx] +
			weights.RecencyWeight*normalizedRecency[idx]
		items = append(items, RerankItem{
			CandidateID: candidate.ID,
			Score:       score,
		})
	}

	sort.SliceStable(items, func(i int, j int) bool {
		left := items[i]
		right := items[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}

		return left.CandidateID < right.CandidateID
	})

	rerankTraceLogEvent(req, name).
		Int("bounded_candidate_count", len(candidates)).
		Int("result_count", len(items)).
		Float64("lexical_weight", weights.LexicalWeight).
		Float64("vector_weight", weights.VectorWeight).
		Float64("fused_weight", weights.FusedWeight).
		Float64("recency_weight", weights.RecencyWeight).
		Msg("rerank completed")

	return RerankResult{Reranker: name, Items: items}, nil
}

func RerankWithFallback(
	ctx context.Context,
	primary Reranker,
	fallback Reranker,
	req RerankRequest) (RerankResult, error) {
	if primary == nil {
		rerankDebugLogEvent(req, "fallback").Msg("rerank primary missing, using fallback")
		return rerankFallback(ctx, fallback, req)
	}
	if err := ValidateReranker(primary); err != nil {
		return RerankResult{}, err
	}
	if fallback != nil {
		if err := ValidateReranker(fallback); err != nil {
			return RerankResult{}, err
		}
	}

	result, err := primary.Rerank(ctx, req)
	if err != nil {
		rerankDebugLogEvent(req, "fallback").Err(err).Msg("rerank primary failed, using fallback")
		return rerankFallback(ctx, fallback, req)
	}
	candidates, err := boundedCandidates(req.Candidates, req.Options.MaxCandidates)
	if err != nil {
		rerankDebugLogEvent(req, "fallback").Err(err).Msg("rerank result candidate bound failed, using fallback")
		return rerankFallback(ctx, fallback, req)
	}
	if err := ValidateRerankResult(candidates, result); err != nil {
		rerankDebugLogEvent(req, "fallback").Err(err).Msg("rerank primary result rejected, using fallback")
		return rerankFallback(ctx, fallback, req)
	}
	if strings.TrimSpace(result.Reranker) == "" {
		result.Reranker = primary.Name()
	}

	rerankTraceLogEvent(req, "primary").Int("result_count", len(result.Items)).Msg("rerank primary result accepted")

	return result, nil
}

func ValidateRerankResult(candidates []Candidate, result RerankResult) error {
	if len(candidates) == 0 {
		if len(result.Items) != 0 {
			return errors.New("rerank result must be empty when candidates are empty")
		}
		return nil
	}
	if len(result.Items) == 0 {
		return errors.New("rerank result is empty")
	}
	if len(result.Items) > len(candidates) {
		return errors.New("rerank result cannot contain more items than candidates")
	}

	knownIDs := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		knownIDs[candidate.ID] = struct{}{}
	}

	seenIDs := make(map[string]struct{}, len(result.Items))
	for _, item := range result.Items {
		candidateID := strings.TrimSpace(item.CandidateID)
		if candidateID == "" {
			return errors.New("rerank item candidate id is required")
		}
		if candidateID != item.CandidateID {
			return errors.New("rerank item candidate id must be trimmed")
		}
		if !finite(item.Score) {
			return errors.New("rerank item score must be finite")
		}
		if _, ok := knownIDs[candidateID]; !ok {
			return fmt.Errorf("rerank item candidate id %q is unknown", candidateID)
		}
		if _, ok := seenIDs[candidateID]; ok {
			return fmt.Errorf("rerank item candidate id %q is duplicated", candidateID)
		}
		seenIDs[candidateID] = struct{}{}
	}

	return nil
}

func boundedCandidates(candidates []Candidate, maxCandidates int) ([]Candidate, error) {
	if maxCandidates < 0 {
		return nil, errors.New("max candidates must be greater than or equal to zero")
	}
	if maxCandidates == 0 || len(candidates) <= maxCandidates {
		return candidates, nil
	}

	return candidates[:maxCandidates], nil
}

func rerankFallback(ctx context.Context, fallback Reranker, req RerankRequest) (RerankResult, error) {
	if fallback == nil {
		fallback = DeterministicReranker{}
	}
	if err := ValidateReranker(fallback); err != nil {
		return RerankResult{}, err
	}

	result, err := fallback.Rerank(ctx, req)
	if strings.TrimSpace(result.Reranker) == "" {
		result.Reranker = fallback.Name()
	}

	return result, err
}

func ValidateReranker(reranker Reranker) error {
	if reranker == nil {
		return nil
	}
	switch strings.TrimSpace(strings.ToLower(reranker.Name())) {
	case RerankerNoop, RerankerDeterministic, RerankerLLM:
		return nil
	default:
		return errors.New("reranker must be one of: noop, deterministic, llm")
	}
}

func normalizeRerankWeights(opts RerankOptions) (RerankOptions, error) {
	lexicalDirection, err := normalizeScoreDirection(opts.LexicalDirection)
	if err != nil {
		return RerankOptions{}, err
	}
	vectorDirection, err := normalizeScoreDirection(opts.VectorDirection)
	if err != nil {
		return RerankOptions{}, err
	}
	fusedDirection, err := normalizeScoreDirection(opts.FusedDirection)
	if err != nil {
		return RerankOptions{}, err
	}

	weights := RerankOptions{
		LexicalDirection: lexicalDirection,
		VectorDirection:  vectorDirection,
		FusedDirection:   fusedDirection,
		LexicalWeight:    nonNegativeWeight(opts.LexicalWeight),
		VectorWeight:     nonNegativeWeight(opts.VectorWeight),
		FusedWeight:      nonNegativeWeight(opts.FusedWeight),
		RecencyWeight:    nonNegativeWeight(opts.RecencyWeight),
	}
	total := weights.LexicalWeight + weights.VectorWeight + weights.FusedWeight + weights.RecencyWeight
	if total == 0 {
		weights.LexicalWeight = defaultRerankLexicalWeight
		weights.VectorWeight = defaultRerankVectorWeight
		weights.FusedWeight = defaultRerankFusedWeight
		total = 1
	}

	weights.LexicalWeight = weights.LexicalWeight / total
	weights.VectorWeight = weights.VectorWeight / total
	weights.FusedWeight = weights.FusedWeight / total
	weights.RecencyWeight = weights.RecencyWeight / total

	return weights, nil
}

func normalizeScoreDirection(direction ScoreDirection) (ScoreDirection, error) {
	if direction == ScoreHigherIsBetter {
		return ScoreHigherIsBetter, nil
	}
	if direction == ScoreLowerIsBetter {
		return ScoreLowerIsBetter, nil
	}

	return ScoreHigherIsBetter, errors.New("score direction is not supported")
}

func nonNegativeWeight(value float64) float64 {
	if value < 0 || !finite(value) {
		return 0
	}

	return value
}

func candidateRecencyScore(candidate Candidate) float64 {
	value := candidate.UpdatedAt
	if value.IsZero() {
		value = candidate.CreatedAt
	}
	if value.IsZero() {
		return 0
	}

	return float64(value.UTC().UnixNano()) / float64(time.Second)
}

func rerankTraceLogEvent(req RerankRequest, reranker string) *zerolog.Event {
	return rerankBaseLogEvent(retrievalLog.Trace(), req, reranker)
}

func rerankDebugLogEvent(req RerankRequest, reranker string) *zerolog.Event {
	return rerankBaseLogEvent(retrievalLog.Debug(), req, reranker)
}

func rerankBaseLogEvent(event *zerolog.Event, req RerankRequest, reranker string) *zerolog.Event {
	event = event.
		Str("reranker", reranker).
		Str("caller", strings.TrimSpace(req.Caller)).
		Str("trace_id", strings.TrimSpace(req.TraceID)).
		Str("source_kind", strings.TrimSpace(string(req.SourceKind))).
		Int("max_candidates", req.Options.MaxCandidates)

	if query := strings.TrimSpace(req.Query); query != "" {
		event = event.Int("query_chars", len([]rune(query)))
	}

	return event
}
