package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage/retrieval"
	base "github.com/wandxy/hand/internal/storage/session"
	storagevector "github.com/wandxy/hand/internal/storage/vector"
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

func TestSQLiteStore_GetMessagesByIDsReturnsTranscriptOrderedRecords(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "m1", CreatedAt: now},
		{Role: handmsg.RoleAssistant, Content: "m2", CreatedAt: now.Add(time.Second)},
		{Role: handmsg.RoleTool, Name: "process", ToolCallID: "call-1", Content: "m3", CreatedAt: now.Add(2 * time.Second)},
	}))

	records, err := store.GetMessagesByIDs(context.Background(), testSessionA, []uint{3, 1})
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, []uint{1, 3}, []uint{records[0].Message.ID, records[1].Message.ID})
	require.Equal(t, []int{0, 2}, []int{records[0].Offset, records[1].Offset})
}

func TestSQLiteStore_GetMessagesByIDs_ValidationAndErrors(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "m1", CreatedAt: now},
	}))

	records, err := store.GetMessagesByIDs(context.Background(), "", []uint{1})
	require.NoError(t, err)
	require.Nil(t, records)

	records, err = store.GetMessagesByIDs(context.Background(), testSessionA, nil)
	require.NoError(t, err)
	require.Nil(t, records)

	_, err = store.GetMessagesByIDs(context.Background(), "bad-session-id", []uint{1})
	require.ErrorContains(t, err, "session id")

	records, err = store.GetMessagesByIDs(context.Background(), testSessionA, []uint{99})
	require.NoError(t, err)
	require.Nil(t, records)

	boom := errors.New("query failed")
	require.NoError(t, store.db.Callback().Query().Before("gorm:query").Register("test:get-messages-by-ids-error", func(tx *gorm.DB) {
		tx.AddError(boom)
	}))
	defer func() {
		require.NoError(t, store.db.Callback().Query().Remove("test:get-messages-by-ids-error"))
	}()

	_, err = store.GetMessagesByIDs(context.Background(), testSessionA, []uint{1})
	require.ErrorIs(t, err, boom)
}

func TestSQLiteStore_GetMessageWindowReturnsBoundedAnchorContext(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "m1", CreatedAt: now},
		{Role: handmsg.RoleAssistant, Content: "m2", CreatedAt: now.Add(time.Second)},
		{Role: handmsg.RoleTool, Name: "process", ToolCallID: "call-1", Content: "m3", CreatedAt: now.Add(2 * time.Second)},
		{Role: handmsg.RoleAssistant, Content: "m4", CreatedAt: now.Add(3 * time.Second)},
	}))

	records, err := store.GetMessageWindow(context.Background(), testSessionA, 3, 1, 1)
	require.NoError(t, err)
	require.Len(t, records, 3)
	require.Equal(t, []int{1, 2, 3}, []int{records[0].Offset, records[1].Offset, records[2].Offset})
	require.Equal(t, []uint{2, 3, 4}, []uint{records[0].Message.ID, records[1].Message.ID, records[2].Message.ID})
}

func TestSQLiteStore_GetMessageWindow_ValidationNotFoundAndErrors(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "m1", CreatedAt: now},
		{Role: handmsg.RoleAssistant, Content: "m2", CreatedAt: now.Add(time.Second)},
	}))

	records, err := store.GetMessageWindow(context.Background(), "", 1, 0, 0)
	require.NoError(t, err)
	require.Nil(t, records)

	records, err = store.GetMessageWindow(context.Background(), testSessionA, 0, 0, 0)
	require.NoError(t, err)
	require.Nil(t, records)

	_, err = store.GetMessageWindow(context.Background(), "bad-session-id", 1, 0, 0)
	require.ErrorContains(t, err, "session id")

	_, err = store.GetMessageWindow(context.Background(), testSessionA, 1, -1, 0)
	require.EqualError(t, err, "before and after must be greater than or equal to zero")

	records, err = store.GetMessageWindow(context.Background(), testSessionA, 99, 1, 1)
	require.NoError(t, err)
	require.Nil(t, records)

	boom := errors.New("anchor lookup failed")
	require.NoError(t, store.db.Callback().Query().Before("gorm:query").Register("test:get-message-window-anchor-error", func(tx *gorm.DB) {
		tx.AddError(boom)
	}))

	_, err = store.GetMessageWindow(context.Background(), testSessionA, 1, 0, 0)
	require.ErrorIs(t, err, boom)
	require.NoError(t, store.db.Callback().Query().Remove("test:get-message-window-anchor-error"))

	queryCount := 0
	boom = errors.New("window lookup failed")
	require.NoError(t, store.db.Callback().Query().Before("gorm:query").Register("test:get-message-window-range-error", func(tx *gorm.DB) {
		queryCount++
		if queryCount == 2 {
			tx.AddError(boom)
		}
	}))
	defer func() {
		require.NoError(t, store.db.Callback().Query().Remove("test:get-message-window-range-error"))
	}()

	_, err = store.GetMessageWindow(context.Background(), testSessionA, 1, 0, 1)
	require.ErrorIs(t, err, boom)
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
	require.EqualError(
		t,
		store.AppendMessages(
			context.Background(),
			testSessionA,
			[]handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}},
		),
		"session store is required",
	)

	session, ok, err := store.Get(context.Background(), testSessionA)
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	sessions, err := store.List(context.Background())
	require.EqualError(t, err, "session store is required")
	require.Nil(t, sessions)

	require.EqualError(
		t,
		store.CreateArchive(context.Background(), ArchivedSession{ID: testArchiveOne}),
		"session store is required",
	)
	require.EqualError(
		t,
		store.DeleteArchive(context.Background(), testArchiveOne),
		"session store is required",
	)

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

	records, err := store.GetMessagesByIDs(context.Background(), testSessionA, []uint{1})
	require.EqualError(t, err, "session store is required")
	require.Nil(t, records)
	records, err = store.GetMessageWindow(context.Background(), testSessionA, 1, 0, 0)
	require.EqualError(t, err, "session store is required")
	require.Nil(t, records)

	count, err := store.CountMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.Zero(t, count)

	require.EqualError(
		t,
		store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}),
		"session store is required",
	)
	require.EqualError(
		t,
		store.SaveSummary(
			context.Background(),
			SessionSummary{SessionID: testSessionA, SessionSummary: "summary"},
		),
		"session store is required",
	)

	summary, ok, err := store.GetSummary(context.Background(), testSessionA)
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Equal(t, SessionSummary{}, summary)

	require.EqualError(
		t,
		store.DeleteSummary(context.Background(), testSessionA),
		"session store is required",
	)

	require.EqualError(
		t,
		store.DeleteExpiredArchives(context.Background(), time.Now().UTC()),
		"session store is required",
	)
	require.EqualError(
		t,
		store.SetCurrent(context.Background(), DefaultSessionID),
		"session store is required",
	)

	current, ok, err := store.Current(context.Background())
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Empty(t, current)
}

func TestSQLiteStore_MessageEncodingHelpers(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	require.Nil(t, messagesToMessageModels("session-1", nil))
	require.Nil(t, messagesToArchivedMessageModels(testArchiveOne, nil))
	require.Nil(t, messageModelsToMessages(nil))
	require.Nil(t, archivedMessageModelsToMessages(nil))

	sessionRecords := messagesToMessageModels("session-1", []handmsg.Message{
		{
			ID:         1,
			Role:       handmsg.RoleUser,
			Name:       "user",
			Content:    "hello",
			ToolCallID: "call-1",
			CreatedAt:  now,
		},
	})
	require.Len(t, sessionRecords, 1)
	require.Equal(t, "session-1", sessionRecords[0].SessionID)
	require.Equal(t, 0, sessionRecords[0].Sequence)
	require.Equal(t, 1, int(sessionRecords[0].ID))

	archiveRecords := messagesToArchivedMessageModels(testArchiveOne, []handmsg.Message{
		{
			ID:         2,
			Role:       handmsg.RoleAssistant,
			Name:       "assistant",
			Content:    "world",
			ToolCallID: "call-2",
			CreatedAt:  now.Add(time.Second),
		},
	})
	require.Len(t, archiveRecords, 1)
	require.Equal(t, testArchiveOne, archiveRecords[0].ArchiveID)
	require.Equal(t, 0, archiveRecords[0].Sequence)

	decodedSession := messageModelsToMessages(sessionRecords)
	require.Len(t, decodedSession, 1)
	require.Equal(t, sessionRecords[0].ID, decodedSession[0].ID)
	require.Equal(t, handmsg.RoleUser, decodedSession[0].Role)
	require.Equal(t, "user", decodedSession[0].Name)
	require.Equal(t, "hello", decodedSession[0].Content)
	require.Equal(t, "call-1", decodedSession[0].ToolCallID)
	require.Equal(t, 1, int(decodedSession[0].ID))
	require.Equal(t, now, decodedSession[0].CreatedAt)

	decodedArchive := archivedMessageModelsToMessages(archiveRecords)
	require.Len(t, decodedArchive, 1)
	require.Equal(t, archiveRecords[0].ID, decodedArchive[0].ID)
	require.Equal(t, handmsg.RoleAssistant, decodedArchive[0].Role)
	require.Equal(t, "assistant", decodedArchive[0].Name)
	require.Equal(t, "world", decodedArchive[0].Content)
	require.Equal(t, "call-2", decodedArchive[0].ToolCallID)
	require.Equal(t, 2, int(decodedArchive[0].ID))
	require.Equal(t, now.Add(time.Second), decodedArchive[0].CreatedAt)
}

func TestSQLiteStore_GetAndHelperFunctionsEdgeCases(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	session, ok, err := store.Get(context.Background(), "ses_invalid")
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.False(t, ok)
	require.Equal(t, Session{}, session)

	require.Equal(t, base.MessageOrderAsc, messageQueryOrder(MessageQueryOptions{}))
	require.Equal(t, base.MessageOrderAsc, messageQueryOrder(MessageQueryOptions{Order: "bogus"}))

	message := handmsg.Message{
		Role:    handmsg.RoleTool,
		Content: `{"status":"running"}`,
	}
	require.Contains(t, handmsg.MessageSearchText(message), "status running")
	require.Empty(t, handmsg.MessageSearchText(handmsg.Message{Role: handmsg.RoleUser, Content: "plain"}))

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.db.Callback().Query().After("gorm:after_query").Register("test:blank_session_id", func(tx *gorm.DB) {
		record, ok := tx.Statement.Dest.(*sessionModel)
		if ok {
			record.ID = "   "
		}
	}))
	t.Cleanup(func() {
		store.db.Callback().Query().Remove("test:blank_session_id")
	})

	session, ok, err = store.Get(context.Background(), testSessionA)
	require.EqualError(t, err, "session id is required")
	require.False(t, ok)
	require.Equal(t, Session{}, session)
}

func TestSQLiteStore_ArchivedFiltersSupportName(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleTool, Name: "process", Content: "one", CreatedAt: now},
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: "two", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              testArchiveA,
		SourceSessionID: DefaultSessionID,
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	messages, err := store.GetMessages(context.Background(), testArchiveA, MessageQueryOptions{
		Archived: true,
		Role:     handmsg.RoleTool,
		Name:     "plan_tool",
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "two", messages[0].Content)
}

func TestSQLiteStore_GetMessagesPopulatesMessageIDs(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA}))

	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello"},
		{Role: handmsg.RoleAssistant, Content: "world"},
	}))

	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.NotZero(t, messages[0].ID)
	require.NotZero(t, messages[1].ID)
	require.NotEqual(t, messages[0].ID, messages[1].ID)
}

func TestSQLiteStore_DecodeRecordHelpers(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	session, err := sessionModelToSession(sessionModel{ID: testSessionA, UpdatedAt: now})
	require.NoError(t, err)
	require.Equal(t, testSessionA, session.ID)
	require.True(t, session.CreatedAt.IsZero())

	archive, err := archiveModelToArchivedSession(archiveModel{
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
		Compaction: SessionCompaction{
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
		Compaction: SessionCompaction{
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
		Compaction: SessionCompaction{
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
	require.Equal(t, SessionCompaction{}, session.Compaction)
}

func TestSQLiteStore_CreateArchiveClearsDefaultSessionCompactionMetadata(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{
		ID: DefaultSessionID,
		Compaction: SessionCompaction{
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
	require.Equal(t, SessionCompaction{}, session.Compaction)
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

func TestSQLiteStore_GetMessagesRejectsInvalidOrder(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{Order: "sideways"})

	require.EqualError(t, err, "message order must be asc or desc")
	require.Nil(t, messages)
}

func TestSQLiteStore_CountMessagesRejectsInvalidOrder(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	count, err := store.CountMessages(context.Background(), testSessionA, MessageQueryOptions{Order: "sideways"})

	require.EqualError(t, err, "message order must be asc or desc")
	require.Zero(t, count)
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

func TestSQLiteStore_GetAndCountMessagesSupportRoleAndNameFilters(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: "plan-1", ToolCallID: "call-1", CreatedAt: now},
		{Role: handmsg.RoleTool, Name: "other_tool", Content: "other", ToolCallID: "call-2", CreatedAt: now},
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: "plan-2", ToolCallID: "call-3", CreatedAt: now},
	}))

	count, err := store.CountMessages(context.Background(), testSessionA, MessageQueryOptions{
		Role: handmsg.RoleTool,
		Name: "plan_tool",
	})
	require.NoError(t, err)
	require.Equal(t, 2, count)

	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{
		Role:   handmsg.RoleTool,
		Name:   "plan_tool",
		Offset: 1,
		Limit:  1,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "plan-2", messages[0].Content)
	require.Equal(t, "plan_tool", messages[0].Name)
}

func TestSQLiteStore_SearchMessagesSupportsStructuredAndToolFilters(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello plain text", CreatedAt: now},
		{Role: handmsg.RoleAssistant, Content: "assistant summary", ToolCalls: []handmsg.ToolCall{
			{ID: "call-1", Name: "process", Input: `{"action":"start"}`},
			{ID: "call-2", Name: "search_files", Input: `{"pattern":"needle"}`},
		}, CreatedAt: now.Add(time.Second)},
		{Role: handmsg.RoleTool, Name: "process", Content: `{"status":"running"}`,
			ToolCallID: "call-3", CreatedAt: now.Add(2 * time.Second)},
	}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query: "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 1, results[0].MatchCount)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, handmsg.RoleAssistant, results[0].Messages[0].Message.Role)
	require.Equal(t, "search_files", results[0].Messages[0].MatchedToolName)

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query: "summary",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, handmsg.RoleAssistant, results[0].Messages[0].Message.Role)
	require.Equal(t, "assistant summary", results[0].Messages[0].MatchedText)

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:    "needle",
		ToolName: "process",
	})
	require.NoError(t, err)
	require.Empty(t, results)

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:    "running",
		ToolName: "process",
		Role:     handmsg.RoleTool,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, "process", results[0].Messages[0].Message.Name)
	require.Equal(t, "call-3", results[0].Messages[0].Message.ToolCallID)
	require.Equal(t, "process", results[0].Messages[0].MatchedToolName)
}

func TestSQLiteStore_SearchMessagesUsesDerivedStructuredSearchText(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
		Role: handmsg.RoleAssistant,
		ToolCalls: []handmsg.ToolCall{
			{ID: "call-1", Name: "process", Input: `{"action":"start"}`},
			{ID: "call-2", Name: "search_files", Input: `{"pattern":"needle"}`},
		},
		CreatedAt: now,
	}}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query: "tool_name process",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, handmsg.RoleAssistant, results[0].Messages[0].Message.Role)
	require.Equal(t, "process", results[0].Messages[0].MatchedToolName)

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:    "tool_name process",
		ToolName: "process",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestSQLiteStore_SearchMessagesSelectsBestDuplicateFTSRowByScore(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
		Role:    handmsg.RoleAssistant,
		Content: "needle needle needle durable ranking context",
		ToolCalls: []handmsg.ToolCall{
			{ID: "call-1", Name: "lookup", Input: `{"pattern":"needle"}`},
		},
		CreatedAt: now,
	}}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query: "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, "needle needle needle durable ranking context", results[0].Messages[0].MatchedText)
	require.Empty(t, results[0].Messages[0].MatchedToolName)
}

func TestSQLiteStore_SearchMessagesSupportsCrossSessionScope(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "origin needle", CreatedAt: now},
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "other needle", CreatedAt: now.Add(time.Second)},
	}))

	results, err := store.SearchMessages(context.Background(), "", SearchMessageOptions{
		IgnoreSessionID: testSessionA,
		Query:           "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, testSessionB, results[0].SessionID)
	require.Equal(t, "other needle", results[0].Messages[0].MatchedText)

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query: "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "origin needle", results[0].Messages[0].MatchedText)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{
		Query: "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, []string{testSessionB, testSessionA}, []string{
		results[0].SessionID,
		results[1].SessionID,
	})
	require.Equal(t, "other needle", results[0].Messages[0].MatchedText)
	require.Equal(t, "origin needle", results[1].Messages[0].MatchedText)

	// search session with ignore session id
	// ignore directive has no effect when session id is provided
	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		IgnoreSessionID: testSessionA,
		Query:           "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "origin needle", results[0].Messages[0].MatchedText)
}

func TestSQLiteStore_SearchMessagesGroupedEdgeCases(t *testing.T) {
	var nilStore *SessionStore

	results, err := nilStore.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{Query: "hello"})
	require.EqualError(t, err, "session store is required")
	require.Nil(t, results)

	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{Query: "hello"})
	require.NoError(t, err)
	require.Nil(t, results)

	results, err = store.SearchMessages(context.Background(), "ses_invalid", SearchMessageOptions{Query: "hello"})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Nil(t, results)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{
		IgnoreSessionID: "ses_invalid",
		Query:           "hello",
	})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Nil(t, results)

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{})
	require.NoError(t, err)
	require.Nil(t, results)
}

func TestSQLiteStore_SearchMessagesSupportsGroupedLimitsAndQueryErrors(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello zero", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "hello one", CreatedAt: now.Add(time.Second)},
		{Role: handmsg.RoleUser, Content: "hello two", CreatedAt: now.Add(2 * time.Second)},
	}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:                 "hello",
		MaxMessagesPerSession: 2,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 3, results[0].MatchCount)
	require.Equal(t, []string{"hello two", "hello one"}, []string{
		results[0].Messages[0].MatchedText,
		results[0].Messages[1].MatchedText,
	})

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:    "hello",
		ToolName: "missing",
	})
	require.NoError(t, err)
	require.Nil(t, results)

	require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))
	_, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{Query: "hello"})
	require.Error(t, err)
}

func TestSQLiteStore_SearchMessagesSupportsGroupedResults(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "needle zero", CreatedAt: now},
		{Role: handmsg.RoleAssistant, Content: "needle one", CreatedAt: now.Add(time.Second)},
		{Role: handmsg.RoleUser, Content: "needle two", CreatedAt: now.Add(2 * time.Second)},
	}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:                 "needle",
		MaxMessagesPerSession: 2,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, testSessionA, results[0].SessionID)
	require.Equal(t, 3, results[0].MatchCount)
	require.Equal(t, now.Add(2*time.Second), results[0].LastMatchedAt)
	require.Equal(t, []string{"needle two", "needle one"}, []string{
		results[0].Messages[0].MatchedText,
		results[0].Messages[1].MatchedText,
	})
}

func TestSQLiteStore_SearchMessagesRanksByRelevanceBeforeRecency(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "needle durable needle durable needle durable", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "needle durable", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "needle durable", CreatedAt: now.Add(2 * time.Second)},
	}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:                 "needle durable",
		MaxMessagesPerSession: 1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 2, results[0].MatchCount)
	require.Equal(t, "needle durable needle durable needle durable", results[0].Messages[0].MatchedText)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{
		Query:       "needle durable",
		MaxSessions: 1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, testSessionA, results[0].SessionID)
	require.Equal(t, 2, results[0].MatchCount)
}

func TestSQLiteStore_SearchMessagesUsesRecencyWhenScoresTie(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "needle tie", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "needle tie", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "needle tie", CreatedAt: now.Add(2 * time.Second)},
	}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:                 "needle tie",
		MaxMessagesPerSession: 1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 2, results[0].MatchCount)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, now.Add(time.Second), results[0].Messages[0].Message.CreatedAt)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{
		Query:       "needle tie",
		MaxSessions: 1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, testSessionB, results[0].SessionID)
	require.Equal(t, now.Add(2*time.Second), results[0].LastMatchedAt)
}

func TestSQLiteStore_SearchMessagesSupportsCrossSessionScopeAndOrdering(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionB, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "needle alpha", CreatedAt: now},
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionB, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "needle beta", CreatedAt: now},
	}))

	results, err := store.SearchMessages(context.Background(), "", SearchMessageOptions{
		Query:       "needle",
		MaxSessions: 1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, testSessionA, results[0].SessionID)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{
		IgnoreSessionID: testSessionA,
		Query:           "needle",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, testSessionB, results[0].SessionID)
}

func TestSQLiteStore_SearchMessagesEdgeCases(t *testing.T) {
	var nilStore *SessionStore

	results, err := nilStore.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{Query: "hello"})
	require.EqualError(t, err, "session store is required")
	require.Nil(t, results)

	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{Query: "hello"})
	require.NoError(t, err)
	require.Nil(t, results)

	results, err = store.SearchMessages(context.Background(), "ses_invalid", SearchMessageOptions{Query: "hello"})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Nil(t, results)

	results, err = store.SearchMessages(context.Background(), "", SearchMessageOptions{
		IgnoreSessionID: "ses_invalid",
		Query:           "hello",
	})
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	require.Nil(t, results)

	results, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{})
	require.NoError(t, err)
	require.Nil(t, results)

	require.NoError(t, store.db.Migrator().DropTable(&messageModel{}))
	_, err = store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{Query: "hello"})
	require.Error(t, err)
}

func TestSQLiteStore_SearchMessagesSupportsRoleAndToolFilters(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleAssistant, ToolCalls: []handmsg.ToolCall{
			{ID: "call-1", Name: "process", Input: `{"action":"start"}`},
		}, CreatedAt: now},
		{Role: handmsg.RoleTool, Name: "process", Content: `{"status":"running"}`,
			ToolCallID: "call-1", CreatedAt: now.Add(time.Second)},
	}))

	results, err := store.SearchMessages(context.Background(), testSessionA, SearchMessageOptions{
		Query:    "running",
		Role:     handmsg.RoleTool,
		ToolName: "process",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Messages, 1)
	require.Equal(t, "process", results[0].Messages[0].MatchedToolName)
}

func TestSearchMessageResultTime_EdgeCases(t *testing.T) {
	require.True(t, searchSessionResultTime("").IsZero())
	require.True(t, searchSessionResultTime("not-a-time").IsZero())
}

func TestSQLiteStore_GetMessagesSupportsDescendingOrder(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: "plan-1", ToolCallID: "call-1", CreatedAt: now},
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: "plan-2", ToolCallID: "call-2", CreatedAt: now},
	}))

	messages, err := store.GetMessages(context.Background(), testSessionA, MessageQueryOptions{
		Role:  handmsg.RoleTool,
		Name:  "plan_tool",
		Order: "desc",
		Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "plan-2", messages[0].Content)
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
	require.Nil(t, jsonToToolCalls("{invalid"))
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
	require.Equal(t, "", stringsToJSON(nil))
	require.Equal(t, `["one","two"]`, stringsToJSON([]string{"one", "two"}))

	require.Nil(t, jsonToStrings(""))
	require.Nil(t, jsonToStrings("not-json"))
	require.Equal(t, []string{"one", "two"}, jsonToStrings(`["one","two"]`))
}

func TestSQLiteStore_SearchIndexHelpers(t *testing.T) {
	t.Run("ensure search index validates db and surfaces create errors", func(t *testing.T) {
		require.EqualError(t, ensureSearchIndex(nil), "session db is required")
	})

	t.Run("ensure search index and store creation fail on readonly databases", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "session.db")

		writableDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, writableDB.AutoMigrate(
			&sessionModel{},
			&archiveModel{},
			&stateModel{},
			&summaryModel{},
			&messageModel{},
			&archivedMessageModel{},
		))

		readonlyDB, err := gorm.Open(sqlite.Open("file:"+path+"?mode=ro"), &gorm.Config{})
		require.NoError(t, err)

		err = ensureSearchIndex(readonlyDB)
		require.ErrorContains(t, err, "failed to create session message search index")

		_, err = NewSessionStoreFromDB(readonlyDB)
		require.ErrorContains(t, err, "failed to create session message search index")
	})

	t.Run("insert and delete search rows handle no-op and query errors", func(t *testing.T) {
		require.NoError(t, insertSearchRows(nil, []searchRow{{MessageID: 1}}))

		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)

		require.NoError(t, insertSearchRows(store.db, nil))
		require.NoError(t, deleteSearchRows(nil, testSessionA))

		require.NoError(t, store.db.Exec(`DROP TABLE `+sessionMessageSearchTable).Error)

		err = insertSearchRows(store.db, []searchRow{{
			MessageID: 1,
			SessionID: testSessionA,
			Role:      "user",
			Body:      "hello",
		}})
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to insert session message search row")

		err = deleteSearchRows(store.db, testSessionA)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to delete session message search rows")
	})

	t.Run("message model search row conversions cover remaining branches", func(t *testing.T) {
		require.Nil(t, searchRowsFromMessageModels(nil))

		rows := searchRowsFromMessageModels([]messageModel{
			{
				ID:        1,
				SessionID: testSessionA,
				Role:      string(handmsg.RoleUser),
				Content:   "plain text",
			},
			{
				ID:        2,
				SessionID: testSessionA,
				Role:      string(handmsg.RoleTool),
				Name:      "process",
				Content:   "running",
			},
		})
		require.Len(t, rows, 2)

		require.Nil(t, searchRowsFromMessageModel(messageModel{
			ID:        3,
			SessionID: testSessionA,
			Role:      string(handmsg.RoleAssistant),
		}))

		require.Nil(t, searchRowsFromMessageModel(messageModel{
			ID:        4,
			SessionID: testSessionA,
			Role:      string(handmsg.RoleTool),
			Name:      "process",
		}))

		require.Nil(t, searchRowsFromMessageModel(messageModel{
			ID:        5,
			SessionID: testSessionA,
			Role:      string(handmsg.RoleUser),
		}))

		rows = searchRowsFromMessageModel(messageModel{
			ID:        6,
			SessionID: testSessionA,
			Role:      string(handmsg.RoleAssistant),
			Content:   "assistant fallback",
		})
		require.Len(t, rows, 1)
		require.Equal(t, "assistant fallback", rows[0].Body)

		rows = searchRowsFromMessageModel(messageModel{
			ID:        7,
			SessionID: testSessionA,
			Role:      string(handmsg.RoleAssistant),
			Content:   "search fallback",
		})
		require.Len(t, rows, 1)
		require.Equal(t, "search fallback", rows[0].Body)

		rows = searchRowsFromMessageModel(messageModel{
			ID:        8,
			SessionID: testSessionA,
			Role:      string(handmsg.RoleAssistant),
			ToolCalls: `[{}]`,
		})
		require.Empty(t, rows)

		rows = searchRowsFromMessageModel(messageModel{
			ID:        9,
			SessionID: testSessionA,
			Role:      string(handmsg.RoleAssistant),
			Content:   "assistant summary",
			ToolCalls: `[{"id":"call-1","name":"process","input":"{\"action\":\"start\"}"}]`,
		})
		require.Len(t, rows, 2)
		require.Equal(t, "assistant summary", rows[0].Body)
		require.Equal(t, "process", rows[1].ToolName)
	})

	t.Run("search tokenization drops punctuation-only segments", func(t *testing.T) {
		require.Nil(t, searchTokens("... ---"))
	})
}

func TestSQLiteStore_FTSErrorPathsFromBrokenIndex(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	t.Run("append messages returns fts insert errors", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Exec(`DROP TABLE `+sessionMessageSearchTable).Error)

		err = store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		})
		require.ErrorContains(t, err, "failed to insert session message search row")
	})

	t.Run("delete returns fts delete errors", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		}))
		require.NoError(t, store.db.Exec(`DROP TABLE `+sessionMessageSearchTable).Error)

		err = store.Delete(context.Background(), testSessionA)
		require.ErrorContains(t, err, "failed to delete session message search rows")
	})

	t.Run("create archive returns fts delete errors", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		}))
		require.NoError(t, store.db.Exec(`DROP TABLE `+sessionMessageSearchTable).Error)

		err = store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveOne,
			SourceSessionID: testSessionA,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.ErrorContains(t, err, "failed to delete session message search rows")
	})

	t.Run("clear messages returns fts delete errors", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		}))
		require.NoError(t, store.db.Exec(`DROP TABLE `+sessionMessageSearchTable).Error)

		err = store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{})
		require.ErrorContains(t, err, "failed to delete session message search rows")
	})
}

func TestSQLiteStore_VectorStoreAppendAndRebuild(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	provider := &sqliteTestEmbeddingProvider{dimensions: 3}
	vectorStore := &sqliteTestVectorStore{}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: provider,
		VectorStore:       vectorStore,
		EmbeddingModel:    "text-embedding-test",
	}))

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{{
		Role:    handmsg.RoleAssistant,
		Content: "assistant summary",
		ToolCalls: []handmsg.ToolCall{
			{ID: "call-1", Name: "process", Input: `{"action":"start"}`},
			{ID: "call-2", Name: "search", Input: `{"query":"needle"}`},
		},
		CreatedAt: now,
	}}))

	require.Len(t, provider.requests, 1)
	require.Equal(t, "text-embedding-test", provider.requests[0].Model)
	require.Len(t, provider.requests[0].Inputs, 3)
	require.Equal(t, "assistant summary", provider.requests[0].Inputs[0].Text)
	require.Contains(t, provider.requests[0].Inputs[1].Text, "process")
	require.Contains(t, provider.requests[0].Inputs[2].Text, "search")

	require.Len(t, vectorStore.upserts, 1)
	require.Len(t, vectorStore.upserts[0], 3)
	sourceID := vectorStore.upserts[0][0].SourceID
	require.Equal(t, retrieval.SourceKindSessionMessage, vectorStore.upserts[0][0].SourceKind)
	require.Equal(t, sourceID+":row:1", vectorStore.upserts[0][0].ID)
	require.Equal(t, sourceID+":row:2", vectorStore.upserts[0][1].ID)
	require.Equal(t, sourceID+":row:3", vectorStore.upserts[0][2].ID)
	require.Equal(t, sourceID, vectorStore.upserts[0][1].SourceID)
	require.Equal(t, sourceID, vectorStore.upserts[0][2].SourceID)

	require.NoError(t, store.RebuildVectorStore(context.Background(), testSessionA))
	require.Len(t, vectorStore.deletes, 1)
	require.Equal(t, sourceID, vectorStore.deletes[0].SourceID)
	require.Len(t, vectorStore.upserts, 2)
	require.Equal(t, sqliteTestRecordIDs(vectorStore.upserts[0]), sqliteTestRecordIDs(vectorStore.upserts[1]))
	require.Equal(t, vectorStore.upserts[0][0].ContentHash, vectorStore.upserts[1][0].ContentHash)
}

func TestSQLiteStore_VectorStoreDeletesSessionVectors(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	t.Run("clear messages deletes vectors", func(t *testing.T) {
		store, vectorStore := sqliteVectorStoreTestStore(t)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		}))
		sourceID := vectorStore.upserts[0][0].SourceID

		require.NoError(t, store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}))

		require.Equal(t, []retrieval.VectorDeleteRequest{{
			SourceKind: retrieval.SourceKindSessionMessage,
			SourceID:   sourceID,
		}}, vectorStore.deletes)
	})

	t.Run("delete session deletes vectors", func(t *testing.T) {
		store, vectorStore := sqliteVectorStoreTestStore(t)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		}))
		sourceID := vectorStore.upserts[0][0].SourceID

		require.NoError(t, store.Delete(context.Background(), testSessionA))

		require.Equal(t, []retrieval.VectorDeleteRequest{{
			SourceKind: retrieval.SourceKindSessionMessage,
			SourceID:   sourceID,
		}}, vectorStore.deletes)
	})

	t.Run("archive session deletes source vectors", func(t *testing.T) {
		store, vectorStore := sqliteVectorStoreTestStore(t)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now},
		}))
		sourceID := vectorStore.upserts[0][0].SourceID

		require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveOne,
			SourceSessionID: testSessionA,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		}))

		require.Equal(t, []retrieval.VectorDeleteRequest{{
			SourceKind: retrieval.SourceKindSessionMessage,
			SourceID:   sourceID,
		}}, vectorStore.deletes)
	})
}

func TestSQLiteStore_VectorStoreUsesSharedSQLiteVectorStore(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "hand.db")))
	require.NoError(t, err)

	store, err := NewSessionStoreFromDB(db)
	require.NoError(t, err)
	vectorStore, err := storagevector.NewStoreFromDB(db)
	require.NoError(t, err)
	provider := &sqliteTestEmbeddingProvider{dimensions: 3}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: provider,
		VectorStore:       vectorStore,
		EmbeddingModel:    "text-embedding-test",
	}))

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "shared sqlite vector", CreatedAt: now},
	}))

	embedRes, err := provider.Embed(context.Background(), retrieval.EmbeddingRequest{
		Model: "text-embedding-test",
		Inputs: []retrieval.EmbeddingInput{{
			ID:         "query",
			Text:       "shared sqlite vector",
			SourceKind: retrieval.SourceKindSessionMessage,
		}},
	})
	require.NoError(t, err)

	result, err := vectorStore.Search(context.Background(), storagevector.SearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    embedRes.Items[0].Vector,
		Limit:          1,
		Filter: storagevector.Filter{
			SourceKind: storagevector.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Matches, 1)
	require.Equal(t, retrieval.SourceKindSessionMessage, result.Matches[0].Record.SourceKind)
	require.Equal(t, retrieval.VectorContentHash("shared sqlite vector"), result.Matches[0].Record.ContentHash)

	require.NoError(t, store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}))
	result, err = vectorStore.Search(context.Background(), storagevector.SearchRequest{
		EmbeddingModel: "text-embedding-test",
		Dimensions:     3,
		QueryVector:    embedRes.Items[0].Vector,
		Limit:          1,
		Filter: storagevector.Filter{
			SourceKind: storagevector.SourceKindSessionMessage,
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Matches)
}

func TestSQLiteStore_VectorStoreBestEffortAndRequiredErrors(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	embedErr := errors.New("embed failed")

	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{err: embedErr},
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
	}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "best effort", CreatedAt: now},
	}))

	requiredStore, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, requiredStore.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, requiredStore.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{err: embedErr},
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
		Required:          true,
	}))
	err = requiredStore.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "required", CreatedAt: now},
	})
	require.ErrorIs(t, err, embedErr)
}

func TestSQLiteStore_RebuildVectorStoreContinuesAfterBestEffortDeleteError(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	vectorStore := &sqliteTestVectorStore{}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       vectorStore,
		EmbeddingModel:    "text-embedding-test",
	}))

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "repair me", CreatedAt: now},
	}))

	vectorStore.deleteErr = errors.New("delete failed")
	require.NoError(t, store.RebuildVectorStore(context.Background(), testSessionA))
	require.Len(t, vectorStore.deletes, 1)
	require.Len(t, vectorStore.upserts, 2)
}

func TestSQLiteStore_RebuildVectorStoreBatchesMessages(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	vectorStore := &sqliteTestVectorStore{}
	provider := &sqliteTestEmbeddingProvider{dimensions: 3}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: provider,
		VectorStore:       vectorStore,
		EmbeddingModel:    "text-embedding-test",
		RebuildBatchSize:  2,
	}))

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "first", CreatedAt: now},
		{Role: handmsg.RoleUser, Content: "second", CreatedAt: now.Add(time.Second)},
		{Role: handmsg.RoleUser, Content: "third", CreatedAt: now.Add(2 * time.Second)},
	}))

	provider.requests = nil
	vectorStore.upserts = nil
	vectorStore.deletes = nil

	require.NoError(t, store.RebuildVectorStore(context.Background(), testSessionA))

	require.Len(t, provider.requests, 2)
	require.Len(t, provider.requests[0].Inputs, 2)
	require.Len(t, provider.requests[1].Inputs, 1)
	require.Len(t, vectorStore.upserts, 2)
	require.Len(t, vectorStore.upserts[0], 2)
	require.Len(t, vectorStore.upserts[1], 1)
	require.Len(t, vectorStore.deletes, 3)
}

func TestSQLiteStore_VectorStoreConfigurationValidation(t *testing.T) {
	var nilStore *SessionStore
	require.EqualError(t, nilStore.ConfigureVectorStore(VectorStoreOptions{}), "session store is required")

	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	require.EqualError(t, store.ConfigureVectorStore(VectorStoreOptions{
		VectorStore:    &sqliteTestVectorStore{},
		EmbeddingModel: "text-embedding-test",
	}), "vector store embedding provider is required")
	require.EqualError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		EmbeddingModel:    "text-embedding-test",
	}), "vector store is required")
	require.EqualError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       &sqliteTestVectorStore{},
	}), "vector store embedding model is required")
	require.EqualError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
		RebuildBatchSize:  -1,
	}), "vector store rebuild batch size must be greater than or equal to zero")
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{}))

	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
	}))
	require.EqualError(t, store.RebuildVectorStore(context.Background(), testMissingSession), "session not found")
}

func TestSQLiteStore_RebuildVectorStoreValidationAndErrorPaths(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	var nilStore *SessionStore
	require.EqualError(t, nilStore.RebuildVectorStore(context.Background(), testSessionA), "session store is required")

	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.EqualError(t, store.RebuildVectorStore(context.Background(), " "), "session id is required")
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.RebuildVectorStore(context.Background(), testSessionA))

	brokenStore, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, brokenStore.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, brokenStore.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
	}))
	require.NoError(t, brokenStore.db.Exec(`DROP TABLE session_messages`).Error)
	require.Error(t, brokenStore.RebuildVectorStore(context.Background(), testSessionA))

	brokenSessionStore, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, brokenSessionStore.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
	}))
	require.NoError(t, brokenSessionStore.db.Exec(`DROP TABLE sessions`).Error)
	require.Error(t, brokenSessionStore.RebuildVectorStore(context.Background(), testSessionA))

	deleteErr := errors.New("delete failed")
	requiredDeleteStore, vectorStore := sqliteVectorStoreTestStore(t)
	require.NoError(t, requiredDeleteStore.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       vectorStore,
		EmbeddingModel:    "text-embedding-test",
		Required:          true,
	}))
	require.NoError(t, requiredDeleteStore.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, requiredDeleteStore.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "delete required", CreatedAt: now},
	}))
	vectorStore.deleteErr = deleteErr
	require.ErrorIs(t, requiredDeleteStore.RebuildVectorStore(context.Background(), testSessionA), deleteErr)

	upsertErr := errors.New("upsert failed")
	upsertStore, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	upsertVectorStore := &sqliteTestVectorStore{}
	require.NoError(t, upsertStore.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       upsertVectorStore,
		EmbeddingModel:    "text-embedding-test",
		Required:          true,
	}))
	require.NoError(t, upsertStore.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, upsertStore.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "upsert required", CreatedAt: now},
	}))
	upsertVectorStore.upsertErr = upsertErr
	require.ErrorIs(t, upsertStore.RebuildVectorStore(context.Background(), testSessionA), upsertErr)
}

func TestSQLiteStore_VectorStoreMutationDatabaseErrors(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	t.Run("delete returns message id query error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.db.Exec(`DROP TABLE session_messages`).Error)

		err = store.Delete(context.Background(), testSessionA)
		require.Error(t, err)
	})

	t.Run("clear returns message delete error", func(t *testing.T) {
		store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
		require.NoError(t, err)
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "clear db error", CreatedAt: now},
		}))

		deleteErr := errors.New("delete message failed")
		err = store.db.Callback().Delete().
			Before("gorm:delete").
			Register("test:delete_message_error", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "session_messages" {
					tx.AddError(deleteErr)
				}
			})
		require.NoError(t, err)

		require.ErrorIs(t, store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}), deleteErr)
	})
}

func TestSQLiteStore_VectorStorePostMutationErrors(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	deleteErr := errors.New("delete failed")

	t.Run("delete returns required vector delete error", func(t *testing.T) {
		store, vectorStore := sqliteVectorStoreTestStore(t)
		require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
			EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
			VectorStore:       vectorStore,
			EmbeddingModel:    "text-embedding-test",
			Required:          true,
		}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "delete vector", CreatedAt: now},
		}))

		vectorStore.deleteErr = deleteErr
		require.ErrorIs(t, store.Delete(context.Background(), testSessionA), deleteErr)
	})

	t.Run("archive returns required vector delete error", func(t *testing.T) {
		store, vectorStore := sqliteVectorStoreTestStore(t)
		require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
			EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
			VectorStore:       vectorStore,
			EmbeddingModel:    "text-embedding-test",
			Required:          true,
		}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "archive vector", CreatedAt: now},
		}))

		vectorStore.deleteErr = deleteErr
		err := store.CreateArchive(context.Background(), ArchivedSession{
			ID:              testArchiveOne,
			SourceSessionID: testSessionA,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(time.Hour),
		})
		require.ErrorIs(t, err, deleteErr)
	})

	t.Run("clear returns required vector delete error", func(t *testing.T) {
		store, vectorStore := sqliteVectorStoreTestStore(t)
		require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
			EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
			VectorStore:       vectorStore,
			EmbeddingModel:    "text-embedding-test",
			Required:          true,
		}))
		require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
		require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
			{Role: handmsg.RoleUser, Content: "clear vector", CreatedAt: now},
		}))

		vectorStore.deleteErr = deleteErr
		require.ErrorIs(t, store.ClearMessages(context.Background(), testSessionA, MessageQueryOptions{}), deleteErr)
	})
}

func TestSQLiteStore_VectorStoreHelperBranches(t *testing.T) {
	require.Nil(t, vectorInputsFromSearchRows(nil))
	require.Nil(t, sourceIDsFromModels(nil))
	require.Nil(t, uniqueStrings(nil))
	require.Equal(t, []string{"one", "two"}, uniqueStrings([]string{" one ", "", "two", "one"}))
}

func TestSQLiteStore_VectorStoreSkipsEmptySearchRows(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	provider := &sqliteTestEmbeddingProvider{dimensions: 3}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: provider,
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
	}))

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, CreatedAt: now},
	}))
	require.Empty(t, provider.requests)
}

func TestSQLiteStore_VectorStoreRejectsInvalidEmbeddings(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	provider := &sqliteTestEmbeddingProvider{
		mutate: func(result retrieval.EmbeddingResult) retrieval.EmbeddingResult {
			result.Items[0].ContentHash = "wrong"
			return result
		},
		dimensions: 3,
	}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: provider,
		VectorStore:       &sqliteTestVectorStore{},
		EmbeddingModel:    "text-embedding-test",
		Required:          true,
	}))

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: testSessionA, UpdatedAt: now}))
	err = store.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "bad embedding", CreatedAt: now},
	})
	require.EqualError(t, err, "embedding content hash must match input text")
}

func sqliteVectorStoreTestStore(t *testing.T) (*SessionStore, *sqliteTestVectorStore) {
	t.Helper()

	store, err := NewSessionStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	vectorStore := &sqliteTestVectorStore{}
	require.NoError(t, store.ConfigureVectorStore(VectorStoreOptions{
		EmbeddingProvider: &sqliteTestEmbeddingProvider{dimensions: 3},
		VectorStore:       vectorStore,
		EmbeddingModel:    "text-embedding-test",
	}))

	return store, vectorStore
}

func sqliteTestRecordIDs(records []retrieval.VectorRecord) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}

	return ids
}

type sqliteTestEmbeddingProvider struct {
	err        error
	mutate     func(retrieval.EmbeddingResult) retrieval.EmbeddingResult
	requests   []retrieval.EmbeddingRequest
	dimensions int
}

func (p *sqliteTestEmbeddingProvider) Embed(_ context.Context, req retrieval.EmbeddingRequest) (retrieval.EmbeddingResult, error) {
	p.requests = append(p.requests, req)
	if p.err != nil {
		return retrieval.EmbeddingResult{}, p.err
	}
	if p.dimensions == 0 {
		p.dimensions = 3
	}

	items := make([]retrieval.Embedding, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		vector := make([]float64, p.dimensions)
		for idx := range vector {
			vector[idx] = float64(len(input.Text) + idx)
		}
		items = append(items, retrieval.Embedding{
			ID:          input.ID,
			ContentHash: retrieval.VectorContentHash(input.Text),
			Vector:      vector,
		})
	}

	result := retrieval.EmbeddingResult{
		Model:      req.Model,
		Dimensions: p.dimensions,
		Items:      items,
	}
	if p.mutate != nil {
		result = p.mutate(result)
	}

	return result, nil
}

type sqliteTestVectorStore struct {
	err       error
	deleteErr error
	upsertErr error
	upserts   [][]retrieval.VectorRecord
	deletes   []retrieval.VectorDeleteRequest
}

func (s *sqliteTestVectorStore) Upsert(_ context.Context, records []retrieval.VectorRecord) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	if s.err != nil {
		return s.err
	}
	s.upserts = append(s.upserts, append([]retrieval.VectorRecord(nil), records...))

	return nil
}

func (s *sqliteTestVectorStore) Delete(_ context.Context, req retrieval.VectorDeleteRequest) error {
	s.deletes = append(s.deletes, req)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if s.err != nil {
		return s.err
	}

	return nil
}

func (s *sqliteTestVectorStore) Search(context.Context, retrieval.VectorSearchRequest) (retrieval.VectorSearchResult, error) {
	return retrieval.VectorSearchResult{}, nil
}

func (s *sqliteTestVectorStore) Metadata(context.Context) (retrieval.VectorStoreMetadata, error) {
	return retrieval.VectorStoreMetadata{}, nil
}
