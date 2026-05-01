package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryMatchesQuery_AppliesStatusKindTagsAndText(t *testing.T) {
	item := MemoryItem{
		Kind:   MemoryKindSemantic,
		Status: MemoryStatusActive,
		Title:  "Plan preference",
		Text:   "Use focused plans",
		Tags:   []string{"go", "planning"},
	}

	require.True(t, MemoryMatchesQuery(item, MemorySearchQuery{
		Text:  "focused",
		Kinds: []MemoryKind{MemoryKindSemantic},
		Tags:  []string{"planning"},
	}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Statuses: []MemoryStatus{MemoryStatusCandidate}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Kinds: []MemoryKind{MemoryKindProcedural}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Tags: []string{"missing"}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Text: "missing"}))
}
