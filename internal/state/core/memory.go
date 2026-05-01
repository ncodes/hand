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
	SessionID     string
	MessageIDs    []uint
	Offsets       []int
	SummaryID     string
	CreatedBy     string
	CreatedReason string
}

type MemoryItem struct {
	ID          string
	Kind        MemoryKind
	Status      MemoryStatus
	Title       string
	Text        string
	Tags        []string
	Metadata    map[string]string
	SourceLinks []MemorySourceLink
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (item MemoryItem) GuardrailSource() string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return "memory:" + id
	}
	return "memory"
}

type MemorySearchQuery struct {
	Text     string
	Kinds    []MemoryKind
	Statuses []MemoryStatus
	Tags     []string
	Limit    int
	MaxChars int
}

type MemorySearchHit struct {
	Item  MemoryItem
	Score float64
}

type MemorySearchResult struct {
	Hits []MemorySearchHit
}

type MemoryDeleteRequest struct {
	ID     string
	Reason string
}

type MemoryStore interface {
	SearchMemory(context.Context, MemorySearchQuery) (MemorySearchResult, error)
	UpsertMemory(context.Context, MemoryItem) (MemoryItem, error)
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

func MemoryMatchesQuery(item MemoryItem, query MemorySearchQuery) bool {
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
	if len(query.Tags) > 0 && !ContainsAllMemoryTags(item.Tags, query.Tags) {
		return false
	}

	text := strings.TrimSpace(strings.ToLower(query.Text))
	if text == "" {
		return true
	}

	return strings.Contains(strings.ToLower(item.Title), text) || strings.Contains(strings.ToLower(item.Text), text)
}

func SimpleMemoryScore(item MemoryItem, query string) float64 {
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

func ContainsAllMemoryTags(itemTags []string, queryTags []string) bool {
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

func MemoryKindStrings(kinds []MemoryKind) []string {
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		value := strings.TrimSpace(string(kind))
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func MemoryStatusStrings(statuses []MemoryStatus) []string {
	values := make([]string, 0, len(statuses))
	for _, status := range statuses {
		value := strings.TrimSpace(string(status))
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func MemoryLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(value) + "%"
}
