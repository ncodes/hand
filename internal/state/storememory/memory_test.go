package storememory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	statememory "github.com/wandxy/hand/internal/state/core"
)

func TestMemoryStore_SearchWriteDeleteAndSourceLinks(t *testing.T) {
	store := NewStore()
	createdAt := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)

	item, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{
		ID:        "  mem_one  ",
		Kind:      statememory.MemoryKindSemantic,
		Status:    statememory.MemoryStatusActive,
		Title:     "Go preference",
		Text:      "Use focused tests",
		Tags:      []string{"Go", "Style"},
		CreatedAt: createdAt,
		Metadata:  map[string]string{"project": "hand"},
		SourceLinks: []statememory.MemorySourceLink{{
			SessionID:     "session",
			MessageIDs:    []uint{1},
			Offsets:       []int{2},
			SummaryID:     "summary",
			CreatedBy:     "reflection",
			CreatedReason: "preference",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, "mem_one", item.ID)
	require.Equal(t, createdAt, item.CreatedAt)
	require.False(t, item.UpdatedAt.IsZero())

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text: "focused",
		Tags: []string{"go", "style"},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, 1.0, result.Hits[0].Score)
	require.Equal(t, []uint{1}, result.Hits[0].Item.SourceLinks[0].MessageIDs)
	require.Equal(t, []int{2}, result.Hits[0].Item.SourceLinks[0].Offsets)
	require.Equal(t, "summary", result.Hits[0].Item.SourceLinks[0].SummaryID)

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		IDs: []string{" mem_one "},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_one", result.Hits[0].Item.ID)

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		IDs: []string{"mem_missing"},
	})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	require.NoError(t, store.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{ID: item.ID}))

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{Text: "focused"})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text:     "focused",
		Statuses: []statememory.MemoryStatus{statememory.MemoryStatusDeleted},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, statememory.MemoryStatusDeleted, result.Hits[0].Item.Status)
}

func TestMemoryStore_DefaultsToCandidateAndActiveOnlySearch(t *testing.T) {
	store := NewStore()

	item, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{Text: "candidate"})
	require.NoError(t, err)
	require.NotEmpty(t, item.ID)
	require.Equal(t, statememory.MemoryStatusCandidate, item.Status)

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{Text: "candidate"})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text:     "candidate",
		Statuses: []statememory.MemoryStatus{statememory.MemoryStatusCandidate},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
}

func TestMemoryStore_SearchOrdersAndLimitsResults(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)

	for _, item := range []statememory.MemoryItem{
		{ID: "mem_low", Status: statememory.MemoryStatusActive, Text: "plan"},
		{ID: "mem_high_old", Status: statememory.MemoryStatusActive, Title: "plan", Text: "plan"},
		{ID: "mem_high_new", Status: statememory.MemoryStatusActive, Title: "plan", Text: "plan"},
		{ID: "mem_high_same_b", Status: statememory.MemoryStatusActive, Title: "plan", Text: "plan"},
		{ID: "mem_high_same_a", Status: statememory.MemoryStatusActive, Title: "plan", Text: "plan"},
	} {
		_, err := store.UpsertMemory(context.Background(), item)
		require.NoError(t, err)
	}

	store.mu.Lock()
	for id, item := range store.memoryItems {
		switch id {
		case "mem_low":
			item.UpdatedAt = now.Add(4 * time.Hour)
		case "mem_high_new":
			item.UpdatedAt = now.Add(3 * time.Hour)
		case "mem_high_old":
			item.UpdatedAt = now
		case "mem_high_same_a", "mem_high_same_b":
			item.UpdatedAt = now.Add(time.Hour)
		}
		store.memoryItems[id] = item
	}
	store.mu.Unlock()

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text:  "plan",
		Limit: 4,
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 4)
	require.Equal(t, []string{
		"mem_high_new",
		"mem_high_same_a",
		"mem_high_same_b",
		"mem_high_old",
	}, []string{
		result.Hits[0].Item.ID,
		result.Hits[1].Item.ID,
		result.Hits[2].Item.ID,
		result.Hits[3].Item.ID,
	})
}

func TestMemoryStore_ReranksBeforeLimiting(t *testing.T) {
	store := NewStore()

	for _, item := range []statememory.MemoryItem{
		{
			ID:     "mem_broad",
			Status: statememory.MemoryStatusActive,
			Text:   "plan",
		},
		{
			ID:         "mem_confident",
			Status:     statememory.MemoryStatusActive,
			Text:       "plan",
			Confidence: 1,
		},
		{
			ID:     "mem_other",
			Status: statememory.MemoryStatusActive,
			Text:   "plan",
		},
	} {
		_, err := store.UpsertMemory(context.Background(), item)
		require.NoError(t, err)
	}

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text:  "plan",
		Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_confident", result.Hits[0].Item.ID)
}

func TestMemoryStore_UpdatePreservesCreatedAtAndClonesItems(t *testing.T) {
	store := &Store{}
	createdAt := time.Date(2026, 4, 30, 11, 0, 0, 0, time.UTC)

	first, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{
		ID:        "mem_update",
		Status:    statememory.MemoryStatusActive,
		Title:     "first",
		Tags:      []string{"old"},
		Metadata:  map[string]string{"project": "hand"},
		CreatedAt: createdAt,
		SourceLinks: []statememory.MemorySourceLink{{
			MessageIDs: []uint{1},
			Offsets:    []int{2},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, createdAt, first.CreatedAt)

	first.Tags[0] = "mutated"
	first.Metadata["project"] = "mutated"
	first.SourceLinks[0].MessageIDs[0] = 99
	first.SourceLinks[0].Offsets[0] = 99

	second, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{
		ID:     "mem_update",
		Status: statememory.MemoryStatusActive,
		Title:  "second",
		Tags:   []string{"new"},
	})
	require.NoError(t, err)
	require.Equal(t, createdAt, second.CreatedAt)
	require.True(t, second.UpdatedAt.After(second.CreatedAt))

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{Text: "second"})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, []string{"new"}, result.Hits[0].Item.Tags)
	require.Empty(t, result.Hits[0].Item.Metadata)
	require.Empty(t, result.Hits[0].Item.SourceLinks)
	require.NotEqual(t, "mutated", result.Hits[0].Item.Title)

	result.Hits[0].Item.Tags[0] = "mutated"
	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{Text: "second"})
	require.NoError(t, err)
	require.Equal(t, []string{"new"}, result.Hits[0].Item.Tags)
}

func TestMemoryStore_NilReceiverAndValidationErrors(t *testing.T) {
	var nilStore *Store

	_, err := nilStore.SearchMemory(context.Background(), statememory.MemorySearchQuery{})
	require.EqualError(t, err, "store is required")

	_, err = nilStore.UpsertMemory(context.Background(), statememory.MemoryItem{})
	require.EqualError(t, err, "store is required")

	err = nilStore.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{ID: "mem"})
	require.EqualError(t, err, "store is required")

	store := NewStore()
	require.EqualError(t, store.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{}), "memory id is required")
	require.NoError(t, store.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{ID: "missing"}))
}
