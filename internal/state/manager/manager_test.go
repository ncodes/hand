package manager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state"
	storagemock "github.com/wandxy/hand/internal/state/mock"
	storagememory "github.com/wandxy/hand/internal/state/storememory"
	"github.com/wandxy/hand/pkg/nanoid"
)

var (
	testSessionA       = nanoid.MustFromSeed(storage.SessionIDPrefix, "project-a", "SessionTestSeedValue123")
	testMissingSession = nanoid.MustFromSeed(storage.SessionIDPrefix, "missing", "SessionTestSeedValue123")
	testArchiveOne     = nanoid.MustFromSeed(storage.ArchiveIDPrefix, "archive-1", "SessionTestSeedValue123")
	testArchiveTwo     = nanoid.MustFromSeed(storage.ArchiveIDPrefix, "archive-2", "SessionTestSeedValue123")
)

func TestManager_ResolveChatSessionCreatesDefault(t *testing.T) {
	manager, err := NewManager(storagememory.NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	session, err := manager.Resolve(context.Background(), "")

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, session.ID)
}

func TestManager_RunMaintenanceArchivesExpiredDefault(t *testing.T) {
	store := storagememory.NewStore()
	expiredAt := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), storage.Session{
		ID:        storage.DefaultSessionID,
		CreatedAt: expiredAt.Add(-time.Hour),
		UpdatedAt: expiredAt,
	}))
	require.NoError(t, store.AppendMessages(context.Background(), storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: expiredAt.Add(-time.Minute)},
	}))
	defaultSession, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)

	manager, err := NewManager(store, time.Hour, 48*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return defaultSession.UpdatedAt.Add(2 * time.Hour) }

	err = manager.runMaintenance(context.Background())
	require.NoError(t, err)

	archives, err := store.ListArchives(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.Len(t, archives, 1)
	messages, err := store.GetMessages(context.Background(), archives[0].ID, storage.MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "hello", messages[0].Content)

	liveMessages, err := store.GetMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.Empty(t, liveMessages)
}

func TestManager_RunMaintenanceClearsDefaultSessionCompactionMetadata(t *testing.T) {
	store := storagememory.NewStore()
	expiredAt := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), storage.Session{
		ID: storage.DefaultSessionID,
		Compaction: storage.SessionCompaction{
			Status:             storage.CompactionStatusRunning,
			RequestedAt:        expiredAt.Add(-30 * time.Minute),
			StartedAt:          expiredAt.Add(-29 * time.Minute),
			TargetMessageCount: 9,
			TargetOffset:       1,
		},
		CreatedAt: expiredAt.Add(-time.Hour),
		UpdatedAt: expiredAt,
	}))
	require.NoError(t, store.AppendMessages(context.Background(), storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: expiredAt.Add(-time.Minute)},
	}))
	defaultSession, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)

	manager, err := NewManager(store, time.Hour, 48*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return defaultSession.UpdatedAt.Add(2 * time.Hour) }

	require.NoError(t, manager.runMaintenance(context.Background()))

	session, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.SessionCompaction{}, session.Compaction)
}

func TestManager_CurrentSessionDefaultsToDefault(t *testing.T) {
	manager, err := NewManager(storagememory.NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	current, err := manager.CurrentSession(context.Background())

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, current)
}

func TestManager_StartResolvesDefaultSession(t *testing.T) {
	store := storagememory.NewStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	err = manager.Start(context.Background())
	require.NoError(t, err)

	session, ok, err := store.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.DefaultSessionID, session.ID)
}

func TestManager_UseSessionUsesResolvedDefaultSession(t *testing.T) {
	store := storagememory.NewStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	require.NoError(t, manager.Start(context.Background()))
	err = manager.UseSession(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.DefaultSessionID, current)
}

func TestNewManager_ValidationAndNilManagerErrors(t *testing.T) {
	_, err := NewManager(nil, time.Hour, 24*time.Hour)
	require.EqualError(t, err, "store is required")

	_, err = NewManager(storagememory.NewStore(), 0, 24*time.Hour)
	require.EqualError(t, err, "session default idle expiry must be greater than zero")

	_, err = NewManager(storagememory.NewStore(), time.Hour, 0)
	require.EqualError(t, err, "session archive retention must be greater than zero")

	var manager *Manager

	_, err = manager.Resolve(context.Background(), "")
	require.EqualError(t, err, "state manager is required")
	require.EqualError(t, manager.runMaintenance(context.Background()), "state manager is required")
	require.EqualError(t, manager.Start(context.Background()), "state manager is required")

	require.EqualError(t, manager.AppendMessages(context.Background(), storage.DefaultSessionID, nil), "state manager is required")
	_, err = manager.CountMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.EqualError(t, err, "state manager is required")
	_, err = manager.SearchMessages(context.Background(), storage.DefaultSessionID, storage.SearchMessageOptions{})
	require.EqualError(t, err, "state manager is required")
	require.EqualError(t, manager.Save(context.Background(), storage.Session{}), "state manager is required")
	_, _, err = manager.Get(context.Background(), storage.DefaultSessionID)
	require.EqualError(t, err, "state manager is required")
	_, _, err = manager.GetMessage(context.Background(), storage.DefaultSessionID, 0, storage.MessageQueryOptions{})
	require.EqualError(t, err, "state manager is required")
	_, err = manager.GetMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.EqualError(t, err, "state manager is required")
	_, err = manager.GetMessagesByIDs(context.Background(), storage.DefaultSessionID, []uint{1})
	require.EqualError(t, err, "state manager is required")
	_, err = manager.GetMessageWindow(context.Background(), storage.DefaultSessionID, 1, 1, 1)
	require.EqualError(t, err, "state manager is required")
	require.EqualError(t, manager.UpdateLastPromptTokens(context.Background(), storage.DefaultSessionID, 1), "state manager is required")

	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.EqualError(t, err, "state manager is required")

	_, err = manager.ListSessions(context.Background())
	require.EqualError(t, err, "state manager is required")

	require.EqualError(t, manager.UseSession(context.Background(), storage.DefaultSessionID), "state manager is required")
	require.EqualError(t, manager.DeleteSession(context.Background(), storage.DefaultSessionID), "state manager is required")

	current, err := manager.CurrentSession(context.Background())
	require.EqualError(t, err, "state manager is required")
	require.Empty(t, current)
}

func TestManager_CountMessages_ForwardsToStore(t *testing.T) {
	var capturedID string
	var capturedOpts storage.MessageQueryOptions
	manager, err := NewManager(&storagemock.Store{
		CountMessagesFunc: func(_ context.Context, id string, opts storage.MessageQueryOptions) (int, error) {
			capturedID = id
			capturedOpts = opts
			return 7, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	count, err := manager.CountMessages(context.Background(), "  "+testSessionA+"  ", storage.MessageQueryOptions{
		Archived: true,
		Limit:    2,
		Offset:   3,
	})
	require.NoError(t, err)
	require.Equal(t, 7, count)
	require.Equal(t, testSessionA, capturedID)
	require.Equal(t, storage.MessageQueryOptions{Archived: true, Limit: 2, Offset: 3}, capturedOpts)
}

func TestManager_SearchMessages_ForwardsToStore(t *testing.T) {
	var capturedID string
	var capturedOpts storage.SearchMessageOptions
	manager, err := NewManager(&storagemock.Store{
		SearchMessagesFunc: func(_ context.Context, id string, opts storage.SearchMessageOptions) ([]storage.SearchMessageResult, error) {
			capturedID = id
			capturedOpts = opts
			return []storage.SearchMessageResult{{SessionID: testSessionA, MatchCount: 1}}, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	results, err := manager.SearchMessages(context.Background(), "  "+testSessionA+"  ", storage.SearchMessageOptions{
		IgnoreSessionID:       "  " + storage.DefaultSessionID + "  ",
		Query:                 "hello",
		ToolName:              "process",
		MaxSessions:           2,
		MaxMessagesPerSession: 3,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, testSessionA, capturedID)
	require.Equal(t, testSessionA, results[0].SessionID)
	require.Equal(t, storage.SearchMessageOptions{
		IgnoreSessionID:       storage.DefaultSessionID,
		Query:                 "hello",
		ToolName:              "process",
		MaxSessions:           2,
		MaxMessagesPerSession: 3,
	}, capturedOpts)
}

func TestManager_GetMessagesByIDs_ForwardsToStore(t *testing.T) {
	var capturedID string
	var capturedMessageIDs []uint
	manager, err := NewManager(&storagemock.Store{
		GetMessagesByIDsFunc: func(_ context.Context, id string, messageIDs []uint) ([]storage.MessageRecord, error) {
			capturedID = id
			capturedMessageIDs = append([]uint(nil), messageIDs...)
			return []storage.MessageRecord{{Offset: 3, Message: handmsg.Message{ID: 7}}}, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	records, err := manager.GetMessagesByIDs(context.Background(), "  "+testSessionA+"  ", []uint{7, 9})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, testSessionA, capturedID)
	require.Equal(t, []uint{7, 9}, capturedMessageIDs)
}

func TestManager_GetMessageWindow_ForwardsToStore(t *testing.T) {
	var capturedID string
	var capturedAnchor uint
	var capturedBefore int
	var capturedAfter int
	manager, err := NewManager(&storagemock.Store{
		GetMessageWindowFunc: func(_ context.Context, id string, anchorMessageID uint, before int, after int) ([]storage.MessageRecord, error) {
			capturedID = id
			capturedAnchor = anchorMessageID
			capturedBefore = before
			capturedAfter = after
			return []storage.MessageRecord{{Offset: 2, Message: handmsg.Message{ID: anchorMessageID}}}, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	records, err := manager.GetMessageWindow(context.Background(), "  "+testSessionA+"  ", 7, 1, 2)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, testSessionA, capturedID)
	require.Equal(t, uint(7), capturedAnchor)
	require.Equal(t, 1, capturedBefore)
	require.Equal(t, 2, capturedAfter)
}

func TestManager_UpdateLastPromptTokens(t *testing.T) {
	var saved storage.Session

	manager, err := NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: testSessionA, UpdatedAt: time.Now().UTC()}, true, nil
		},
		SaveFunc: func(_ context.Context, session storage.Session) error {
			saved = session
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	require.NoError(t, manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42))
	require.Equal(t, 42, saved.LastPromptTokens)

	require.NoError(t, manager.UpdateLastPromptTokens(context.Background(), testSessionA, 0))
	require.NoError(t, manager.UpdateLastPromptTokens(context.Background(), testSessionA, -1))
}

func TestManager_UpdateLastPromptTokensReturnsSaveError(t *testing.T) {
	manager, err := NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: testSessionA, UpdatedAt: time.Now().UTC()}, true, nil
		},
		SaveFunc: func(context.Context, storage.Session) error {
			return errors.New("save failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
	require.EqualError(t, err, "save failed")
}

func TestManager_SummaryLifecycleMethods(t *testing.T) {
	t.Run("nil manager", func(t *testing.T) {
		var manager *Manager

		require.EqualError(t, manager.SaveSummary(context.Background(), storage.SessionSummary{}), "state manager is required")

		summary, ok, err := manager.GetSummary(context.Background(), testSessionA)
		require.EqualError(t, err, "state manager is required")
		require.False(t, ok)
		require.Equal(t, storage.SessionSummary{}, summary)

		require.EqualError(t, manager.DeleteSummary(context.Background(), testSessionA), "state manager is required")
	})

	t.Run("save summary forwards to store", func(t *testing.T) {
		expected := storage.SessionSummary{
			SessionID:      testSessionA,
			SessionSummary: "summary",
		}

		var captured storage.SessionSummary
		manager, err := NewManager(&storagemock.Store{
			SaveSummaryFunc: func(_ context.Context, summary storage.SessionSummary) error {
				captured = summary
				return nil
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.NoError(t, manager.SaveSummary(context.Background(), expected))
		require.Equal(t, expected, captured)
	})

	t.Run("save summary returns store error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.Store{
			SaveSummaryFunc: func(context.Context, storage.SessionSummary) error {
				return errors.New("save summary failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.SaveSummary(context.Background(), storage.SessionSummary{})
		require.EqualError(t, err, "save summary failed")
	})

	t.Run("get summary trims session id and returns result", func(t *testing.T) {
		expected := storage.SessionSummary{
			SessionID:      testSessionA,
			SessionSummary: "summary",
		}

		var capturedID string
		manager, err := NewManager(&storagemock.Store{
			GetSummaryFunc: func(_ context.Context, sessionID string) (storage.SessionSummary, bool, error) {
				capturedID = sessionID
				return expected, true, nil
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		summary, ok, err := manager.GetSummary(context.Background(), "  "+testSessionA+"  ")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, expected, summary)
		require.Equal(t, testSessionA, capturedID)
	})

	t.Run("get summary returns store error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.Store{
			GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
				return storage.SessionSummary{}, false, errors.New("get summary failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		summary, ok, err := manager.GetSummary(context.Background(), testSessionA)
		require.EqualError(t, err, "get summary failed")
		require.False(t, ok)
		require.Equal(t, storage.SessionSummary{}, summary)
	})

	t.Run("delete summary trims session id", func(t *testing.T) {
		var capturedID string
		manager, err := NewManager(&storagemock.Store{
			DeleteSummaryFunc: func(_ context.Context, sessionID string) error {
				capturedID = sessionID
				return nil
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.NoError(t, manager.DeleteSummary(context.Background(), "  "+testSessionA+"  "))
		require.Equal(t, testSessionA, capturedID)
	})

	t.Run("delete summary returns store error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.Store{
			DeleteSummaryFunc: func(context.Context, string) error {
				return errors.New("delete summary failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.DeleteSummary(context.Background(), testSessionA)
		require.EqualError(t, err, "delete summary failed")
	})
}

func TestManager_UpdateLastPromptTokens_Errors(t *testing.T) {
	t.Run("empty id", func(t *testing.T) {
		manager, err := NewManager(storagememory.NewStore(), time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), "", 42)
		require.EqualError(t, err, "session id is required")
	})

	t.Run("get error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.Store{
			GetFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
		require.EqualError(t, err, "get failed")
	})

	t.Run("session not found", func(t *testing.T) {
		manager, err := NewManager(&storagemock.Store{}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
		require.EqualError(t, err, "session not found")
	})
}
