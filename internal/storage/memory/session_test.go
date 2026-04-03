package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	base "github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/pkg/nanoid"
)

const DefaultSessionID = base.DefaultSessionID
const SessionIDPrefix = base.SessionIDPrefix
const ArchiveIDPrefix = base.ArchiveIDPrefix

var (
	testSessionA       = nanoid.MustFromSeed(SessionIDPrefix, "project-a", "SessionTestSeedValue123")
	testSessionB       = nanoid.MustFromSeed(SessionIDPrefix, "project-b", "SessionTestSeedValue123")
	testSessionOne     = nanoid.MustFromSeed(SessionIDPrefix, "session-1", "SessionTestSeedValue123")
	testSessionOlder   = nanoid.MustFromSeed(SessionIDPrefix, "older", "SessionTestSeedValue123")
	testSessionNewer   = nanoid.MustFromSeed(SessionIDPrefix, "newer", "SessionTestSeedValue123")
	testSessionAlpha   = nanoid.MustFromSeed(SessionIDPrefix, "alpha", "SessionTestSeedValue123")
	testSessionZeta    = nanoid.MustFromSeed(SessionIDPrefix, "zeta", "SessionTestSeedValue123")
	testMissingSession = nanoid.MustFromSeed(SessionIDPrefix, "missing", "SessionTestSeedValue123")
	testArchiveOne     = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-1", "SessionTestSeedValue123")
	testArchiveOld     = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-old", "SessionTestSeedValue123")
	testArchiveNew     = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-new", "SessionTestSeedValue123")
	testArchiveA       = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-a", "SessionTestSeedValue123")
	testArchiveB       = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-b", "SessionTestSeedValue123")
	testArchiveAlpha   = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-alpha", "SessionTestSeedValue123")
	testArchiveInvalid = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-invalid", "SessionTestSeedValue123")
	testArchiveMissing = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-missing", "SessionTestSeedValue123")
	testArchiveSummary = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-summary", "SessionTestSeedValue123")
	testArchiveZeta    = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-zeta", "SessionTestSeedValue123")
)

func TestMemoryStore_SaveAndGet(t *testing.T) {
	store := NewSessionStore()
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
	store := NewSessionStore()

	loaded, ok, err := store.Get(context.Background(), testMissingSession)

	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, loaded)
}

func TestMemoryStore_ListOrdersNewestFirst(t *testing.T) {
	store := NewSessionStore()
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
	store := NewSessionStore()
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
	store := NewSessionStore()
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
	store := NewSessionStore()

	require.EqualError(t, store.Save(context.Background(), Session{}), "session id is required")
}

func TestMemoryStore_SaveRejectsInvalidSessionID(t *testing.T) {
	store := NewSessionStore()

	require.EqualError(t, store.Save(context.Background(), Session{ID: "ses_invalid"}), "session id must be a valid ses_ nanoid")
}

func TestMemoryStore_NilReceiverErrors(t *testing.T) {
	var store *SessionStore

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

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: testArchiveOne, SourceSessionID: DefaultSessionID, ExpiresAt: time.Now().UTC()}), "session store is required")
	require.EqualError(t, store.DeleteArchive(context.Background(), testArchiveOne), "session store is required")
	archive, ok, err := store.GetArchive(context.Background(), testArchiveOne)
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
	count, err := store.CountMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.Zero(t, count)

	require.EqualError(t, store.DeleteExpiredArchives(context.Background(), time.Now().UTC()), "session store is required")
	require.EqualError(t, store.ClearMessages(context.Background(), DefaultSessionID, MessageQueryOptions{}), "session store is required")
	require.EqualError(t, store.SetCurrent(context.Background(), DefaultSessionID), "session store is required")

	current, ok, err := store.Current(context.Background())
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Empty(t, current)
}

func TestMemoryStore_ArchiveLifecycleAndFiltering(t *testing.T) {
	store := NewSessionStore()
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
		ID:              testArchiveOld,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now.Add(-2 * time.Hour),
		ExpiresAt:       now.Add(-time.Hour),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveNew,
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archives, err := store.ListArchives(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, testArchiveNew, archives[0].ID)
	require.Equal(t, testArchiveOld, archives[1].ID)

	messages, err := store.GetMessages(context.Background(), testArchiveNew, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "new", messages[0].Content)

	message, ok, err := store.GetMessage(context.Background(), testArchiveNew, 0, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "new", message.Content)

	message, ok, err = store.GetMessage(context.Background(), testArchiveNew, 1, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	filtered, err := store.ListArchives(context.Background(), testSessionA)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, testArchiveNew, filtered[0].ID)

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
	require.Equal(t, testArchiveNew, archives[0].ID)
}

func TestMemoryStore_GetArchive(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	archive, ok, err := store.GetArchive(context.Background(), "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	archive, ok, err = store.GetArchive(context.Background(), testArchiveMissing)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveA,
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archive, ok, err = store.GetArchive(context.Background(), testArchiveA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testArchiveA, archive.ID)
	require.Equal(t, testSessionA, archive.SourceSessionID)
	require.Equal(t, now, archive.ArchivedAt)
	require.Equal(t, now.Add(time.Hour), archive.ExpiresAt)
}

func TestMemoryStore_ListArchivesOrdersByIDWhenTimesMatch(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveZeta,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "hello-again", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveAlpha,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archives, err := store.ListArchives(context.Background(), DefaultSessionID)

	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, testArchiveAlpha, archives[0].ID)
	require.Equal(t, testArchiveZeta, archives[1].ID)
}

func TestMemoryStore_DeleteArchive(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.DeleteArchive(context.Background(), ""), "archive id is required")
	require.EqualError(t, store.DeleteArchive(context.Background(), testArchiveMissing), "archive not found")
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "world", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveA,
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveB,
		SourceSessionID: testSessionB,
		ArchivedAt:      now.Add(time.Minute),
		ExpiresAt:       now.Add(time.Hour),
	}))

	require.NoError(t, store.DeleteArchive(context.Background(), testArchiveA))

	archives, err := store.ListArchives(context.Background(), testSessionA)
	require.NoError(t, err)
	require.Empty(t, archives)
	messages, err := store.GetMessages(context.Background(), testArchiveA, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Nil(t, messages)
	otherArchives, err := store.ListArchives(context.Background(), testSessionB)
	require.NoError(t, err)
	require.Len(t, otherArchives, 1)
}

func TestMemoryStore_SetCurrentAndCloneMessages(t *testing.T) {
	store := NewSessionStore()
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
	store := NewSessionStore()
	expiresAt := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{}), "archive id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: testArchiveOne, ExpiresAt: expiresAt}), "source session id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: testArchiveOne, SourceSessionID: DefaultSessionID}), "archive expiry is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveOne,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      time.Date(2026, 3, 31, 11, 0, 0, 0, time.UTC),
		ExpiresAt:       expiresAt,
	}), "source session has no messages")
}

func TestMemoryStore_MessageEdgeCases(t *testing.T) {
	store := NewSessionStore()
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
	require.EqualError(t, store.ClearMessages(context.Background(), testArchiveMissing, MessageQueryOptions{Archived: true}), "archive not found")

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveA,
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	require.NoError(t, store.ClearMessages(context.Background(), testArchiveA, MessageQueryOptions{Archived: true}))

	archivedMessages, err := store.GetMessages(context.Background(), testArchiveA, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Nil(t, archivedMessages)
}

func TestMemoryStore_SaveUpdatesLastPromptTokens(t *testing.T) {
	store := NewSessionStore()
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
	store := NewSessionStore()
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
	store := NewSessionStore()
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
	store := NewSessionStore()
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

func TestMemoryStore_SaveRoundTripsCompactionMetadata(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{
		ID: testSessionA,
		Compaction: base.SessionCompaction{
			CompletedAt:        now.Add(3 * time.Minute),
			FailedAt:           now.Add(2 * time.Minute),
			LastError:          "failed before retry",
			RequestedAt:        now,
			StartedAt:          now.Add(time.Minute),
			Status:             base.CompactionStatusFailed,
			TargetMessageCount: 12,
			TargetOffset:       4,
		},
		UpdatedAt: now,
	}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, base.CompactionStatusFailed, session.Compaction.Status)
	require.Equal(t, "failed before retry", session.Compaction.LastError)
	require.Equal(t, 12, session.Compaction.TargetMessageCount)
	require.Equal(t, 4, session.Compaction.TargetOffset)
	require.Equal(t, now, session.Compaction.RequestedAt)
	require.Equal(t, now.Add(time.Minute), session.Compaction.StartedAt)
	require.Equal(t, now.Add(2*time.Minute), session.Compaction.FailedAt)
	require.Equal(t, now.Add(3*time.Minute), session.Compaction.CompletedAt)

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, session.Compaction, sessions[0].Compaction)
}

func TestMemoryStore_SavePreservesExistingCompactionMetadataOnPartialUpdate(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{
		ID: testSessionA,
		Compaction: base.SessionCompaction{
			LastError:          "failed before retry",
			RequestedAt:        now,
			Status:             base.CompactionStatusFailed,
			TargetMessageCount: 12,
			TargetOffset:       4,
		},
		UpdatedAt: now,
	}))

	require.NoError(t, store.Save(context.Background(), Session{
		ID:        testSessionA,
		UpdatedAt: now.Add(time.Minute),
	}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, base.CompactionStatusFailed, session.Compaction.Status)
	require.Equal(t, "failed before retry", session.Compaction.LastError)
	require.Equal(t, 12, session.Compaction.TargetMessageCount)
	require.Equal(t, 4, session.Compaction.TargetOffset)
}

func TestMemoryStore_ClearsCompactionMetadataWhenLiveHistoryIsCleared(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{
		ID: testSessionA,
		Compaction: base.SessionCompaction{
			Status:             base.CompactionStatusRunning,
			TargetMessageCount: 12,
			TargetOffset:       4,
		},
		UpdatedAt: now,
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
	require.NoError(t, store.SaveSummary(context.Background(), SessionSummary{
		SessionID:      testSessionA,
		SessionSummary: "Older work",
	}))

	require.NoError(t, store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, base.SessionCompaction{}, session.Compaction)
}

func TestMemoryStore_CreateArchiveClearsDefaultSessionCompactionMetadata(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{
		ID: DefaultSessionID,
		Compaction: base.SessionCompaction{
			Status:             base.CompactionStatusSucceeded,
			TargetMessageCount: 12,
			TargetOffset:       4,
		},
		UpdatedAt: now,
	}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
	require.NoError(t, store.SaveSummary(context.Background(), SessionSummary{
		SessionID:      DefaultSessionID,
		SessionSummary: "Older work",
	}))

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveSummary,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	session, ok, err := store.Get(context.Background(), DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, base.SessionCompaction{}, session.Compaction)
}

func TestMemoryStore_SaveTrimsIDOnCreate(t *testing.T) {
	store := NewSessionStore()

	require.NoError(t, store.Save(context.Background(), Session{ID: "  " + testSessionA + "  "}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionA, session.ID)
	require.False(t, session.CreatedAt.IsZero())
	require.False(t, session.UpdatedAt.IsZero())
}

func TestMemoryStore_GetMessagesRejectsInvalidLiveID(t *testing.T) {
	store := NewSessionStore()

	messages, err := store.GetMessages(context.Background(), "ses_invalid", MessageQueryOptions{})

	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Nil(t, messages)
}

func TestMemoryStore_GetMessagesAllowsArchivedLookupWithoutSessionIDValidation(t *testing.T) {
	store := NewSessionStore()
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "archived"},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveInvalid,
		SourceSessionID: DefaultSessionID,
		ExpiresAt:       time.Now().UTC().Add(time.Hour),
	}))

	messages, err := store.GetMessages(context.Background(), testArchiveInvalid, MessageQueryOptions{Archived: true})

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "archived", messages[0].Content)
}

func TestMemoryStore_GetMessagesSupportsOffsetAndLimit(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "one", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "two", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "three", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "four", CreatedAt: now},
	}))

	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{Offset: 1, Limit: 2})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "two", messages[0].Content)
	require.Equal(t, "three", messages[1].Content)

	messages, err = store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{Offset: -1, Limit: 2})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "one", messages[0].Content)
	require.Equal(t, "two", messages[1].Content)

	messages, err = store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{Offset: 99})
	require.NoError(t, err)
	require.Nil(t, messages)

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveA,
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))
	messages, err = store.GetMessages(context.Background(), testArchiveA, MessageQueryOptions{Offset: 0, Limit: 2, Archived: true})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "one", messages[0].Content)
	require.Equal(t, "two", messages[1].Content)

	messages, err = store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)
}

func TestMemoryStore_CountMessagesSupportsLiveAndArchivedQueries(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "one", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "two", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "three", CreatedAt: now},
	}))

	count, err := store.CountMessages(context.Background(), DefaultSessionID, MessageQueryOptions{Offset: 1, Limit: 1})
	require.NoError(t, err)
	require.Equal(t, 3, count)

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveA,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	count, err = store.CountMessages(context.Background(), testArchiveA, MessageQueryOptions{Archived: true, Offset: 1, Limit: 1})
	require.NoError(t, err)
	require.Equal(t, 3, count)
}

func TestMemoryStore_CountMessagesEdgeCases(t *testing.T) {
	store := NewSessionStore()

	count, err := store.CountMessages(context.Background(), "", MessageQueryOptions{})
	require.NoError(t, err)
	require.Zero(t, count)

	count, err = store.CountMessages(context.Background(), "ses_invalid", MessageQueryOptions{})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Zero(t, count)

	count, err = store.CountMessages(context.Background(), "archive_invalid", MessageQueryOptions{Archived: true})
	require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	require.Zero(t, count)
}

func TestMemoryStore_GetMessageRejectsInvalidLiveID(t *testing.T) {
	store := NewSessionStore()

	message, ok, err := store.GetMessage(context.Background(), "ses_invalid", 0, MessageQueryOptions{})

	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)
}

func TestMemoryStore_GetMessageAllowsArchivedLookupWithoutSessionIDValidation(t *testing.T) {
	store := NewSessionStore()
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "archived"},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveInvalid,
		SourceSessionID: DefaultSessionID,
		ExpiresAt:       time.Now().UTC().Add(time.Hour),
	}))

	message, ok, err := store.GetMessage(context.Background(), testArchiveInvalid, 0, MessageQueryOptions{Archived: true})

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "archived", message.Content)
}

func TestMemoryStore_ArchiveLookupsRejectInvalidArchiveID(t *testing.T) {
	store := NewSessionStore()

	messages, err := store.GetMessages(context.Background(), "archive_invalid", MessageQueryOptions{Archived: true})
	require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	require.Nil(t, messages)

	message, ok, err := store.GetMessage(context.Background(), "archive_invalid", 0, MessageQueryOptions{Archived: true})
	require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	archive, ok, err := store.GetArchive(context.Background(), "archive_invalid")
	require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	require.EqualError(t, store.ClearMessages(context.Background(), "archive_invalid", MessageQueryOptions{Archived: true}), "archive id must be a valid arc_ nanoid")
}

func TestMemoryStore_ClearMessagesClearsLiveMessagesAndRefreshesUpdatedAt(t *testing.T) {
	store := NewSessionStore()
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

func TestMemoryStore_SummaryRoundTripAndCleanup(t *testing.T) {
	store := NewSessionStore()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))

	summary := SessionSummary{
		SessionID:          testSessionA,
		SourceEndOffset:    4,
		SourceMessageCount: 12,
		UpdatedAt:          now,
		SessionSummary:     "Older work",
		CurrentTask:        "Finish phase 3",
		Discoveries:        []string{"one"},
		OpenQuestions:      []string{"two"},
		NextActions:        []string{"three"},
	}

	require.NoError(t, store.SaveSummary(context.Background(), summary))

	loaded, ok, err := store.GetSummary(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, summary.SessionSummary, loaded.SessionSummary)
	require.Equal(t, summary.CurrentTask, loaded.CurrentTask)
	require.Equal(t, summary.Discoveries, loaded.Discoveries)

	loaded.Discoveries[0] = "changed"
	loadedAgain, ok, err := store.GetSummary(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "one", loadedAgain.Discoveries[0])

	require.NoError(t, store.DeleteSummary(context.Background(), testSessionA))
	_, ok, err = store.GetSummary(context.Background(), testSessionA)
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, store.SaveSummary(context.Background(), summary))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveSummary,
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	_, ok, err = store.GetSummary(context.Background(), testSessionA)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestMemoryStore_SummaryErrors(t *testing.T) {
	var nilStore *SessionStore

	require.EqualError(t, nilStore.SaveSummary(context.Background(), SessionSummary{}), "session store is required")

	summary, ok, err := nilStore.GetSummary(context.Background(), testSessionA)
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, SessionSummary{}, summary)

	require.EqualError(t, nilStore.DeleteSummary(context.Background(), testSessionA), "session store is required")

	store := NewSessionStore()

	require.EqualError(t, store.SaveSummary(context.Background(), SessionSummary{}), "session id is required")
	require.EqualError(t, store.SaveSummary(context.Background(), SessionSummary{
		SessionID:      testMissingSession,
		SessionSummary: "summary",
	}), "session not found")

	summary, ok, err = store.GetSummary(context.Background(), "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, SessionSummary{}, summary)

	summary, ok, err = store.GetSummary(context.Background(), "ses_invalid")
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.False(t, ok)
	require.Equal(t, SessionSummary{}, summary)

	summary, ok, err = store.GetSummary(context.Background(), testMissingSession)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, SessionSummary{}, summary)

	require.EqualError(t, store.DeleteSummary(context.Background(), ""), "session id is required")
	require.EqualError(t, store.DeleteSummary(context.Background(), "ses_invalid"), "session id must be a valid ses_ nanoid")
}
