package sqlite

import (
	"context"
	"os"
	"path/filepath"
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
	testSessionA            = nanoid.MustFromSeed(SessionIDPrefix, "project-a", "SessionTestSeedValue123")
	testSessionB            = nanoid.MustFromSeed(SessionIDPrefix, "project-b", "SessionTestSeedValue123")
	testMissingSession      = nanoid.MustFromSeed(SessionIDPrefix, "missing", "SessionTestSeedValue123")
	testSessionAlpha        = nanoid.MustFromSeed(SessionIDPrefix, "alpha", "SessionTestSeedValue123")
	testSessionZeta         = nanoid.MustFromSeed(SessionIDPrefix, "zeta", "SessionTestSeedValue123")
	testSessionZero         = nanoid.MustFromSeed(SessionIDPrefix, "project-zero", "SessionTestSeedValue123")
	testArchiveOne          = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-1", "SessionTestSeedValue123")
	testArchiveOld          = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-old", "SessionTestSeedValue123")
	testArchiveNew          = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-new", "SessionTestSeedValue123")
	testArchiveA            = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-a", "SessionTestSeedValue123")
	testArchiveAlpha        = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-alpha", "SessionTestSeedValue123")
	testArchiveB            = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-b", "SessionTestSeedValue123")
	testArchiveEmpty        = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-empty", "SessionTestSeedValue123")
	testArchiveBad          = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-bad", "SessionTestSeedValue123")
	testArchiveFuture       = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-future", "SessionTestSeedValue123")
	testArchiveExpired      = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-expired", "SessionTestSeedValue123")
	testArchiveClear        = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-clear", "SessionTestSeedValue123")
	testArchiveMissing      = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-missing", "SessionTestSeedValue123")
	testArchiveSummary      = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-summary", "SessionTestSeedValue123")
	testArchiveSourceError  = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-source-error", "SessionTestSeedValue123")
	testArchiveDeleteError  = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-delete-error", "SessionTestSeedValue123")
	testArchiveSessionError = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-session-delete-error", "SessionTestSeedValue123")
	testArchiveStateError   = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-state-delete-error", "SessionTestSeedValue123")
	testArchiveSummaryError = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-summary-error", "SessionTestSeedValue123")
	testArchiveZeta         = nanoid.MustFromSeed(ArchiveIDPrefix, "archive-zeta", "SessionTestSeedValue123")
)

func TestSQLiteStore_NewStoreValidationAndSchema(t *testing.T) {
	_, err := NewSessionStore("")
	require.EqualError(t, err, "session sqlite path is required")

	blockerPath := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blockerPath, []byte("x"), 0o600))

	_, err = NewSessionStore(filepath.Join(blockerPath, "session.db"))
	require.ErrorContains(t, err, "failed to create session db directory")

	_, err = NewSessionStore(t.TempDir())
	require.ErrorContains(t, err, "failed to open session db")

	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.Equal(t, "sessions", sessionModel{}.TableName())
	require.Equal(t, "session_archives", archiveModel{}.TableName())
	require.Equal(t, "session_state", stateModel{}.TableName())
	require.Equal(t, "session_summaries", summaryModel{}.TableName())
	require.Equal(t, "session_messages", messageModel{}.TableName())
	require.Equal(t, "archived_session_messages", archivedMessageModel{}.TableName())
	require.True(t, store.db.Migrator().HasTable(&sessionModel{}))
	require.True(t, store.db.Migrator().HasTable(&archiveModel{}))
	require.True(t, store.db.Migrator().HasTable(&stateModel{}))
	require.True(t, store.db.Migrator().HasTable(&summaryModel{}))
	require.True(t, store.db.Migrator().HasTable(&messageModel{}))
	require.True(t, store.db.Migrator().HasTable(&archivedMessageModel{}))
	require.False(t, store.db.Migrator().HasColumn(&sessionModel{}, "messages"))
}

func TestSQLiteStore_SessionLifecycle(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Empty(t, sessions)

	session, ok, err := store.Get(context.Background(), "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	session, ok, err = store.Get(context.Background(), testMissingSession)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	require.EqualError(t, store.Save(context.Background(), Session{}), "session id is required")

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "first", CreatedAt: now},
		{Role: handmsg.RoleAssistant, Name: "bot", Content: "second", ToolCallID: "call-1", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{
		ID:        testSessionA,
		UpdatedAt: now.Add(-time.Minute),
	}))

	loaded, ok, err := store.Get(context.Background(), testSessionB)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionB, loaded.ID)
	require.False(t, loaded.CreatedAt.IsZero())
	createdAt := loaded.CreatedAt
	messages, err := store.GetMessages(context.Background(), testSessionB, MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "first", messages[0].Content)
	require.Equal(t, "bot", messages[1].Name)
	require.Equal(t, "call-1", messages[1].ToolCallID)
	message, ok, err := store.GetMessage(context.Background(), testSessionB, 1, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "second", message.Content)

	messages[0].Content = "mutated"
	loadedAgain, ok, err := store.Get(context.Background(), testSessionB)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionB, loadedAgain.ID)
	require.Equal(t, createdAt, loadedAgain.CreatedAt)
	messagesAgain, err := store.GetMessages(context.Background(), testSessionB, MessageQueryOptions{})
	require.NoError(t, err)
	require.Equal(t, "first", messagesAgain[0].Content)

	require.NoError(t, store.ClearMessages(context.Background(), testSessionB, MessageQueryOptions{}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "replacement", CreatedAt: now.Add(2 * time.Second)},
	}))

	sessions, err = store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, testSessionB, sessions[0].ID)
	require.Equal(t, testSessionA, sessions[1].ID)
	messages, err = store.GetMessages(context.Background(), testSessionB, MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "replacement", messages[0].Content)

	var messageCount int64
	require.NoError(t, store.db.Model(&messageModel{}).Where("session_id = ?", testSessionB).Count(&messageCount).Error)
	require.EqualValues(t, 1, messageCount)

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)

	require.EqualError(t, store.SetCurrent(context.Background(), ""), "session id is required")
	require.EqualError(t, store.SetCurrent(context.Background(), testMissingSession), "session not found")
	require.NoError(t, store.SetCurrent(context.Background(), testSessionB))

	current, ok, err = store.Current(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionB, current)

	require.NoError(t, store.db.Save(&stateModel{
		Key:       currentSessionStateKey,
		Value:     "   ",
		UpdatedAt: now,
	}).Error)

	current, ok, err = store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)
}

func TestSQLiteStore_MessageRoundTripPreservesAssistantToolCalls(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{
			Role:      handmsg.RoleAssistant,
			ToolCalls: []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: `{"zone":"utc"}`}},
			CreatedAt: now,
		},
	}))

	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Empty(t, messages[0].Content)
	require.Equal(t, []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: `{"zone":"utc"}`}}, messages[0].ToolCalls)

	message, ok, err := store.GetMessage(context.Background(), testSessionA, 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, message.Content)
	require.Equal(t, []handmsg.ToolCall{{ID: "call-1", Name: "time", Input: `{"zone":"utc"}`}}, message.ToolCalls)
}

func TestSQLiteStore_GetPreservesZeroUpdatedAt(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	require.NoError(t, store.db.Exec("INSERT INTO sessions (id, updated_at, created_at) VALUES (?, ?, ?)", testSessionZero, time.Time{}, time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)).Error)

	session, ok, err := store.Get(context.Background(), testSessionZero)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, session.UpdatedAt.IsZero())

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, testSessionZero, sessions[0].ID)
	require.True(t, sessions[0].UpdatedAt.IsZero())
}

func TestSQLiteStore_ListOrdersTiesByID(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.db.Exec("INSERT INTO sessions (id, updated_at, created_at) VALUES (?, ?, ?), (?, ?, ?)", testSessionZeta, now, now, testSessionAlpha, now, now).Error)

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, testSessionAlpha, sessions[0].ID)
	require.Equal(t, testSessionZeta, sessions[1].ID)
}

func TestSQLiteStore_Delete(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.EqualError(t, store.Delete(context.Background(), ""), "session id is required")
	require.EqualError(t, store.Delete(context.Background(), DefaultSessionID), "default session cannot be deleted")
	require.EqualError(t, store.Delete(context.Background(), testMissingSession), "session not found")
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.SetCurrent(context.Background(), testSessionA))

	require.NoError(t, store.Delete(context.Background(), testSessionA))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)
	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)
	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)
}

func TestSQLiteStore_ArchiveLifecycle(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{}), "archive id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: testArchiveOne, ExpiresAt: now}), "source session id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: testArchiveOne, SourceSessionID: DefaultSessionID}), "archive expiry is required")

	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "old", CreatedAt: now.Add(-2 * time.Hour)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
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

func TestSQLiteStore_GetArchive(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_DeleteArchive(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_NilReceiverErrors(t *testing.T) {
	var store *SessionStore

	require.EqualError(t, store.Save(context.Background(), Session{ID: testSessionA}), "session store is required")
	require.EqualError(t, store.Delete(context.Background(), testSessionA), "session store is required")
	require.EqualError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}), "session store is required")

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	sessions, err := store.List(context.Background())
	require.EqualError(t, err, "session store is required")
	require.Nil(t, sessions)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: testArchiveOne}), "session store is required")
	require.EqualError(t, store.DeleteArchive(context.Background(), testArchiveOne), "session store is required")
	archive, ok, err := store.GetArchive(context.Background(), testArchiveOne)
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	archives, err := store.ListArchives(context.Background(), "")
	require.EqualError(t, err, "session store is required")
	require.Nil(t, archives)
	messages, err := store.GetMessages(context.Background(), "", MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.Nil(t, messages)
	message, ok, err := store.GetMessage(context.Background(), "", 0, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)
	count, err := store.CountMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.Zero(t, count)
	require.EqualError(t, store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}), "session store is required")
	require.EqualError(t, store.SaveSummary(context.Background(), SessionSummary{SessionID: testSessionA, SessionSummary: "summary"}), "session store is required")
	summary, ok, err := store.GetSummary(context.Background(), testSessionA)
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, SessionSummary{}, summary)
	require.EqualError(t, store.DeleteSummary(context.Background(), testSessionA), "session store is required")

	require.EqualError(t, store.DeleteExpiredArchives(context.Background(), time.Now().UTC()), "session store is required")
	require.EqualError(t, store.SetCurrent(context.Background(), DefaultSessionID), "session store is required")

	current, ok, err := store.Current(context.Background())
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Empty(t, current)
}

func TestSQLiteStore_MessageEncodingHelpers(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.Nil(t, encodeSessionMessages("session-1", nil))
	require.Nil(t, encodeArchivedMessages(testArchiveOne, nil))
	require.Nil(t, decodeSessionMessages(nil))
	require.Nil(t, decodeArchivedMessages(nil))

	sessionRecords := encodeSessionMessages("session-1", []handmsg.Message{
		{Role: handmsg.RoleUser, Name: "user", Content: "hello", ToolCallID: "call-1", CreatedAt: now},
	})
	require.Len(t, sessionRecords, 1)
	require.Equal(t, "session-1", sessionRecords[0].SessionID)
	require.Equal(t, 0, sessionRecords[0].Sequence)

	archiveRecords := encodeArchivedMessages(testArchiveOne, []handmsg.Message{
		{Role: handmsg.RoleAssistant, Name: "assistant", Content: "world", ToolCallID: "call-2", CreatedAt: now.Add(time.Second)},
	})
	require.Len(t, archiveRecords, 1)
	require.Equal(t, testArchiveOne, archiveRecords[0].ArchiveID)
	require.Equal(t, 0, archiveRecords[0].Sequence)

	decodedSession := decodeSessionMessages(sessionRecords)
	require.Len(t, decodedSession, 1)
	require.Equal(t, handmsg.RoleUser, decodedSession[0].Role)
	require.Equal(t, "user", decodedSession[0].Name)
	require.Equal(t, "hello", decodedSession[0].Content)
	require.Equal(t, "call-1", decodedSession[0].ToolCallID)
	require.Equal(t, now, decodedSession[0].CreatedAt)

	decodedArchive := decodeArchivedMessages(archiveRecords)
	require.Len(t, decodedArchive, 1)
	require.Equal(t, handmsg.RoleAssistant, decodedArchive[0].Role)
	require.Equal(t, "assistant", decodedArchive[0].Name)
	require.Equal(t, "world", decodedArchive[0].Content)
	require.Equal(t, "call-2", decodedArchive[0].ToolCallID)
	require.Equal(t, now.Add(time.Second), decodedArchive[0].CreatedAt)
}

func TestSQLiteStore_DecodeRecordHelpers(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	session, err := decodeSessionRecord(sessionModel{ID: testSessionA, UpdatedAt: now})
	require.NoError(t, err)
	require.Equal(t, testSessionA, session.ID)
	require.True(t, session.CreatedAt.IsZero())

	archive, err := decodeArchiveRecord(archiveModel{
		ID:              testArchiveOne,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, testArchiveOne, archive.ID)
}

func TestSQLiteStore_MigrationFailsOnReadOnlyDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.db")
	store, err := NewSessionStore(path)
	require.NoError(t, err)
	require.NoError(t, store.db.Migrator().DropTable(&stateModel{}))
	sqlDB, err := store.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	_, err = NewSessionStore("file:session.db?mode=ro")
	require.ErrorContains(t, err, "failed to migrate session db")
}

func TestSQLiteStore_ConstructorsValidateInputs(t *testing.T) {
	_, err := NewSessionStoreFromDB(nil)
	require.EqualError(t, err, "session db is required")

	_, err = gormOpenSQLite("")
	require.EqualError(t, err, "session sqlite path is required")
}

func TestSQLiteStore_ErrorPathsFromBrokenTables(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("clear messages delete branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))

		err = store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("save parent branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

		err = store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now})
		require.Error(t, err)
	})

	t.Run("get first branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

		_, _, err = store.Get(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("get message branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&sessionModel{ID: testSessionA, UpdatedAt: now}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))

		_, err = store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("list records branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

		_, err = store.List(context.Background())
		require.Error(t, err)
	})

	t.Run("list messages branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&sessionModel{ID: testSessionA, UpdatedAt: now}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))

		_, err = store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("list decode branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Exec("INSERT INTO sessions (id, updated_at, created_at) VALUES (?, ?, ?)", "   ", now, now).Error)

		_, err = store.List(context.Background())
		require.Error(t, err)
	})

	t.Run("append create branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_session_message_insert BEFORE INSERT ON session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}})
		require.Error(t, err)
	})

	t.Run("archive zero messages", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveEmpty,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.EqualError(t, err, "source session has no messages")
		var count int64
		require.NoError(t, store.db.Model(&archiveModel{}).Where("id = ?", testArchiveEmpty).Count(&count).Error)
		require.Zero(t, count)
	})

	t.Run("archive delete branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Migrator().DropTable(&archivedMessageModel{}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveOne,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("archive parent branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Migrator().DropTable(&archiveModel{}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveOne,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("archive create branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_archive_message_insert BEFORE INSERT ON archived_session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveOne,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("list archives records branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&archiveModel{}))

		_, err = store.ListArchives(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("list archives messages branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&archiveModel{
			ID:              testArchiveOne,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&archivedMessageModel{}))

		_, err = store.GetMessages(context.Background(), testArchiveOne, MessageQueryOptions{Archived: true})
		require.Error(t, err)
	})

	t.Run("list archives decode branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Exec("INSERT INTO session_archives (id, source_session_id, archived_at, expires_at, created_at) VALUES (?, ?, ?, ?, ?)", testArchiveOne, "", now, now.Add(time.Hour), now).Error)

		_, err = store.ListArchives(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("delete expired branch", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&archiveModel{}))

		err = store.DeleteExpiredArchives(context.Background(), now)
		require.Error(t, err)
	})

	t.Run("set current get error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

		err = store.SetCurrent(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("current query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&stateModel{}))

		_, _, err = store.Current(context.Background())
		require.Error(t, err)
	})
}

func TestSQLiteStore_SaveRoundTripsLastPromptTokens(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, store.Save(context.Background(), Session{
		ID:               testSessionA,
		LastPromptTokens: 321,
		UpdatedAt:        now,
	}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 321, session.LastPromptTokens)

	session.LastPromptTokens = 42
	require.NoError(t, store.Save(context.Background(), session))
	session, ok, err = store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 42, session.LastPromptTokens)

	session.LastPromptTokens = 0
	require.NoError(t, store.Save(context.Background(), session))
	session, ok, err = store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, session.LastPromptTokens)
}

func TestSQLiteStore_SaveRejectsInvalidSessionID(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	err = store.Save(context.Background(), Session{ID: "ses_invalid"})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
}

func TestSQLiteStore_SavePreservesExistingCreatedAtAndAllowsPromptTokenOverwrite(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_SaveRefreshesUpdatedAtOnUpdate(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_SaveRoundTripsCompactionMetadata(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_SavePreservesExistingCompactionMetadataOnPartialUpdate(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_ClearMessagesClearsCompactionMetadata(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_CreateArchiveClearsDefaultSessionCompactionMetadata(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_SaveTrimsIDOnCreate(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	require.NoError(t, store.Save(context.Background(), Session{ID: "  " + testSessionA + "  "}))

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSessionA, session.ID)
	require.False(t, session.CreatedAt.IsZero())
	require.False(t, session.UpdatedAt.IsZero())
}

func TestSQLiteStore_GetRejectsInvalidSessionID(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	session, ok, err := store.Get(context.Background(), "ses_invalid")

	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.False(t, ok)
	require.Equal(t, Session{}, session)
}

func TestSQLiteStore_AppendMessagesEdgeCases(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	require.EqualError(t, store.AppendMessages(context.Background(), "ses_invalid", []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}), "session id must be a valid ses_ nanoid")
	require.EqualError(t, store.AppendMessages(context.Background(), testMissingSession, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}), "session not found")
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, nil))

	require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))
	require.Error(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}))
}

func TestSQLiteStore_GetMessagesEdgeCases(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	messages, err := store.GetMessages(context.Background(), "", MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)

	messages, err = store.GetMessages(context.Background(), "ses_invalid", MessageQueryOptions{})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Nil(t, messages)

	messages, err = store.GetMessages(context.Background(), "archive_invalid", MessageQueryOptions{Archived: true})
	require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	require.Nil(t, messages)
}

func TestSQLiteStore_GetMessagesSupportsOffsetAndLimit(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_CountMessagesSupportsLiveAndArchivedQueries(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_CountMessagesEdgeCases(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_CountMessagesReturnsQueryErrors(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	t.Run("live query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))

		_, err = store.CountMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("archived query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&archiveModel{
			ID:              testArchiveA,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&archivedMessageModel{}))

		_, err = store.CountMessages(context.Background(), testArchiveA, MessageQueryOptions{Archived: true})
		require.Error(t, err)
	})
}

func TestSQLiteStore_GetMessageEdgeCases(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	message, ok, err := store.GetMessage(context.Background(), "", 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	message, ok, err = store.GetMessage(context.Background(), testSessionA, -1, MessageQueryOptions{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	message, ok, err = store.GetMessage(context.Background(), "ses_invalid", 0, MessageQueryOptions{})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	message, ok, err = store.GetMessage(context.Background(), "archive_invalid", 0, MessageQueryOptions{Archived: true})
	require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)

	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))
	message, ok, err = store.GetMessage(context.Background(), testSessionA, 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handmsg.Message{}, message)
}

func TestSQLiteStore_CreateArchiveErrorBranches(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("source query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveSourceError,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("source delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_session_message_delete BEFORE DELETE ON session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveDeleteError,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("session delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveSessionError,
			SourceSessionID: testSessionA,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("state delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.SetCurrent(context.Background(), testSessionA))
		require.NoError(t, store.db.Migrator().DropTable(&stateModel{}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveStateError,
			SourceSessionID: testSessionA,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})
}

func TestSQLiteStore_GetArchiveErrorBranches(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&archiveModel{}))

		_, _, err = store.GetArchive(context.Background(), testArchiveA)
		require.Error(t, err)
	})

	t.Run("decode error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Exec(
			"INSERT INTO session_archives (id, source_session_id, archived_at, expires_at, created_at) VALUES (?, ?, ?, ?, ?)",
			testArchiveBad, "", now, now.Add(time.Hour), now,
		).Error)

		_, _, err = store.GetArchive(context.Background(), testArchiveBad)
		require.Error(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		_, _, err = store.GetArchive(context.Background(), "archive_invalid")
		require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	})
}

func TestSQLiteStore_ClearMessagesEdgeCases(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("live validation and missing", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		require.EqualError(t, store.ClearMessages(context.Background(), "ses_invalid", MessageQueryOptions{}), "session id must be a valid ses_ nanoid")
		require.EqualError(t, store.ClearMessages(context.Background(), testMissingSession, MessageQueryOptions{}), "session not found")
	})

	t.Run("archived clear success", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveClear,
			SourceSessionID: testSessionA,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}))

		require.NoError(t, store.ClearMessages(context.Background(), testArchiveClear, MessageQueryOptions{Archived: true}))

		messages, err := store.GetMessages(context.Background(), testArchiveClear, MessageQueryOptions{Archived: true})
		require.NoError(t, err)
		require.Nil(t, messages)
	})

	t.Run("archived query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&archiveModel{}))

		err = store.ClearMessages(context.Background(), testArchiveClear, MessageQueryOptions{Archived: true})
		require.Error(t, err)
	})

	t.Run("archived validation error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		err = store.ClearMessages(context.Background(), "archive_invalid", MessageQueryOptions{Archived: true})
		require.EqualError(t, err, "archive id must be a valid arc_ nanoid")
	})
}

func TestSQLiteStore_DeleteErrorBranches(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("first query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

		err = store.Delete(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("message delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_session_message_delete BEFORE DELETE ON session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.Delete(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("session delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.Delete(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("state delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.SetCurrent(context.Background(), testSessionA))
		require.NoError(t, store.db.Migrator().DropTable(&stateModel{}))

		err = store.Delete(context.Background(), testSessionA)
		require.Error(t, err)
	})
}

func TestSQLiteStore_DeleteArchiveErrorBranches(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("first query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&archiveModel{}))

		err = store.DeleteArchive(context.Background(), testArchiveA)
		require.Error(t, err)
	})

	t.Run("archived message delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveA,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_archived_message_delete BEFORE DELETE ON archived_session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.DeleteArchive(context.Background(), testArchiveA)
		require.Error(t, err)
	})
}

func TestSQLiteStore_DeleteExpiredArchivesEdgeCases(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("no expired archives", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveFuture,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}))

		require.NoError(t, store.DeleteExpiredArchives(context.Background(), now))

		archive, ok, err := store.GetArchive(context.Background(), testArchiveFuture)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, testArchiveFuture, archive.ID)
	})

	t.Run("archived message delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveExpired,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(-time.Minute),
		}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_archived_message_delete BEFORE DELETE ON archived_session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.DeleteExpiredArchives(context.Background(), now)
		require.Error(t, err)
	})
}

func TestSQLiteStore_DecodeToolCallsRejectsInvalidJSON(t *testing.T) {
	require.Nil(t, decodeToolCalls("{invalid"))
}

func TestSQLiteStore_SaveReturnsTransactionSaveError(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, store.db.Exec("CREATE TRIGGER fail_session_save BEFORE INSERT ON sessions BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

	err = store.Save(context.Background(), Session{ID: testSessionA})
	require.Error(t, err)
}

func TestSQLiteStore_AppendMessagesReturnsLookupErrorWhenSessionsTableIsUnavailable(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

	err = store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}})
	require.Error(t, err)
}

func TestSQLiteStore_GetMessageReturnsQueryErrors(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("live query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))

		_, _, err = store.GetMessage(context.Background(), testSessionA, 0, MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("archived query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&archiveModel{
			ID:              testArchiveA,
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&archivedMessageModel{}))

		_, _, err = store.GetMessage(context.Background(), testArchiveA, 0, MessageQueryOptions{Archived: true})
		require.Error(t, err)
	})
}

func TestSQLiteStore_ClearMessagesReturnsMissingArchiveAndLiveLookupErrors(t *testing.T) {
	t.Run("missing archive", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		err = store.ClearMessages(context.Background(), testArchiveMissing, MessageQueryOptions{Archived: true})
		require.EqualError(t, err, "archive not found")
	})

	t.Run("live lookup error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

		err = store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.Error(t, err)
	})
}

func TestSQLiteStore_SummaryRoundTripAndCleanup(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

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

func TestSQLiteStore_SummaryErrors(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	t.Run("save summary validation and write errors", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		require.EqualError(t, store.SaveSummary(context.Background(), SessionSummary{}), "session id is required")
		require.EqualError(t, store.SaveSummary(context.Background(), SessionSummary{
			SessionID:      testMissingSession,
			SessionSummary: "summary",
		}), "session not found")

		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&summaryModel{}))

		err = store.SaveSummary(context.Background(), SessionSummary{
			SessionID:      testSessionA,
			SessionSummary: "summary",
		})
		require.Error(t, err)
	})

	t.Run("save summary session lookup error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sessionModel{}))

		err = store.SaveSummary(context.Background(), SessionSummary{
			SessionID:      testSessionA,
			SessionSummary: "summary",
		})
		require.Error(t, err)
	})

	t.Run("get summary validation and read errors", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		summary, ok, err := store.GetSummary(context.Background(), "")
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

		require.NoError(t, store.db.Migrator().DropTable(&summaryModel{}))
		_, _, err = store.GetSummary(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("get summary decode error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&summaryModel{
			SessionID:          testSessionA,
			SourceEndOffset:    3,
			SourceMessageCount: 1,
			UpdatedAt:          now,
			SessionSummary:     "summary",
		}).Error)

		_, _, err = store.GetSummary(context.Background(), testSessionA)
		require.EqualError(t, err, "summary source end offset cannot exceed source message count")
	})

	t.Run("delete summary validation and delete errors", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		require.EqualError(t, store.DeleteSummary(context.Background(), ""), "session id is required")
		require.EqualError(t, store.DeleteSummary(context.Background(), "ses_invalid"), "session id must be a valid ses_ nanoid")

		require.NoError(t, store.db.Migrator().DropTable(&summaryModel{}))
		err = store.DeleteSummary(context.Background(), testSessionA)
		require.Error(t, err)
	})
}

func TestSQLiteStore_SummaryDeleteCleanupErrors(t *testing.T) {
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	t.Run("delete session summary cleanup error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&summaryModel{}))

		err = store.Delete(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("clear messages summary cleanup error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&summaryModel{}))

		err = store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.Error(t, err)
	})
}

func TestSQLiteStore_CreateArchiveReturnsSummaryCleanupError(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.db.Migrator().DropTable(&summaryModel{}))

	err = store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveSummaryError,
		SourceSessionID: testSessionA,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	})
	require.Error(t, err)
}

func TestSQLiteStore_StringEncodingHelpers(t *testing.T) {
	require.Equal(t, "", encodeStrings(nil))
	require.Equal(t, `["one","two"]`, encodeStrings([]string{"one", "two"}))

	require.Nil(t, decodeStrings(""))
	require.Nil(t, decodeStrings("not-json"))
	require.Equal(t, []string{"one", "two"}, decodeStrings(`["one","two"]`))
}
