package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryMatchesQuery_AppliesStatusKindTagsAndText(t *testing.T) {
	item := MemoryItem{
		ID:          "mem_plan",
		Kind:        MemoryKindSemantic,
		Status:      MemoryStatusActive,
		Title:       "Plan preference",
		Text:        "Use focused plans",
		Tags:        []string{"go", "planning"},
		SourceLinks: []MemorySourceLink{{SessionID: DefaultSessionID}},
		Reflected:   true,
	}

	require.True(t, MemoryMatchesQuery(item, MemorySearchQuery{
		Text:      "focused",
		SessionID: DefaultSessionID,
		IDs:       []string{" mem_plan "},
		Kinds:     []MemoryKind{MemoryKindSemantic},
		Tags:      []string{"planning"},
		Reflected: new(true),
	}))
	require.True(t, MemoryMatchesQuery(item, MemorySearchQuery{}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{SessionID: "other"}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{IDs: []string{"mem_other"}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Statuses: []MemoryStatus{MemoryStatusCandidate}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Kinds: []MemoryKind{MemoryKindProcedural}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Tags: []string{"missing"}}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Reflected: new(false)}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{Text: "missing"}))
	require.False(t, MemoryMatchesQuery(MemoryItem{ID: "mem_candidate", Status: MemoryStatusCandidate}, MemorySearchQuery{}))
}

func TestMemoryMatchesSessionQuery_AppliesSessionKindAndStatus(t *testing.T) {
	item := MemoryItem{
		ID:     "mem_episode",
		Kind:   MemoryKindEpisodic,
		Status: MemoryStatusCandidate,
		Metadata: map[string]string{
			"source_session_id": DefaultSessionID,
		},
		SourceLinks: []MemorySourceLink{{
			SessionID: "linked",
		}},
	}

	require.True(t, MemoryBelongsToSession(item, DefaultSessionID))
	require.True(t, MemoryBelongsToSession(MemoryItem{
		SourceLinks: []MemorySourceLink{{SessionID: " " + DefaultSessionID + " "}},
	}, DefaultSessionID))
	require.False(t, MemoryBelongsToSession(item, ""))
	require.False(t, MemoryBelongsToSession(item, "other"))

	require.True(t, MemoryMatchesSessionQuery(item, SessionMemoryQuery{
		SessionID: DefaultSessionID,
		Kinds:     []MemoryKind{MemoryKindEpisodic},
		Statuses:  []MemoryStatus{MemoryStatusCandidate},
	}))
	require.False(t, MemoryMatchesSessionQuery(item, SessionMemoryQuery{SessionID: "other"}))
	require.False(t, MemoryMatchesSessionQuery(item, SessionMemoryQuery{
		SessionID: DefaultSessionID,
		Kinds:     []MemoryKind{MemoryKindSemantic},
	}))
	require.False(t, MemoryMatchesSessionQuery(item, SessionMemoryQuery{
		SessionID: DefaultSessionID,
		Statuses:  []MemoryStatus{MemoryStatusActive},
	}))
	require.False(t, MemoryMatchesSessionQuery(item, SessionMemoryQuery{SessionID: DefaultSessionID}))
}

func TestNormalizeMemoryIDs_TrimsDedupesAndSorts(t *testing.T) {
	require.Equal(t, []string{"mem_a", "mem_b"}, NormalizeMemoryIDs([]string{" mem_b ", "", "mem_a", "mem_b"}))
}
