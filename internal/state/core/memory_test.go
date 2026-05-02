package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryMatchesQuery_AppliesStatusKindTagsAndText(t *testing.T) {
	item := MemoryItem{
		ID:     "mem_plan",
		Kind:   MemoryKindSemantic,
		Status: MemoryStatusActive,
		Title:  "Plan preference",
		Text:   "Use focused plans",
		Tags:   []string{"go", "planning"},
	}

	require.True(t, MemoryMatchesQuery(item, MemorySearchQuery{
		Text:  "focused",
		IDs:   []string{" mem_plan "},
		Kinds: []MemoryKind{MemoryKindSemantic},
		Tags:  []string{"planning"},
	}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{IDs: []string{"mem_other"}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Statuses: []MemoryStatus{MemoryStatusCandidate}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Kinds: []MemoryKind{MemoryKindProcedural}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Tags: []string{"missing"}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Text: "missing"}))
}

func TestNormalizeMemoryIDs_TrimsDedupesAndSorts(t *testing.T) {
	require.Equal(t, []string{"mem_a", "mem_b"}, NormalizeMemoryIDs([]string{" mem_b ", "", "mem_a", "mem_b"}))
}
