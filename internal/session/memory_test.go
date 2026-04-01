package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
)

func TestMemoryStore_SaveAndGet(t *testing.T) {
	store := NewStore()
	session := Session{ID: testSessionOne}

	require.NoError(t, store.Save(context.Background(), session))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionOne, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: time.Now().UTC()},
	}))

	loaded, ok, err := store.Get(context.Background(), testSessionOne)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionOne, loaded.ID)
	require.False(t, loaded.CreatedAt.IsZero())
	messages, err := store.GetMessages(context.Background(), testSessionOne, MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, handmsg.RoleUser, messages[0].Role)
	message, ok, err := store.GetMessage(context.Background(), testSessionOne, 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", message.Content)
	require.False(t, loaded.UpdatedAt.IsZero())
}

func TestMemoryStore_GetReturnsFalseWhenMissing(t *testing.T) {
	store := NewStore()

	loaded, ok, err := store.Get(context.Background(), testMissingSession)

	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)
}

func TestMemoryStore_ListOrdersNewestFirst(t *testing.T) {
	store := NewStore()
	older := time.Now().UTC().Add(-time.Minute)
	newer := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionOlder, UpdatedAt: older}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionNewer, UpdatedAt: newer}))

	sessions, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, testSessionNewer, sessions[0].ID)
	require.Equal(t, testSessionOlder, sessions[1].ID)
}

func TestMemoryStore_ListOrdersByIDWhenTimesMatch(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionZeta, UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionAlpha, UpdatedAt: now}))

	sessions, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, testSessionAlpha, sessions[0].ID)
	require.Equal(t, testSessionZeta, sessions[1].ID)
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()

	require.EqualError(t, store.Delete(context.Background(), ""), "session id is required")
	require.EqualError(t, store.Delete(context.Background(), DefaultSessionID), "default session cannot be deleted")
	require.EqualError(t, store.Delete(context.Background(), testMissingSession), "session not found")

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.SetCurrent(context.Background(), testSessionA))
	require.NoError(t, store.Delete(context.Background(), testSessionA))

	loaded, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)
	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)
	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)
}

func TestMemoryStore_SaveRejectsMissingID(t *testing.T) {
	store := NewStore()

	require.EqualError(t, store.Save(context.Background(), Session{}), "session id is required")
}

func TestMemoryStore_SaveRejectsInvalidSessionID(t *testing.T) {
	store := NewStore()

	require.EqualError(t, store.Save(context.Background(), Session{ID: "ses_invalid"}), "session id must be a valid ses_ nanoid")
}

func TestMemoryStore_NilReceiverErrors(t *testing.T) {
	var store *MemoryStore

	require.EqualError(t, store.Save(context.Background(), Session{ID: "session-1"}), "session store is required")
	require.EqualError(t, store.Delete(context.Background(), "session-1"), "session store is required")
	require.EqualError(t, store.AppendMessages(context.Background(), "session-1", []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}), "session store is required")

	loaded, ok, err := store.Get(context.Background(), "session-1")
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)

	listed, err := store.List(context.Background())
	require.EqualError(t, err, "session store is required")
	require.Nil(t, listed)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", SourceSessionID: DefaultSessionID, ExpiresAt: time.Now().UTC()}), "session store is required")
	require.EqualError(t, store.DeleteArchives(context.Background(), "archive-1"), "session store is required")
	archive, ok, err := store.GetArchive(context.Background(), "archive-1")
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	archives, err := store.ListArchives(context.Background(), DefaultSessionID)
	require.EqualError(t, err, "session store is required")
	require.Nil(t, archives)
	messages, err := store.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.Nil(t, messages)
	message, ok, err := store.GetMessage(context.Background(), DefaultSessionID, 0, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	require.EqualError(t, store.DeleteExpiredArchives(context.Background(), time.Now().UTC()), "session store is required")
	require.EqualError(t, store.ClearMessages(context.Background(), DefaultSessionID, MessageQueryOptions{}), "session store is required")
	require.EqualError(t, store.SetCurrent(context.Background(), DefaultSessionID), "session store is required")

	current, ok, err := store.Current(context.Background())
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Empty(t, current)
}

func TestMemoryStore_ArchiveLifecycleAndFiltering(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "old", CreatedAt: now.Add(-2 * time.Hour)},
	}))

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "new", CreatedAt: now},
	}))
	require.NoError(t, store.SetCurrent(context.Background(), testSessionA))

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-old",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now.Add(-2 * time.Hour),
		ExpiresAt:       now.Add(-time.Hour),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-new",
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archives, err := store.ListArchives(context.Background(), "")
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
	require.Equal(t, handmsg.Message{}, message)

	filtered, err := store.ListArchives(context.Background(), testSessionA)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "archive-new", filtered[0].ID)

	defaultSession, ok, err := store.Get(context.Background(), DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, DefaultSessionID, defaultSession.ID)

	defaultMessages, err := store.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, defaultMessages)

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	liveMessages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, liveMessages)

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)

	require.NoError(t, store.DeleteExpiredArchives(context.Background(), now))

	archives, err = store.ListArchives(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, archives, 1)
	require.Equal(t, "archive-new", archives[0].ID)
}

func TestMemoryStore_GetArchive(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	archive, ok, err := store.GetArchive(context.Background(), "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	archive, ok, err = store.GetArchive(context.Background(), testMissingSession)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-a",
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archive, ok, err = store.GetArchive(context.Background(), "archive-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "archive-a", archive.ID)
	require.Equal(t, testSessionA, archive.SourceSessionID)
	require.Equal(t, now, archive.ArchivedAt)
	require.Equal(t, now.Add(time.Hour), archive.ExpiresAt)
}

func TestMemoryStore_ListArchivesOrdersByIDWhenTimesMatch(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "zeta",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "hello-again", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "alpha",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archives, err := store.ListArchives(context.Background(), DefaultSessionID)

	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, "alpha", archives[0].ID)
	require.Equal(t, "zeta", archives[1].ID)
}

func TestMemoryStore_DeleteArchives(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.DeleteArchives(context.Background(), ""), "archive id is required")
	require.EqualError(t, store.DeleteArchives(context.Background(), testMissingSession), "archive not found")
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "world", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-a",
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-b",
		SourceSessionID: testSessionB,
		ArchivedAt:      now.Add(time.Minute),
		ExpiresAt:       now.Add(time.Hour),
	}))

	require.NoError(t, store.DeleteArchives(context.Background(), "archive-a"))

	archives, err := store.ListArchives(context.Background(), testSessionA)
	require.NoError(t, err)
	require.Empty(t, archives)
	messages, err := store.GetMessages(context.Background(), "archive-a", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Nil(t, messages)
	otherArchives, err := store.ListArchives(context.Background(), testSessionB)
	require.NoError(t, err)
	require.Len(t, otherArchives, 1)
}

func TestMemoryStore_SetCurrentAndCloneMessages(t *testing.T) {
	store := NewStore()
	message := handmsg.Message{Role: handmsg.RoleUser, Content: "hello", CreatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)}
	session := Session{ID: testSessionA}

	require.NoError(t, store.Save(context.Background(), session))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{message}))
	message.Content = "mutated-after-save"

	loaded, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	createdAt := loaded.CreatedAt
	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello", messages[0].Content)

	require.EqualError(t, store.SetCurrent(context.Background(), ""), "session id is required")
	require.EqualError(t, store.SetCurrent(context.Background(), testMissingSession), "session not found")
	require.NoError(t, store.SetCurrent(context.Background(), testSessionA))

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionA, current)

	messages[0].Content = "mutated-after-get"
	loadedAgain, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, createdAt, loadedAgain.CreatedAt)
	messagesAgain, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello", messagesAgain[0].Content)
	messageAgain, ok, err := store.GetMessage(context.Background(), testSessionA, 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", messageAgain.Content)
}

func TestMemoryStore_ArchiveValidation(t *testing.T) {
	store := NewStore()
	expiresAt := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{}), "archive id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", ExpiresAt: expiresAt}), "source session id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", SourceSessionID: DefaultSessionID}), "archive expiry is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-1",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      time.Date(2026, 3, 31, 11, 0, 0, 0, time.UTC),
		ExpiresAt:       expiresAt,
	}), "source session has no messages")
}

func TestMemoryStore_MessageEdgeCases(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.AppendMessages(context.Background(), "", []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}), "session id is required")
	require.NoError(t, store.AppendMessages(context.Background(), testMissingSession, nil))
	require.EqualError(t, store.AppendMessages(context.Background(), testMissingSession, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}), "session not found")

	messages, err := store.GetMessages(context.Background(), "", MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)

	message, ok, err := store.GetMessage(context.Background(), "", 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	message, ok, err = store.GetMessage(context.Background(), testMissingSession, -1, MessageQueryOptions{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	require.EqualError(t, store.ClearMessages(context.Background(), "", MessageQueryOptions{}), "session id is required")
	require.EqualError(t, store.ClearMessages(context.Background(), testMissingSession, MessageQueryOptions{}), "session not found")
	require.EqualError(t, store.ClearMessages(context.Background(), "archive-missing", MessageQueryOptions{Archived: true}), "archive not found")

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-a",
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.ClearMessages(context.Background(), "archive-a", MessageQueryOptions{Archived: true}))

	archivedMessages, err := store.GetMessages(context.Background(), "archive-a", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Nil(t, archivedMessages)
}

func TestMemoryStore_SaveUpdatesLastPromptTokens(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{
		ID:               testSessionA,
		LastPromptTokens: 123,
		UpdatedAt:        now,
	}))
	require.NoError(t, store.Save(context.Background(), Session{
		ID:               testSessionA,
		LastPromptTokens: 0,
		UpdatedAt:        now.Add(time.Minute),
	}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, session.LastPromptTokens)
}

func TestMemoryStore_SaveTrimsIDBeforeExistingSessionLookup(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{
		ID:               testSessionA,
		LastPromptTokens: 123,
		UpdatedAt:        now,
	}))
	require.NoError(t, store.Save(context.Background(), Session{
		ID:               "  " + testSessionA + "  ",
		LastPromptTokens: 0,
		UpdatedAt:        now.Add(time.Minute),
	}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, session.LastPromptTokens)
}

func TestMemoryStore_SavePreservesExistingCreatedAtAndAllowsPromptTokenOverwrite(t *testing.T) {
	store := NewStore()
	originalCreatedAt := time.Now().UTC().Add(-time.Hour)
	updatedAt := time.Now().UTC()

	require.NoError(t, store.Save(context.Background(), Session{
		ID:               testSessionA,
		CreatedAt:        originalCreatedAt,
		LastPromptTokens: 123,
		UpdatedAt:        updatedAt,
	}))
	require.NoError(t, store.Save(context.Background(), Session{
		ID:               testSessionA,
		CreatedAt:        time.Now().UTC(),
		LastPromptTokens: 456,
		UpdatedAt:        updatedAt.Add(time.Minute),
	}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, originalCreatedAt, session.CreatedAt)
	require.Equal(t, 456, session.LastPromptTokens)
}

func TestMemoryStore_SaveRefreshesUpdatedAtOnUpdate(t *testing.T) {
	store := NewStore()
	originalUpdatedAt := time.Now().UTC().Add(-time.Hour)

	require.NoError(t, store.Save(context.Background(), Session{
		ID:        testSessionA,
		UpdatedAt: originalUpdatedAt,
	}))

	require.NoError(t, store.Save(context.Background(), Session{
		ID:        testSessionA,
		UpdatedAt: originalUpdatedAt,
	}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, session.UpdatedAt.After(originalUpdatedAt))
}

func TestMemoryStore_SaveTrimsIDOnCreate(t *testing.T) {
	store := NewStore()

	require.NoError(t, store.Save(context.Background(), Session{ID: "  " + testSessionA + "  "}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionA, session.ID)
	require.False(t, session.CreatedAt.IsZero())
	require.False(t, session.UpdatedAt.IsZero())
}

func TestMemoryStore_GetMessagesRejectsInvalidLiveID(t *testing.T) {
	store := NewStore()

	messages, err := store.GetMessages(context.Background(), "ses_invalid", MessageQueryOptions{})

	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Nil(t, messages)
}

func TestMemoryStore_GetMessagesAllowsArchivedLookupWithoutSessionIDValidation(t *testing.T) {
	store := NewStore()
	store.archiveMessages["archive-invalid"] = []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "archived"},
	}

	messages, err := store.GetMessages(context.Background(), "archive-invalid", MessageQueryOptions{Archived: true})

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "archived", messages[0].Content)
}

func TestMemoryStore_GetMessageRejectsInvalidLiveID(t *testing.T) {
	store := NewStore()

	message, ok, err := store.GetMessage(context.Background(), "ses_invalid", 0, MessageQueryOptions{})

	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)
}

func TestMemoryStore_GetMessageAllowsArchivedLookupWithoutSessionIDValidation(t *testing.T) {
	store := NewStore()
	store.archiveMessages["archive-invalid"] = []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "archived"},
	}

	message, ok, err := store.GetMessage(context.Background(), "archive-invalid", 0, MessageQueryOptions{Archived: true})

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "archived", message.Content)
}

func TestMemoryStore_ClearMessagesClearsLiveMessagesAndRefreshesUpdatedAt(t *testing.T) {
	store := NewStore()
	originalUpdatedAt := time.Now().UTC().Add(-time.Hour)

	require.NoError(t, store.Save(context.Background(), Session{
		ID:        testSessionA,
		UpdatedAt: originalUpdatedAt,
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: time.Now().UTC()},
	}))

	require.NoError(t, store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}))

	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, session.UpdatedAt.After(originalUpdatedAt))
}
