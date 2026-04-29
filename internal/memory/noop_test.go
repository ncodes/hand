package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopProvider_BehaviorAndHooks(t *testing.T) {
	guardrails := &fakeGuardrails{}
	tracer := &fakeTracer{}
	provider := NewNoopProvider(Options{
		Guardrails:    guardrails,
		Observability: fakeObservability{tracer: tracer},
	})

	caps, err := provider.Capabilities(context.Background())
	require.NoError(t, err)
	require.True(t, caps.SupportsObservability)
	require.False(t, caps.SupportsSearch)

	items, err := provider.LoadPinned(context.Background(), SearchQuery{Text: "hello"})
	require.NoError(t, err)
	require.Nil(t, items)
	require.Equal(t, 1, guardrails.validateSearchCalls)
	require.Contains(t, tracer.events, "memory.pinned.noop")

	result, err := provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.NoError(t, err)
	require.Empty(t, result.Hits)
	require.Equal(t, 2, guardrails.validateSearchCalls)
	require.Contains(t, tracer.events, "memory.search.noop")

	upserted, err := provider.Upsert(context.Background(), MemoryItem{Text: "hello", Tags: []string{"test"}})
	require.NoError(t, err)
	require.Equal(t, "hello", upserted.Text)
	require.Equal(t, []string{"test"}, upserted.Tags)
	require.Equal(t, 1, guardrails.validateWriteCalls)
	require.Equal(t, 1, guardrails.safetyScanCalls)

	require.NoError(t, provider.Delete(context.Background(), DeleteRequest{ID: "mem_123"}))
	require.Equal(t, 1, guardrails.validateDeleteCalls)
	require.NoError(t, provider.Close())
}

func TestNoopProvider_ConfigureObservability(t *testing.T) {
	provider := NewNoopProvider(Options{})
	tracer := &fakeTracer{}

	require.NoError(t, provider.ConfigureObservability(fakeObservability{tracer: tracer}))

	_, err := provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.NoError(t, err)
	require.Contains(t, tracer.events, "memory.search.noop")
}

func TestNoopProvider_ReturnsGuardrailErrors(t *testing.T) {
	searchErr := errors.New("search blocked")
	writeErr := errors.New("write blocked")
	deleteErr := errors.New("delete blocked")
	provider := NewNoopProvider(Options{
		Guardrails: &fakeGuardrails{
			searchErr: searchErr,
			writeErr:  writeErr,
			deleteErr: deleteErr,
		},
	})

	_, err := provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, searchErr)

	_, err = provider.LoadPinned(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, searchErr)

	_, err = provider.Upsert(context.Background(), MemoryItem{Text: "hello"})
	require.ErrorIs(t, err, writeErr)

	err = provider.Delete(context.Background(), DeleteRequest{ID: "mem_123"})
	require.ErrorIs(t, err, deleteErr)
}
