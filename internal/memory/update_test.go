package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryProvider_UpdateSupersedesActiveMemoryAndPromotesReplacement(t *testing.T) {
	tracer := &fakeTracer{}
	provider := defaultMemoryTestProvider(t, Options{
		Observability: fakeObservability{tracer: tracer},
	})
	old := MemoryItem{
		ID:         "mem_old",
		Kind:       KindSemantic,
		Status:     StatusActive,
		Title:      "Old status preference",
		Text:       "Use the old project codename in status reports.",
		Confidence: 1,
		Metadata: map[string]string{
			"source_session_id": "default",
		},
	}
	_, err := provider.Upsert(context.Background(), old)
	require.NoError(t, err)

	result, err := provider.Update(context.Background(), UpdateRequest{
		ID:     old.ID,
		Reason: "user corrected codename",
		Replacement: MemoryItem{
			Kind:       KindSemantic,
			Title:      "Updated status preference",
			Text:       "Use ember-lake as the project codename in status reports.",
			Confidence: 1,
			Metadata: map[string]string{
				"source_session_id": "default",
			},
		},
	})

	require.NoError(t, err)
	require.True(t, result.Lifecycle.Decision.Approved)
	require.Equal(t, StatusSuperseded, result.Previous.Status)
	require.Equal(t, StatusActive, result.Replacement.Status)
	require.Equal(t, result.Replacement.ID, result.Previous.Metadata[supersededByMemoryIDMetadataKey])
	require.Equal(t, old.ID, result.Replacement.Metadata[supersedesMemoryIDMetadataKey])
	require.Contains(t, tracer.events, "memory.update.completed")

	active, err := provider.Search(context.Background(), SearchQuery{
		IDs:      []string{old.ID},
		Statuses: []Status{StatusActive},
	})
	require.NoError(t, err)
	require.Empty(t, active.Hits)

	superseded, err := provider.Search(context.Background(), SearchQuery{
		IDs:      []string{old.ID},
		Statuses: []Status{StatusSuperseded},
	})
	require.NoError(t, err)
	require.Len(t, superseded.Hits, 1)

	defaultSearch, err := provider.Search(context.Background(), SearchQuery{Text: "old project codename"})
	require.NoError(t, err)
	require.Empty(t, defaultSearch.Hits)
}

func TestMemoryProvider_UpdateRestoresPreviousWhenReplacementPromotionIsRejected(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})
	old := MemoryItem{
		ID:         "mem_old",
		Kind:       KindSemantic,
		Status:     StatusActive,
		Title:      "Old preference",
		Text:       "Use the old status preference.",
		Confidence: 1,
		Metadata: map[string]string{
			"source_session_id": "default",
		},
	}
	_, err := provider.Upsert(context.Background(), old)
	require.NoError(t, err)

	result, err := provider.Update(context.Background(), UpdateRequest{
		ID: old.ID,
		Replacement: MemoryItem{
			Kind:       KindSemantic,
			Title:      "Weak replacement",
			Text:       "Use an uncertain replacement.",
			Confidence: 0.1,
			Metadata: map[string]string{
				"source_session_id": "default",
			},
		},
	})

	require.NoError(t, err)
	require.False(t, result.Lifecycle.Decision.Approved)

	active, err := provider.Search(context.Background(), SearchQuery{
		IDs:      []string{old.ID},
		Statuses: []Status{StatusActive},
	})
	require.NoError(t, err)
	require.Len(t, active.Hits, 1)
	require.Equal(t, old.ID, active.Hits[0].Item.ID)
}

func TestMemoryProvider_UpdateRecordsProceduralReplacementAndInheritsSession(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})
	old := MemoryItem{
		ID:         "mem_old",
		Kind:       KindSemantic,
		Status:     StatusActive,
		Title:      "Old workflow",
		Text:       "Use the old review workflow.",
		Confidence: 1,
		Metadata: map[string]string{
			"source_session_id": "default",
		},
	}
	_, err := provider.Upsert(context.Background(), old)
	require.NoError(t, err)

	result, err := provider.Update(context.Background(), UpdateRequest{
		ID: old.ID,
		Replacement: MemoryItem{
			Kind:       KindProcedural,
			Title:      "Updated workflow",
			Text:       "Group logs by subsystem, identify anomalies, explain the timeline, and propose fixes.",
			Confidence: 1,
		},
	})

	require.NoError(t, err)
	require.True(t, result.Lifecycle.Decision.Approved)
	require.Equal(t, KindProcedural, result.Replacement.Kind)
	require.Equal(t, "default", result.Replacement.Metadata["source_session_id"])
	require.Equal(t, old.ID, result.Replacement.Metadata[supersedesMemoryIDMetadataKey])
}

func TestMemoryProvider_UpdateValidationAndLoadErrors(t *testing.T) {
	var missing *MemoryProvider
	_, err := missing.Update(context.Background(), UpdateRequest{ID: "mem_old"})
	require.EqualError(t, err, "memory provider is required")

	provider := &MemoryProvider{}
	_, err = provider.Update(context.Background(), UpdateRequest{ID: "mem_old"})
	require.EqualError(t, err, "memory provider is required")

	provider = &MemoryProvider{manager: fakeMemoryManager{}}
	_, err = provider.Update(context.Background(), UpdateRequest{})
	require.EqualError(t, err, "memory id is required")

	_, err = provider.Update(context.Background(), UpdateRequest{ID: "mem_missing"})
	require.EqualError(t, err, "memory item not found")

	managerErr := errors.New("load failed")
	provider = &MemoryProvider{manager: fakeMemoryManager{searchErr: managerErr}}
	_, err = provider.Update(context.Background(), UpdateRequest{ID: "mem_old"})
	require.ErrorIs(t, err, managerErr)
}

func TestMemoryProvider_UpdateRejectsUnsupportedReplacementKind(t *testing.T) {
	provider := &MemoryProvider{
		manager: fakeMemoryManager{
			searchResult: SearchResult{Hits: []SearchHit{{
				Item: MemoryItem{ID: "mem_old", Kind: KindSemantic, Status: StatusActive, Title: "Old"},
			}}},
		},
	}

	_, err := provider.Update(context.Background(), UpdateRequest{
		ID: "mem_old",
		Replacement: MemoryItem{
			Kind:       KindPinned,
			Title:      "Pinned replacement",
			Confidence: 1,
		},
	})

	require.EqualError(t, err, "replacement memory kind must be semantic or procedural")
}

func TestMemoryProvider_UpdateReturnsReplacementRecordErrors(t *testing.T) {
	provider := &MemoryProvider{
		manager: fakeMemoryManager{
			searchResult: SearchResult{Hits: []SearchHit{{
				Item: MemoryItem{ID: "mem_old", Kind: KindSemantic, Status: StatusActive, Title: "Old"},
			}}},
		},
	}

	_, err := provider.Update(context.Background(), UpdateRequest{
		ID: "mem_old",
		Replacement: MemoryItem{
			Kind:       KindSemantic,
			Title:      "Replacement without provenance",
			Confidence: 1,
		},
	})

	require.EqualError(t, err, "memory candidate source provenance is required")
}

func TestMemoryProvider_UpdateReturnsSupersedeErrors(t *testing.T) {
	managerErr := errors.New("supersede failed")
	manager := &recordingMemoryManager{
		fakeMemoryManager: fakeMemoryManager{
			searchResult: SearchResult{Hits: []SearchHit{{
				Item: MemoryItem{
					ID:     "mem_old",
					Kind:   KindSemantic,
					Status: StatusActive,
					Title:  "Old",
					Metadata: map[string]string{
						"source_session_id": "default",
					},
				},
			}}},
		},
		patchErrs: []error{managerErr},
	}
	provider := &MemoryProvider{manager: manager}

	_, err := provider.Update(context.Background(), UpdateRequest{
		ID: "mem_old",
		Replacement: MemoryItem{
			Kind:       KindSemantic,
			Title:      "Replacement",
			Confidence: 1,
		},
	})

	require.ErrorIs(t, err, managerErr)
	require.Len(t, manager.upsertItems, 1)
	require.Len(t, manager.patches, 1)
}

func TestMemoryProvider_UpdateRestoresPreviousWhenPromotionReturnsError(t *testing.T) {
	manager := &recordingMemoryManager{
		searchResults: []SearchResult{{
			Hits: []SearchHit{{
				Item: MemoryItem{
					ID:     "mem_old",
					Kind:   KindSemantic,
					Status: StatusActive,
					Title:  "Old",
					Metadata: map[string]string{
						"source_session_id": "default",
					},
				},
			}},
		}},
	}
	provider := &MemoryProvider{manager: manager}

	_, err := provider.Update(context.Background(), UpdateRequest{
		ID: "mem_old",
		Replacement: MemoryItem{
			Kind:       KindSemantic,
			Title:      "Replacement",
			Confidence: 1,
		},
	})

	require.EqualError(t, err, "memory item not found")
	require.Len(t, manager.upsertItems, 2)
	require.Equal(t, "mem_old", manager.upsertItems[1].ID)
	require.Equal(t, StatusActive, manager.upsertItems[1].Status)
}
