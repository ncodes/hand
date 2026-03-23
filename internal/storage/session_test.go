package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handcontext "github.com/wandxy/hand/internal/context"
)

func TestMemorySessionStore_SaveAndGet(t *testing.T) {
	store := NewSessionStore()
	session := Session{
		ID: "session-1",
		Messages: []handcontext.Message{
			{Role: handcontext.RoleUser, Content: "hello", CreatedAt: time.Now().UTC()},
		},
	}

	require.NoError(t, store.Save(context.Background(), session))

	loaded, ok, err := store.Get(context.Background(), "session-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "session-1", loaded.ID)
	require.Len(t, loaded.Messages, 1)
	require.Equal(t, handcontext.RoleUser, loaded.Messages[0].Role)
	require.False(t, loaded.UpdatedAt.IsZero())
}

func TestMemorySessionStore_GetReturnsFalseWhenMissing(t *testing.T) {
	store := NewSessionStore()

	loaded, ok, err := store.Get(context.Background(), "missing")

	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)
}

func TestMemorySessionStore_ListOrdersNewestFirst(t *testing.T) {
	store := NewSessionStore()
	older := time.Now().UTC().Add(-time.Minute)
	newer := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{ID: "older", UpdatedAt: older}))
	require.NoError(t, store.Save(context.Background(), Session{ID: "newer", UpdatedAt: newer}))

	sessions, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "newer", sessions[0].ID)
	require.Equal(t, "older", sessions[1].ID)
}

func TestMemorySessionStore_ListOrdersByIDWhenTimesMatch(t *testing.T) {
	store := NewSessionStore()
	now := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{ID: "zeta", UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: "alpha", UpdatedAt: now}))

	sessions, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "alpha", sessions[0].ID)
	require.Equal(t, "zeta", sessions[1].ID)
}

func TestMemorySessionStore_SaveRejectsMissingID(t *testing.T) {
	store := NewSessionStore()

	require.EqualError(t, store.Save(context.Background(), Session{}), "session id is required")
}

func TestMemorySessionStore_NilReceiverErrors(t *testing.T) {
	var store *InMemorySessionStore

	require.EqualError(t, store.Save(context.Background(), Session{ID: "session-1"}), "session store is required")

	loaded, ok, err := store.Get(context.Background(), "session-1")
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)

	listed, err := store.List(context.Background())
	require.EqualError(t, err, "session store is required")
	require.Nil(t, listed)
}
