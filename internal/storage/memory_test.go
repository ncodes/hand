package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInMemoryMemoryStore_SaveAndGet(t *testing.T) {
	store := NewMemoryStore()
	entry := MemoryEntry{Key: "workspace", Value: "wandxy"}

	require.NoError(t, store.Upsert(context.Background(), entry))

	loaded, ok, err := store.Get(context.Background(), "workspace")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "wandxy", loaded.Value)
	require.False(t, loaded.UpdatedAt.IsZero())
}

func TestInMemoryMemoryStore_GetReturnsFalseWhenMissing(t *testing.T) {
	store := NewMemoryStore()

	loaded, ok, err := store.Get(context.Background(), "missing")

	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, MemoryEntry{}, loaded)
}

func TestInMemoryMemoryStore_ListOrdersNewestFirst(t *testing.T) {
	store := NewMemoryStore()
	older := time.Now().UTC().Add(-time.Minute)
	newer := time.Now().UTC()

	require.NoError(t, store.Upsert(context.Background(), MemoryEntry{Key: "older", UpdatedAt: older}))
	require.NoError(t, store.Upsert(context.Background(), MemoryEntry{Key: "newer", UpdatedAt: newer}))

	memories, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, memories, 2)
	require.Equal(t, "newer", memories[0].Key)
	require.Equal(t, "older", memories[1].Key)
}

func TestInMemoryMemoryStore_ListOrdersByKeyWhenTimesMatch(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()

	require.NoError(t, store.Upsert(context.Background(), MemoryEntry{Key: "zeta", UpdatedAt: now}))
	require.NoError(t, store.Upsert(context.Background(), MemoryEntry{Key: "alpha", UpdatedAt: now}))

	memories, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, memories, 2)
	require.Equal(t, "alpha", memories[0].Key)
	require.Equal(t, "zeta", memories[1].Key)
}

func TestInMemoryMemoryStore_SaveRejectsMissingKey(t *testing.T) {
	store := NewMemoryStore()

	require.EqualError(t, store.Upsert(context.Background(), MemoryEntry{}), "memory key is required")
}

func TestInMemoryMemoryStore_NilReceiverErrors(t *testing.T) {
	var store *InMemoryMemoryStore

	require.EqualError(t, store.Upsert(context.Background(), MemoryEntry{Key: "workspace"}), "memory store is required")

	loaded, ok, err := store.Get(context.Background(), "workspace")
	require.EqualError(t, err, "memory store is required")
	require.False(t, ok)
	require.Equal(t, MemoryEntry{}, loaded)

	listed, err := store.List(context.Background())
	require.EqualError(t, err, "memory store is required")
	require.Nil(t, listed)
}
