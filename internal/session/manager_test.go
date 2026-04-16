package session

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
	common "github.com/wandxy/hand/internal/storage/common"
	storagememory "github.com/wandxy/hand/internal/storage/memory"
	storagemock "github.com/wandxy/hand/internal/storage/mock"
	"github.com/wandxy/hand/pkg/nanoid"
)

var (
	testSessionA       = nanoid.MustFromSeed(storage.SessionIDPrefix, "project-a", "SessionTestSeedValue123")
	testMissingSession = nanoid.MustFromSeed(storage.SessionIDPrefix, "missing", "SessionTestSeedValue123")
	testArchiveOne     = nanoid.MustFromSeed(storage.ArchiveIDPrefix, "archive-1", "SessionTestSeedValue123")
	testArchiveTwo     = nanoid.MustFromSeed(storage.ArchiveIDPrefix, "archive-2", "SessionTestSeedValue123")
)

func TestManager_ResolveChatSessionCreatesDefault(t *testing.T) {
	manager, err := NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	session, err := manager.Resolve(context.Background(), "")

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, session.ID)
}

func TestManager_RunMaintenanceArchivesExpiredDefault(t *testing.T) {
	store := storagememory.NewSessionStore()
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
	store := storagememory.NewSessionStore()
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
	manager, err := NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	current, err := manager.CurrentSession(context.Background())

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, current)
}

func TestManager_StartResolvesDefaultSession(t *testing.T) {
	store := storagememory.NewSessionStore()
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
	store := storagememory.NewSessionStore()
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
	require.EqualError(t, err, "session store is required")

	_, err = NewManager(storagememory.NewSessionStore(), 0, 24*time.Hour)
	require.EqualError(t, err, "session default idle expiry must be greater than zero")

	_, err = NewManager(storagememory.NewSessionStore(), time.Hour, 0)
	require.EqualError(t, err, "session archive retention must be greater than zero")

	var manager *Manager

	_, err = manager.Resolve(context.Background(), "")
	require.EqualError(t, err, "session manager is required")
	require.EqualError(t, manager.runMaintenance(context.Background()), "session manager is required")
	require.EqualError(t, manager.Start(context.Background()), "session manager is required")

	require.EqualError(t, manager.AppendMessages(context.Background(), storage.DefaultSessionID, nil), "session manager is required")
	_, err = manager.CountMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.EqualError(t, err, "session manager is required")
	require.EqualError(t, manager.Save(context.Background(), storage.Session{}), "session manager is required")
	_, _, err = manager.Get(context.Background(), storage.DefaultSessionID)
	require.EqualError(t, err, "session manager is required")
	_, _, err = manager.GetMessage(context.Background(), storage.DefaultSessionID, 0, storage.MessageQueryOptions{})
	require.EqualError(t, err, "session manager is required")
	_, err = manager.GetMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.EqualError(t, err, "session manager is required")
	require.EqualError(t, manager.UpdateLastPromptTokens(context.Background(), storage.DefaultSessionID, 1), "session manager is required")

	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.EqualError(t, err, "session manager is required")

	_, err = manager.ListSessions(context.Background())
	require.EqualError(t, err, "session manager is required")

	require.EqualError(t, manager.UseSession(context.Background(), storage.DefaultSessionID), "session manager is required")
	require.EqualError(t, manager.DeleteSession(context.Background(), storage.DefaultSessionID), "session manager is required")

	current, err := manager.CurrentSession(context.Background())
	require.EqualError(t, err, "session manager is required")
	require.Empty(t, current)
}

func TestManager_CountMessages_ForwardsToStore(t *testing.T) {
	var capturedID string
	var capturedOpts storage.MessageQueryOptions
	manager, err := NewManager(&storagemock.SessionStore{
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
	manager, err := NewManager(&storagemock.SessionStore{
		SearchMessagesFunc: func(_ context.Context, id string, opts storage.SearchMessageOptions) ([]handmsg.Message, error) {
			capturedID = id
			capturedOpts = opts
			return []handmsg.Message{{Content: "hello"}}, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	messages, err := manager.SearchMessages(context.Background(), "  "+testSessionA+"  ", storage.SearchMessageOptions{
		Query:    "hello",
		ToolName: "process",
		Limit:    2,
		Offset:   3,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, testSessionA, capturedID)
	require.Equal(t, storage.SearchMessageOptions{Query: "hello", ToolName: "process", Limit: 2, Offset: 3}, capturedOpts)
}

func TestManager_Save_ForwardsToStore(t *testing.T) {
	expected := storage.Session{
		ID: testSessionA,
		Compaction: storage.SessionCompaction{
			Status:       storage.CompactionStatusSucceeded,
			TargetOffset: 3,
		},
		LastPromptTokens: 42,
	}

	var captured storage.Session
	manager, err := NewManager(&storagemock.SessionStore{
		SaveFunc: func(_ context.Context, session storage.Session) error {
			captured = session
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	require.NoError(t, manager.Save(context.Background(), expected))
	require.Equal(t, expected, captured)
}

func TestManager_Save_ReturnsStoreError(t *testing.T) {
	manager, err := NewManager(&storagemock.SessionStore{
		SaveFunc: func(context.Context, storage.Session) error {
			return errors.New("save failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	err = manager.Save(context.Background(), storage.Session{ID: testSessionA})
	require.EqualError(t, err, "save failed")
}

func TestManager_Get_TrimsIDAndReturnsStoreResult(t *testing.T) {
	expected := storage.Session{
		ID:               testSessionA,
		LastPromptTokens: 9,
	}

	var capturedID string
	manager, err := NewManager(&storagemock.SessionStore{
		GetFunc: func(_ context.Context, id string) (storage.Session, bool, error) {
			capturedID = id
			return expected, true, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	session, ok, err := manager.Get(context.Background(), "  "+testSessionA+"  ")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, expected, session)
	require.Equal(t, testSessionA, capturedID)
}

func TestManager_Get_ReturnsStoreError(t *testing.T) {
	manager, err := NewManager(&storagemock.SessionStore{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{}, false, errors.New("get failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, _, err = manager.Get(context.Background(), testSessionA)
	require.EqualError(t, err, "get failed")
}

func TestManager_CreateSaveListAndResolveNonDefaultSession(t *testing.T) {
	store := storagememory.NewSessionStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	created, err := manager.CreateSession(context.Background(), testSessionA)
	require.NoError(t, err)
	require.Equal(t, testSessionA, created.ID)
	require.False(t, created.CreatedAt.IsZero())

	require.EqualError(t, manager.AppendMessages(context.Background(), "", nil), "session id is required")
	require.NoError(t, manager.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)},
	}))

	resolved, err := manager.Resolve(context.Background(), testSessionA)
	require.NoError(t, err)
	require.Equal(t, testSessionA, resolved.ID)
	messages, err := manager.GetMessages(context.Background(), testSessionA, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "hello", messages[0].Content)
	message, ok, err := manager.GetMessage(context.Background(), testSessionA, 0, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", message.Content)

	sessions, err := manager.ListSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, storage.DefaultSessionID, sessions[0].ID)
	require.Equal(t, testSessionA, sessions[1].ID)
}

func TestManager_CreateUseAndResolveErrors(t *testing.T) {
	store := storagememory.NewSessionStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	created, err := manager.CreateSession(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, common.ValidateSessionID(created.ID))

	_, err = manager.CreateSession(context.Background(), "project-a")
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")

	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.NoError(t, err)

	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.EqualError(t, err, "session already exists")

	_, err = manager.Resolve(context.Background(), testMissingSession)
	require.EqualError(t, err, "session not found")

	_, err = manager.Resolve(context.Background(), "project-a")
	require.EqualError(t, err, "session id must be a valid ses_ nanoid")

	require.EqualError(t, manager.UseSession(context.Background(), "project-a"), "session id must be a valid ses_ nanoid")
	require.EqualError(t, manager.UseSession(context.Background(), testMissingSession), "session not found")
}

func TestManager_DeleteSessionKeepsArchives(t *testing.T) {
	store := storagememory.NewSessionStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), storage.ArchivedSession{
		ID:              testArchiveOne,
		SourceSessionID: testSessionA,
		ArchivedAt:      time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC),
		ExpiresAt:       time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
	}))
	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), testSessionA, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello-again", CreatedAt: time.Date(2026, 3, 30, 13, 30, 0, 0, time.UTC)},
	}))

	require.NoError(t, store.CreateArchive(context.Background(), storage.ArchivedSession{
		ID:              testArchiveTwo,
		SourceSessionID: testSessionA,
		ArchivedAt:      time.Date(2026, 3, 30, 14, 0, 0, 0, time.UTC),
		ExpiresAt:       time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC),
	}))
	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.NoError(t, err)
	require.NoError(t, manager.UseSession(context.Background(), testSessionA))
	require.NoError(t, manager.DeleteSession(context.Background(), testSessionA))

	_, ok, err := store.Get(context.Background(), testSessionA)
	require.NoError(t, err)
	require.False(t, ok)
	archives, err := store.ListArchives(context.Background(), testSessionA)
	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, testArchiveTwo, archives[0].ID)
	require.Equal(t, testArchiveOne, archives[1].ID)
	current, err := manager.CurrentSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, current)
}

func TestManager_CurrentSessionUsesStoredSelection(t *testing.T) {
	store := storagememory.NewSessionStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = manager.CreateSession(context.Background(), testSessionA)
	require.NoError(t, err)
	require.NoError(t, manager.UseSession(context.Background(), testSessionA))

	current, err := manager.CurrentSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, testSessionA, current)
}

func TestManager_ResolveDefaultSessionKeepsActiveMessagesBeforeExpiry(t *testing.T) {
	store := storagememory.NewSessionStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "still-active", CreatedAt: now.Add(-5 * time.Minute)},
	}))
	require.NoError(t, store.Save(context.Background(), storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now}))

	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return now.Add(30 * time.Minute) }

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, session.ID)
	messages, err := manager.GetMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "still-active", messages[0].Content)

	archives, err := store.ListArchives(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.Empty(t, archives)
}

func TestManager_ResolveChatSessionDoesNotRunMaintenance(t *testing.T) {
	store := storagememory.NewSessionStore()
	expiredAt := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), storage.Session{ID: storage.DefaultSessionID, UpdatedAt: expiredAt}))
	require.NoError(t, store.AppendMessages(context.Background(), storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "hello", CreatedAt: expiredAt.Add(-time.Minute)},
	}))
	require.NoError(t, store.Save(context.Background(), storage.Session{ID: storage.DefaultSessionID, UpdatedAt: expiredAt}))

	manager, err := NewManager(store, time.Hour, 48*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return expiredAt.Add(2 * time.Hour) }

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, session.ID)

	messages, err := store.GetMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)

	archives, err := store.ListArchives(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.Empty(t, archives)
}

func TestManager_StartRunsMaintenance(t *testing.T) {
	store := storagememory.NewSessionStore()
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

	ctx := t.Context()

	require.NoError(t, manager.startMaintenanceWorker(ctx, 5*time.Millisecond))

	messages, err := store.GetMessages(context.Background(), storage.DefaultSessionID, storage.MessageQueryOptions{})
	require.NoError(t, err)
	require.Empty(t, messages)

	archives, err := store.ListArchives(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.Len(t, archives, 1)
}

func TestManager_ErrorBranchesAndWorkerTick(t *testing.T) {
	t.Run("resolve non-default get error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			GetFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.Resolve(context.Background(), testSessionA)
		require.EqualError(t, err, "get failed")
	})

	t.Run("run maintenance delete error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			DeleteExpiredArchivesFunc: func(context.Context, time.Time) error {
				return errors.New("delete failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.runMaintenance(context.Background())
		require.EqualError(t, err, "delete failed")
	})

	t.Run("start worker defaults interval", func(t *testing.T) {
		var deleteCalls atomic.Int32
		manager, err := NewManager(&storagemock.SessionStore{
			DeleteExpiredArchivesFunc: func(context.Context, time.Time) error {
				deleteCalls.Add(1)
				return nil
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.NoError(t, manager.startMaintenanceWorker(context.TODO(), 0))
		require.EqualValues(t, 1, deleteCalls.Load())
	})

	t.Run("start worker runs ticker maintenance", func(t *testing.T) {
		var deleteCalls atomic.Int32
		manager, err := NewManager(&storagemock.SessionStore{
			DeleteExpiredArchivesFunc: func(context.Context, time.Time) error {
				deleteCalls.Add(1)
				return nil
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		require.NoError(t, manager.startMaintenanceWorker(ctx, 5*time.Millisecond))
		require.Eventually(t, func() bool {
			return deleteCalls.Load() >= 2
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("start worker returns maintenance error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			DeleteExpiredArchivesFunc: func(context.Context, time.Time) error {
				return errors.New("delete failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.startMaintenanceWorker(context.TODO(), time.Second)
		require.EqualError(t, err, "delete failed")
	})

	t.Run("start worker returns default resolution error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			GetFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.startMaintenanceWorker(context.TODO(), time.Second)
		require.EqualError(t, err, "get failed")
	})

	t.Run("start worker uses background when context is canceled", func(t *testing.T) {
		var captured context.Context
		manager, err := NewManager(&storagemock.SessionStore{
			GetFunc: func(ctx context.Context, _ string) (storage.Session, bool, error) {
				captured = ctx
				return storage.Session{}, false, nil
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		require.NoError(t, manager.startMaintenanceWorker(ctx, time.Second))
		require.NotNil(t, captured)
		require.NoError(t, captured.Err())
	})

	t.Run("create session get error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			GetFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CreateSession(context.Background(), testSessionA)
		require.EqualError(t, err, "get failed")
	})

	t.Run("create session generated id error", func(t *testing.T) {
		originalGenerateSessionID := generateSessionID
		t.Cleanup(func() {
			generateSessionID = originalGenerateSessionID
		})
		generateSessionID = func() (string, error) {
			return "", errors.New("generate failed")
		}

		manager, err := NewManager(&storagemock.SessionStore{}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CreateSession(context.Background(), "")
		require.EqualError(t, err, "generate failed")
	})

	t.Run("create session save error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			SaveFunc: func(context.Context, storage.Session) error {
				return errors.New("save failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CreateSession(context.Background(), testSessionA)
		require.EqualError(t, err, "save failed")
	})

	t.Run("list sessions resolve default error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			GetFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.ListSessions(context.Background())
		require.EqualError(t, err, "get failed")
	})

	t.Run("use default resolve error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			GetFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UseSession(context.Background(), storage.DefaultSessionID)
		require.EqualError(t, err, "get failed")
	})

	t.Run("delete session validation and errors", func(t *testing.T) {
		manager, err := NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSession(context.Background(), ""), "session id is required")
		require.EqualError(t, manager.DeleteSession(context.Background(), storage.DefaultSessionID), "default session cannot be deleted")
		require.EqualError(t, manager.DeleteSession(context.Background(), testSessionA), "session not found")

		manager, err = NewManager(&storagemock.SessionStore{
			DeleteFunc: func(context.Context, string) error {
				return errors.New("delete failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSession(context.Background(), testSessionA), "delete failed")

		require.EqualError(t, manager.DeleteSession(context.Background(), "project-a"), "session id must be a valid ses_ nanoid")
	})

	t.Run("current session error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			CurrentFunc: func(context.Context) (string, bool, error) {
				return "", false, errors.New("current failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CurrentSession(context.Background())
		require.EqualError(t, err, "current failed")
	})

	t.Run("resolve default save error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			SaveFunc: func(context.Context, storage.Session) error {
				return errors.New("save failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.resolveDefaultSession(context.Background(), time.Now().UTC())
		require.EqualError(t, err, "save failed")
	})

	t.Run("clear idle default errors and no-op paths", func(t *testing.T) {
		t.Run("get error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&storagemock.SessionStore{
				GetFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{}, false, errors.New("get failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "get failed")
		})

		t.Run("session not found - no-op", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&storagemock.SessionStore{}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.NoError(t, manager.clearIdleDefaultSession(context.Background(), now))
		})

		t.Run("get messages error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&storagemock.SessionStore{
				GetFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return nil, errors.New("messages failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "messages failed")
		})

		t.Run("create archive error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&storagemock.SessionStore{
				GetFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
				},
				CreateArchiveFunc: func(context.Context, storage.ArchivedSession) error {
					return errors.New("archive failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "archive failed")
		})

		t.Run("clear messages error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&storagemock.SessionStore{
				GetFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
				},
				ClearMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) error {
					return errors.New("clear failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "clear failed")
		})

		t.Run("save error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&storagemock.SessionStore{
				GetFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				GetMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
				},
				SaveFunc: func(context.Context, storage.Session) error {
					return errors.New("save failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "save failed")
		})
	})
}

func TestManager_UpdateLastPromptTokens(t *testing.T) {
	var saved storage.Session

	manager, err := NewManager(&storagemock.SessionStore{
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
	manager, err := NewManager(&storagemock.SessionStore{
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

		require.EqualError(t, manager.SaveSummary(context.Background(), storage.SessionSummary{}), "session manager is required")

		summary, ok, err := manager.GetSummary(context.Background(), testSessionA)
		require.EqualError(t, err, "session manager is required")
		require.False(t, ok)
		require.Equal(t, storage.SessionSummary{}, summary)

		require.EqualError(t, manager.DeleteSummary(context.Background(), testSessionA), "session manager is required")
	})

	t.Run("save summary forwards to store", func(t *testing.T) {
		expected := storage.SessionSummary{
			SessionID:      testSessionA,
			SessionSummary: "summary",
		}

		var captured storage.SessionSummary
		manager, err := NewManager(&storagemock.SessionStore{
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
		manager, err := NewManager(&storagemock.SessionStore{
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
		manager, err := NewManager(&storagemock.SessionStore{
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
		manager, err := NewManager(&storagemock.SessionStore{
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
		manager, err := NewManager(&storagemock.SessionStore{
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
		manager, err := NewManager(&storagemock.SessionStore{
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
		manager, err := NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), "", 42)
		require.EqualError(t, err, "session id is required")
	})

	t.Run("get error", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{
			GetFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
		require.EqualError(t, err, "get failed")
	})

	t.Run("session not found", func(t *testing.T) {
		manager, err := NewManager(&storagemock.SessionStore{}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
		require.EqualError(t, err, "session not found")
	})
}
