package session

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
)

type managerStoreStub struct {
	getFunc                   func(context.Context, string) (Session, bool, error)
	listFunc                  func(context.Context) ([]Session, error)
	saveFunc                  func(context.Context, Session) error
	deleteFunc                func(context.Context, string) error
	setCurrentFunc            func(context.Context, string) error
	currentFunc               func(context.Context) (string, bool, error)
	appendMessagesFunc        func(context.Context, string, []handctx.Message) error
	getMessageFunc            func(context.Context, string, int, MessageQueryOptions) (handctx.Message, bool, error)
	getMessagesFunc           func(context.Context, string, MessageQueryOptions) ([]handctx.Message, error)
	clearMessagesFunc         func(context.Context, string, MessageQueryOptions) error
	createArchiveFunc         func(context.Context, ArchivedSession) error
	ListArchivesFunc          func(context.Context, string) ([]ArchivedSession, error)
	deleteArchivesFunc        func(context.Context, string) error
	deleteExpiredArchivesFunc func(context.Context, time.Time) error
}

func (s *managerStoreStub) Save(ctx context.Context, session Session) error {
	if s.saveFunc != nil {
		return s.saveFunc(ctx, session)
	}
	return nil
}

func (s *managerStoreStub) Get(ctx context.Context, id string) (Session, bool, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}
	return Session{}, false, nil
}

func (s *managerStoreStub) List(ctx context.Context) ([]Session, error) {
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

func (s *managerStoreStub) CreateArchive(ctx context.Context, archive ArchivedSession) error {
	if s.createArchiveFunc != nil {
		return s.createArchiveFunc(ctx, archive)
	}
	return nil
}

func (s *managerStoreStub) ListArchives(ctx context.Context, sourceSessionID string) ([]ArchivedSession, error) {
	if s.ListArchivesFunc != nil {
		return s.ListArchivesFunc(ctx, sourceSessionID)
	}
	return nil, nil
}

func (s *managerStoreStub) DeleteArchives(ctx context.Context, archiveID string) error {
	if s.deleteArchivesFunc != nil {
		return s.deleteArchivesFunc(ctx, archiveID)
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

func (s *managerStoreStub) AppendMessages(ctx context.Context, id string, messages []handctx.Message) error {
	if s.appendMessagesFunc != nil {
		return s.appendMessagesFunc(ctx, id, messages)
	}
	return nil
}

func (s *managerStoreStub) GetMessage(ctx context.Context, id string, index int, opts MessageQueryOptions) (handctx.Message, bool, error) {
	if s.getMessageFunc != nil {
		return s.getMessageFunc(ctx, id, index, opts)
	}
	return handctx.Message{}, false, nil
}

func (s *managerStoreStub) GetMessages(ctx context.Context, id string, opts MessageQueryOptions) ([]handctx.Message, error) {
	if s.getMessagesFunc != nil {
		return s.getMessagesFunc(ctx, id, opts)
	}
	return nil, nil
}

func (s *managerStoreStub) ClearMessages(ctx context.Context, id string, opts MessageQueryOptions) error {
	if s.clearMessagesFunc != nil {
		return s.clearMessagesFunc(ctx, id, opts)
	}
	return nil
}

func Test_Manager_ResolveChatSessionCreatesDefault(t *testing.T) {
	manager, err := NewManager(NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	session, err := manager.ResolveChatSession(context.Background(), "")

	require.NoError(t, err)
	require.Equal(t, DefaultSessionID, session.ID)
}

func Test_Manager_RunMaintenanceArchivesExpiredDefault(t *testing.T) {
	store := NewStore()
	expiredAt := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: expiredAt}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: expiredAt.Add(-time.Minute)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: expiredAt}))

	manager, err := NewManager(store, time.Hour, 48*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return expiredAt.Add(2 * time.Hour) }

	err = manager.runMaintenance(context.Background())
	require.NoError(t, err)

	archives, err := store.ListArchives(context.Background(), DefaultSessionID)
	require.NoError(t, err)
	require.Len(t, archives, 1)
	messages, err := store.GetMessages(context.Background(), archives[0].ID, MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "hello", messages[0].Content)

	liveMessages, err := store.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.NoError(t, err)
	require.Empty(t, liveMessages)
}

func Test_Manager_CurrentSessionDefaultsToDefault(t *testing.T) {
	manager, err := NewManager(NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	current, err := manager.CurrentSession(context.Background())

	require.NoError(t, err)
	require.Equal(t, DefaultSessionID, current)
}

func Test_Manager_UseSessionMaterializesDefaultSession(t *testing.T) {
	store := NewStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	err = manager.UseSession(context.Background(), DefaultSessionID)

	require.NoError(t, err)
	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, DefaultSessionID, current)
}

func Test_NewManager_ValidationAndNilManagerErrors(t *testing.T) {
	_, err := NewManager(nil, time.Hour, 24*time.Hour)
	require.EqualError(t, err, "session store is required")

	_, err = NewManager(NewStore(), 0, 24*time.Hour)
	require.EqualError(t, err, "session default idle expiry must be greater than zero")

	_, err = NewManager(NewStore(), time.Hour, 0)
	require.EqualError(t, err, "session archive retention must be greater than zero")

	var manager *Manager

	_, err = manager.ResolveChatSession(context.Background(), "")
	require.EqualError(t, err, "session manager is required")
	require.EqualError(t, manager.runMaintenance(context.Background()), "session manager is required")
	require.EqualError(t, manager.StartMaintenanceWorker(context.Background(), time.Second), "session manager is required")

	require.EqualError(t, manager.AppendMessages(context.Background(), DefaultSessionID, nil), "session manager is required")
	_, _, err = manager.GetMessage(context.Background(), DefaultSessionID, 0, MessageQueryOptions{})
	require.EqualError(t, err, "session manager is required")
	_, err = manager.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.EqualError(t, err, "session manager is required")

	_, err = manager.CreateSession(context.Background(), "project-a")
	require.EqualError(t, err, "session manager is required")

	_, err = manager.ListSessions(context.Background())
	require.EqualError(t, err, "session manager is required")

	require.EqualError(t, manager.UseSession(context.Background(), DefaultSessionID), "session manager is required")
	require.EqualError(t, manager.DeleteSession(context.Background(), DefaultSessionID), "session manager is required")
	require.EqualError(t, manager.DeleteSessionArchives(context.Background(), DefaultSessionID), "session manager is required")

	current, err := manager.CurrentSession(context.Background())
	require.EqualError(t, err, "session manager is required")
	require.Empty(t, current)
}

func Test_Manager_CreateSaveListAndResolveNonDefaultSession(t *testing.T) {
	store := NewStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	created, err := manager.CreateSession(context.Background(), "project-a")
	require.NoError(t, err)
	require.Equal(t, "project-a", created.ID)
	require.False(t, created.CreatedAt.IsZero())

	require.EqualError(t, manager.AppendMessages(context.Background(), "", nil), "session id is required")
	require.NoError(t, manager.AppendMessages(context.Background(), "project-a", []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)},
	}))

	resolved, err := manager.ResolveChatSession(context.Background(), "project-a")
	require.NoError(t, err)
	require.Equal(t, "project-a", resolved.ID)
	messages, err := manager.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "hello", messages[0].Content)
	message, ok, err := manager.GetMessage(context.Background(), "project-a", 0, MessageQueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", message.Content)

	sessions, err := manager.ListSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, DefaultSessionID, sessions[0].ID)
	require.Equal(t, "project-a", sessions[1].ID)
}

func Test_Manager_CreateUseAndResolveErrors(t *testing.T) {
	store := NewStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = manager.CreateSession(context.Background(), "")
	require.EqualError(t, err, "session id is required")

	_, err = manager.CreateSession(context.Background(), "project-a")
	require.NoError(t, err)

	_, err = manager.CreateSession(context.Background(), "project-a")
	require.EqualError(t, err, "session already exists")

	_, err = manager.ResolveChatSession(context.Background(), "missing")
	require.EqualError(t, err, "session not found")

	require.EqualError(t, manager.UseSession(context.Background(), "missing"), "session not found")
}

func Test_Manager_DeleteSessionAndArchives(t *testing.T) {
	store := NewStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = manager.CreateSession(context.Background(), "project-a")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), "project-a", []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-1",
		SourceSessionID: "project-a",
		ArchivedAt:      time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC),
		ExpiresAt:       time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
	}))

	require.NoError(t, manager.DeleteSessionArchives(context.Background(), "project-a"))
	archives, err := store.ListArchives(context.Background(), "project-a")
	require.NoError(t, err)
	require.Empty(t, archives)

	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-2",
		SourceSessionID: "project-a",
		ArchivedAt:      time.Date(2026, 3, 30, 14, 0, 0, 0, time.UTC),
		ExpiresAt:       time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC),
	}))
	require.NoError(t, manager.UseSession(context.Background(), "project-a"))
	require.NoError(t, manager.DeleteSession(context.Background(), "project-a"))

	_, ok, err := store.Get(context.Background(), "project-a")
	require.NoError(t, err)
	require.False(t, ok)
	archives, err = store.ListArchives(context.Background(), "project-a")
	require.NoError(t, err)
	require.Empty(t, archives)
	current, err := manager.CurrentSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, DefaultSessionID, current)
}

func Test_Manager_CurrentSessionUsesStoredSelection(t *testing.T) {
	store := NewStore()
	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	_, err = manager.CreateSession(context.Background(), "project-a")
	require.NoError(t, err)
	require.NoError(t, manager.UseSession(context.Background(), "project-a"))

	current, err := manager.CurrentSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, "project-a", current)
}

func Test_Manager_ResolveDefaultSessionKeepsActiveMessagesBeforeExpiry(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{
		{Role: handctx.RoleUser, Content: "still-active", CreatedAt: now.Add(-5 * time.Minute)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: now}))

	manager, err := NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return now.Add(30 * time.Minute) }

	session, err := manager.ResolveChatSession(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, DefaultSessionID, session.ID)
	messages, err := manager.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "still-active", messages[0].Content)

	archives, err := store.ListArchives(context.Background(), DefaultSessionID)
	require.NoError(t, err)
	require.Empty(t, archives)
}

func Test_Manager_ResolveChatSessionDoesNotRunMaintenance(t *testing.T) {
	store := NewStore()
	expiredAt := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: expiredAt}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: expiredAt.Add(-time.Minute)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: expiredAt}))

	manager, err := NewManager(store, time.Hour, 48*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return expiredAt.Add(2 * time.Hour) }

	session, err := manager.ResolveChatSession(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, DefaultSessionID, session.ID)

	messages, err := store.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)

	archives, err := store.ListArchives(context.Background(), DefaultSessionID)
	require.NoError(t, err)
	require.Empty(t, archives)
}

func Test_Manager_StartMaintenanceWorkerRunsMaintenance(t *testing.T) {
	store := NewStore()
	expiredAt := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: expiredAt}))
	require.NoError(t, store.AppendMessages(context.Background(), DefaultSessionID, []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: expiredAt.Add(-time.Minute)},
	}))
	require.NoError(t, store.Save(context.Background(), Session{ID: DefaultSessionID, UpdatedAt: expiredAt}))

	manager, err := NewManager(store, time.Hour, 48*time.Hour)
	require.NoError(t, err)
	manager.now = func() time.Time { return expiredAt.Add(2 * time.Hour) }

	ctx := t.Context()

	require.NoError(t, manager.StartMaintenanceWorker(ctx, 5*time.Millisecond))

	messages, err := store.GetMessages(context.Background(), DefaultSessionID, MessageQueryOptions{})
	require.NoError(t, err)
	require.Empty(t, messages)

	archives, err := store.ListArchives(context.Background(), DefaultSessionID)
	require.NoError(t, err)
	require.Len(t, archives, 1)
}

func Test_Manager_ErrorBranchesAndWorkerTick(t *testing.T) {
	t.Run("resolve non-default get error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.ResolveChatSession(context.Background(), "project-a")
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

	t.Run("start worker defaults interval and handles nil context", func(t *testing.T) {
		var deleteCalls atomic.Int32
		manager, err := NewManager(&managerStoreStub{
			deleteExpiredArchivesFunc: func(context.Context, time.Time) error {
				deleteCalls.Add(1)
				return nil
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.NoError(t, manager.StartMaintenanceWorker(nil, 0))
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

		require.NoError(t, manager.StartMaintenanceWorker(ctx, 5*time.Millisecond))
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

		err = manager.StartMaintenanceWorker(nil, time.Second)
		require.EqualError(t, err, "delete failed")
	})

	t.Run("create session get error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CreateSession(context.Background(), "project-a")
		require.EqualError(t, err, "get failed")
	})

	t.Run("create session save error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			saveFunc: func(context.Context, Session) error {
				return errors.New("save failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.CreateSession(context.Background(), "project-a")
		require.EqualError(t, err, "save failed")
	})

	t.Run("list sessions resolve default error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.ListSessions(context.Background())
		require.EqualError(t, err, "get failed")
	})

	t.Run("use default resolve error", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		err = manager.UseSession(context.Background(), DefaultSessionID)
		require.EqualError(t, err, "get failed")
	})

	t.Run("delete session validation and errors", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			ListArchivesFunc: func(context.Context, string) ([]ArchivedSession, error) {
				return []ArchivedSession{{ID: "archive-1"}}, nil
			},
			deleteArchivesFunc: func(context.Context, string) error {
				return errors.New("delete archives failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSession(context.Background(), ""), "session id is required")
		require.EqualError(t, manager.DeleteSession(context.Background(), DefaultSessionID), "default session cannot be deleted")
		require.EqualError(t, manager.DeleteSession(context.Background(), "project-a"), "delete archives failed")

		manager, err = NewManager(&managerStoreStub{
			ListArchivesFunc: func(context.Context, string) ([]ArchivedSession, error) {
				return nil, errors.New("get archives failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSession(context.Background(), "project-a"), "get archives failed")

		manager, err = NewManager(&managerStoreStub{
			ListArchivesFunc: func(context.Context, string) ([]ArchivedSession, error) {
				return nil, nil
			},
			deleteFunc: func(context.Context, string) error {
				return errors.New("delete failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSession(context.Background(), "project-a"), "delete failed")
	})

	t.Run("delete session archives validation and errors", func(t *testing.T) {
		manager, err := NewManager(&managerStoreStub{
			ListArchivesFunc: func(context.Context, string) ([]ArchivedSession, error) {
				return []ArchivedSession{{ID: "archive-1"}}, nil
			},
			deleteArchivesFunc: func(context.Context, string) error {
				return errors.New("delete archives failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSessionArchives(context.Background(), ""), "session id is required")
		require.EqualError(t, manager.DeleteSessionArchives(context.Background(), "project-a"), "delete archives failed")

		manager, err = NewManager(&managerStoreStub{
			ListArchivesFunc: func(context.Context, string) ([]ArchivedSession, error) {
				return nil, errors.New("get archives failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		require.EqualError(t, manager.DeleteSessionArchives(context.Background(), "project-a"), "get archives failed")
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
			saveFunc: func(context.Context, Session) error {
				return errors.New("save failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)

		_, err = manager.resolveDefaultSession(context.Background(), time.Now().UTC())
		require.EqualError(t, err, "save failed")
	})

	t.Run("clear idle default errors and no-op paths", func(t *testing.T) {
		now := time.Now().UTC()

		manager, err := NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{}, false, errors.New("get failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)
		require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "get failed")

		manager, err = NewManager(&managerStoreStub{}, time.Hour, 24*time.Hour)
		require.NoError(t, err)
		require.NoError(t, manager.clearIdleDefaultSession(context.Background(), now))

		manager, err = NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{ID: DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
			},
			getMessagesFunc: func(context.Context, string, MessageQueryOptions) ([]handctx.Message, error) {
				return nil, errors.New("messages failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)
		require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "messages failed")

		manager, err = NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{ID: DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
			},
			getMessagesFunc: func(context.Context, string, MessageQueryOptions) ([]handctx.Message, error) {
				return []handctx.Message{{Role: handctx.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
			},
			createArchiveFunc: func(context.Context, ArchivedSession) error {
				return errors.New("archive failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)
		require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "archive failed")

		manager, err = NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{ID: DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
			},
			getMessagesFunc: func(context.Context, string, MessageQueryOptions) ([]handctx.Message, error) {
				return []handctx.Message{{Role: handctx.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
			},
			clearMessagesFunc: func(context.Context, string, MessageQueryOptions) error {
				return errors.New("clear failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)
		require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "clear failed")

		manager, err = NewManager(&managerStoreStub{
			getFunc: func(context.Context, string) (Session, bool, error) {
				return Session{ID: DefaultSessionID, UpdatedAt: now.Add(-2 * time.Hour)}, true, nil
			},
			getMessagesFunc: func(context.Context, string, MessageQueryOptions) ([]handctx.Message, error) {
				return []handctx.Message{{Role: handctx.RoleUser, Content: "hello", CreatedAt: now.Add(-3 * time.Hour)}}, nil
			},
			saveFunc: func(context.Context, Session) error {
				return errors.New("save failed")
			},
		}, time.Hour, 24*time.Hour)
		require.NoError(t, err)
		require.EqualError(t, manager.clearIdleDefaultSession(context.Background(), now), "save failed")
	})
}

func Test_SQLiteStore_PersistsSessionsAndCurrentSelection(t *testing.T) {
	store, err := NewSQLiteStore(t.TempDir() + "/session.db")
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-a", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-a", []handctx.Message{
		{Role: handctx.RoleUser, Content: "hello", CreatedAt: now},
	}))
	require.NoError(t, store.SetCurrent(context.Background(), "project-a"))

	loaded, ok, err := store.Get(context.Background(), "project-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "project-a", loaded.ID)
	messages, err := store.GetMessages(context.Background(), "project-a", MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "hello", messages[0].Content)

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "project-a", current)

	require.True(t, store.db.Migrator().HasTable(&sqliteRecord{}))
	require.True(t, store.db.Migrator().HasTable(&sqliteMessageRecord{}))
	require.False(t, store.db.Migrator().HasColumn(&sqliteRecord{}, "messages"))

	var storedMessages int64
	require.NoError(t, store.db.Model(&sqliteMessageRecord{}).Where("session_id = ?", "project-a").Count(&storedMessages).Error)
	require.EqualValues(t, 1, storedMessages)
}

func Test_SQLiteStore_ArchiveLifecycleAndCurrentSelection(t *testing.T) {
	store, err := NewSQLiteStore(t.TempDir() + "/session.db")
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: "project-b", UpdatedAt: now}))
	require.NoError(t, store.AppendMessages(context.Background(), "project-b", []handctx.Message{
		{Role: handctx.RoleUser, Content: "one", CreatedAt: now},
		{Role: handctx.RoleAssistant, Content: "two", CreatedAt: now.Add(time.Second)},
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-expired",
		SourceSessionID: "project-b",
		ArchivedAt:      now.Add(-time.Hour),
		ExpiresAt:       now.Add(-time.Minute),
	}))
	require.NoError(t, store.CreateArchive(context.Background(), ArchivedSession{
		ID:              "archive-active",
		SourceSessionID: "project-b",
		ArchivedAt:      now,
		ExpiresAt:       now.Add(time.Hour),
	}))

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	messages, err := store.GetMessages(context.Background(), "project-b", MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "one", messages[0].Content)
	require.Equal(t, "two", messages[1].Content)

	archives, err := store.ListArchives(context.Background(), "project-b")
	require.NoError(t, err)
	require.Len(t, archives, 2)
	require.Equal(t, "archive-active", archives[0].ID)
	messages, err = store.GetMessages(context.Background(), "archive-active", MessageQueryOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "one", messages[0].Content)

	require.NoError(t, store.DeleteExpiredArchives(context.Background(), now))

	archives, err = store.ListArchives(context.Background(), "project-b")
	require.NoError(t, err)
	require.Len(t, archives, 1)
	require.Equal(t, "archive-active", archives[0].ID)

	current, ok, err := store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)

	require.EqualError(t, store.SetCurrent(context.Background(), ""), "session id is required")
	require.EqualError(t, store.SetCurrent(context.Background(), "missing"), "session not found")
}

func Test_SQLiteStore_ValidationAndGetBehavior(t *testing.T) {
	_, err := NewSQLiteStore("")
	require.EqualError(t, err, "session sqlite path is required")

	store, err := NewSQLiteStore(t.TempDir() + "/session.db")
	require.NoError(t, err)

	session, ok, err := store.Get(context.Background(), "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Session{}, session)
}

func Test_SQLiteStore_ReplacesMessagesAndOrdersSessions(t *testing.T) {
	store, err := NewSQLiteStore(t.TempDir() + "/session.db")
	require.NoError(t, err)

	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(context.Background(), Session{ID: "same-time-b", UpdatedAt: now.Add(-time.Minute)}))
	require.NoError(t, store.AppendMessages(context.Background(), "same-time-b", []handctx.Message{
		{Role: handctx.RoleUser, Content: "first", CreatedAt: now},
	}))
	require.NoError(t, store.Save(context.Background(), Session{
		ID:        "same-time-a",
		UpdatedAt: now,
	}))
	require.NoError(t, store.ClearMessages(context.Background(), "same-time-b", MessageQueryOptions{}))
	require.NoError(t, store.AppendMessages(context.Background(), "same-time-b", []handctx.Message{
		{Role: handctx.RoleAssistant, Content: "replaced", CreatedAt: now.Add(time.Second)},
	}))

	sessions, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "same-time-b", sessions[0].ID)
	require.Equal(t, "same-time-a", sessions[1].ID)
	messages, err := store.GetMessages(context.Background(), "same-time-b", MessageQueryOptions{})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "replaced", messages[0].Content)

	var count int64
	require.NoError(t, store.db.Model(&sqliteMessageRecord{}).Where("session_id = ?", "same-time-b").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func Test_SQLiteStore_BlankCurrentStateAndNilReceiverErrors(t *testing.T) {
	var nilStore *SQLiteStore
	require.EqualError(t, nilStore.Save(context.Background(), Session{ID: "project-a"}), "session store is required")
	_, ok, err := nilStore.Get(context.Background(), "project-a")
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	_, err = nilStore.List(context.Background())
	require.EqualError(t, err, "session store is required")
	require.EqualError(t, nilStore.CreateArchive(context.Background(), ArchivedSession{ID: "archive-1"}), "session store is required")
	_, err = nilStore.ListArchives(context.Background(), "")
	require.EqualError(t, err, "session store is required")
	_, err = nilStore.GetMessages(context.Background(), "", MessageQueryOptions{})
	require.EqualError(t, err, "session store is required")
	require.EqualError(t, nilStore.DeleteExpiredArchives(context.Background(), time.Now().UTC()), "session store is required")
	require.EqualError(t, nilStore.SetCurrent(context.Background(), DefaultSessionID), "session store is required")
	current, ok, err := nilStore.Current(context.Background())
	require.EqualError(t, err, "session store is required")
	require.False(t, ok)
	require.Empty(t, current)

	store, err := NewSQLiteStore(t.TempDir() + "/session.db")
	require.NoError(t, err)
	require.NoError(t, store.db.Save(&sqliteStateRecord{
		Key:       currentSessionStateKey,
		Value:     "   ",
		UpdatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}).Error)

	current, ok, err = store.Current(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, current)
}

func Test_OpenStore_DefaultsToSQLiteForEmptyBackend(t *testing.T) {
	store, err := OpenStore(&config.Config{})

	require.NoError(t, err)
	_, ok := store.(*SQLiteStore)
	require.True(t, ok)
}

func Test_OpenStore_SupportsMemoryAndRejectsInvalidBackend(t *testing.T) {
	store, err := OpenStore(&config.Config{SessionBackend: "memory"})
	require.NoError(t, err)
	_, ok := store.(*MemoryStore)
	require.True(t, ok)

	_, err = OpenStore(&config.Config{SessionBackend: "invalid"})
	require.EqualError(t, err, "session backend must be one of: memory, sqlite")

	_, err = OpenStore(nil)
	require.EqualError(t, err, "config is required")
}
