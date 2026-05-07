package storememory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/messages"
	base "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
)

type searchCandidate struct {
	search.CandidateMatch
	Message messages.Message
}

type searchCandidateSet = search.SearchCandidateSet[string, *searchCandidate]

func (candidate *searchCandidate) CandidateMatchRef() *search.CandidateMatch {
	if candidate == nil {
		return nil
	}

	return &candidate.CandidateMatch
}

func (s *Store) ConfigureVectorStore(opts search.VectorStoreOptions) error {
	if s == nil {
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

	rerankMax := opts.RerankMaxCandidates
	if rerankMax < 0 {
		return errors.New("vector store rerank max candidates must be greater than or equal to zero")
	}

	if rerankMax == 0 {
		rerankMax = search.DefaultRerankCandidateLimit
	}

	rerankEnabled := true
	if opts.EnableRerank != nil {
		rerankEnabled = *opts.EnableRerank
	}

	if err := search.ValidateReranker(opts.Reranker); err != nil {
		return err
	}

	s.vectors = &search.VectorConfig{
		Provider:    opts.Embedder,
		Reranker:    opts.Reranker,
		Store:       opts.VectorStore,
		Model:       model,
		RerankMax:   rerankMax,
		Diagnostics: opts.Diagnostics,
		Rerank:      rerankEnabled,
		Required:    opts.Required,
	}

	return nil
}

func (s *Store) SupportsVectorSearch() bool {
	return s != nil && s.vectors != nil
}

func (s *Store) searchMessagesHybrid(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	query string,
) ([]base.SearchMessageResult, error) {
	candidateLimit := search.HybridRetrievalCandidateLimit(opts)
	lexicalCandidates := s.searchMessagesLexicalCandidates(id, opts, query, candidateLimit)

	s.logSearchEvent("lexical candidates gathered", id, opts).
		Int("candidate_count", len(lexicalCandidates)).
		Msg("session search lexical candidates gathered")

	vectorCandidates, err := s.searchMessagesVector(ctx, id, opts, candidateLimit)
	if err != nil {
		logSafeError(s.logSearchEvent("vector search failed", id, opts), err).
			Msg("session search vector search failed")
		return nil, err
	}

	s.logSearchEvent("vector candidates gathered", id, opts).
		Int("candidate_count", len(vectorCandidates)).
		Msg("session search vector candidates gathered")

	candidates := lexicalCandidates
	beforeMerge := len(candidates)
	candidates.Merge(vectorCandidates, searchCandidateKey)
	if len(candidates) == 0 {
		s.logSearchEvent("no candidates", id, opts).Msg("session search returned no hybrid candidates")
		return nil, nil
	}

	s.logSearchEvent("hybrid candidates merged", id, opts).
		Int("lexical_candidate_count", beforeMerge).
		Int("vector_candidate_count", len(vectorCandidates)).
		Int("merged_candidate_count", len(candidates)).
		Msg("session search hybrid candidates merged")

	s.logCandidateDiagnostics("candidate merged", candidates.Sorted(lessSearchCandidate))

	ranked := candidates.Sorted(lessSearchCandidate)
	if s.rerankEnabled() {
		ranked = s.rerankSearchCandidates(ctx, opts, candidates)
		s.logCandidateDiagnostics("candidate reranked", ranked)
	} else {
		s.logSearchEvent("rerank skipped", id, opts).
			Str("reranker", s.rerankerName()).
			Msg("session search rerank skipped")
	}

	results := searchResultsFromCandidates(ranked, opts)

	s.logSearchEvent("results ranked", id, opts).
		Int("session_count", len(results)).
		Int("message_count", countSearchResultMessages(results)).
		Msg("session search hybrid results ranked")

	return results, nil
}

func (s *Store) searchMessagesLexicalCandidates(
	id string,
	opts base.SearchMessageOptions,
	query string,
	limit int,
) searchCandidateSet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make(searchCandidateSet)
	addHits := func(sessionID string, msgs []messages.Message) {
		hits := matchingMessageHits(sessionID, msgs, query, opts)
		sort.Slice(hits, func(i, j int) bool {
			if hits[i].Message.CreatedAt.Equal(hits[j].Message.CreatedAt) {
				return hits[i].Message.ID > hits[j].Message.ID
			}
			return hits[i].Message.CreatedAt.After(hits[j].Message.CreatedAt)
		})
		for _, hit := range hits {
			if limit > 0 && len(candidates) >= limit {
				return
			}
			candidates[search.SourceIDForMessage(sessionID, hit.Message.ID)] = &searchCandidate{
				CandidateMatch: search.CandidateMatch{
					SessionID:       sessionID,
					MatchedText:     hit.MatchedText,
					MatchedToolName: hit.MatchedToolName,
					LexicalScore:    lexicalScore(hit.MatchedText, query),
					LexicalRank:     len(candidates) + 1,
					HasLexical:      true,
				},
				Message: cloneMessages([]messages.Message{hit.Message})[0],
			}
		}
	}

	if id != "" {
		addHits(id, s.messages[id])
		return candidates
	}

	sessionIDs := make([]string, 0, len(s.messages))
	for sessionID := range s.messages {
		if sessionID != opts.IgnoreSessionID {
			sessionIDs = append(sessionIDs, sessionID)
		}
	}
	sort.Strings(sessionIDs)
	for _, sessionID := range sessionIDs {
		addHits(sessionID, s.messages[sessionID])
		if limit > 0 && len(candidates) >= limit {
			break
		}
	}

	return candidates
}

func (s *Store) searchMessagesVector(
	ctx context.Context,
	id string,
	opts base.SearchMessageOptions,
	candidateLimit int,
) ([]*searchCandidate, error) {
	req := search.EmbeddingRequest{
		Model: s.vectors.Model,
		Inputs: []search.EmbeddingInput{{
			ID:         "query",
			Text:       strings.TrimSpace(opts.Query),
			SourceKind: search.SourceKindSessionMessage,
		}},
	}

	s.logSearchEvent("query embedding started", id, opts).
		Str("embedding_model", req.Model).
		Msg("session search query embedding started")

	embedding, err := s.vectors.Provider.Embed(ctx, req)
	if err != nil {
		logSafeError(s.logSearchEvent("query embedding failed", id, opts), err).
			Msg("session search query embedding failed")
		return nil, err
	}
	if err := search.ValidateEmbeddingResult(req, embedding); err != nil {
		logSafeError(s.logSearchEvent("query embedding validation failed", id, opts), err).
			Msg("session search query embedding validation failed")
		return nil, err
	}

	s.logSearchEvent("query embedding completed", id, opts).
		Int("dimensions", embedding.Dimensions).
		Str("embedding_model", strings.TrimSpace(embedding.Model)).
		Msg("session search query embedding completed")

	s.logSearchEvent("vector search started", id, opts).
		Int("limit", candidateLimit).
		Int("dimensions", embedding.Dimensions).
		Str("embedding_model", s.vectors.Model).
		Msg("session search vector retrieval started")

	result, err := s.vectors.Store.Search(ctx, search.VectorSearchRequest{
		EmbeddingModel: s.vectors.Model,
		Dimensions:     embedding.Dimensions,
		QueryVector:    embedding.Items[0].Vector,
		Limit:          candidateLimit,
		Filter: search.VectorFilter{
			SourceKind:      search.SourceKindSessionMessage,
			SessionID:       id,
			IgnoreSessionID: opts.IgnoreSessionID,
			Role:            strings.TrimSpace(string(opts.Role)),
			ToolName:        base.NormalizeMatchValue(opts.ToolName),
		},
	})
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

	candidates := s.vectorMatchesToCandidates(id, opts, result.Matches)

	s.logSearchEvent("vector matches resolved", id, opts).
		Int("match_count", len(result.Matches)).
		Int("candidate_count", len(candidates)).
		Msg("session search vector matches resolved")

	return candidates, nil
}

func (s *Store) vectorMatchesToCandidates(
	id string,
	opts base.SearchMessageOptions,
	matches []search.VectorSearchMatch,
) []*searchCandidate {
	if len(matches) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]*searchCandidate, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for idx, match := range matches {
		sessionID, messageID, ok := search.MessageRefFromSourceID(match.Record.SourceID)
		if !ok {
			continue
		}
		if _, ok := seen[match.Record.SourceID]; ok {
			continue
		}
		message, ok := findMessageByID(s.messages[sessionID], messageID)
		if !ok || !messageMatchesSearchOptions(sessionID, message, id, opts) {
			continue
		}
		row, ok := search.MessageIndexRowForVectorRecord(
			search.MessageIndexRowsFromMessage(sessionID, message),
			match.Record.ID,
		)
		if !ok || !search.MessageIndexRowMatchesSearchOptions(row, opts) {
			continue
		}

		candidates = append(candidates, &searchCandidate{
			CandidateMatch: search.CandidateMatch{
				SessionID:       sessionID,
				MatchedText:     row.Body,
				MatchedToolName: row.ToolName,
				VectorScore:     match.Score,
				VectorRank:      idx + 1,
				HasVector:       true,
			},
			Message: cloneMessages([]messages.Message{message})[0],
		})
		seen[match.Record.SourceID] = struct{}{}
	}

	return candidates
}

func searchCandidateKey(candidate *searchCandidate) string {
	if candidate == nil {
		return ""
	}

	return search.SourceIDForMessage(candidate.SessionID, candidate.Message.ID)
}

func lessSearchCandidate(left *searchCandidate, right *searchCandidate) bool {
	return compareSearchCandidates(left, right) < 0
}

func (s *Store) rerankSearchCandidates(
	ctx context.Context,
	opts base.SearchMessageOptions,
	candidates searchCandidateSet,
) []*searchCandidate {
	items := candidates.Sorted(lessSearchCandidate)
	if len(items) == 0 {
		return nil
	}

	maxCandidates := s.vectors.RerankMax
	if maxCandidates > 0 && len(items) > maxCandidates {
		items = items[:maxCandidates]
	}

	reranker := s.vectors.Reranker
	if reranker == nil {
		reranker = search.DeterministicReranker{}
	}
	rerankerName := s.rerankerName()

	s.logSearchEvent("rerank started", "", opts).
		Str("configured_reranker", rerankerName).
		Int("candidate_count", len(items)).
		Int("max_candidates", maxCandidates).
		Msg("session search rerank started")

	retrievalCandidates := make([]search.Candidate, 0, len(items))
	candidateByID := make(map[string]*searchCandidate, len(items))
	for _, candidate := range items {
		item := retrievalCandidateFromSearchCandidate(candidate)
		retrievalCandidates = append(retrievalCandidates, item)
		candidateByID[item.ID] = candidate
	}

	result, err := search.RerankWithFallback(ctx, reranker, search.DeterministicReranker{}, search.RerankRequest{
		Query:      strings.TrimSpace(opts.Query),
		Caller:     "session_search",
		SourceKind: search.SourceKindSessionMessage,
		Candidates: retrievalCandidates,
		Options: search.RerankOptions{
			LexicalDirection: search.ScoreLowerIsBetter,
			VectorDirection:  search.ScoreHigherIsBetter,
			FusedDirection:   search.ScoreHigherIsBetter,
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
		candidate := candidateByID[item.CandidateID]
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

func retrievalCandidateFromSearchCandidate(candidate *searchCandidate) search.Candidate {
	text := strings.TrimSpace(candidate.MatchedText)
	if text == "" {
		text = strings.TrimSpace(candidate.Message.Content)
	}

	return search.Candidate{
		CreatedAt:    candidate.Message.CreatedAt,
		UpdatedAt:    candidate.Message.CreatedAt,
		ID:           search.SourceIDForMessage(candidate.SessionID, candidate.Message.ID),
		SourceKind:   search.SourceKindSessionMessage,
		SessionID:    candidate.SessionID,
		Text:         text,
		LexicalScore: candidate.LexicalScore,
		VectorScore:  candidate.VectorScore,
		FusedScore:   candidate.FusedScore,
		MessageID:    candidate.Message.ID,
	}
}

func searchResultsFromCandidates(
	candidates []*searchCandidate,
	opts base.SearchMessageOptions,
) []base.SearchMessageResult {
	groups := make(map[string][]*searchCandidate)
	for _, candidate := range candidates {
		groups[candidate.SessionID] = append(groups[candidate.SessionID], candidate)
	}

	sessionIDs := make([]string, 0, len(groups))
	for sessionID := range groups {
		sessionIDs = append(sessionIDs, sessionID)
		sort.SliceStable(groups[sessionID], func(i, j int) bool {
			return compareSearchCandidates(groups[sessionID][i], groups[sessionID][j]) < 0
		})
	}
	sort.SliceStable(sessionIDs, func(i, j int) bool {
		left := groups[sessionIDs[i]][0]
		right := groups[sessionIDs[j]][0]
		leftScore := search.CandidateRankingScore(left.HasRerank, left.RerankScore, left.FusedScore)
		rightScore := search.CandidateRankingScore(right.HasRerank, right.RerankScore, right.FusedScore)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if !left.Message.CreatedAt.Equal(right.Message.CreatedAt) {
			return left.Message.CreatedAt.After(right.Message.CreatedAt)
		}
		return sessionIDs[i] < sessionIDs[j]
	})
	if opts.MaxSessions > 0 && len(sessionIDs) > opts.MaxSessions {
		sessionIDs = sessionIDs[:opts.MaxSessions]
	}

	results := make([]base.SearchMessageResult, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		sessionCandidates := groups[sessionID]
		matchCount := len(sessionCandidates)
		if opts.MaxMessagesPerSession > 0 && len(sessionCandidates) > opts.MaxMessagesPerSession {
			sessionCandidates = sessionCandidates[:opts.MaxMessagesPerSession]
		}

		hits := make([]base.SearchMessageHit, 0, len(sessionCandidates))
		var lastMatchedAt time.Time
		for _, candidate := range sessionCandidates {
			if candidate.Message.CreatedAt.After(lastMatchedAt) {
				lastMatchedAt = candidate.Message.CreatedAt
			}
			hits = append(hits, base.SearchMessageHit{
				SessionID:       candidate.SessionID,
				Message:         cloneMessages([]messages.Message{candidate.Message})[0],
				MatchedText:     candidate.MatchedText,
				MatchedToolName: candidate.MatchedToolName,
			})
		}

		results = append(results, base.SearchMessageResult{
			SessionID:     sessionID,
			LastMatchedAt: lastMatchedAt,
			MatchCount:    matchCount,
			Messages:      hits,
		})
	}

	return cloneSearchMessageResults(results)
}

func (s *Store) indexVectors(ctx context.Context, sessionID string, messages []messages.Message) error {
	records, err := s.vectorRecordsForMessages(ctx, sessionID, messages)
	if err != nil || len(records) == 0 {
		return err
	}

	return s.upsertVectorRecords(ctx, records)
}

func (s *Store) vectorRecordsForMessages(
	ctx context.Context,
	sessionID string,
	messages []messages.Message,
) ([]search.VectorRecord, error) {
	if s == nil || s.vectors == nil || len(messages) == 0 {
		return nil, nil
	}

	rows := make([]search.MessageIndexRow, 0, len(messages))
	for _, message := range messages {
		rows = append(rows, search.MessageIndexRowsFromMessage(sessionID, message)...)
	}
	inputs := search.VectorInputsFromIndexRows(rows)
	if len(inputs) == 0 {
		return nil, nil
	}

	embeddingInputs := make([]search.EmbeddingInput, 0, len(inputs))
	for _, input := range inputs {
		embeddingInputs = append(embeddingInputs, search.EmbeddingInput{
			ID:         input.ID,
			Text:       input.Text,
			SourceKind: search.SourceKindSessionMessage,
		})
	}

	req := search.EmbeddingRequest{Model: s.vectors.Model, Inputs: embeddingInputs}

	s.logVectorEvent("embedding started").
		Int("input_count", len(req.Inputs)).
		Int("message_count", len(messages)).
		Int("row_count", len(inputs)).
		Str("embedding_model", strings.TrimSpace(req.Model)).
		Str("purpose", "index_session_message_rows").
		Str("source_kind", string(search.SourceKindSessionMessage)).
		Msg("session vector embedding started")

	result, err := s.vectors.Provider.Embed(ctx, req)
	if err != nil {
		logSafeError(s.logVectorEvent("embedding failed"), err).Msg("session vector embedding failed")
		return nil, err
	}
	if err := search.ValidateEmbeddingResult(req, result); err != nil {
		logSafeError(s.logVectorEvent("embedding validation failed"), err).
			Msg("session vector embedding validation failed")
		return nil, err
	}

	s.logVectorEvent("embedding completed").
		Int("input_count", len(req.Inputs)).
		Int("message_count", len(messages)).
		Int("row_count", len(inputs)).
		Int("dimensions", result.Dimensions).
		Str("embedding_model", strings.TrimSpace(result.Model)).
		Str("purpose", "index_session_message_rows").
		Str("source_kind", string(search.SourceKindSessionMessage)).
		Msg("session vector embedding completed")

	inputByID := make(map[string]search.VectorInput, len(inputs))
	for _, input := range inputs {
		inputByID[input.ID] = input
	}

	records := make([]search.VectorRecord, 0, len(result.Items))
	for _, item := range result.Items {
		input := inputByID[item.ID]
		records = append(records, search.VectorRecord{
			CreatedAt:      input.CreatedAt,
			UpdatedAt:      input.UpdatedAt,
			ID:             item.ID,
			SourceKind:     search.SourceKindSessionMessage,
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

	return records, nil
}

func (s *Store) upsertVectorRecords(ctx context.Context, records []search.VectorRecord) error {
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

func (s *Store) deleteVectorRows(ctx context.Context, sourceIDs []string) error {
	if s == nil || s.vectors == nil || len(sourceIDs) == 0 {
		return nil
	}

	sourceIDs = base.UniqueStrings(sourceIDs)
	if len(sourceIDs) == 0 {
		return nil
	}

	req := search.VectorDeleteRequest{
		SourceKind: search.SourceKindSessionMessage,
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

func (s *Store) handleVectorStoreError(err error) error {
	if err == nil || s == nil || s.vectors == nil || !s.vectors.Required {
		return nil
	}

	return err
}

func findMessageByID(msgs []messages.Message, messageID uint) (messages.Message, bool) {
	for _, message := range msgs {
		if message.ID == messageID {
			return message, true
		}
	}

	return messages.Message{}, false
}

func messageMatchesSearchOptions(
	sessionID string,
	message messages.Message,
	id string,
	opts base.SearchMessageOptions,
) bool {
	if id != "" && sessionID != id {
		return false
	}
	if opts.IgnoreSessionID != "" && sessionID == opts.IgnoreSessionID {
		return false
	}
	if opts.Role != "" && message.Role != opts.Role {
		return false
	}

	return true
}

func compareSearchCandidates(left *searchCandidate, right *searchCandidate) int {
	return search.CompareCandidateOrder(
		search.CandidateRankingScore(left.HasRerank, left.RerankScore, left.FusedScore),
		search.CandidateRankingScore(right.HasRerank, right.RerankScore, right.FusedScore),
		left.Message.CreatedAt,
		right.Message.CreatedAt,
		left.SessionID,
		right.SessionID,
		left.Message.ID,
		right.Message.ID,
	)
}

func lexicalScore(text string, query string) float64 {
	count := strings.Count(strings.ToLower(text), strings.ToLower(query))
	if count <= 0 {
		return 0
	}

	return -float64(count)
}

func (s *Store) rerankEnabled() bool {
	return s != nil && s.vectors != nil && s.vectors.Rerank
}

func (s *Store) rerankerName() string {
	if s == nil || s.vectors == nil || s.vectors.Reranker == nil {
		return search.RerankerDeterministic
	}

	return strings.TrimSpace(strings.ToLower(s.vectors.Reranker.Name()))
}

func (s *Store) diagnosticsEnabled() bool {
	return s != nil && s.vectors != nil && s.vectors.Diagnostics
}

func searchRerankResultName(result search.RerankResult, fallback string) string {
	if name := strings.TrimSpace(strings.ToLower(result.Reranker)); name != "" {
		return name
	}

	return strings.TrimSpace(strings.ToLower(fallback))
}

func countSearchResultMessages(results []base.SearchMessageResult) int {
	var count int
	for _, result := range results {
		count += len(result.Messages)
	}

	return count
}
