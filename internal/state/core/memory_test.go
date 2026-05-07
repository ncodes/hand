package core

import (
	"testing"
	"time"

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
	require.True(t, MemoryMatchesQuery(item, MemorySearchQuery{Text: "PLAN"}))
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

func TestMemoryMatchesQuery_AppliesPromotionEvaluationFilters(t *testing.T) {
	evaluatedAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	item := MemoryItem{
		ID:                   "mem_candidate",
		Status:               MemoryStatusCandidate,
		PromotionEvaluatedAt: evaluatedAt,
	}

	require.True(t, MemoryMatchesQuery(item, MemorySearchQuery{
		Statuses:                 []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluated:       new(true),
		PromotionEvaluatedAfter:  evaluatedAt.Add(-time.Minute),
		PromotionEvaluatedBefore: evaluatedAt.Add(time.Minute),
	}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{
		Statuses:           []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluated: new(false),
	}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{
		Statuses:                 []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluatedBefore: evaluatedAt,
	}))
	require.False(t, MemoryMatchesQuery(item, MemorySearchQuery{
		Statuses:                []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluatedAfter: evaluatedAt,
	}))
	require.False(t, MemoryMatchesQuery(MemoryItem{ID: "mem_new", Status: MemoryStatusCandidate}, MemorySearchQuery{
		Statuses:           []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluated: new(true),
	}))
	require.True(t, MemoryMatchesQuery(MemoryItem{ID: "mem_new", Status: MemoryStatusCandidate}, MemorySearchQuery{
		Statuses:           []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluated: new(false),
	}))
	require.False(t, MemoryMatchesQuery(MemoryItem{ID: "mem_new", Status: MemoryStatusCandidate}, MemorySearchQuery{
		Statuses:                 []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluatedBefore: evaluatedAt.Add(time.Minute),
	}))
	require.False(t, MemoryMatchesQuery(MemoryItem{ID: "mem_new", Status: MemoryStatusCandidate}, MemorySearchQuery{
		Statuses:                []MemoryStatus{MemoryStatusCandidate},
		PromotionEvaluatedAfter: evaluatedAt.Add(-time.Minute),
	}))
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
	require.True(t, MemoryMatchesSessionQuery(MemoryItem{
		Status:   MemoryStatusActive,
		Metadata: map[string]string{"source_session_id": DefaultSessionID},
	}, SessionMemoryQuery{SessionID: DefaultSessionID}))
}

func TestMemoryItem_GuardrailSource(t *testing.T) {
	require.Equal(t, "memory:mem_one", MemoryItem{ID: " mem_one "}.GuardrailSource())
	require.Equal(t, "memory", MemoryItem{}.GuardrailSource())
}

func TestMemoryItem_CloneDeepCopiesMutableFields(t *testing.T) {
	item := MemoryItem{
		Tags:     []string{"go"},
		Metadata: map[string]string{"source": "original"},
		SourceLinks: []MemorySourceLink{{
			SessionID:  "session",
			MessageIDs: []uint{1},
			Offsets:    []int{2},
		}},
	}

	cloned := item.Clone()
	cloned.Tags[0] = "changed"
	cloned.Metadata["source"] = "changed"
	cloned.SourceLinks[0].MessageIDs[0] = 9
	cloned.SourceLinks[0].Offsets[0] = 8

	require.Equal(t, []string{"go"}, item.Tags)
	require.Equal(t, "original", item.Metadata["source"])
	require.Equal(t, []uint{1}, item.SourceLinks[0].MessageIDs)
	require.Equal(t, []int{2}, item.SourceLinks[0].Offsets)
	require.Nil(t, MemoryItem{}.Clone().Metadata)
}

func TestApplyMemoryPatch_UpdatesOnlyProvidedFieldsAndClones(t *testing.T) {
	kind := MemoryKindProcedural
	status := MemoryStatusActive
	title := "Patched title"
	text := "Patched text"
	tags := []string{"Patch"}
	sourceLinks := []MemorySourceLink{{
		SessionID:  DefaultSessionID,
		MessageIDs: []uint{2},
		Offsets:    []int{3},
	}}
	confidence := 0.8
	reflected := true
	evaluatedAt := time.Date(2026, 5, 7, 12, 0, 0, 10, time.FixedZone("offset", 3600))
	updatedAt := time.Date(2026, 5, 7, 13, 0, 0, 20, time.FixedZone("offset", 3600))

	item := MemoryItem{
		ID:         "mem_patch",
		Kind:       MemoryKindEpisodic,
		Status:     MemoryStatusCandidate,
		Title:      "Original",
		Text:       "Original text",
		Tags:       []string{"old"},
		Confidence: 0.2,
		SourceLinks: []MemorySourceLink{{
			SessionID:  "old",
			MessageIDs: []uint{1},
			Offsets:    []int{1},
		}},
	}

	patched := ApplyMemoryPatch(item, MemoryPatch{
		ID:                   item.ID,
		Kind:                 &kind,
		Status:               &status,
		Title:                &title,
		Text:                 &text,
		Tags:                 &tags,
		Metadata:             map[string]string{" kept ": "yes", " ": "ignored"},
		SourceLinks:          &sourceLinks,
		Confidence:           &confidence,
		Reflected:            &reflected,
		PromotionEvaluatedAt: &evaluatedAt,
	}, updatedAt)

	require.Equal(t, MemoryKindProcedural, patched.Kind)
	require.Equal(t, MemoryStatusActive, patched.Status)
	require.Equal(t, "Patched title", patched.Title)
	require.Equal(t, "Patched text", patched.Text)
	require.Equal(t, []string{"Patch"}, patched.Tags)
	require.Equal(t, map[string]string{"kept": "yes"}, patched.Metadata)
	require.Equal(t, []uint{2}, patched.SourceLinks[0].MessageIDs)
	require.Equal(t, []int{3}, patched.SourceLinks[0].Offsets)
	require.Equal(t, 0.8, patched.Confidence)
	require.True(t, patched.Reflected)
	require.Equal(t, evaluatedAt.UTC(), patched.PromotionEvaluatedAt)
	require.Equal(t, updatedAt.UTC(), patched.UpdatedAt)

	tags[0] = "mutated"
	sourceLinks[0].MessageIDs[0] = 99
	sourceLinks[0].Offsets[0] = 98
	require.Equal(t, []string{"Patch"}, patched.Tags)
	require.Equal(t, []uint{2}, patched.SourceLinks[0].MessageIDs)
	require.Equal(t, []int{3}, patched.SourceLinks[0].Offsets)
	require.Equal(t, MemoryKindEpisodic, item.Kind)
}

func TestApplyMemoryPatch_MergesMetadataIntoExistingMap(t *testing.T) {
	patched := ApplyMemoryPatch(MemoryItem{
		Metadata: map[string]string{"existing": "kept"},
	}, MemoryPatch{
		Metadata: map[string]string{"next": "value"},
	}, time.Date(2026, 5, 7, 13, 0, 0, 0, time.UTC))

	require.Equal(t, map[string]string{
		"existing": "kept",
		"next":     "value",
	}, patched.Metadata)
}

func TestApplyMemoryPatch_EmptyPatchOnlyUpdatesTimestamp(t *testing.T) {
	createdAt := time.Date(2026, 5, 7, 11, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	item := MemoryItem{
		ID:         "mem_empty_patch",
		Kind:       MemoryKindSemantic,
		Status:     MemoryStatusActive,
		Title:      "Stable title",
		Text:       "Stable text",
		Tags:       []string{"stable"},
		Metadata:   map[string]string{"source": "kept"},
		Confidence: 0.7,
		CreatedAt:  createdAt,
	}

	patched := ApplyMemoryPatch(item, MemoryPatch{ID: item.ID}, updatedAt)

	require.Equal(t, item.ID, patched.ID)
	require.Equal(t, item.Kind, patched.Kind)
	require.Equal(t, item.Status, patched.Status)
	require.Equal(t, item.Title, patched.Title)
	require.Equal(t, item.Text, patched.Text)
	require.Equal(t, item.Tags, patched.Tags)
	require.Equal(t, item.Metadata, patched.Metadata)
	require.Equal(t, item.Confidence, patched.Confidence)
	require.Equal(t, createdAt, patched.CreatedAt)
	require.Equal(t, updatedAt, patched.UpdatedAt)
}

func TestNormalizeMemoryIDs_TrimsDedupesAndSorts(t *testing.T) {
	require.Equal(t, []string{"mem_a", "mem_b"}, NormalizeMemoryIDs([]string{" mem_b ", "", "mem_a", "mem_b"}))
}

func TestNormalizeMemoryTags_TrimsLowercasesDedupesAndSorts(t *testing.T) {
	require.Equal(t, []string{"go", "memory"}, NormalizeMemoryTags([]string{" Memory ", "", "go", "GO"}))
}

func TestContainsAllMemoryTags_TrimsLowercasesAndRequiresAllQueryTags(t *testing.T) {
	itemTags := []string{" Go ", "Memory"}

	require.True(t, ContainsAllMemoryTags(itemTags, nil))
	require.True(t, ContainsAllMemoryTags(itemTags, []string{"go", " memory "}))
	require.False(t, ContainsAllMemoryTags(itemTags, []string{"go", "missing"}))
}

func TestSimpleMemoryScore(t *testing.T) {
	item := MemoryItem{
		Title: "Plan preference",
		Text:  "Use a plan for complex tasks",
	}

	require.Equal(t, 0.0, SimpleMemoryScore(item, ""))
	require.Equal(t, 2.0, SimpleMemoryScore(item, "preference"))
	require.Equal(t, 1.0, SimpleMemoryScore(item, "complex"))
	require.Equal(t, 3.0, SimpleMemoryScore(item, "plan"))
}

func TestMemoryKindAndStatusStrings_FilterEmptyValues(t *testing.T) {
	require.Equal(t, []string{"semantic", "episodic"}, MemoryKindStrings([]MemoryKind{
		MemoryKindSemantic,
		"",
		MemoryKindEpisodic,
	}))
	require.Equal(t, []string{"candidate", "active"}, MemoryStatusStrings([]MemoryStatus{
		MemoryStatusCandidate,
		"",
		MemoryStatusActive,
	}))
}

func TestMemoryLikePattern_EscapesWildcardsAndBackslashes(t *testing.T) {
	require.Equal(t, `%100\%\_ready\\now%`, MemoryLikePattern(`100%_ready\now`))
}
