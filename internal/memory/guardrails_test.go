package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGuardrailsHelpers_PassThroughWhenUnset(t *testing.T) {
	item := MemoryItem{ID: "mem_123", Text: "remember this"}

	require.NoError(t, validateSearch(context.Background(), nil, SearchQuery{Text: "remember"}))
	require.NoError(t, validateWrite(context.Background(), nil, item))
	require.NoError(t, validateDelete(context.Background(), nil, DeleteRequest{ID: "mem_123"}))

	redacted, err := redactItem(context.Background(), nil, item)
	require.NoError(t, err)
	require.Equal(t, item, redacted)
}

func TestGuardrailsHelpers_CallConfiguredGuardrails(t *testing.T) {
	guardrails := &fakeGuardrails{redactText: "redacted"}

	require.NoError(t, validateSearch(context.Background(), guardrails, SearchQuery{Text: "remember"}))
	require.NoError(t, validateWrite(context.Background(), guardrails, MemoryItem{Text: "remember"}))
	require.NoError(t, validateDelete(context.Background(), guardrails, DeleteRequest{ID: "mem_123"}))
	redacted, err := redactItem(context.Background(), guardrails, MemoryItem{Text: "remember"})
	require.NoError(t, err)

	require.Equal(t, 1, guardrails.validateSearchCalls)
	require.Equal(t, 1, guardrails.validateWriteCalls)
	require.Equal(t, 1, guardrails.safetyScanCalls)
	require.Equal(t, 1, guardrails.validateDeleteCalls)
	require.Equal(t, 1, guardrails.redactCalls)
	require.Equal(t, "redacted", redacted.Text)
}

func TestGuardrailsHelpers_ReturnGuardrailErrors(t *testing.T) {
	searchErr := errors.New("search blocked")
	writeErr := errors.New("write blocked")
	safetyErr := errors.New("safety blocked")
	deleteErr := errors.New("delete blocked")
	redactErr := errors.New("redact blocked")

	require.ErrorIs(
		t,
		validateSearch(context.Background(), &fakeGuardrails{searchErr: searchErr}, SearchQuery{Text: "hello"}),
		searchErr,
	)
	require.ErrorIs(
		t,
		validateWrite(context.Background(), &fakeGuardrails{writeErr: writeErr}, MemoryItem{Text: "hello"}),
		writeErr,
	)
	require.ErrorIs(
		t,
		validateWrite(context.Background(), &fakeGuardrails{safetyErr: safetyErr}, MemoryItem{Text: "hello"}),
		safetyErr,
	)
	require.ErrorIs(
		t,
		validateDelete(context.Background(), &fakeGuardrails{deleteErr: deleteErr}, DeleteRequest{ID: "mem_123"}),
		deleteErr,
	)
	_, err := redactItem(context.Background(), &fakeGuardrails{redactErr: redactErr}, MemoryItem{Text: "hello"})
	require.ErrorIs(t, err, redactErr)
}

func TestMemoryProvider_ReturnsGuardrailErrors(t *testing.T) {
	searchErr := errors.New("search blocked")
	writeErr := errors.New("write blocked")
	safetyErr := errors.New("safety blocked")
	deleteErr := errors.New("delete blocked")
	redactErr := errors.New("redact blocked")

	provider := defaultMemoryTestProvider(t, Options{Guardrails: &fakeGuardrails{searchErr: searchErr}})
	_, err := provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, searchErr)
	_, err = provider.LoadPinned(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, searchErr)

	provider = defaultMemoryTestProvider(t, Options{Guardrails: &fakeGuardrails{writeErr: writeErr}})
	_, err = provider.Upsert(context.Background(), MemoryItem{Text: "hello"})
	require.ErrorIs(t, err, writeErr)

	provider = defaultMemoryTestProvider(t, Options{Guardrails: &fakeGuardrails{safetyErr: safetyErr}})
	_, err = provider.Upsert(context.Background(), MemoryItem{Text: "hello"})
	require.ErrorIs(t, err, safetyErr)

	provider = defaultMemoryTestProvider(t, Options{Guardrails: &fakeGuardrails{deleteErr: deleteErr}})
	err = provider.Delete(context.Background(), DeleteRequest{ID: "mem_123"})
	require.ErrorIs(t, err, deleteErr)

	provider = defaultMemoryTestProvider(t, Options{Guardrails: &fakeGuardrails{redactErr: redactErr}})
	_, err = provider.Upsert(context.Background(), MemoryItem{Status: StatusActive, Text: "hello"})
	require.NoError(t, err)
	_, err = provider.Search(context.Background(), SearchQuery{Text: "hello"})
	require.ErrorIs(t, err, redactErr)
}

func TestMemoryProvider_DeleteWithoutGuardrails(t *testing.T) {
	provider := &MemoryProvider{manager: fakeMemoryManager{}}

	require.NoError(t, provider.Delete(context.Background(), DeleteRequest{ID: " mem_123 "}))
}
