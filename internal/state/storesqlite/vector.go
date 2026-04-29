package storesqlite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	base "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/indexing"
	"github.com/wandxy/hand/internal/state/retrieval"
	statevector "github.com/wandxy/hand/internal/state/vector"
)

const defaultVectorStoreRebuildBatchSize = 100
const defaultHybridRetrievalCandidateLimit = indexing.DefaultHybridRetrievalCandidateLimit
const defaultRerankCandidateLimit = indexing.DefaultRerankCandidateLimit
const maxHybridRetrievalCandidateLimit = indexing.MaxHybridRetrievalCandidateLimit

// searchCandidate is a merged lexical/vector candidate before final grouping.
type searchCandidate struct {
	indexing.CandidateMatch
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ID         uint
	Role       string
	Name       string
	Content    string
	ToolCalls  string
	ToolCallID string
}

// searchCandidateSet collects merged search candidates keyed by message row ID.
type searchCandidateSet = indexing.SearchCandidateSet[uint, *searchCandidate]

// CandidateMatchRef returns the mutable ranking metadata for this candidate.
func (candidate *searchCandidate) CandidateMatchRef() *indexing.CandidateMatch {
	if candidate == nil {
		return nil
	}

	return &candidate.CandidateMatch
}

type VectorStoreOptions = statevector.VectorStoreOptions

// vectorConfig holds normalized vector dependencies and operational limits.
type vectorConfig struct {
	statevector.VectorConfig
	batchSize int
}

type vectorInput = statevector.VectorInput

// ConfigureVectorStore enables or disables vector indexing and hybrid search for this store.
func (s *Store) ConfigureVectorStore(opts VectorStoreOptions) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	model := strings.TrimSpace(opts.EmbeddingModel)
	if opts.Embedder == nil && opts.VectorStore == nil && model == "" {
		s.vectors = nil
		return nil
	}
	if opts.Embedder == nil {
		return errors.New("vector store embedding provider is required")
	}
	if opts.VectorStore == nil {
		return errors.New("vector store is required")
	}
	if model == "" {
		return errors.New("vector store embedding model is required")
	}
	batchSize := opts.RebuildBatchSize
	if batchSize < 0 {
		return errors.New("vector store rebuild batch size must be greater than or equal to zero")
	}
	if batchSize == 0 {
		batchSize = defaultVectorStoreRebuildBatchSize
	}
	rerankMax := opts.RerankMaxCandidates
	if rerankMax < 0 {
		return errors.New("vector store rerank max candidates must be greater than or equal to zero")
	}
	if rerankMax == 0 {
		rerankMax = defaultRerankCandidateLimit
	}
	rerankEnabled := true
	if opts.EnableRerank != nil {
		rerankEnabled = *opts.EnableRerank
	}

	if err := retrieval.ValidateReranker(opts.Reranker); err != nil {
		return err
	}

	s.vectors = &vectorConfig{
		VectorConfig: statevector.VectorConfig{
			Provider:    opts.Embedder,
			Reranker:    opts.Reranker,
			Store:       opts.VectorStore,
			Model:       model,
			RerankMax:   rerankMax,
			Diagnostics: opts.Diagnostics,
			Rerank:      rerankEnabled,
			Required:    opts.Required,
		},
		batchSize: batchSize,
	}

	return nil
}

// rerankEnabled reports whether hybrid search should rerank merged candidates.
func (s *Store) rerankEnabled() bool {
	return s != nil && s.vectors != nil && s.vectors.Rerank
}

// rerankerName returns the configured reranker name or the deterministic fallback name.
func (s *Store) rerankerName() string {
	if s == nil || s.vectors == nil || s.vectors.Reranker == nil {
		return retrieval.RerankerDeterministic
	}

	return strings.TrimSpace(strings.ToLower(s.vectors.Reranker.Name()))
}

// diagnosticsEnabled reports whether vector search diagnostics should be logged.
func (s *Store) diagnosticsEnabled() bool {
	return s != nil && s.vectors != nil && s.vectors.Diagnostics
}

// searchMessagesHybrid merges lexical and vector candidates, reranks them, and maps them to public results.
func (s *Store) searchMessagesHybrid(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	queryText string,
) ([]base.SearchMessageResult, error) {

	candidateLimit := hybridCandidateLimit(opts)
	lexicalRows, err := s.searchMessagesLexical(ctx, id, opts, queryText, candidateLimit, false)
	if err != nil {
		logSafeError(s.logSearchEvent("lexical search failed", id, opts), err).
			Msg("session search lexical search failed")
		return nil, err
	}
	candidates := searchCandidatesFromLexicalRows(lexicalRows)

	s.logSearchEvent("lexical candidates gathered", id, opts).
		Int("candidate_count", len(candidates)).
		Int("row_count", len(lexicalRows)).
		Msg("session search lexical candidates gathered")

	vectorRows, err := s.searchMessagesVector(ctx, id, opts, candidateLimit)
	if err != nil {
		logSafeError(s.logSearchEvent("vector search failed", id, opts), err).
			Msg("session search vector search failed")
		return nil, err
	}

	s.logSearchEvent("vector candidates gathered", id, opts).
		Int("candidate_count", len(vectorRows)).
		Msg("session search vector candidates gathered")

	beforeMerge := len(candidates)
	candidates.Merge(vectorRows, searchCandidateKey)
	if len(candidates) == 0 {
		s.logSearchEvent("no candidates", id, opts).Msg("session search returned no hybrid candidates")
		return nil, nil
	}

	s.logSearchEvent("hybrid candidates merged", id, opts).
		Int("lexical_candidate_count", beforeMerge).
		Int("vector_candidate_count", len(vectorRows)).
		Int("merged_candidate_count", len(candidates)).
		Msg("session search hybrid candidates merged")

	s.logCandidateDiagnostics("candidate merged", candidates.Sorted(lessSearchCandidate))

	matchCounts, lastMatchedAt := searchCandidateSessionStats(candidates)
	reranked := candidates.Sorted(lessSearchCandidate)
	if s.rerankEnabled() {
		reranked = s.rerankSearchCandidates(ctx, opts, candidates)
		s.logCandidateDiagnostics("candidate reranked", reranked)
	} else {
		s.logSearchEvent("rerank skipped", id, opts).
			Str("reranker", s.rerankerName()).
			Msg("session search rerank skipped")
	}
	rows := rankedSearchRowsFromCandidateSlice(reranked, opts, matchCounts, lastMatchedAt)
	results := searchMessageResultRowsToResults(rows)

	s.logSearchEvent("results ranked", id, opts).
		Int("session_count", len(results)).
		Int("message_count", len(rows)).
		Msg("session search hybrid results ranked")

	return results, nil
}

// searchCandidatesFromLexicalRows converts ranked lexical rows into mergeable candidates.
func searchCandidatesFromLexicalRows(rows []searchSessionResultRow) searchCandidateSet {
	candidates := make(searchCandidateSet, len(rows))
	for idx, row := range rows {
		candidates[row.ID] = &searchCandidate{
			CandidateMatch: indexing.CandidateMatch{
				SessionID:       row.SessionID,
				MatchedText:     row.MatchedText,
				MatchedToolName: row.MatchedToolName,
				LexicalScore:    row.Score,
				LexicalRank:     idx + 1,
				HasLexical:      true,
			},
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
			ID:         row.ID,
			Role:       row.Role,
			Name:       row.Name,
			Content:    row.Content,
			ToolCalls:  row.ToolCalls,
			ToolCallID: row.ToolCallID,
		}
	}

	return candidates
}

// rankedSearchRowsFromCandidates ranks candidates and returns rows shaped for public search results.
func rankedSearchRowsFromCandidates(
	candidates searchCandidateSet,
	opts base.SearchMessageOptions,
) []searchSessionResultRow {
	return rankedSearchRowsFromCandidateSlice(candidates.Sorted(lessSearchCandidate), opts, nil, nil)
}

// rankedSearchRowsFromCandidateSlice groups ranked candidates by session and applies result limits.
func rankedSearchRowsFromCandidateSlice(
	candidates []*searchCandidate,
	opts base.SearchMessageOptions,
	matchCounts map[string]int,
	lastMatchedAt map[string]time.Time,
) []searchSessionResultRow {
	groups := make(map[string][]*searchCandidate)
	for _, candidate := range candidates {
		groups[candidate.SessionID] = append(groups[candidate.SessionID], candidate)
	}

	sessions := make([]string, 0, len(groups))
	bestScoreBySession := make(map[string]float64, len(groups))
	lastMatchedBySession := make(map[string]time.Time, len(groups))
	for sessionID, sessionCandidates := range groups {
		slices.SortStableFunc(sessionCandidates, compareCandidatesWithinSession)
		bestScoreBySession[sessionID] = searchCandidateRankingScore(sessionCandidates[0])
		for _, candidate := range sessionCandidates {
			if candidate.CreatedAt.After(lastMatchedBySession[sessionID]) {
				lastMatchedBySession[sessionID] = candidate.CreatedAt
			}
		}
		if lastMatchedAt[sessionID].After(lastMatchedBySession[sessionID]) {
			lastMatchedBySession[sessionID] = lastMatchedAt[sessionID]
		}
		sessions = append(sessions, sessionID)
	}

	slices.SortStableFunc(sessions, func(left string, right string) int {
		return compareRankedSessions(left, right, bestScoreBySession, lastMatchedBySession)
	})
	if opts.MaxSessions > 0 && len(sessions) > opts.MaxSessions {
		sessions = sessions[:opts.MaxSessions]
	}

	rows := make([]searchSessionResultRow, 0, len(candidates))
	for _, sessionID := range sessions {
		sessionCandidates := groups[sessionID]
		if opts.MaxMessagesPerSession > 0 && len(sessionCandidates) > opts.MaxMessagesPerSession {
			sessionCandidates = sessionCandidates[:opts.MaxMessagesPerSession]
		}
		lastMatchedAt := ""
		if !lastMatchedBySession[sessionID].IsZero() {
			lastMatchedAt = lastMatchedBySession[sessionID].UTC().Format(time.RFC3339Nano)
		}
		for _, candidate := range sessionCandidates {
			rows = append(rows, searchSessionResultRow{
				CreatedAt:       candidate.CreatedAt,
				UpdatedAt:       candidate.UpdatedAt,
				ID:              candidate.ID,
				SessionID:       candidate.SessionID,
				Sequence:        0,
				Role:            candidate.Role,
				Name:            candidate.Name,
				Content:         candidate.Content,
				ToolCalls:       candidate.ToolCalls,
				ToolCallID:      candidate.ToolCallID,
				MatchedText:     candidate.MatchedText,
				MatchedToolName: candidate.MatchedToolName,
				Score:           searchCandidateRankingScore(candidate),
				BestScore:       bestScoreBySession[sessionID],
				MatchCount:      searchCandidateMatchCount(sessionID, groups, matchCounts),
				LastMatchedAt:   lastMatchedAt,
			})
		}
	}

	return rows
}

// compareCandidatesWithinSession orders candidates inside a single session.
func compareCandidatesWithinSession(left *searchCandidate, right *searchCandidate) int {
	leftScore := searchCandidateRankingScore(left)
	rightScore := searchCandidateRankingScore(right)
	if leftScore != rightScore {
		if leftScore > rightScore {
			return -1
		}
		return 1
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		if left.CreatedAt.After(right.CreatedAt) {
			return -1
		}
		return 1
	}
	if left.ID > right.ID {
		return -1
	}
	if left.ID < right.ID {
		return 1
	}
	return 0
}

// compareRankedSessions orders result sessions by best score, recency, and session ID.
func compareRankedSessions(
	left string,
	right string,
	bestScoreBySession map[string]float64,
	lastMatchedBySession map[string]time.Time,
) int {
	if bestScoreBySession[left] != bestScoreBySession[right] {
		if bestScoreBySession[left] > bestScoreBySession[right] {
			return -1
		}
		return 1
	}
	if !lastMatchedBySession[left].Equal(lastMatchedBySession[right]) {
		if lastMatchedBySession[left].After(lastMatchedBySession[right]) {
			return -1
		}
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

// searchCandidateKey returns the durable message row ID used to merge candidates.
func searchCandidateKey(candidate *searchCandidate) uint {
	if candidate == nil {
		return 0
	}

	return candidate.ID
}

// lessSearchCandidate reports whether left should sort before right.
func lessSearchCandidate(left *searchCandidate, right *searchCandidate) bool {
	return compareSearchCandidates(left, right) < 0
}

// searchCandidateSessionStats returns per-session candidate counts and latest timestamps.
func searchCandidateSessionStats(candidates searchCandidateSet) (map[string]int, map[string]time.Time) {
	matchCounts := make(map[string]int)
	lastMatchedAt := make(map[string]time.Time)
	for _, candidate := range candidates {
		matchCounts[candidate.SessionID]++
		if candidate.CreatedAt.After(lastMatchedAt[candidate.SessionID]) {
			lastMatchedAt[candidate.SessionID] = candidate.CreatedAt
		}
	}

	return matchCounts, lastMatchedAt
}

// searchCandidateMatchCount returns the original match count for a result session.
func searchCandidateMatchCount(
	sessionID string,
	groups map[string][]*searchCandidate,
	matchCounts map[string]int,
) int {
	if matchCounts[sessionID] > 0 {
		return matchCounts[sessionID]
	}

	return len(groups[sessionID])
}

// compareSearchCandidates orders candidates by score, recency, session ID, and message ID.
func compareSearchCandidates(left *searchCandidate, right *searchCandidate) int {
	return indexing.CompareCandidateOrder(
		searchCandidateRankingScore(left),
		searchCandidateRankingScore(right),
		left.CreatedAt,
		right.CreatedAt,
		left.SessionID,
		right.SessionID,
		left.ID,
		right.ID,
	)
}

// searchCandidateRankingScore returns the final score used for ordering a candidate.
func searchCandidateRankingScore(candidate *searchCandidate) float64 {
	return indexing.CandidateRankingScore(candidate.HasRerank, candidate.RerankScore, candidate.FusedScore)
}

// hybridCandidateLimit returns the shared lexical/vector candidate limit.
func hybridCandidateLimit(opts base.SearchMessageOptions) int {
	return indexing.HybridRetrievalCandidateLimit(opts)
}

// rerankSearchCandidates converts merged search candidates to the shared retrieval reranker contract.
func (s *Store) rerankSearchCandidates(
	ctx context.Context,
	opts base.SearchMessageOptions,
	candidates searchCandidateSet,
) []*searchCandidate {
	items := candidates.Sorted(lessSearchCandidate)
	if len(items) == 0 {
		return nil
	}

	maxCandidates := defaultRerankCandidateLimit
	reranker := retrieval.Reranker(retrieval.DeterministicReranker{})
	rerankerName := retrieval.RerankerDeterministic
	if s != nil && s.vectors != nil {
		maxCandidates = s.vectors.RerankMax
		rerankerName = s.rerankerName()
		if s.vectors.Reranker != nil {
			reranker = s.vectors.Reranker
		}
	}
	if maxCandidates > 0 && len(items) > maxCandidates {
		items = items[:maxCandidates]
	}
	s.logSearchEvent("rerank started", "", opts).
		Str("configured_reranker", rerankerName).
		Int("candidate_count", len(items)).
		Int("max_candidates", maxCandidates).
		Msg("session search rerank started")

	retrievalCandidates := make([]retrieval.Candidate, 0, len(items))
	searchCandidateByID := make(map[string]*searchCandidate, len(items))
	for _, candidate := range items {
		retrievalCandidate := retrievalCandidateFromSearchCandidate(candidate)
		retrievalCandidates = append(retrievalCandidates, retrievalCandidate)
		searchCandidateByID[retrievalCandidate.ID] = candidate
	}

	result, err := retrieval.RerankWithFallback(ctx, reranker, retrieval.DeterministicReranker{}, retrieval.RerankRequest{
		Query:      strings.TrimSpace(opts.Query),
		Caller:     "session_search",
		SourceKind: retrieval.SourceKindSessionMessage,
		Candidates: retrievalCandidates,
		Options: retrieval.RerankOptions{
			LexicalDirection: retrieval.ScoreLowerIsBetter,
			VectorDirection:  retrieval.ScoreHigherIsBetter,
			FusedDirection:   retrieval.ScoreHigherIsBetter,
		},
	})
	if err != nil {
		s.logSearchEvent("rerank fallback failed", "", opts).
			Err(err).
			Str("configured_reranker", rerankerName).
			Int("candidate_count", len(items)).
			Msg("session search rerank fallback failed")
		return items
	}

	reranked := make([]*searchCandidate, 0, len(result.Items))
	for _, item := range result.Items {
		candidate := searchCandidateByID[item.CandidateID]
		candidate.RerankScore = item.Score
		candidate.HasRerank = true
		reranked = append(reranked, candidate)
	}

	s.logSearchEvent("rerank completed", "", opts).
		Str("configured_reranker", rerankerName).
		Str("reranker", searchRerankResultName(result, rerankerName)).
		Int("candidate_count", len(items)).
		Int("result_count", len(reranked)).
		Msg("session search rerank completed")

	return reranked
}

// searchRerankResultName returns the reranker reported by a result or the configured fallback.
func searchRerankResultName(result retrieval.RerankResult, fallback string) string {
	if name := strings.TrimSpace(strings.ToLower(result.Reranker)); name != "" {
		return name
	}

	return strings.TrimSpace(strings.ToLower(fallback))
}

// retrievalCandidateFromSearchCandidate converts a search candidate to the reranker contract.
func retrievalCandidateFromSearchCandidate(candidate *searchCandidate) retrieval.Candidate {
	text := strings.TrimSpace(candidate.MatchedText)
	if text == "" {
		text = strings.TrimSpace(candidate.Content)
	}

	return retrieval.Candidate{
		CreatedAt:    candidate.CreatedAt,
		UpdatedAt:    candidate.UpdatedAt,
		ID:           sourceIDForMessage(candidate.SessionID, candidate.ID),
		SourceKind:   retrieval.SourceKindSessionMessage,
		SessionID:    candidate.SessionID,
		Text:         text,
		LexicalScore: candidate.LexicalScore,
		VectorScore:  candidate.VectorScore,
		FusedScore:   candidate.FusedScore,
		MessageID:    candidate.ID,
	}
}

// searchMessagesVector embeds the query and asks the vector store for session-message candidates.
func (s *Store) searchMessagesVector(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	candidateLimit int,
) ([]*searchCandidate, error) {
	embeddingReq := retrieval.EmbeddingRequest{
		Model: strings.TrimSpace(s.vectors.Model),
		Inputs: []retrieval.EmbeddingInput{{
			ID:         "query",
			Text:       strings.TrimSpace(opts.Query),
			SourceKind: retrieval.SourceKindSessionMessage,
		}},
	}

	s.logSearchEvent("query embedding started", id, opts).
		Str("embedding_model", embeddingReq.Model).
		Msg("session search query embedding started")

	embedding, err := s.vectors.Provider.Embed(ctx, embeddingReq)
	if err != nil {
		logSafeError(s.logSearchEvent("query embedding failed", id, opts), err).
			Msg("session search query embedding failed")
		return nil, err
	}
	if err := retrieval.ValidateEmbeddingResult(embeddingReq, embedding); err != nil {
		logSafeError(s.logSearchEvent("query embedding validation failed", id, opts), err).
			Msg("session search query embedding validation failed")
		return nil, err
	}

	s.logSearchEvent("query embedding completed", id, opts).
		Int("dimensions", embedding.Dimensions).
		Str("embedding_model", strings.TrimSpace(embedding.Model)).
		Msg("session search query embedding completed")

	searchReq := retrieval.VectorSearchRequest{
		EmbeddingModel: strings.TrimSpace(s.vectors.Model),
		Dimensions:     embedding.Dimensions,
		QueryVector:    embedding.Items[0].Vector,
		Limit:          candidateLimit,
		Filter: retrieval.VectorFilter{
			SourceKind:      retrieval.SourceKindSessionMessage,
			SessionID:       id,
			IgnoreSessionID: opts.IgnoreSessionID,
			Role:            strings.TrimSpace(string(opts.Role)),
			ToolName:        normalizeSearchValue(opts.ToolName),
		},
	}

	s.logSearchEvent("vector search started", id, opts).
		Int("limit", candidateLimit).
		Int("dimensions", embedding.Dimensions).
		Str("embedding_model", searchReq.EmbeddingModel).
		Msg("session search vector retrieval started")

	result, err := s.vectors.Store.Search(ctx, searchReq)
	if err != nil {
		logSafeError(s.logSearchEvent("vector search failed", id, opts), err).
			Msg("session search vector retrieval failed")
		return nil, err
	}

	s.logSearchEvent("vector search completed", id, opts).
		Int("match_count", len(result.Matches)).
		Int("limit", candidateLimit).
		Int("dimensions", embedding.Dimensions).
		Msg("session search vector retrieval completed")

	candidates, err := s.vectorMatchesToCandidates(ctx, id, opts, result.Matches)
	if err != nil {
		logSafeError(s.logSearchEvent("vector matches resolve failed", id, opts), err).
			Msg("session search vector matches resolve failed")
		return nil, err
	}

	s.logSearchEvent("vector matches resolved", id, opts).
		Int("match_count", len(result.Matches)).
		Int("candidate_count", len(candidates)).
		Msg("session search vector matches resolved")

	return candidates, nil
}

// vectorMatchesToCandidates resolves vector hits back to durable messages and searchable rows.
func (s *Store) vectorMatchesToCandidates(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	matches []retrieval.VectorSearchMatch,
) ([]*searchCandidate, error) {
	if len(matches) == 0 {
		return nil, nil
	}

	refs := make(messageRefs, 0, len(matches))
	for _, match := range matches {
		ref, ok := messageRefFromSourceID(match.Record.SourceID)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return nil, nil
	}

	records, err := s.messagesByRef(ctx, refs)
	if err != nil {
		return nil, err
	}

	candidates := make([]*searchCandidate, 0, len(matches))
	seen := map[uint]struct{}{}
	for idx, match := range matches {
		ref, ok := messageRefFromSourceID(match.Record.SourceID)
		if !ok {
			continue
		}
		record, ok := records.get(ref)
		if !ok || !vectorRecordMatchesOptions(record, id, opts) {
			continue
		}
		if _, ok := seen[record.ID]; ok {
			continue
		}
		row, ok := searchRowForVectorRecord(record, match.Record.ID)
		if !ok || !searchRowMatchesOptions(row, opts) {
			continue
		}

		candidates = append(candidates, &searchCandidate{
			CandidateMatch: indexing.CandidateMatch{
				SessionID:       record.SessionID,
				MatchedText:     row.Body,
				MatchedToolName: row.ToolName,
				VectorScore:     match.Score,
				VectorRank:      idx + 1,
				HasVector:       true,
			},
			CreatedAt:  record.CreatedAt,
			UpdatedAt:  record.UpdatedAt,
			ID:         record.ID,
			Role:       record.Role,
			Name:       record.Name,
			Content:    record.Content,
			ToolCalls:  record.ToolCalls,
			ToolCallID: record.ToolCallID,
		})
		seen[record.ID] = struct{}{}
	}

	return candidates, nil
}

// messagesByRef loads active messages for unique session/message references.
func (s *Store) messagesByRef(ctx context.Context, refs messageRefs) (messageLookup, error) {
	refs = refs.unique()
	records := make(messageLookup, len(refs))
	if len(refs) == 0 {
		return records, nil
	}

	where, args := refs.tupleCondition()
	var rows []messageModel
	if err := s.db.WithContext(ctx).Where("(session_id, id) IN ("+where+")", args...).Find(&rows).Error; err != nil {
		return nil, err
	}

	records = make(messageLookup, len(rows))
	for _, row := range rows {
		records.set(messageRef{SessionID: row.SessionID, MessageID: row.ID}, row)
	}
	return records, nil
}

// messageRef identifies a message row within a session.
type messageRef struct {
	SessionID string
	MessageID uint
}

// messageRefs is a typed slice for building tuple queries by message reference.
type messageRefs []messageRef

// messageLookup stores messages keyed by session/message reference.
type messageLookup map[string]messageModel

// unique removes duplicate message references while preserving first occurrence order.
func (refs messageRefs) unique() messageRefs {
	seen := make(map[string]struct{}, len(refs))
	unique := make(messageRefs, 0, len(refs))
	for _, ref := range refs {
		key := ref.key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, ref)
	}

	return unique
}

// tupleCondition builds a SQLite row-value placeholder list for session/message references.
func (refs messageRefs) tupleCondition() (string, []any) {
	var where strings.Builder
	args := make([]any, 0, len(refs)*2)
	for _, ref := range refs {
		if where.Len() > 0 {
			where.WriteString(", ")
		}
		where.WriteString("(?, ?)")
		args = append(args, ref.SessionID, ref.MessageID)
	}

	return where.String(), args
}

// key returns a stable map key for this message reference.
func (r messageRef) key() string {
	return fmt.Sprintf("%s:%d", r.SessionID, r.MessageID)
}

// get returns the message for a reference.
func (lookup messageLookup) get(ref messageRef) (messageModel, bool) {
	record, ok := lookup[ref.key()]
	return record, ok
}

// set stores a message for a reference.
func (lookup messageLookup) set(ref messageRef, record messageModel) {
	lookup[ref.key()] = record
}

// messageRefFromSourceID parses a vector source ID into a SQLite message reference.
func messageRefFromSourceID(sourceID string) (messageRef, bool) {
	sessionID, messageID, ok := statevector.MessageRefFromSourceID(sourceID)
	if !ok {
		return messageRef{}, false
	}

	return messageRef{SessionID: sessionID, MessageID: messageID}, true
}

// vectorRecordMatchesOptions reports whether a resolved message satisfies vector search filters.
func vectorRecordMatchesOptions(record messageModel, id string, opts base.SearchMessageOptions) bool {
	if id != "" && record.SessionID != id {
		return false
	}
	if opts.IgnoreSessionID != "" && record.SessionID == opts.IgnoreSessionID {
		return false
	}
	if role := strings.TrimSpace(string(opts.Role)); role != "" && record.Role != role {
		return false
	}

	return true
}

// searchRowForVectorRecord returns the searchable row represented by a vector record ID.
func searchRowForVectorRecord(record messageModel, vectorID string) (searchRow, bool) {
	return indexing.MessageIndexRowForVectorRecord([]indexing.MessageIndexRow(searchRowsFromMessageModel(record)), vectorID)
}

// searchRowMatchesOptions reports whether a searchable row satisfies non-text search filters.
func searchRowMatchesOptions(row searchRow, opts base.SearchMessageOptions) bool {
	return indexing.MessageIndexRowMatchesSearchOptions(row, opts)
}

// indexVectors embeds searchable message rows and upserts the resulting vector records.
func (s *Store) indexVectors(ctx context.Context, records []messageModel) error {
	recordsToUpsert, err := s.vectorRecordsForMessages(ctx, records)
	if err != nil || len(recordsToUpsert) == 0 {
		return err
	}

	return s.upsertVectorRecords(ctx, recordsToUpsert)
}

// vectorRecordsForMessages embeds message search rows and builds vector records for storage.
func (s *Store) vectorRecordsForMessages(
	ctx context.Context,
	records []messageModel,
) ([]retrieval.VectorRecord, error) {
	if s == nil || s.vectors == nil || len(records) == 0 {
		return nil, nil
	}

	inputs := messageModels(records).searchRows().vectorInputs()
	if len(inputs) == 0 {
		return nil, nil
	}

	embeddingInputs := make([]retrieval.EmbeddingInput, 0, len(inputs))
	for _, input := range inputs {
		embeddingInputs = append(embeddingInputs, retrieval.EmbeddingInput{
			ID:         input.ID,
			Text:       input.Text,
			SourceKind: retrieval.SourceKindSessionMessage,
		})
	}

	req := retrieval.EmbeddingRequest{
		Model:  s.vectors.Model,
		Inputs: embeddingInputs,
	}

	s.logVectorEvent("embedding started").
		Int("input_count", len(req.Inputs)).
		Str("embedding_model", strings.TrimSpace(req.Model)).
		Msg("session vector embedding started")

	result, err := s.vectors.Provider.Embed(ctx, req)
	if err != nil {
		logSafeError(s.logVectorEvent("embedding failed"), err).Msg("session vector embedding failed")
		return nil, err
	}
	if err := retrieval.ValidateEmbeddingResult(req, result); err != nil {
		logSafeError(s.logVectorEvent("embedding validation failed"), err).
			Msg("session vector embedding validation failed")
		return nil, err
	}

	s.logVectorEvent("embedding completed").
		Int("input_count", len(req.Inputs)).
		Int("dimensions", result.Dimensions).
		Str("embedding_model", strings.TrimSpace(result.Model)).
		Msg("session vector embedding completed")

	inputByID := make(map[string]vectorInput, len(inputs))
	for _, input := range inputs {
		inputByID[input.ID] = input
	}

	recordsToUpsert := make([]retrieval.VectorRecord, 0, len(result.Items))
	for _, item := range result.Items {
		input := inputByID[item.ID]
		recordsToUpsert = append(recordsToUpsert, retrieval.VectorRecord{
			CreatedAt:      input.CreatedAt,
			UpdatedAt:      input.UpdatedAt,
			ID:             item.ID,
			SourceKind:     retrieval.SourceKindSessionMessage,
			SourceID:       input.SourceID,
			SessionID:      input.SessionID,
			Role:           input.Role,
			ToolName:       input.ToolName,
			EmbeddingModel: result.Model,
			ContentHash:    item.ContentHash,
			Vector:         item.Vector,
			Dimensions:     result.Dimensions,
		})
	}

	return recordsToUpsert, nil
}

// upsertVectorRecords writes vector records through the configured vector store.
func (s *Store) upsertVectorRecords(ctx context.Context, records []retrieval.VectorRecord) error {
	if s == nil || s.vectors == nil || len(records) == 0 {
		return nil
	}

	model := records[0].EmbeddingModel
	dimensions := records[0].Dimensions
	s.logVectorEvent("upsert started").
		Int("record_count", len(records)).
		Str("embedding_model", strings.TrimSpace(model)).
		Int("dimensions", dimensions).
		Msg("session vector upsert started")
	if err := s.vectors.Store.Upsert(ctx, records); err != nil {
		logSafeError(s.logVectorEvent("upsert failed"), err).
			Int("record_count", len(records)).
			Msg("session vector upsert failed")
		return err
	}
	s.logVectorEvent("upsert completed").
		Int("record_count", len(records)).
		Str("embedding_model", strings.TrimSpace(model)).
		Int("dimensions", dimensions).
		Msg("session vector upsert completed")

	return nil
}

// deleteVectorRows removes vector records for one or more session-message source IDs.
func (s *Store) deleteVectorRows(ctx context.Context, sourceIDs []string) error {
	if s == nil || s.vectors == nil || len(sourceIDs) == 0 {
		return nil
	}

	sourceIDs = uniqueStrings(sourceIDs)
	if len(sourceIDs) == 0 {
		return nil
	}
	req := retrieval.VectorDeleteRequest{
		SourceKind: retrieval.SourceKindSessionMessage,
		SourceIDs:  sourceIDs,
	}

	s.logVectorEvent("delete started").
		Int("source_id_count", len(sourceIDs)).
		Str("source_kind", string(req.SourceKind)).
		Msg("session vector delete started")

	if err := s.vectors.Store.Delete(ctx, req); err != nil {
		logSafeError(s.logVectorEvent("delete failed"), err).
			Int("source_id_count", len(sourceIDs)).
			Str("source_kind", string(req.SourceKind)).
			Msg("session vector delete failed")
		return err
	}

	s.logVectorEvent("delete completed").
		Int("source_id_count", len(sourceIDs)).
		Str("source_kind", string(req.SourceKind)).
		Msg("session vector delete completed")

	return nil
}

// handleVectorStoreError applies best-effort versus required vector indexing semantics.
func (s *Store) handleVectorStoreError(err error) error {
	if err == nil || s == nil || s.vectors == nil || !s.vectors.Required {
		return nil
	}

	return err
}

// vectorInputs maps search rows to stable embedding inputs.
func (rows searchRows) vectorInputs() []vectorInput {
	return statevector.VectorInputsFromIndexRows([]indexing.MessageIndexRow(rows))
}

// sourceIDs returns stable vector source IDs for active message models.
func (records messageModels) sourceIDs() []string {
	if len(records) == 0 {
		return nil
	}

	sourceIDs := make([]string, 0, len(records))
	for _, record := range records {
		sourceIDs = append(sourceIDs, sourceIDForMessage(record.SessionID, record.ID))
	}

	return sourceIDs
}

// sourceIDsFromMessageIDs returns vector source IDs for message IDs in one session.
func sourceIDsFromMessageIDs(sessionID string, messageIDs []uint) []string {
	if len(messageIDs) == 0 {
		return nil
	}

	sourceIDs := make([]string, 0, len(messageIDs))
	for _, messageID := range messageIDs {
		sourceIDs = append(sourceIDs, sourceIDForMessage(sessionID, messageID))
	}

	return sourceIDs
}

// sourceIDForMessage returns the vector source ID for a message row.
func sourceIDForMessage(sessionID string, messageID uint) string {
	return statevector.SourceIDForMessage(sessionID, messageID)
}

// uniqueStrings returns non-empty strings without duplicates.
func uniqueStrings(values []string) []string {
	return base.UniqueStrings(values)
}

// normalizeSearchValue canonicalizes a search filter value.
func normalizeSearchValue(value string) string {
	return base.NormalizeMatchValue(value)
}
