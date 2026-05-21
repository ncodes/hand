package core

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

type MemoryKind string

const (
	MemoryKindPinned     MemoryKind = "pinned"
	MemoryKindSemantic   MemoryKind = "semantic"
	MemoryKindEpisodic   MemoryKind = "episodic"
	MemoryKindProcedural MemoryKind = "procedural"
)

type MemoryStatus string

const (
	MemoryStatusCandidate  MemoryStatus = "candidate"
	MemoryStatusActive     MemoryStatus = "active"
	MemoryStatusSuperseded MemoryStatus = "superseded"
	MemoryStatusDeleted    MemoryStatus = "deleted"
)

type MemorySourceLink struct {
	SessionID         string
	MessageIDs        []uint
	Offsets           []int
	SummaryID         string
	CreatedBy         string
	CreatedReason     string
	SourceProfile     string
	SourcePersonality string
	ParentSessionID   string
	ChildSessionID    string
	RunID             string
	StateMode         string
	SourceTrigger     string
}

type MemoryItem struct {
	ID                   string
	Kind                 MemoryKind
	Status               MemoryStatus
	Title                string
	Text                 string
	Tags                 []string
	Metadata             map[string]string
	SourceLinks          []MemorySourceLink
	Confidence           float64
	Reflected            bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	PromotionEvaluatedAt time.Time
}

type MemoryPatch struct {
	ID                   string
	Kind                 *MemoryKind
	Status               *MemoryStatus
	Title                *string
	Text                 *string
	Tags                 *[]string
	Metadata             map[string]string
	SourceLinks          *[]MemorySourceLink
	Confidence           *float64
	Reflected            *bool
	PromotionEvaluatedAt *time.Time
}

func (item MemoryItem) GuardrailSource() string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return "memory:" + id
	}
	return "memory"
}

type MemorySearchQuery struct {
	Text                     string
	SessionID                string
	RerankerUseCase          string
	IDs                      []string
	Kinds                    []MemoryKind
	Statuses                 []MemoryStatus
	Tags                     []string
	Limit                    int
	MaxChars                 int
	Reflected                *bool
	PromotionEvaluated       *bool
	PromotionEvaluatedBefore time.Time
	PromotionEvaluatedAfter  time.Time
}

const (
	MemoryRerankerUseCaseDefault            = "memory_search"
	MemoryRerankerUseCaseTurnRetrieval      = "memory_retrieval"
	MemoryRerankerUseCaseToolSearch         = "memory_tool_search"
	MemoryRerankerUseCasePinned             = "memory_pinned"
	MemoryRerankerUseCasePromotion          = "memory_promotion"
	MemoryRerankerUseCaseReflection         = "memory_reflection"
	MemoryRerankerUseCaseEpisodicExtraction = "memory_episodic_extraction"
)

type SessionMemoryQuery struct {
	SessionID string
	Kinds     []MemoryKind
	Statuses  []MemoryStatus
	Limit     int
}

type MemorySearchHit struct {
	Item         MemoryItem
	Score        float64
	LexicalScore float64
	VectorScore  float64
}

type MemorySearchResult struct {
	Hits []MemorySearchHit
}

type SessionMemoriesResult struct {
	Items []MemoryItem
}

type MemoryDeleteRequest struct {
	ID     string
	Reason string
}

type MemoryStore interface {
	SearchMemory(context.Context, MemorySearchQuery) (MemorySearchResult, error)
	ListSessionMemories(context.Context, SessionMemoryQuery) (SessionMemoriesResult, error)
	UpsertMemory(context.Context, MemoryItem) (MemoryItem, error)
	PatchMemory(context.Context, MemoryPatch) (MemoryItem, error)
	DeleteMemory(context.Context, MemoryDeleteRequest) error
}

func (item MemoryItem) Clone() MemoryItem {
	item.Tags = append([]string(nil), item.Tags...)
	if len(item.Metadata) > 0 {
		metadata := make(map[string]string, len(item.Metadata))
		maps.Copy(metadata, item.Metadata)
		item.Metadata = metadata
	}
	if len(item.SourceLinks) > 0 {
		links := make([]MemorySourceLink, 0, len(item.SourceLinks))
		for _, link := range item.SourceLinks {
			link.MessageIDs = append([]uint(nil), link.MessageIDs...)
			link.Offsets = append([]int(nil), link.Offsets...)
			links = append(links, link)
		}
		item.SourceLinks = links
	}
	return item
}

func ApplyMemoryPatch(item MemoryItem, patch MemoryPatch, updatedAt time.Time) MemoryItem {
	if patch.Kind != nil {
		item.Kind = *patch.Kind
	}
	if patch.Status != nil {
		item.Status = *patch.Status
	}
	if patch.Title != nil {
		item.Title = *patch.Title
	}
	if patch.Text != nil {
		item.Text = *patch.Text
	}
	if patch.Tags != nil {
		item.Tags = append([]string(nil), (*patch.Tags)...)
	}
	if len(patch.Metadata) > 0 {
		if item.Metadata == nil {
			item.Metadata = make(map[string]string, len(patch.Metadata))
		}
		for key, value := range patch.Metadata {
			if key = strings.TrimSpace(key); key != "" {
				item.Metadata[key] = value
			}
		}
	}
	if patch.SourceLinks != nil {
		item.SourceLinks = append([]MemorySourceLink(nil), (*patch.SourceLinks)...)
	}
	if patch.Confidence != nil {
		item.Confidence = *patch.Confidence
	}
	if patch.Reflected != nil {
		item.Reflected = *patch.Reflected
	}
	if patch.PromotionEvaluatedAt != nil {
		item.PromotionEvaluatedAt = patch.PromotionEvaluatedAt.UTC()
	}
	item.UpdatedAt = updatedAt.UTC()

	return item.Clone()
}

func NormalizeMemoryTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	results := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		results = append(results, tag)
	}
	sort.Strings(results)
	return results
}

func NormalizeMemoryIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	results := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		results = append(results, id)
	}
	sort.Strings(results)
	return results
}

func CheckMemoryMatchesQuery(item MemoryItem, query MemorySearchQuery) bool {
	if sessionID := strings.TrimSpace(query.SessionID); sessionID != "" && !CheckMemoryBelongsToSession(item, sessionID) {
		return false
	}
	if ids := NormalizeMemoryIDs(query.IDs); len(ids) > 0 && !slices.Contains(ids, strings.TrimSpace(item.ID)) {
		return false
	}
	if len(query.Kinds) > 0 && !slices.Contains(query.Kinds, item.Kind) {
		return false
	}
	if len(query.Statuses) > 0 {
		if !slices.Contains(query.Statuses, item.Status) {
			return false
		}
	} else if item.Status != MemoryStatusActive {
		return false
	}
	if len(query.Tags) > 0 && !HasAllMemoryTags(item.Tags, query.Tags) {
		return false
	}
	if query.Reflected != nil && item.Reflected != *query.Reflected {
		return false
	}
	if query.PromotionEvaluated != nil {
		evaluated := !item.PromotionEvaluatedAt.IsZero()
		if evaluated != *query.PromotionEvaluated {
			return false
		}
	}
	if !query.PromotionEvaluatedBefore.IsZero() {
		if item.PromotionEvaluatedAt.IsZero() || !item.PromotionEvaluatedAt.Before(query.PromotionEvaluatedBefore) {
			return false
		}
	}
	if !query.PromotionEvaluatedAfter.IsZero() {
		if item.PromotionEvaluatedAt.IsZero() || !item.PromotionEvaluatedAt.After(query.PromotionEvaluatedAfter) {
			return false
		}
	}

	text := strings.TrimSpace(strings.ToLower(query.Text))
	if text == "" {
		return true
	}

	return strings.Contains(strings.ToLower(item.Title), text) || strings.Contains(strings.ToLower(item.Text), text)
}

func CheckMemoryMatchesSessionQuery(item MemoryItem, query SessionMemoryQuery) bool {
	sessionID := strings.TrimSpace(query.SessionID)
	if sessionID == "" || !CheckMemoryBelongsToSession(item, sessionID) {
		return false
	}
	if len(query.Kinds) > 0 && !slices.Contains(query.Kinds, item.Kind) {
		return false
	}
	if len(query.Statuses) > 0 {
		return slices.Contains(query.Statuses, item.Status)
	}

	return item.Status == MemoryStatusActive
}

func CheckMemoryBelongsToSession(item MemoryItem, sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}

	for _, link := range item.SourceLinks {
		if strings.TrimSpace(link.SessionID) == sessionID {
			return true
		}
	}

	return strings.TrimSpace(item.Metadata["source_session_id"]) == sessionID
}

func GetSimpleMemoryScore(item MemoryItem, query string) float64 {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return 0
	}

	score := 0.0
	if strings.Contains(strings.ToLower(item.Title), query) {
		score += 2
	}
	if strings.Contains(strings.ToLower(item.Text), query) {
		score++
	}
	return score
}

func HasAllMemoryTags(itemTags []string, queryTags []string) bool {
	tags := make(map[string]struct{}, len(itemTags))
	for _, tag := range itemTags {
		tags[strings.TrimSpace(strings.ToLower(tag))] = struct{}{}
	}
	for _, tag := range queryTags {
		if _, ok := tags[strings.TrimSpace(strings.ToLower(tag))]; !ok {
			return false
		}
	}
	return true
}

func MemoryKindsToStrings(kinds []MemoryKind) []string {
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		value := strings.TrimSpace(string(kind))
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func MemoryStatusesToStrings(statuses []MemoryStatus) []string {
	values := make([]string, 0, len(statuses))
	for _, status := range statuses {
		value := strings.TrimSpace(string(status))
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func MemoryValueToLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(value) + "%"
}
