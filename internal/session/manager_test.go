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
	"github.com/wandxy/hand/pkg/nanoid"
)

var (
	testSessionA       = nanoid.MustFromSeed(storage.SessionIDPrefix, "project-a", "SessionTestSeedValue123")
	testMissingSession = nanoid.MustFromSeed(storage.SessionIDPrefix, "missing", "SessionTestSeedValue123")
)

type managerStoreStub struct {
	getFunc                   func(context.Context, string) (storage.Session, bool, error)
	listFunc                  func(context.Context) ([]storage.Session, error)
	saveFunc                  func(context.Context, storage.Session) error
	deleteFunc                func(context.Context, string) error
	setCurrentFunc            func(context.Context, string) error
	currentFunc               func(context.Context) (string, bool, error)
	appendMessagesFunc        func(context.Context, string, []handmsg.Message) error
	getMessageFunc            func(context.Context, string, int, storage.MessageQueryOptions) (handmsg.Message, bool, error)
	getMessagesFunc           func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error)
	clearMessagesFunc         func(context.Context, string, storage.MessageQueryOptions) error
	createArchiveFunc         func(context.Context, storage.ArchivedSession) error
	getArchiveFunc            func(context.Context, string) (storage.ArchivedSession, bool, error)
	ListArchivesFunc          func(context.Context, string) ([]storage.ArchivedSession, error)
	deleteArchiveFunc         func(context.Context, string) error
	deleteExpiredArchivesFunc func(context.Context, time.Time) error
}

func (s *managerStoreStub) Save(ctx context.Context, session storage.Session) error {
	if s.saveFunc != nil {
		return s.saveFunc(ctx, session)
	}

	return nil
}

func (s *managerStoreStub) Get(ctx context.Context, id string) (storage.Session, bool, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}

	return storage.Session{}, false, nil
}

func (s *managerStoreStub) List(ctx context.Context) ([]storage.Session, error) {
	if s.listFunc != nil {
		return s.listFunc(ctx)
	}

	return nil, nil
}

func (s *managerStoreStub) Delete(ctx context.Context, id string) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, id)
	}

	return nil
}

func (s *managerStoreStub) CreateArchive(ctx context.Context, archive storage.ArchivedSession) error {
	if s.createArchiveFunc != nil {
		return s.createArchiveFunc(ctx, archive)
	}

	return nil
}

func (s *managerStoreStub) GetArchive(ctx context.Context, id string) (storage.ArchivedSession, bool, error) {
	if s.getArchiveFunc != nil {
		return s.getArchiveFunc(ctx, id)
	}

	return storage.ArchivedSession{}, false, nil
}

func (s *managerStoreStub) ListArchives(ctx context.Context, sourceSessionID string) ([]storage.ArchivedSession, error) {
	if s.ListArchivesFunc != nil {
		return s.ListArchivesFunc(ctx, sourceSessionID)
	}

	return nil, nil
}

func (s *managerStoreStub) DeleteArchive(ctx context.Context, archiveID string) error {
	if s.deleteArchiveFunc != nil {
		return s.deleteArchiveFunc(ctx, archiveID)
	}

	return nil
}

func (s *managerStoreStub) DeleteExpiredArchives(ctx context.Context, now time.Time) error {
	if s.deleteExpiredArchivesFunc != nil {
		return s.deleteExpiredArchivesFunc(ctx, now)
	}

	return nil
}

func (s *managerStoreStub) SetCurrent(ctx context.Context, id string) error {
	if s.setCurrentFunc != nil {
		return s.setCurrentFunc(ctx, id)
	}

	return nil
}

func (s *managerStoreStub) Current(ctx context.Context) (string, bool, error) {
	if s.currentFunc != nil {
		return s.currentFunc(ctx)
	}

	return "", false, nil
}

func (s *managerStoreStub) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if s.appendMessagesFunc != nil {
		return s.appendMessagesFunc(ctx, id, messages)
	}

	return nil
}

func (s *managerStoreStub) GetMessage(ctx context.Context, id string, index int, opts storage.MessageQueryOptions) (handmsg.Message, bool, error) {
	if s.getMessageFunc != nil {
		return s.getMessageFunc(ctx, id, index, opts)
	}

	return handmsg.Message{}, false, nil
}

func (s *managerStoreStub) GetMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
	if s.getMessagesFunc != nil {
		return s.getMessagesFunc(ctx, id, opts)
	}

	return nil, nil
}

func (s *managerStoreStub) ClearMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) error {
	if s.clearMessagesFunc != nil {
		return s.clearMessagesFunc(ctx, id, opts)
	}

	return nil
}

func TestManager_ResolveChatSessionCreatesDefault(t *testing.T) {
	manager, err := NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	session, err := manager.ResolveSession(context.Background(), "")

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

	_, err = manager.ResolveSession(context.Background(), "")
	require.EqualError(t, err, "session manager is required")
	require.EqualError(t, manager.runMaintenance(context.Background()), "session manager is required")
	require.EqualError(t, manager.Start(context.Background()), "session manager is required")

	require.EqualError(t, manager.AppendMessages(context.Background(), storage.DefaultSessionID, nil), "session manager is required")
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

	resolved, err := manager.ResolveSession(context.Background(), testSessionA)
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

	_, err = manager.ResolveSession(context.Background(), testMissingSession)
	require.EqualError(t, err, "session not found")

	_, err = manager.ResolveSession(context.Background(), "project-a")
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
		ID:              "archive-1",
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
		ID:              "archive-2",
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
	require.Equal(t, "archive-2", archives[0].ID)
	require.Equal(t, "archive-1", archives[1].ID)
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

	session, err := manager.ResolveSession(context.Background(), "")
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

	session, err := manager.ResolveSession(context.Background(), "")
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
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.ResolveSession(context.Background(), testSessionA)
		require.EqualError(t, err, "get failed")
	})

	t.Run("run maintenance delete error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			deleteExpiredArchivesFunc: func(context.Context, time.Time) error {
				return errors.New("delete failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.runMaintenance(context.Background())
		require.EqualError(t, err, "delete failed")
	})

	t.Run("start worker defaults interval", func(t *testing.T) {
		var deleteCalls atomic.Int32
		manager, err := NewManager(&managerStoreStub{
			deleteExpiredArchivesFunc: func(context.Context, time.Time) error {
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
		manager, err := NewManager(&managerStoreStub{
			deleteExpiredArchivesFunc: func(context.Context, time.Time) error {
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
		manager, err := NewManager(&managerStoreStub{
			deleteExpiredArchivesFunc: func(context.Context, time.Time) error {
				return errors.New("delete failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.startMaintenanceWorker(context.TODO(), time.Second)
		require.EqualError(t, err, "delete failed")
	})

	t.Run("start worker returns default resolution error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.startMaintenanceWorker(context.TODO(), time.Second)
		require.EqualError(t, err, "get failed")
	})

	t.Run("start worker uses background when context is canceled", func(t *testing.T) {
		var captured context.Context
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(ctx context.Context, _ string) (storage.Session, bool, error) {
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
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (storage.Session, bool, error) {
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

		manager, err := NewManager(&managerStoreStub{}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CreateSession(context.Background(), "")
		require.EqualError(t, err, "generate failed")
	})

	t.Run("create session save error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			saveFunc: func(context.Context, storage.Session) error {
				return errors.New("save failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CreateSession(context.Background(), testSessionA)
		require.EqualError(t, err, "save failed")
	})

	t.Run("list sessions resolve default error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.ListSessions(context.Background())
		require.EqualError(t, err, "get failed")
	})

	t.Run("use default resolve error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (storage.Session, bool, error) {
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

		manager, err = NewManager(&managerStoreStub{
			deleteFunc: func(context.Context, string) error {
				return errors.New("delete failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSession(context.Background(), testSessionA), "delete failed")

		require.EqualError(t, manager.DeleteSession(context.Background(), "project-a"), "session id must be a valid ses_ nanoid")
	})

	t.Run("current session error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			currentFunc: func(context.Context) (string, bool, error) {
				return "", false, errors.New("current failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CurrentSession(context.Background())
		require.EqualError(t, err, "current failed")
	})

	t.Run("resolve default save error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			saveFunc: func(context.Context, storage.Session) error {
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

			manager, err := NewManager(&managerStoreStub{
				getFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{}, false, errors.New("get failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "get failed")
		})

		t.Run("session not found - no-op", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&managerStoreStub{}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.NoError(t, manager.clearIdleDefaultSession(context.Background(), now))
		})

		t.Run("get messages error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&managerStoreStub{
				getFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				getMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return nil, errors.New("messages failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "messages failed")
		})

		t.Run("create archive error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&managerStoreStub{
				getFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				getMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
				},
				createArchiveFunc: func(context.Context, storage.ArchivedSession) error {
					return errors.New("archive failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "archive failed")
		})

		t.Run("clear messages error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&managerStoreStub{
				getFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				getMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
				},
				clearMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) error {
					return errors.New("clear failed")
				},
			}, time.Hour, 24*time.Hour)
			require.NoError(t, err)
			require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "clear failed")
		})

		t.Run("save error", func(t *testing.T) {
			now := time.Now().UTC()

			manager, err := NewManager(&managerStoreStub{
				getFunc: func(context.Context, string) (storage.Session, bool, error) {
					return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
				},
				getMessagesFunc: func(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
					return []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
				},
				saveFunc: func(context.Context, storage.Session) error {
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

	manager, err := NewManager(&managerStoreStub{
		getFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: testSessionA, UpdatedAt: time.Now().UTC()}, true, nil
		},
		saveFunc: func(_ context.Context, session storage.Session) error {
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
	manager, err := NewManager(&managerStoreStub{
		getFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: testSessionA, UpdatedAt: time.Now().UTC()}, true, nil
		},
		saveFunc: func(context.Context, storage.Session) error {
			return errors.New("save failed")
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
	require.EqualError(t, err, "save failed")
}

func TestManager_UpdateLastPromptTokens_Errors(t *testing.T) {
	t.Run("empty id", func(t *testing.T) {
		manager, err := NewManager(storagememory.NewSessionStore(), time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), "", 42)
		require.EqualError(t, err, "session id is required")
	})

	t.Run("get error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (storage.Session, bool, error) {
				return storage.Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
		require.EqualError(t, err, "get failed")
	})

	t.Run("session not found", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UpdateLastPromptTokens(context.Background(), testSessionA, 42)
		require.EqualError(t, err, "session not found")
	})
}
