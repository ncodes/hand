package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/pkg/nanoid"
)

var (
	testSessionID        = nanoid.MustFromSeed(storage.SessionIDPrefix, "project-a", "SessionUtilTestSeedValue123")
	testArchiveSessionID = nanoid.MustFromSeed(storage.SessionIDPrefix, "archive-source", "SessionUtilTestSeedValue123")
)

func TestValidateSessionID(t *testing.T) {
	t.Run("rejects empty id", func(t *testing.T) {
		err := ValidateSessionID("   ")
		require.EqualError(t, err, "session id is required")
	})

	t.Run("accepts default id", func(t *testing.T) {
		require.NoError(t, ValidateSessionID(storage.DefaultSessionID))
	})

	t.Run("rejects invalid id", func(t *testing.T) {
		err := ValidateSessionID("ses_invalid")
		require.EqualError(t, err, "session id must be a valid ses_ nanoid")
	})

	t.Run("accepts valid generated id", func(t *testing.T) {
		require.NoError(t, ValidateSessionID(testSessionID))
	})
}

func TestNormalizeCreateArchive(t *testing.T) {
	t.Run("rejects missing archive id", func(t *testing.T) {
		archive, err := NormalizeCreateArchive(storage.ArchivedSession{})
		require.EqualError(t, err, "archive id is required")
		require.Equal(t, storage.ArchivedSession{}, archive)
	})

	t.Run("rejects missing source session id", func(t *testing.T) {
		archive, err := NormalizeCreateArchive(storage.ArchivedSession{
			ID:        "archive-a",
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		})
		require.EqualError(t, err, "source session id is required")
		require.Equal(t, storage.ArchivedSession{}, archive)
	})

	t.Run("rejects invalid source session id", func(t *testing.T) {
		archive, err := NormalizeCreateArchive(storage.ArchivedSession{
			ID:              "archive-a",
			SourceSessionID: "ses_invalid",
			ExpiresAt:       time.Now().UTC().Add(time.Hour),
		})
		require.EqualError(t, err, "session id must be a valid ses_ nanoid")
		require.Equal(t, storage.ArchivedSession{}, archive)
	})

	t.Run("rejects missing expiry", func(t *testing.T) {
		archive, err := NormalizeCreateArchive(storage.ArchivedSession{
			ID:              "archive-a",
			SourceSessionID: testArchiveSessionID,
		})
		require.EqualError(t, err, "archive expiry is required")
		require.Equal(t, storage.ArchivedSession{}, archive)
	})

	t.Run("defaults archived at and trims source session id", func(t *testing.T) {
		expiresAt := time.Now().Add(2 * time.Hour)

		archive, err := NormalizeCreateArchive(storage.ArchivedSession{
			ID:              "archive-a",
			SourceSessionID: "  " + storage.DefaultSessionID + "  ",
			ExpiresAt:       expiresAt,
		})
		require.NoError(t, err)
		require.Equal(t, "archive-a", archive.ID)
		require.Equal(t, storage.DefaultSessionID, archive.SourceSessionID)
		require.False(t, archive.ArchivedAt.IsZero())
		require.Equal(t, time.UTC, archive.ArchivedAt.Location())
		require.Equal(t, expiresAt.UTC(), archive.ExpiresAt)
	})

	t.Run("normalizes explicit timestamps to utc", func(t *testing.T) {
		location := time.FixedZone("UTC+2", 2*60*60)
		archivedAt := time.Date(2026, 4, 2, 14, 0, 0, 0, location)
		expiresAt := time.Date(2026, 4, 3, 14, 0, 0, 0, location)

		archive, err := NormalizeCreateArchive(storage.ArchivedSession{
			ID:              "archive-b",
			SourceSessionID: "  " + testArchiveSessionID + "  ",
			ArchivedAt:      archivedAt,
			ExpiresAt:       expiresAt,
		})
		require.NoError(t, err)
		require.Equal(t, "archive-b", archive.ID)
		require.Equal(t, testArchiveSessionID, archive.SourceSessionID)
		require.Equal(t, archivedAt.UTC(), archive.ArchivedAt)
		require.Equal(t, expiresAt.UTC(), archive.ExpiresAt)
	})
}

func TestCloneMessages(t *testing.T) {
	require.Nil(t, CloneMessages(nil))

	createdAt := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	original := []handmsg.Message{{
		Role:      handmsg.RoleAssistant,
		Content:   "reply",
		CreatedAt: createdAt,
		ToolCalls: []handmsg.ToolCall{{
			ID:    "call-1",
			Name:  "lookup",
			Input: "{\"q\":\"hello\"}",
		}},
	}}

	cloned := CloneMessages(original)
	require.Equal(t, original, cloned)

	original[0].Content = "changed"
	original[0].ToolCalls[0].Name = "mutated"

	require.Equal(t, "reply", cloned[0].Content)
	require.Equal(t, "lookup", cloned[0].ToolCalls[0].Name)
}
