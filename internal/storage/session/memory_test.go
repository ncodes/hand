package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handcontext "github.com/wandxy/hand/internal/context"
)

func Test_MemoryStore_SaveAndGet(t *testing.T) {
	store := NewStore()
	session := Session{ID: "session-1"}

	require.NoError(t, store.Save(context.Background(), session))
	require.NoError(t, store.AppendMessages(context.Background(), "session-1", []handcontext.Message{
		{Role: handcontext.RoleUser, Content: "hello", CreatedAt: time.Now().UTC()},
	}))

	loaded, ok, err := store.Get(context.Background(), "session-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "session-1", loaded.ID)
	require.False(t, loaded.CreatedAt.IsZero())
	messages, err := store.GetMessages(context.Background(), "session-1", MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, handcontext.RoleUser, messages[0].Role)
	message, ok, err := store.GetMessage(context.Background(), "session-1", 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", message.Content)
	require.False(t, loaded.UpdatedAt.IsZero())
}

func Test_MemoryStore_GetReturnsFalseWhenMissing(t *testing.T) {
	store := NewStore()

	loaded, ok, err := store.Get(context.Background(), "missing")

	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)
}

func Test_MemoryStore_ListOrdersNewestFirst(t *testing.T) {
	store := NewStore()
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

func Test_MemoryStore_ListOrdersByIDWhenTimesMatch(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{ID: "zeta", UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: "alpha", UpdatedAt: now}))

	sessions, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "alpha", sessions[0].ID)
	require.Equal(t, "zeta", sessions[1].ID)
}

func Test_MemoryStore_Delete(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()

	require.EqualError(t, store.Delete(context.Background(), ""), "session id is required")
	require.EqualError(t, store.Delete(context.Background(), DefaultSessionID), "default session cannot be deleted")
	require.EqualError(t, store.Delete(context.Background(), "missing"), "session not found")

	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handcontext.Message{
		{Role: handcontext.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.SetCurrent(context.Background(), "project-a"))
	require.NoError(t, store.Delete(context.Background(), "project-a"))

	loaded, ok, err := store.Get(context.Background(), "project-a")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)
	messages, err := store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)
	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)
}

func Test_MemoryStore_SaveRejectsMissingID(t *testing.T) {
	store := NewStore()

	require.EqualError(t, store.Save(context.Background(), Session{}), "session id is required")
}

func Test_MemoryStore_NilReceiverErrors(t *testing.T) {
	var store *MemoryStore

	require.EqualError(t, store.Save(context.Background(), Session{ID: "session-1"}), "session store is required")
	require.EqualError(t, store.Delete(context.Background(), "session-1"), "session store is required")
	require.EqualError(t, store.AppendMessages(context.Background(), "session-1", []handcontext.Message{{Role: handcontext.RoleUser, Content: "hello"}}), "session store is required")

	loaded, ok, err := store.Get(context.Background(), "session-1")
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)

	listed, err := store.List(context.Background())
	require.EqualError(t, err, "session store is required")
	require.Nil(t, listed)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", SourceSessionID: DefaultSessionID, ExpiresAt: time.Now().UTC()}), "session store is required")
	require.EqualError(t, store.DeleteArchives(context.Background(), "archive-1"), "session store is required")

	archives, err := store.GetArchives(context.Background(), DefaultSessionID)
	require.EqualError(t, err, "session store is required")
	require.Nil(t, archives)
	messages, err := store.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.Nil(t, messages)
	message, ok, err := store.GetMessage(context.Background(), DefaultSessionID, 0, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, handcontext.Message{}, message)

	require.EqualError(t, store.DeleteExpiredArchives(context.Background(), time.Now().UTC()), "session store is required")
	require.EqualError(t, store.ClearMessages(context.Background(), DefaultSessionID, MessageQueryOptions{}), "session store is required")
	require.EqualError(t, store.SetCurrent(context.Background(), DefaultSessionID), "session store is required")

	current, ok, err := store.Current(context.Background())
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Empty(t, current)
}

func Test_MemoryStore_ArchiveLifecycleAndFiltering(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handcontext.Message{
		{Role: handcontext.RoleUser, Content: "old", CreatedAt: now.Add(-2 * time.Hour)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a"}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handcontext.Message{
		{Role: handcontext.RoleAssistant, Content: "new", CreatedAt: now},
	}))

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-old",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now.Add(-2 * time.Hour),
		ExpiresAt:       now.Add(-time.Hour),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-new",
		SourceSessionID: "project-a",
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archives, err := store.GetArchives(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, "archive-new", archives[0].ID)
	require.Equal(t, "archive-old", archives[1].ID)
	messages, err := store.GetMessages(context.Background(), "archive-new", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "new", messages[0].Content)
	message, ok, err := store.GetMessage(context.Background(), "archive-new", 0, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "new", message.Content)
	message, ok, err = store.GetMessage(context.Background(), "archive-new", 1, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handcontext.Message{}, message)

	filtered, err := store.GetArchives(context.Background(), "project-a")
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "archive-new", filtered[0].ID)

	require.NoError(t, store.DeleteExpiredArchives(context.Background(), now))

	archives, err = store.GetArchives(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, archives, 1)
	require.Equal(t, "archive-new", archives[0].ID)
}

func Test_MemoryStore_ListArchivesOrdersByIDWhenTimesMatch(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "zeta",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "alpha",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archives, err := store.GetArchives(context.Background(), DefaultSessionID)

	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, "alpha", archives[0].ID)
	require.Equal(t, "zeta", archives[1].ID)
}

func Test_MemoryStore_DeleteArchives(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.DeleteArchives(context.Background(), ""), "archive id is required")
	require.EqualError(t, store.DeleteArchives(context.Background(), "missing"), "archive not found")
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handcontext.Message{
		{Role: handcontext.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-a",
		SourceSessionID: "project-a",
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-b",
		SourceSessionID: "project-b",
		ArchivedAt:      now.Add(time.Minute),
		ExpiresAt:       now.Add(time.Hour),
	}))

	require.NoError(t, store.DeleteArchives(context.Background(), "archive-a"))

	archives, err := store.GetArchives(context.Background(), "project-a")
	require.NoError(t, err)
	require.Empty(t, archives)
	messages, err := store.GetMessages(context.Background(), "archive-a", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Nil(t, messages)
	otherArchives, err := store.GetArchives(context.Background(), "project-b")
	require.NoError(t, err)
	require.Len(t, otherArchives, 1)
}

func Test_MemoryStore_SetCurrentAndCloneMessages(t *testing.T) {
	store := NewStore()
	message := handcontext.Message{Role: handcontext.RoleUser, Content: "hello", CreatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)}
	session := Session{ID: "project-a"}

	require.NoError(t, store.Save(context.Background(), session))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handcontext.Message{message}))
	message.Content = "mutated-after-save"

	loaded, ok, err := store.Get(context.Background(), "project-a")
	require.NoError(t, err)
	require.True(t, ok)
	createdAt := loaded.CreatedAt
	messages, err := store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello", messages[0].Content)

	require.EqualError(t, store.SetCurrent(context.Background(), ""), "session id is required")
	require.EqualError(t, store.SetCurrent(context.Background(), "missing"), "session not found")
	require.NoError(t, store.SetCurrent(context.Background(), "project-a"))

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "project-a", current)

	messages[0].Content = "mutated-after-get"
	loadedAgain, ok, err := store.Get(context.Background(), "project-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, createdAt, loadedAgain.CreatedAt)
	messagesAgain, err := store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello", messagesAgain[0].Content)
	messageAgain, ok, err := store.GetMessage(context.Background(), "project-a", 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", messageAgain.Content)
}

func Test_MemoryStore_ArchiveValidation(t *testing.T) {
	store := NewStore()
	expiresAt := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{}), "archive id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", ExpiresAt: expiresAt}), "source session id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", SourceSessionID: DefaultSessionID}), "archive expiry is required")
}

func Test_MemoryStore_MessageEdgeCases(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.AppendMessages(context.Background(), "", []handcontext.Message{{Role: handcontext.RoleUser, Content: "hello", CreatedAt: now}}), "session id is required")
	require.NoError(t, store.AppendMessages(context.Background(), "missing", nil))
	require.EqualError(t, store.AppendMessages(context.Background(), "missing", []handcontext.Message{{Role: handcontext.RoleUser, Content: "hello", CreatedAt: now}}), "session not found")

	messages, err := store.GetMessages(context.Background(), "", MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)

	message, ok, err := store.GetMessage(context.Background(), "", 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handcontext.Message{}, message)

	message, ok, err = store.GetMessage(context.Background(), "missing", -1, MessageQueryOptions{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handcontext.Message{}, message)

	require.EqualError(t, store.ClearMessages(context.Background(), "", MessageQueryOptions{}), "session id is required")
	require.EqualError(t, store.ClearMessages(context.Background(), "missing", MessageQueryOptions{}), "session not found")
	require.EqualError(t, store.ClearMessages(context.Background(), "archive-missing", MessageQueryOptions{Archived: true}), "archive not found")

	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handcontext.Message{{Role: handcontext.RoleUser, Content: "hello", CreatedAt: now}}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-a",
		SourceSessionID: "project-a",
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.ClearMessages(context.Background(), "archive-a", MessageQueryOptions{Archived: true}))

	archivedMessages, err := store.GetMessages(context.Background(), "archive-a", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Nil(t, archivedMessages)
}
