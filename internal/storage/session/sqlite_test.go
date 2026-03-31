package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handctx "github.com/wandxy/hand/internal/context"
)

func Test_SQLiteStore_NewStoreValidationAndSchema(t *testing.T) {
	_, err := NewSQLiteStore("")
	require.EqualError(t, err, "session sqlite path is required")

	blockerPath := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blockerPath, []byte("x"), 0o600))

	_, err = NewSQLiteStore(filepath.Join(blockerPath, "session.db"))
	require.ErrorContains(t, err, "failed to create session db directory")

	_, err = NewSQLiteStore(t.TempDir())
	require.ErrorContains(t, err, "failed to open session db")

	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.Equal(t, "sessions", sqliteRecord{}.TableName())
	require.Equal(t, "session_archives", sqliteArchiveRecord{}.TableName())
	require.Equal(t, "session_state", sqliteStateRecord{}.TableName())
	require.Equal(t, "session_messages", sqliteMessageRecord{}.TableName())
	require.Equal(t, "archived_session_messages", sqliteArchivedMessageRecord{}.TableName())
	require.True(t, store.db.Migrator().HasTable(&sqliteRecord{}))
	require.True(t, store.db.Migrator().HasTable(&sqliteArchiveRecord{}))
	require.True(t, store.db.Migrator().HasTable(&sqliteStateRecord{}))
	require.True(t, store.db.Migrator().HasTable(&sqliteMessageRecord{}))
	require.True(t, store.db.Migrator().HasTable(&sqliteArchivedMessageRecord{}))
	require.False(t, store.db.Migrator().HasColumn(&sqliteRecord{}, "messages"))
}

func Test_SQLiteStore_SessionLifecycle(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Empty(t, sessions)

	session, ok, err := store.Get(context.Background(), "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	session, ok, err = store.Get(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	require.EqualError(t, store.Save(context.Background(), Session{}), "session id is required")

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-b", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-b", []handctx.Message{
		{Role: handctx.RoleUser, Content: "first", CreatedAt: now},
		{Role: handctx.RoleAssistant, Name: "bot", Content: "second", ToolCallID: "call-1", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{
		ID:        "project-a",
		UpdatedAt: now.Add(-time.Minute),
	}))

	loaded, ok, err := store.Get(context.Background(), "project-b")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "project-b", loaded.ID)
	require.False(t, loaded.CreatedAt.IsZero())
	createdAt := loaded.CreatedAt
	messages, err := store.GetMessages(context.Background(), "project-b", MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "first", messages[0].Content)
	require.Equal(t, "bot", messages[1].Name)
	require.Equal(t, "call-1", messages[1].ToolCallID)
	message, ok, err := store.GetMessage(context.Background(), "project-b", 1, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "second", message.Content)

	messages[0].Content = "mutated"
	loadedAgain, ok, err := store.Get(context.Background(), "project-b")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "project-b", loadedAgain.ID)
	require.Equal(t, createdAt, loadedAgain.CreatedAt)
	messagesAgain, err := store.GetMessages(context.Background(), "project-b", MessageQueryOptions{})
	require.NoError(t, err)
	require.Equal(t, "first", messagesAgain[0].Content)

	require.NoError(t, store.ClearMessages(context.Background(), "project-b", MessageQueryOptions{}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-b", []handctx.Message{
		{Role: handctx.RoleAssistant, Content: "replacement", CreatedAt: now.Add(2 * time.Second)},
	}))

	sessions, err = store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "project-b", sessions[0].ID)
	require.Equal(t, "project-a", sessions[1].ID)
	messages, err = store.GetMessages(context.Background(), "project-b", MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "replacement", messages[0].Content)

	var messageCount int64
	require.NoError(t, store.db.Model(&sqliteMessageRecord{}).Where("session_id = ?", "project-b").Count(&messageCount).Error)
	require.EqualValues(t, 1, messageCount)

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)

	require.EqualError(t, store.SetCurrent(context.Background(), ""), "session id is required")
	require.EqualError(t, store.SetCurrent(context.Background(), "missing"), "session not found")
	require.NoError(t, store.SetCurrent(context.Background(), "project-b"))

	current, ok, err = store.Current(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "project-b", current)

	require.NoError(t, store.db.Save(&sqliteStateRecord{
		Key:       currentSessionStateKey,
		Value:     "   ",
		UpdatedAt: now,
	}).Error)

	current, ok, err = store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)
}

func Test_SQLiteStore_GetPreservesZeroUpdatedAt(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	require.NoError(t, store.db.Exec("INSERT INTO sessions (id, updated_at, created_at) VALUES (?, ?, ?)", "project-zero", time.Time{}, time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)).Error)

	session, ok, err := store.Get(context.Background(), "project-zero")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, session.UpdatedAt.IsZero())

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "project-zero", sessions[0].ID)
	require.True(t, sessions[0].UpdatedAt.IsZero())
}

func Test_SQLiteStore_ListOrdersTiesByID(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.db.Exec("INSERT INTO sessions (id, updated_at, created_at) VALUES (?, ?, ?), (?, ?, ?)", "zeta", now, now, "alpha", now, now).Error)

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "alpha", sessions[0].ID)
	require.Equal(t, "zeta", sessions[1].ID)
}

func Test_SQLiteStore_Delete(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.EqualError(t, store.Delete(context.Background(), ""), "session id is required")
	require.EqualError(t, store.Delete(context.Background(), DefaultSessionID), "default session cannot be deleted")
	require.EqualError(t, store.Delete(context.Background(), "missing"), "session not found")
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.SetCurrent(context.Background(), "project-a"))

	require.NoError(t, store.Delete(context.Background(), "project-a"))

	session, ok, err := store.Get(context.Background(), "project-a")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)
	messages, err := store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
	require.NoError(t, err)
	require.Nil(t, messages)
	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)
}

func Test_SQLiteStore_ArchiveLifecycle(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{}), "archive id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", ExpiresAt: now}), "source session id is required")
	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1", SourceSessionID: DefaultSessionID}), "archive expiry is required")

	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{
		{Role: handctx.RoleUser, Content: "old", CreatedAt: now.Add(-2 * time.Hour)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handctx.Message{
		{Role: handctx.RoleAssistant, Content: "new", CreatedAt: now},
	}))
	require.NoError(t, store.SetCurrent(context.Background(), "project-a"))

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

	archives, err := store.ListArchives(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, "archive-new", archives[0].ID)
	require.Equal(t, "archive-old", archives[1].ID)

	messages, err := store.GetMessages(context.Background(), "archive-new", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Equal(t, "new", messages[0].Content)

	message, ok, err := store.GetMessage(context.Background(), "archive-new", 0, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "new", message.Content)

	message, ok, err = store.GetMessage(context.Background(), "archive-new", 1, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, handctx.Message{}, message)

	filtered, err := store.ListArchives(context.Background(), "project-a")
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

	session, ok, err := store.Get(context.Background(), "project-a")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	liveMessages, err := store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
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

func Test_SQLiteStore_GetArchive(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	archive, ok, err := store.GetArchive(context.Background(), "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	archive, ok, err = store.GetArchive(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ArchivedSession{}, archive)

	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-a",
		SourceSessionID: "project-a",
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	archive, ok, err = store.GetArchive(context.Background(), "archive-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "archive-a", archive.ID)
	require.Equal(t, "project-a", archive.SourceSessionID)
	require.Equal(t, now, archive.ArchivedAt)
	require.Equal(t, now.Add(time.Hour), archive.ExpiresAt)
}

func Test_SQLiteStore_DeleteArchives(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.EqualError(t, store.DeleteArchives(context.Background(), ""), "archive id is required")
	require.EqualError(t, store.DeleteArchives(context.Background(), "missing"), "archive not found")
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-b", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-b", []handctx.Message{
		{Role: handctx.RoleAssistant, Content: "world", CreatedAt: now.Add(time.Second)},
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

	archives, err := store.ListArchives(context.Background(), "project-a")
	require.NoError(t, err)
	require.Empty(t, archives)
	messages, err := store.GetMessages(context.Background(), "archive-a", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Nil(t, messages)
	otherArchives, err := store.ListArchives(context.Background(), "project-b")
	require.NoError(t, err)
	require.Len(t, otherArchives, 1)
}

func Test_SQLiteStore_NilReceiverErrors(t *testing.T) {
	var store *SQLiteStore

	require.EqualError(t, store.Save(context.Background(), Session{ID: "project-a"}), "session store is required")
	require.EqualError(t, store.Delete(context.Background(), "project-a"), "session store is required")

	session, ok, err := store.Get(context.Background(), "project-a")
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	sessions, err := store.List(context.Background())
	require.EqualError(t, err, "session store is required")
	require.Nil(t, sessions)

	require.EqualError(t, store.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1"}), "session store is required")
	require.EqualError(t, store.DeleteArchives(context.Background(), "archive-1"), "session store is required")
	archive, ok, err := store.GetArchive(context.Background(), "archive-1")
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
	require.Equal(t, handctx.Message{}, message)
	require.EqualError(t, store.ClearMessages(context.Background(), "project-a", MessageQueryOptions{}), "session store is required")

	require.EqualError(t, store.DeleteExpiredArchives(context.Background(), time.Now().UTC()), "session store is required")
	require.EqualError(t, store.SetCurrent(context.Background(), DefaultSessionID), "session store is required")

	current, ok, err := store.Current(context.Background())
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Empty(t, current)
}

func Test_SQLiteStore_MessageEncodingHelpers(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.Nil(t, encodeSessionMessages("session-1", nil))
	require.Nil(t, encodeArchivedMessages("archive-1", nil))
	require.Nil(t, decodeSessionMessages(nil))
	require.Nil(t, decodeArchivedMessages(nil))

	sessionRecords := encodeSessionMessages("session-1", []handctx.Message{
		{Role: handctx.RoleUser, Name: "user", Content: "hello", ToolCallID: "call-1", CreatedAt: now},
	})
	require.Len(t, sessionRecords, 1)
	require.Equal(t, "session-1", sessionRecords[0].SessionID)
	require.Equal(t, 0, sessionRecords[0].Sequence)

	archiveRecords := encodeArchivedMessages("archive-1", []handctx.Message{
		{Role: handctx.RoleAssistant, Name: "assistant", Content: "world", ToolCallID: "call-2", CreatedAt: now.Add(time.Second)},
	})
	require.Len(t, archiveRecords, 1)
	require.Equal(t, "archive-1", archiveRecords[0].ArchiveID)
	require.Equal(t, 0, archiveRecords[0].Sequence)

	decodedSession := decodeSessionMessages(sessionRecords)
	require.Len(t, decodedSession, 1)
	require.Equal(t, handctx.RoleUser, decodedSession[0].Role)
	require.Equal(t, "user", decodedSession[0].Name)
	require.Equal(t, "hello", decodedSession[0].Content)
	require.Equal(t, "call-1", decodedSession[0].ToolCallID)
	require.Equal(t, now, decodedSession[0].CreatedAt)

	decodedArchive := decodeArchivedMessages(archiveRecords)
	require.Len(t, decodedArchive, 1)
	require.Equal(t, handctx.RoleAssistant, decodedArchive[0].Role)
	require.Equal(t, "assistant", decodedArchive[0].Name)
	require.Equal(t, "world", decodedArchive[0].Content)
	require.Equal(t, "call-2", decodedArchive[0].ToolCallID)
	require.Equal(t, now.Add(time.Second), decodedArchive[0].CreatedAt)
}

func Test_SQLiteStore_DecodeRecordHelpers(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	session, err := decodeSessionRecord(sqliteRecord{ID: "project-a", UpdatedAt: now})
	require.NoError(t, err)
	require.Equal(t, "project-a", session.ID)
	require.True(t, session.CreatedAt.IsZero())

	archive, err := decodeArchiveRecord(sqliteArchiveRecord{
		ID:              "archive-1",
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, "archive-1", archive.ID)
}

func Test_SQLiteStore_MigrationFailsOnReadOnlyDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.db")
	store, err := NewSQLiteStore(path)
	require.NoError(t, err)
	require.NoError(t, store.db.Migrator().DropTable(&sqliteStateRecord{}))
	sqlDB, err := store.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	_, err = NewSQLiteStore("file:session.db?mode=ro")
	require.ErrorContains(t, err, "failed to migrate session db")
}

func Test_SQLiteStore_ErrorPathsFromBrokenTables(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("clear messages delete branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
		require.NoError(t, store.db.Migrator().DropTable(&sqliteMessageRecord{}))

		err = store.ClearMessages(context.Background(), "project-a", MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("save parent branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteRecord{}))

		err = store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now})
		require.Error(t, err)
	})

	t.Run("get first branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteRecord{}))

		_, _, err = store.Get(context.Background(), "project-a")
		require.Error(t, err)
	})

	t.Run("get message branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&sqliteRecord{ID: "project-a", UpdatedAt: now}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteMessageRecord{}))

		_, err = store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("list records branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteRecord{}))

		_, err = store.List(context.Background())
		require.Error(t, err)
	})

	t.Run("list messages branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&sqliteRecord{ID: "project-a", UpdatedAt: now}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteMessageRecord{}))

		_, err = store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
		require.Error(t, err)
	})

	t.Run("list decode branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Exec("INSERT INTO sessions (id, updated_at, created_at) VALUES (?, ?, ?)", "   ", now, now).Error)

		_, err = store.List(context.Background())
		require.Error(t, err)
	})

	t.Run("append create branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_session_message_insert BEFORE INSERT ON session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.AppendMessages(context.Background(), "project-a", []handctx.Message{{Role: handctx.RoleUser, Content: "hello", CreatedAt: now}})
		require.Error(t, err)
	})

	t.Run("archive zero messages", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              "archive-empty",
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.EqualError(t, err, "source session has no messages")
		var count int64
		require.NoError(t, store.db.Model(&sqliteArchiveRecord{}).Where("id = ?", "archive-empty").Count(&count).Error)
		require.Zero(t, count)
	})

	t.Run("archive delete branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{{Role: handctx.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Migrator().DropTable(&sqliteArchivedMessageRecord{}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              "archive-1",
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("archive parent branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{{Role: handctx.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Migrator().DropTable(&sqliteArchiveRecord{}))

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              "archive-1",
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("archive create branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{{Role: handctx.RoleUser, Content: "hello", CreatedAt: now}}))
		require.NoError(t, store.db.Exec("CREATE TRIGGER fail_archive_message_insert BEFORE INSERT ON archived_session_messages BEGIN SELECT RAISE(FAIL, 'boom'); END;").Error)

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              "archive-1",
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.Error(t, err)
	})

	t.Run("list archives records branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteArchiveRecord{}))

		_, err = store.ListArchives(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("list archives messages branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Create(&sqliteArchiveRecord{
			ID:              "archive-1",
			SourceSessionID: DefaultSessionID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}).Error)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteArchivedMessageRecord{}))

		_, err = store.GetMessages(context.Background(), "archive-1", MessageQueryOptions{Archived: true})
		require.Error(t, err)
	})

	t.Run("list archives decode branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Exec("INSERT INTO session_archives (id, source_session_id, archived_at, expires_at, created_at) VALUES (?, ?, ?, ?, ?)", "archive-1", "", now, now.Add(time.Hour), now).Error)

		_, err = store.ListArchives(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("delete expired branch", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteArchiveRecord{}))

		err = store.DeleteExpiredArchives(context.Background(), now)
		require.Error(t, err)
	})

	t.Run("set current get error", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteRecord{}))

		err = store.SetCurrent(context.Background(), "project-a")
		require.Error(t, err)
	})

	t.Run("current query error", func(t *testing.T) {
		store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.db.Migrator().DropTable(&sqliteStateRecord{}))

		_, _, err = store.Current(context.Background())
		require.Error(t, err)
	})
}
