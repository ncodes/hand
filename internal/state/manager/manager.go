package manager

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

// Manager manages manager.
type Manager struct {
	store             storage.Store
	defaultIdleExpiry time.Duration
	archiveRetention  time.Duration
	now               func() time.Time
	workerOnce        sync.Once
}

var generateSessionID = storage.NewSessionID

// NewManager returns a state manager backed by the supplied store.
func NewManager(store storage.Store, defaultIdleExpiry, archiveRetention time.Duration) (*Manager, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	if store.Session() == nil {
		return nil, errors.New("session store is required")
	}

	if defaultIdleExpiry <= 0 {
		return nil, errors.New("session default idle expiry must be greater than zero")
	}

	if archiveRetention <= 0 {
		return nil, errors.New("session archive retention must be greater than zero")
	}

	return &Manager{
		store:             store,
		defaultIdleExpiry: defaultIdleExpiry,
		archiveRetention:  archiveRetention,
		now:               func() time.Time { return time.Now().UTC() },
	}, nil
}

func (m *Manager) Close() error {
	if m == nil || m.store == nil {
		return nil
	}

	closer, ok := m.store.(interface{ Close() error })
	if !ok {
		return nil
	}

	return closer.Close()
}

func (m *Manager) sessions() storage.SessionStore {
	return m.store.Session()
}

func (m *Manager) MemoryStore() (storage.MemoryStore, bool) {
	if m == nil || m.store == nil {
		return nil, false
	}

	return m.store.Memory()
}

func (m *Manager) SupportsVectorSearch() bool {
	if m == nil || m.store == nil {
		return false
	}

	return m.store.SupportsVectorSearch()
}

func (m *Manager) SearchMemory(
	ctx context.Context,
	query storage.MemorySearchQuery,
) (storage.MemorySearchResult, error) {
	store, ok := m.MemoryStore()
	if !ok {
		return storage.MemorySearchResult{}, errors.New("memory store is not supported")
	}

	return store.SearchMemory(ctx, query)
}

func (m *Manager) ListSessionMemories(
	ctx context.Context,
	query storage.SessionMemoryQuery,
) (storage.SessionMemoriesResult, error) {
	store, ok := m.MemoryStore()
	if !ok {
		return storage.SessionMemoriesResult{}, errors.New("memory store is not supported")
	}

	return store.ListSessionMemories(ctx, query)
}

func (m *Manager) UpsertMemory(
	ctx context.Context,
	item storage.MemoryItem,
) (storage.MemoryItem, error) {
	store, ok := m.MemoryStore()
	if !ok {
		return storage.MemoryItem{}, errors.New("memory store is not supported")
	}

	return store.UpsertMemory(ctx, item)
}

func (m *Manager) PatchMemory(
	ctx context.Context,
	patch storage.MemoryPatch,
) (storage.MemoryItem, error) {
	store, ok := m.MemoryStore()
	if !ok {
		return storage.MemoryItem{}, errors.New("memory store is not supported")
	}

	return store.PatchMemory(ctx, patch)
}

func (m *Manager) DeleteMemory(ctx context.Context, req storage.MemoryDeleteRequest) error {
	store, ok := m.MemoryStore()
	if !ok {
		return errors.New("memory store is not supported")
	}

	return store.DeleteMemory(ctx, req)
}

func (m *Manager) TraceStore() (storage.TraceStore, bool) {
	if m == nil || m.store == nil {
		return nil, false
	}

	return m.store.Trace()
}

func (m *Manager) AppendTraceEvent(ctx context.Context, event storage.TraceEvent) (storage.TraceEvent, error) {
	store, ok := m.TraceStore()
	if !ok {
		return storage.TraceEvent{}, storage.ErrTraceStoreUnsupported
	}

	return store.AppendTraceEvent(ctx, event)
}

func (m *Manager) ListTraceEvents(ctx context.Context, query storage.TraceQuery) (storage.TraceResult, error) {
	store, ok := m.TraceStore()
	if !ok {
		return storage.TraceResult{}, storage.ErrTraceStoreUnsupported
	}

	return store.ListTraceEvents(ctx, query)
}

func (m *Manager) PruneTraceEvents(ctx context.Context, sessionID string, maxEvents int) error {
	store, ok := m.TraceStore()
	if !ok {
		return storage.ErrTraceStoreUnsupported
	}

	return store.PruneTraceEvents(ctx, sessionID, maxEvents)
}

func (m *Manager) Resolve(ctx context.Context, id string) (storage.Session, error) {
	if m == nil {
		return storage.Session{}, errors.New("state manager is required")
	}

	now := m.now().UTC()

	id = strings.TrimSpace(id)
	if id == "" {
		id = storage.DefaultSessionID
	}

	if id == storage.DefaultSessionID {
		return m.resolveDefaultSession(ctx, now)
	}

	if err := storage.ValidateSessionID(id); err != nil {
		return storage.Session{}, err
	}

	return m.getActiveSession(ctx, id)
}

func (m *Manager) runMaintenance(ctx context.Context) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	now := m.now().UTC()
	if err := m.sessions().DeleteExpiredArchives(ctx, now); err != nil {
		return err
	}

	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	return m.startMaintenanceWorker(ctx, time.Minute)
}

func (m *Manager) startMaintenanceWorker(ctx context.Context, interval time.Duration) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	if interval <= 0 {
		interval = time.Minute
	}

	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}

	if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
		return err
	}

	if err := m.runMaintenance(ctx); err != nil {
		return err
	}

	m.workerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = m.runMaintenance(ctx)
				}
			}
		}()
	})

	return nil
}

func (m *Manager) AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}
	if id == storage.DefaultSessionID {
		if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
			return err
		}
	} else if err := m.checkSessionActive(ctx, id); err != nil {
		return err
	}

	return m.sessions().AppendMessages(ctx, id, storage.CloneMessages(messages))
}

func (m *Manager) Save(ctx context.Context, session storage.Session) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.sessions().Save(ctx, session)
}

func (m *Manager) Get(ctx context.Context, id string, opts storage.SessionGetOptions) (storage.Session, bool, error) {
	if m == nil {
		return storage.Session{}, false, errors.New("state manager is required")
	}

	return m.sessions().Get(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) GetMessages(
	ctx context.Context,
	id string,
	opts storage.MessageQueryOptions,
) ([]handmsg.Message, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	return m.sessions().GetMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) GetMessagesByIDs(
	ctx context.Context,
	id string,
	messageIDs []uint,
) ([]storage.MessageRecord, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	return m.sessions().GetMessagesByIDs(ctx, strings.TrimSpace(id), messageIDs)
}

func (m *Manager) GetMessageWindow(
	ctx context.Context,
	id string,
	anchorMessageID uint,
	before int,
	after int,
) ([]storage.MessageRecord, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	return m.sessions().GetMessageWindow(ctx, strings.TrimSpace(id), anchorMessageID, before, after)
}

func (m *Manager) SearchMessages(
	ctx context.Context,
	id string,
	opts storage.SearchMessageOptions,
) ([]storage.SearchMessageResult, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	opts.IgnoreSessionID = strings.TrimSpace(opts.IgnoreSessionID)

	return m.sessions().SearchMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) RepairVectorStore(
	ctx context.Context,
	opts search.VectorRepairOptions,
) (search.VectorRepairResult, error) {
	if m == nil {
		return search.VectorRepairResult{}, errors.New("state manager is required")
	}

	repairStore, ok := m.store.(search.VectorRepairStore)
	if !ok {
		return search.VectorRepairResult{}, errors.New("session vector repair is not supported")
	}

	opts.SessionID = strings.TrimSpace(opts.SessionID)
	return repairStore.RepairVectorStore(ctx, opts)
}

func (m *Manager) CountMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) (int, error) {
	if m == nil {
		return 0, errors.New("state manager is required")
	}

	return m.sessions().CountMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) UpdateCheckpoints(ctx context.Context, id string, patch storage.CheckpointPatch) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.sessions().UpdateCheckpoints(ctx, strings.TrimSpace(id), patch)
}

func (m *Manager) GetMessage(
	ctx context.Context,
	id string,
	index int,
) (handmsg.Message, bool, error) {
	if m == nil {
		return handmsg.Message{}, false, errors.New("state manager is required")
	}

	return m.sessions().GetMessage(ctx, strings.TrimSpace(id), index)
}

func (m *Manager) SaveSummary(ctx context.Context, summary storage.SessionSummary) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.sessions().SaveSummary(ctx, summary)
}

func (m *Manager) GetSummary(ctx context.Context, sessionID string) (storage.SessionSummary, bool, error) {
	if m == nil {
		return storage.SessionSummary{}, false, errors.New("state manager is required")
	}

	return m.sessions().GetSummary(ctx, strings.TrimSpace(sessionID))
}

func (m *Manager) DeleteSummary(ctx context.Context, sessionID string) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.sessions().DeleteSummary(ctx, strings.TrimSpace(sessionID))
}

func (m *Manager) UpdateLastPromptTokens(ctx context.Context, id string, promptTokens int) error {
	if m == nil {
		return errors.New("state manager is required")
	}
	if promptTokens <= 0 {
		return nil
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}

	session, ok, err := m.sessions().Get(ctx, id, storage.SessionGetOptions{})
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("session not found")
	}

	session.LastPromptTokens = promptTokens
	return m.sessions().Save(ctx, session)
}

func (m *Manager) CreateSession(ctx context.Context, id string) (storage.Session, error) {
	if m == nil {
		return storage.Session{}, errors.New("state manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		generatedID, err := generateSessionID()
		if err != nil {
			return storage.Session{}, err
		}
		id = generatedID
	} else if err := storage.ValidateSessionID(id); err != nil {
		return storage.Session{}, err
	}

	if _, ok, err := m.sessions().Get(ctx, id, storage.SessionGetOptions{}); err != nil {
		return storage.Session{}, err
	} else if ok {
		return storage.Session{}, errors.New("session already exists")
	}

	now := m.now().UTC()
	session := storage.Session{CreatedAt: now, ID: id, UpdatedAt: now}

	if err := m.sessions().Save(ctx, session); err != nil {
		return storage.Session{}, err
	}

	return session, nil
}

func (m *Manager) ListSessions(ctx context.Context, opts ...storage.SessionListOptions) ([]storage.Session, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	listOpts := getSessionListOptions(opts...)
	if listOpts.Archived != nil && !*listOpts.Archived {
		if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
			return nil, err
		}
	}

	return m.sessions().List(ctx, listOpts)
}

func getSessionListOptions(opts ...storage.SessionListOptions) storage.SessionListOptions {
	if len(opts) == 0 {
		active := false
		return storage.SessionListOptions{Archived: &active}
	}

	return opts[0]
}

func (m *Manager) DeleteSession(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	id = strings.TrimSpace(id)
	if id != "" {
		if err := storage.ValidateSessionID(id); err != nil {
			return err
		}
	}

	return m.sessions().Delete(ctx, id)
}

func (m *Manager) ArchiveSession(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}
	if id == storage.DefaultSessionID {
		return errors.New("default session cannot be archived")
	}
	if err := storage.ValidateSessionID(id); err != nil {
		return err
	}

	now := m.now().UTC()
	_, err := m.sessions().Archive(ctx, id, storage.SessionArchiveRequest{
		ArchivedAt: now,
		ExpiresAt:  now.Add(m.archiveRetention),
	})
	return err
}

func (m *Manager) UnarchiveSession(ctx context.Context, id string) (storage.Session, error) {
	if m == nil {
		return storage.Session{}, errors.New("state manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return storage.Session{}, errors.New("session id is required")
	}
	if err := storage.ValidateSessionID(id); err != nil {
		return storage.Session{}, err
	}

	return m.sessions().Unarchive(ctx, id)
}

func (m *Manager) RenameSession(ctx context.Context, id string, title string) (storage.Session, error) {
	if m == nil {
		return storage.Session{}, errors.New("state manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return storage.Session{}, errors.New("session id is required")
	}

	title = storage.NormalizeSessionTitle(title)
	if title == "" {
		return storage.Session{}, errors.New("session title is required")
	}

	if id == storage.DefaultSessionID {
		if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
			return storage.Session{}, err
		}
	} else if err := storage.ValidateSessionID(id); err != nil {
		return storage.Session{}, err
	} else if err := m.checkSessionActive(ctx, id); err != nil {
		return storage.Session{}, err
	}

	return m.sessions().Rename(ctx, storage.SessionRenameRequest{
		SessionID:   id,
		Title:       title,
		TitleSource: storage.SessionTitleSourceManual,
		RenamedAt:   m.now().UTC(),
	})
}

func (m *Manager) UseSession(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	id = strings.TrimSpace(id)
	if id == storage.DefaultSessionID {
		if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
			return err
		}
	} else if err := storage.ValidateSessionID(id); err != nil {
		return err
	} else if err := m.checkSessionActive(ctx, id); err != nil {
		return err
	}

	return m.sessions().SetCurrent(ctx, id)
}

func (m *Manager) checkSessionActive(ctx context.Context, id string) error {
	_, err := m.getActiveSession(ctx, strings.TrimSpace(id))
	return err
}

func (m *Manager) getActiveSession(ctx context.Context, id string) (storage.Session, error) {
	session, ok, err := m.sessions().Get(ctx, id, storage.SessionGetOptions{})
	if err != nil {
		return storage.Session{}, err
	}
	if !ok {
		return storage.Session{}, errors.New("session not found")
	}
	if session.Archived {
		return storage.Session{}, errors.New("session is archived")
	}

	return session, nil
}

func (m *Manager) CurrentSession(ctx context.Context) (string, error) {
	if m == nil {
		return "", errors.New("state manager is required")
	}

	id, ok, err := m.sessions().Current(ctx)
	if err != nil {
		return "", err
	}

	if ok {
		return id, nil
	}

	return storage.DefaultSessionID, nil
}

func (m *Manager) resolveDefaultSession(ctx context.Context, now time.Time) (storage.Session, error) {
	session, ok, err := m.sessions().Get(ctx, storage.DefaultSessionID, storage.SessionGetOptions{})
	if err != nil {
		return storage.Session{}, err
	}

	if !ok {
		session = storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now}
		if err := m.sessions().Save(ctx, session); err != nil {
			return storage.Session{}, err
		}

		return session, nil
	}

	return session, nil
}
