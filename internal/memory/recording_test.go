package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryProvider_RecordSemanticMemoryStoresCandidate(t *testing.T) {
	guardrails := &fakeGuardrails{}
	provider := defaultMemoryTestProvider(t, Options{Guardrails: guardrails})

	item, err := provider.RecordSemanticMemory(context.Background(), SemanticRecord{Item: MemoryItem{
		Title: "User prefers concise commits",
		Text:  "The user prefers concise commit messages without co-author trailers.",
		Metadata: map[string]string{
			"memory_importance":   "high",
			"memory_granularity":  "summary",
			"preference_scope":    "commit_messages",
			"source_session_id":   "default",
			"confidence_reason":   "explicit user instruction",
			"derived_memory_type": "preference",
		},
	}})

	require.NoError(t, err)
	require.NotEmpty(t, item.ID)
	require.Equal(t, KindSemantic, item.Kind)
	require.Equal(t, StatusCandidate, item.Status)
	require.Equal(t, "commit_messages", item.Metadata["preference_scope"])
	require.Equal(t, 1, guardrails.validateWriteCalls)
	require.Equal(t, 1, guardrails.safetyScanCalls)

	result, err := provider.Search(context.Background(), SearchQuery{
		Kinds:    []Kind{KindSemantic},
		Statuses: []Status{StatusCandidate},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, item.ID, result.Hits[0].Item.ID)
}

func TestMemoryProvider_RecordProceduralMemoryStoresCandidate(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})

	item, err := provider.RecordProceduralMemory(context.Background(), ProceduralRecord{Item: MemoryItem{
		Title: "Testing workflow",
		Text:  "Run focused package tests after changing memory provider behavior.",
		SourceLinks: []SourceLink{{
			SessionID:     "default",
			CreatedBy:     "memory_extract",
			CreatedReason: "derived_procedure",
		}},
	}})

	require.NoError(t, err)
	require.Equal(t, KindProcedural, item.Kind)
	require.Equal(t, StatusCandidate, item.Status)
}

func TestMemoryProvider_RecordSemanticMemoryRejectsMalformedCandidates(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})
	valid := MemoryItem{
		Kind: KindSemantic,
		Text: "Remember this",
		Metadata: map[string]string{
			"source_session_id": "default",
		},
	}

	tests := []struct {
		name string
		item MemoryItem
		err  string
	}{
		{
			name: "status",
			item: func() MemoryItem {
				item := valid
				item.Status = StatusActive
				return item
			}(),
			err: "memory candidate must be stored as candidate",
		},
		{
			name: "content",
			item: MemoryItem{Kind: KindSemantic, Metadata: map[string]string{
				"source_session_id": "default",
			}},
			err: "memory candidate text or title is required",
		},
		{
			name: "provenance",
			item: MemoryItem{Kind: KindSemantic, Text: "remember"},
			err:  "memory candidate source provenance is required",
		},
		{
			name: "low importance",
			item: func() MemoryItem {
				item := valid
				item.Metadata = map[string]string{
					"source_session_id": "default",
					"memory_importance": "low",
				}
				return item
			}(),
			err: "low_importance_candidate",
		},
		{
			name: "execution detail",
			item: func() MemoryItem {
				item := valid
				item.Metadata = map[string]string{
					"source_session_id":  "default",
					"memory_granularity": "execution_detail",
				}
				return item
			}(),
			err: "execution_detail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.RecordSemanticMemory(context.Background(), SemanticRecord{Item: tt.item})
			require.EqualError(t, err, tt.err)
		})
	}
}

func TestMemoryProvider_RecordSemanticMemoryDoesNotApplySupersessionMetadata(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})
	previous, err := provider.Upsert(context.Background(), MemoryItem{
		ID:     "mem_previous_preference",
		Kind:   KindSemantic,
		Status: StatusActive,
		Text:   "The user prefers verbose commit messages.",
	})
	require.NoError(t, err)

	item, err := provider.RecordSemanticMemory(context.Background(), SemanticRecord{Item: MemoryItem{
		ID:    "mem_new_preference",
		Title: "Commit message preference",
		Text:  "The user prefers concise commit messages.",
		Metadata: map[string]string{
			"source_session_id":      "default",
			"supersedes_memory_id":   previous.ID,
			"supersession_reason":    "newer explicit correction",
			"memory_importance":      "high",
			"preference_confidence":  "explicit",
			"preference_durability":  "durable",
			"memory_granularity":     "summary",
			"semantic_memory_domain": "preference",
		},
	}})
	require.NoError(t, err)
	require.Equal(t, "mem_new_preference", item.ID)
	require.Equal(t, previous.ID, item.Metadata["supersedes_memory_id"])
	require.Equal(t, "newer explicit correction", item.Metadata["supersession_reason"])

	result, err := provider.Search(context.Background(), SearchQuery{
		IDs:      []string{previous.ID},
		Statuses: []Status{StatusActive},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, StatusActive, result.Hits[0].Item.Status)
}

func TestMemoryProvider_RecordProceduralMemoryAlwaysUsesFreshGeneratedID(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})

	first, err := provider.RecordProceduralMemory(context.Background(), ProceduralRecord{Item: MemoryItem{
		Title: "Memory testing workflow",
		Text:  "Run all package tests after memory changes.",
		Metadata: map[string]string{
			"source_session_id": "default",
		},
	}})
	require.NoError(t, err)

	second, err := provider.RecordProceduralMemory(context.Background(), ProceduralRecord{Item: MemoryItem{
		Title: "Memory testing workflow",
		Text:  "Run focused package tests after memory provider changes.",
		Metadata: map[string]string{
			"source_session_id": "default",
		},
	}})
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)

	result, err := provider.Search(context.Background(), SearchQuery{
		IDs:      []string{second.ID},
		Statuses: []Status{StatusCandidate},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "Run focused package tests after memory provider changes.", result.Hits[0].Item.Text)
}

func TestMemoryProvider_RecordSemanticMemoryPreservesIdentityMetadataWithoutSearch(t *testing.T) {
	manager := &recordingMemoryManager{
		fakeMemoryManager: fakeMemoryManager{searchErr: errors.New("search should not be called")},
	}
	provider := &MemoryProvider{manager: manager}

	candidate, err := provider.RecordSemanticMemory(context.Background(), SemanticRecord{Item: MemoryItem{
		Kind:  KindSemantic,
		Title: "Commit message preference",
		Text:  "The user prefers concise commit messages.",
		Metadata: map[string]string{
			"source_session_id": "default",
			"dedupe_key":        "commit preference",
		},
	}})
	require.NoError(t, err)
	require.NotEmpty(t, candidate.ID)
	require.Equal(t, "commit preference", candidate.Metadata["dedupe_key"])
	require.Empty(t, candidate.Metadata["supersedes_memory_id"])
	require.Len(t, manager.upsertItems, 1)
	require.Equal(t, candidate.ID, manager.upsertItems[0].ID)
}

func TestMemoryProvider_RecordSemanticMemoryReturnsCandidateWriteError(t *testing.T) {
	manager := &recordingMemoryManager{
		fakeMemoryManager: fakeMemoryManager{searchResult: SearchResult{Hits: []SearchHit{{
			Item: MemoryItem{
				ID:     "mem_existing_preference",
				Kind:   KindSemantic,
				Status: StatusActive,
				Text:   "The user prefers verbose commit messages.",
			},
		}}}},
		upsertErr: errors.New("upsert failed"),
	}
	provider := &MemoryProvider{manager: manager}

	_, err := provider.RecordSemanticMemory(context.Background(), SemanticRecord{Item: MemoryItem{
		ID:    "mem_replacement_preference",
		Title: "Commit message preference",
		Text:  "The user prefers concise commit messages.",
		Metadata: map[string]string{
			"source_session_id":    "default",
			"supersedes_memory_id": "mem_existing_preference",
		},
	}})

	require.EqualError(t, err, "upsert failed")
	require.Len(t, manager.upsertItems, 1)
	require.Equal(t, "mem_replacement_preference", manager.upsertItems[0].ID)
	require.Equal(t, StatusCandidate, manager.upsertItems[0].Status)
}

func TestMemoryProvider_CandidateHelpersCoverValidationAndFallbacks(t *testing.T) {
	err := validateMemoryCandidate(MemoryItem{
		Kind:   KindEpisodic,
		Status: StatusCandidate,
		Text:   "Remember this.",
		Metadata: map[string]string{
			"source_session_id": "default",
		},
	})
	require.EqualError(t, err, "memory candidate kind must be semantic or procedural")

	require.Equal(t, "mem_unknown_", getKindAwareMemoryIDPrefix(""))
}
