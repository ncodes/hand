package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInMemoryProvider_SearchWriteDeleteAndObservability(t *testing.T) {
	guardrails := &fakeGuardrails{redactText: "redacted"}
	logger := &fakeLogger{}
	tracer := &fakeTracer{}
	provider := NewInMemoryProvider(Options{
		Guardrails:    guardrails,
		Observability: fakeObservability{logger: logger, tracer: tracer},
	})

	caps, err := provider.Capabilities(context.Background())
	require.NoError(t, err)
	require.True(t, caps.SupportsSearch)
	require.True(t, caps.SupportsWrite)
	require.True(t, caps.SupportsDelete)

	item, err := provider.Upsert(context.Background(), MemoryItem{
		Kind:   KindSemantic,
		Status: StatusActive,
		Title:  "Go preference",
		Text:   "Use focused tests",
		Tags:   []string{"go"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, item.ID)
	require.Equal(t, 1, guardrails.validateWriteCalls)
	require.Equal(t, 1, guardrails.safetyScanCalls)
	require.Contains(t, tracer.events, "memory.upsert.completed")

	result, err := provider.Search(context.Background(), SearchQuery{
		Text:     "focused",
		Kinds:    []Kind{KindSemantic},
		Statuses: []Status{StatusActive},
		Tags:     []string{"go"},
		Limit:    5,
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "redacted", result.Hits[0].Item.Text)
	require.Equal(t, 1.0, result.Hits[0].Score)
	require.Equal(t, 1, guardrails.redactCalls)
	require.NotEmpty(t, logger.debug)
	require.Contains(t, tracer.events, "memory.search.completed")

	require.NoError(t, provider.Delete(context.Background(), DeleteRequest{ID: item.ID}))
	require.NoError(t, provider.Delete(context.Background(), DeleteRequest{ID: "missing"}))
	result, err = provider.Search(context.Background(), SearchQuery{Text: "focused"})
	require.NoError(t, err)
	require.Empty(t, result.Hits)
	require.NoError(t, provider.Close())
}

func TestInMemoryProvider_LoadPinnedAndValidation(t *testing.T) {
	provider := NewInMemoryProvider(Options{})

	_, err := provider.Upsert(context.Background(), MemoryItem{Kind: KindPinned, Status: StatusActive, Text: "always remember"})
	require.NoError(t, err)
	_, err = provider.Upsert(context.Background(), MemoryItem{Kind: KindSemantic, Status: StatusActive, Text: "semantic remember"})
	require.NoError(t, err)

	items, err := provider.LoadPinned(context.Background(), SearchQuery{Text: "remember"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, KindPinned, items[0].Kind)

	err = provider.Delete(context.Background(), DeleteRequest{})
	require.EqualError(t, err, "memory id is required")
}

func TestInMemoryProvider_ConfigureObservability(t *testing.T) {
	provider := NewInMemoryProvider(Options{})
	tracer := &fakeTracer{}

	require.NoError(t, provider.ConfigureObservability(fakeObservability{tracer: tracer}))

	_, err := provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.NoError(t, err)
	require.Contains(t, tracer.events, "memory.search.completed")
}

func TestInMemoryProvider_SearchRanksBeforeLimiting(t *testing.T) {
	provider := NewInMemoryProvider(Options{})

	_, err := provider.Upsert(context.Background(), MemoryItem{
		ID:     "mem_text_only",
		Kind:   KindSemantic,
		Status: StatusActive,
		Title:  "Other",
		Text:   "alpha in body",
	})
	require.NoError(t, err)
	_, err = provider.Upsert(context.Background(), MemoryItem{
		ID:     "mem_title_and_text",
		Kind:   KindSemantic,
		Status: StatusActive,
		Title:  "alpha title",
		Text:   "alpha in body",
	})
	require.NoError(t, err)

	result, err := provider.Search(context.Background(), SearchQuery{Text: "alpha", Limit: 1})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_title_and_text", result.Hits[0].Item.ID)
	require.Equal(t, 3.0, result.Hits[0].Score)
}

func TestInMemoryProvider_SearchFiltersStatusesTagsKindsAndText(t *testing.T) {
	provider := NewInMemoryProvider(Options{})

	_, err := provider.Upsert(context.Background(), MemoryItem{
		ID:     "mem_active",
		Kind:   KindSemantic,
		Status: StatusActive,
		Title:  "Alpha title",
		Text:   "body",
		Tags:   []string{"Go", "Style"},
	})
	require.NoError(t, err)
	_, err = provider.Upsert(context.Background(), MemoryItem{
		ID:     "mem_candidate",
		Kind:   KindProcedural,
		Status: StatusCandidate,
		Text:   "alpha body",
		Tags:   []string{"go"},
	})
	require.NoError(t, err)
	_, err = provider.Upsert(context.Background(), MemoryItem{
		ID:     "mem_superseded",
		Kind:   KindSemantic,
		Status: StatusSuperseded,
		Text:   "alpha body",
		Tags:   []string{"go"},
	})
	require.NoError(t, err)

	result, err := provider.Search(context.Background(), SearchQuery{Text: "alpha", Tags: []string{"go"}})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_active", result.Hits[0].Item.ID)

	result, err = provider.Search(context.Background(), SearchQuery{
		Text:     "alpha",
		Kinds:    []Kind{KindSemantic},
		Tags:     []string{"go", "style"},
		Statuses: []Status{StatusActive},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_active", result.Hits[0].Item.ID)

	result, err = provider.Search(context.Background(), SearchQuery{
		Text:     "alpha",
		Statuses: []Status{StatusCandidate},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_candidate", result.Hits[0].Item.ID)

	result, err = provider.Search(context.Background(), SearchQuery{
		Text:     "alpha",
		Statuses: []Status{StatusSuperseded},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_superseded", result.Hits[0].Item.ID)

	result, err = provider.Search(context.Background(), SearchQuery{Text: "alpha", Tags: []string{"missing"}})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	result, err = provider.Search(context.Background(), SearchQuery{Text: "missing"})
	require.NoError(t, err)
	require.Empty(t, result.Hits)
}

func TestInMemoryProvider_SearchDefaultLimitTruncatesAndSortsByUpdatedAt(t *testing.T) {
	provider := NewInMemoryProvider(Options{})
	base := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		base.Add(time.Second),
		base.Add(2 * time.Second),
		base.Add(3 * time.Second),
	}
	provider.now = func() time.Time {
		next := times[0]
		times = times[1:]
		return next
	}

	for _, id := range []string{"mem_a", "mem_b", "mem_c"} {
		_, err := provider.Upsert(context.Background(), MemoryItem{ID: id, Status: StatusActive})
		require.NoError(t, err)
	}

	result, err := provider.Search(context.Background(), SearchQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, result.Hits, 2)
	require.Equal(t, "mem_c", result.Hits[0].Item.ID)
	require.Equal(t, "mem_b", result.Hits[1].Item.ID)
}

func TestInMemoryProvider_SearchSortsTiesByID(t *testing.T) {
	provider := NewInMemoryProvider(Options{})
	provider.now = func() time.Time {
		return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	}

	_, err := provider.Upsert(context.Background(), MemoryItem{ID: "mem_b", Status: StatusActive})
	require.NoError(t, err)
	_, err = provider.Upsert(context.Background(), MemoryItem{ID: "mem_a", Status: StatusActive})
	require.NoError(t, err)

	result, err := provider.Search(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Len(t, result.Hits, 2)
	require.Equal(t, "mem_a", result.Hits[0].Item.ID)
	require.Equal(t, "mem_b", result.Hits[1].Item.ID)
}

func TestInMemoryProvider_SearchMaxCharsTruncatesRunes(t *testing.T) {
	provider := NewInMemoryProvider(Options{})

	_, err := provider.Upsert(context.Background(), MemoryItem{
		ID:     "mem_unicode",
		Status: StatusActive,
		Text:   "hello",
	})
	require.NoError(t, err)

	result, err := provider.Search(context.Background(), SearchQuery{Text: "hello", MaxChars: 2})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "he", result.Hits[0].Item.Text)
}

func TestInMemoryProvider_UpsertDefaultsAndCloneIsolation(t *testing.T) {
	provider := NewInMemoryProvider(Options{})
	item := MemoryItem{
		ID:       "  mem_clone  ",
		Tags:     []string{"one"},
		Metadata: map[string]string{"key": "value"},
		SourceLinks: []SourceLink{{
			SessionID:  "session",
			MessageIDs: []uint{1},
			Offsets:    []int{2},
		}},
	}

	stored, err := provider.Upsert(context.Background(), item)
	require.NoError(t, err)
	require.Equal(t, "mem_clone", stored.ID)
	require.Equal(t, StatusCandidate, stored.Status)
	require.False(t, stored.CreatedAt.IsZero())
	require.False(t, stored.UpdatedAt.IsZero())

	item.Tags[0] = "changed"
	item.Metadata["key"] = "changed"
	item.SourceLinks[0].MessageIDs[0] = 99
	item.SourceLinks[0].Offsets[0] = 99
	stored.Tags[0] = "changed"
	stored.Metadata["key"] = "changed"
	stored.SourceLinks[0].MessageIDs[0] = 99

	result, err := provider.Search(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	result, err = provider.Search(context.Background(), SearchQuery{Statuses: []Status{StatusCandidate}})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, []string{"one"}, result.Hits[0].Item.Tags)
	require.Equal(t, map[string]string{"key": "value"}, result.Hits[0].Item.Metadata)
	require.Equal(t, []uint{1}, result.Hits[0].Item.SourceLinks[0].MessageIDs)
	require.Equal(t, []int{2}, result.Hits[0].Item.SourceLinks[0].Offsets)
}

func TestInMemoryProvider_ReturnsGuardrailErrors(t *testing.T) {
	searchErr := errors.New("search blocked")
	writeErr := errors.New("write blocked")
	safetyErr := errors.New("safety blocked")
	deleteErr := errors.New("delete blocked")
	redactErr := errors.New("redact blocked")

	provider := NewInMemoryProvider(Options{Guardrails: &fakeGuardrails{searchErr: searchErr}})
	_, err := provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, searchErr)
	_, err = provider.LoadPinned(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, searchErr)

	provider = NewInMemoryProvider(Options{Guardrails: &fakeGuardrails{writeErr: writeErr}})
	_, err = provider.Upsert(context.Background(), MemoryItem{Text: "hello"})
	require.ErrorIs(t, err, writeErr)

	provider = NewInMemoryProvider(Options{Guardrails: &fakeGuardrails{safetyErr: safetyErr}})
	_, err = provider.Upsert(context.Background(), MemoryItem{Text: "hello"})
	require.ErrorIs(t, err, safetyErr)

	provider = NewInMemoryProvider(Options{Guardrails: &fakeGuardrails{deleteErr: deleteErr}})
	err = provider.Delete(context.Background(), DeleteRequest{ID: "mem_123"})
	require.ErrorIs(t, err, deleteErr)

	provider = NewInMemoryProvider(Options{Guardrails: &fakeGuardrails{redactErr: redactErr}})
	_, err = provider.Upsert(context.Background(), MemoryItem{Status: StatusActive, Text: "hello"})
	require.NoError(t, err)
	_, err = provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, redactErr)
}
