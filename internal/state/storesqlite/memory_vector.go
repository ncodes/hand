package storesqlite

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	statememory "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
)

func (s *Store) memoryVectorSearchEnabled(query statememory.MemorySearchQuery) bool {
	return s != nil && s.vectors != nil && strings.TrimSpace(query.Text) != ""
}

func (s *Store) searchMemoryHybrid(
	ctx context.Context,
	query statememory.MemorySearchQuery,
) (statememory.MemorySearchResult, error) {
	resultLimit := search.MemoryResultLimit(query.Limit)
	candidateLimit := search.MemoryCandidateLimit(resultLimit)

	records, err := s.searchMemoryRecords(ctx, query, candidateLimit)
	if err != nil {
		return statememory.MemorySearchResult{}, err
	}
	lexicalHits, err := memorySearchRecordsToHits(records)
	if err != nil {
		return statememory.MemorySearchResult{}, err
	}

	vectorHits, err := s.searchMemoryVector(ctx, query, candidateLimit)
	if err != nil {
		if requiredErr := s.handleVectorStoreError(err); requiredErr != nil {
			return statememory.MemorySearchResult{}, requiredErr
		}
		vectorHits = nil
	}

	return search.RerankMemoryHits(ctx, query, mergeMemoryHits(lexicalHits, vectorHits), search.MemoryRerankOptions{
		Reranker:      s.memoryReranker,
		MaxCandidates: candidateLimit,
		Limit:         resultLimit,
	})
}

func (s *Store) searchMemoryVector(
	ctx context.Context,
	query statememory.MemorySearchQuery,
	limit int,
) ([]statememory.MemorySearchHit, error) {
	req := search.EmbeddingRequest{
		Model:        strings.TrimSpace(s.vectors.Model),
		Relationship: "query_vector_for_memory_item_retrieval",
		Target:       "memory_item_vectors",
		Inputs: []search.EmbeddingInput{{
			ID:         "query",
			Text:       strings.TrimSpace(query.Text),
			SourceKind: search.SourceKindMemoryItem,
		}},
	}

	embedding, err := s.vectors.Provider.Embed(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := search.ValidateEmbeddingResult(req, embedding); err != nil {
		return nil, err
	}

	sourceIDs, filtered, err := s.memoryVectorSourceIDs(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if filtered && len(sourceIDs) == 0 {
		return nil, nil
	}
	filterTags := memoryVectorFilterTags(query)
	filterTagGroups := memoryVectorFilterTagGroups(query)

	result, err := s.vectors.Store.Search(ctx, search.VectorSearchRequest{
		EmbeddingModel: strings.TrimSpace(s.vectors.Model),
		Dimensions:     embedding.Dimensions,
		QueryVector:    embedding.Items[0].Vector,
		Limit:          limit,
		Filter: search.VectorFilter{
			SourceKind: search.SourceKindMemoryItem,
			SourceIDs:  sourceIDs,
			Tags:       filterTags,
			TagGroups:  filterTagGroups,
		},
	})
	if err != nil {
		return nil, err
	}

	return s.memoryVectorMatchesToHits(ctx, query, result.Matches)
}

func (s *Store) memoryVectorSourceIDs(
	ctx context.Context,
	query statememory.MemorySearchQuery,
	limit int,
) ([]string, bool, error) {
	if !memoryQueryNeedsSourceIDFilter(query) {
		return nil, false, nil
	}

	filterQuery := query
	filterQuery.Text = ""
	records, err := s.searchMemoryRecords(ctx, filterQuery, limit)
	if err != nil || len(records) == 0 {
		return nil, true, err
	}

	sourceIDs := make([]string, 0, len(records))
	for _, record := range records {
		sourceIDs = append(sourceIDs, search.StableMemoryItemID(record.ID))
	}
	sort.Strings(sourceIDs)
	return sourceIDs, true, nil
}

func (s *Store) memoryVectorMatchesToHits(
	ctx context.Context,
	query statememory.MemorySearchQuery,
	matches []search.VectorSearchMatch,
) ([]statememory.MemorySearchHit, error) {
	if len(matches) == 0 {
		return nil, nil
	}

	memoryIDs := make([]string, 0, len(matches))
	for _, match := range matches {
		memoryID, ok := search.MemoryIDFromSourceID(match.Record.SourceID)
		if ok {
			memoryIDs = append(memoryIDs, memoryID)
		}
	}
	if len(memoryIDs) == 0 {
		return nil, nil
	}

	records, err := s.memoryModelsByID(ctx, memoryIDs)
	if err != nil {
		return nil, err
	}

	hits := make([]statememory.MemorySearchHit, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	filterQuery := query
	filterQuery.Text = ""
	for _, match := range matches {
		memoryID, ok := search.MemoryIDFromSourceID(match.Record.SourceID)
		if !ok {
			continue
		}
		if _, ok := seen[memoryID]; ok {
			continue
		}
		record, ok := records[memoryID]
		if !ok {
			continue
		}
		item, err := memoryModelToItem(record)
		if err != nil {
			return nil, err
		}
		if !statememory.MemoryMatchesQuery(item, filterQuery) {
			continue
		}
		hits = append(hits, statememory.MemorySearchHit{Item: item.Clone(), Score: match.Score})
		seen[memoryID] = struct{}{}
	}

	return hits, nil
}

func (s *Store) memoryModelsByID(
	ctx context.Context,
	ids []string,
) (map[string]memoryItemModel, error) {
	ids = statememory.NormalizeMemoryIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}

	var records []memoryItemModel
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&records).Error; err != nil {
		return nil, err
	}

	byID := make(map[string]memoryItemModel, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	return byID, nil
}

func (s *Store) indexMemoryVector(ctx context.Context, item statememory.MemoryItem) error {
	if s == nil || s.vectors == nil {
		return nil
	}

	text := memoryVectorText(item)
	if text == "" {
		return nil
	}

	req := search.EmbeddingRequest{
		Model:        strings.TrimSpace(s.vectors.Model),
		Relationship: "memory_item_to_memory_vector_index",
		Target:       "memory_item_vectors",
		Inputs: []search.EmbeddingInput{{
			ID:         search.StableMemoryItemID(item.ID),
			Text:       text,
			SourceKind: search.SourceKindMemoryItem,
		}},
	}

	result, err := s.vectors.Provider.Embed(ctx, req)
	if err != nil {
		return err
	}
	if err := search.ValidateEmbeddingResult(req, result); err != nil {
		return err
	}
	if len(result.Items) != 1 {
		return errors.New("memory embedding result count must be one")
	}

	embedding := result.Items[0]
	return s.vectors.Store.Upsert(ctx, []search.VectorRecord{{
		ID:             embedding.ID,
		SourceKind:     search.SourceKindMemoryItem,
		SourceID:       search.StableMemoryItemID(item.ID),
		SessionID:      memoryVectorSessionID(item),
		Tags:           memoryVectorTags(item),
		EmbeddingModel: result.Model,
		ContentHash:    embedding.ContentHash,
		Vector:         embedding.Vector,
		Dimensions:     result.Dimensions,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}})
}

func (s *Store) deleteMemoryVector(ctx context.Context, memoryID string) error {
	if s == nil || s.vectors == nil {
		return nil
	}

	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return nil
	}

	return s.vectors.Store.Delete(ctx, search.VectorDeleteRequest{
		SourceKind: search.SourceKindMemoryItem,
		SourceIDs:  []string{search.StableMemoryItemID(memoryID)},
	})
}

func memorySearchRecordsToHits(
	records []memorySearchRecord,
) ([]statememory.MemorySearchHit, error) {
	hits := make([]statememory.MemorySearchHit, 0, len(records))
	for _, record := range records {
		item, err := memoryModelToItem(record.model())
		if err != nil {
			return nil, err
		}
		hits = append(hits, statememory.MemorySearchHit{
			Item:  item.Clone(),
			Score: record.Score,
		})
	}
	return hits, nil
}

func mergeMemoryHits(
	lexicalHits []statememory.MemorySearchHit,
	vectorHits []statememory.MemorySearchHit,
) []statememory.MemorySearchHit {
	byID := make(map[string]statememory.MemorySearchHit, len(lexicalHits)+len(vectorHits))
	for _, hit := range lexicalHits {
		id := strings.TrimSpace(hit.Item.ID)
		if id != "" {
			byID[id] = hit
		}
	}
	for _, hit := range vectorHits {
		id := strings.TrimSpace(hit.Item.ID)
		if id == "" {
			continue
		}
		existing, ok := byID[id]
		if !ok || hit.Score > existing.Score {
			byID[id] = hit
		}
	}

	hits := make([]statememory.MemorySearchHit, 0, len(byID))
	for _, hit := range byID {
		hits = append(hits, hit)
	}
	sortMemoryHits(hits)
	return hits
}

func sortMemoryHits(hits []statememory.MemorySearchHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if !hits[i].Item.UpdatedAt.Equal(hits[j].Item.UpdatedAt) {
			return hits[i].Item.UpdatedAt.After(hits[j].Item.UpdatedAt)
		}
		return hits[i].Item.ID < hits[j].Item.ID
	})
}

func memoryQueryNeedsSourceIDFilter(query statememory.MemorySearchQuery) bool {
	return len(query.IDs) > 0 ||
		query.PromotionEvaluated != nil ||
		!query.PromotionEvaluatedBefore.IsZero() ||
		!query.PromotionEvaluatedAfter.IsZero()
}

func memoryVectorFilterTags(query statememory.MemorySearchQuery) []string {
	tags := make([]string, 0, 4+len(query.Tags))
	if sessionID := strings.TrimSpace(query.SessionID); sessionID != "" {
		tags = append(tags, memoryVectorTag("memory_session", sessionID))
	}
	if len(query.Kinds) == 1 {
		tags = append(tags, memoryVectorTag("memory_kind", string(query.Kinds[0])))
	}
	if len(query.Statuses) == 0 {
		tags = append(tags, memoryVectorTag("memory_status", string(statememory.MemoryStatusActive)))
	} else if len(query.Statuses) == 1 {
		tags = append(tags, memoryVectorTag("memory_status", string(query.Statuses[0])))
	}
	for _, tag := range query.Tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, memoryVectorTag("memory_tag", tag))
		}
	}
	if query.Reflected != nil {
		tags = append(tags, memoryVectorTag("memory_reflected", fmt.Sprint(*query.Reflected)))
	}

	return search.NormalizeVectorTags(tags)
}

func memoryVectorFilterTagGroups(query statememory.MemorySearchQuery) [][]string {
	groups := make([][]string, 0, 2)
	if len(query.Kinds) > 1 {
		group := make([]string, 0, len(query.Kinds))
		for _, kind := range query.Kinds {
			if value := strings.TrimSpace(string(kind)); value != "" {
				group = append(group, memoryVectorTag("memory_kind", value))
			}
		}
		groups = append(groups, group)
	}
	if len(query.Statuses) > 1 {
		group := make([]string, 0, len(query.Statuses))
		for _, status := range query.Statuses {
			if value := strings.TrimSpace(string(status)); value != "" {
				group = append(group, memoryVectorTag("memory_status", value))
			}
		}
		groups = append(groups, group)
	}

	return search.NormalizeVectorTagGroups(groups)
}

func memoryVectorTags(item statememory.MemoryItem) []string {
	tags := make([]string, 0, 4+len(item.Tags))
	if kind := strings.TrimSpace(string(item.Kind)); kind != "" {
		tags = append(tags, memoryVectorTag("memory_kind", kind))
	}
	if status := strings.TrimSpace(string(item.Status)); status != "" {
		tags = append(tags, memoryVectorTag("memory_status", status))
	}
	if sessionID := memoryVectorSessionID(item); sessionID != "" {
		tags = append(tags, memoryVectorTag("memory_session", sessionID))
	}
	tags = append(tags, memoryVectorTag("memory_reflected", fmt.Sprint(item.Reflected)))
	for _, tag := range item.Tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, memoryVectorTag("memory_tag", tag))
		}
	}

	return search.NormalizeVectorTags(tags)
}

func memoryVectorTag(key string, value string) string {
	return strings.TrimSpace(key) + ":" + strings.TrimSpace(value)
}

func memoryVectorText(item statememory.MemoryItem) string {
	return strings.TrimSpace(strings.Join([]string{item.Title, item.Text}, "\n"))
}

func memoryVectorSessionID(item statememory.MemoryItem) string {
	if sessionID := strings.TrimSpace(item.Metadata["source_session_id"]); sessionID != "" {
		return sessionID
	}
	for _, link := range item.SourceLinks {
		if sessionID := strings.TrimSpace(link.SessionID); sessionID != "" {
			return sessionID
		}
	}

	return ""
}
