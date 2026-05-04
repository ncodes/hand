package manager

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
)

type Manager struct {
	store             storage.Store
	defaultIdleExpiry time.Duration
	archiveRetention  time.Duration
	now               func() time.Time
	workerOnce        sync.Once
}

var generateSessionID = storage.NewSessionID
var generateArchiveID = storage.NewArchiveID

func NewManager(store storage.Store, defaultIdleExpiry, archiveRetention time.Duration) (*Manager, error) {
	if store == nil {
		return nil, errors.New("store is required")
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

func (m *Manager) MemoryStore() (storage.MemoryStore, bool) {
	if m == nil || m.store == nil {
		return nil, false
	}

	store, ok := m.store.(storage.MemoryStore)
	if !ok {
		return nil, false
	}

	return store, true
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

	store, ok := m.store.(storage.TraceStore)
	if !ok {
		return nil, false
	}

	return store, true
}

func (m *Manager) AppendTraceEvent(ctx context.Context, event storage.TraceEvent) (storage.TraceEvent, error) {
	store, ok := m.TraceStore()
	if !ok {
		return storage.TraceEvent{}, errors.New("trace store is not supported")
	}

	return store.AppendTraceEvent(ctx, event)
}

func (m *Manager) ListTraceEvents(ctx context.Context, query storage.TraceQuery) (storage.TraceResult, error) {
	store, ok := m.TraceStore()
	if !ok {
		return storage.TraceResult{}, errors.New("trace store is not supported")
	}

	return store.ListTraceEvents(ctx, query)
}

func (m *Manager) PruneTraceEvents(ctx context.Context, sessionID string, maxEvents int) error {
	store, ok := m.TraceStore()
	if !ok {
		return errors.New("trace store is not supported")
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

	session, ok, err := m.store.Get(ctx, id)
	if err != nil {
		return storage.Session{}, err
	}

	if !ok {
		return storage.Session{}, errors.New("session not found")
	}

	return session, nil
}

func (m *Manager) runMaintenance(ctx context.Context) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	now := m.now().UTC()
	if err := m.store.DeleteExpiredArchives(ctx, now); err != nil {
		return err
	}

	return m.clearIdleDefaultSession(ctx, now)
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

	return m.store.AppendMessages(ctx, id, storage.CloneMessages(messages))
}

func (m *Manager) Save(ctx context.Context, session storage.Session) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.store.Save(ctx, session)
}

func (m *Manager) Get(ctx context.Context, id string) (storage.Session, bool, error) {
	if m == nil {
		return storage.Session{}, false, errors.New("state manager is required")
	}

	return m.store.Get(ctx, strings.TrimSpace(id))
}

func (m *Manager) GetMessages(
	ctx context.Context,
	id string,
	opts storage.MessageQueryOptions,
) ([]handmsg.Message, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	return m.store.GetMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) GetMessagesByIDs(
	ctx context.Context,
	id string,
	messageIDs []uint,
) ([]storage.MessageRecord, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	return m.store.GetMessagesByIDs(ctx, strings.TrimSpace(id), messageIDs)
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

	return m.store.GetMessageWindow(ctx, strings.TrimSpace(id), anchorMessageID, before, after)
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

	return m.store.SearchMessages(ctx, strings.TrimSpace(id), opts)
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

	return m.store.CountMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) UpdateEpisodicCheckpoint(ctx context.Context, id string, offset int) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.store.UpdateEpisodicCheckpoint(ctx, strings.TrimSpace(id), offset)
}

func (m *Manager) GetMessage(
	ctx context.Context,
	id string,
	index int,
	opts storage.MessageQueryOptions,
) (handmsg.Message, bool, error) {
	if m == nil {
		return handmsg.Message{}, false, errors.New("state manager is required")
	}

	return m.store.GetMessage(ctx, strings.TrimSpace(id), index, opts)
}

func (m *Manager) SaveSummary(ctx context.Context, summary storage.SessionSummary) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.store.SaveSummary(ctx, summary)
}

func (m *Manager) GetSummary(ctx context.Context, sessionID string) (storage.SessionSummary, bool, error) {
	if m == nil {
		return storage.SessionSummary{}, false, errors.New("state manager is required")
	}

	return m.store.GetSummary(ctx, strings.TrimSpace(sessionID))
}

func (m *Manager) DeleteSummary(ctx context.Context, sessionID string) error {
	if m == nil {
		return errors.New("state manager is required")
	}

	return m.store.DeleteSummary(ctx, strings.TrimSpace(sessionID))
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

	session, ok, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("session not found")
	}

	session.LastPromptTokens = promptTokens
	return m.store.Save(ctx, session)
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

	if _, ok, err := m.store.Get(ctx, id); err != nil {
		return storage.Session{}, err
	} else if ok {
		return storage.Session{}, errors.New("session already exists")
	}

	now := m.now().UTC()
	session := storage.Session{CreatedAt: now, ID: id, UpdatedAt: now}

	if err := m.store.Save(ctx, session); err != nil {
		return storage.Session{}, err
	}

	return session, nil
}

func (m *Manager) ListSessions(ctx context.Context) ([]storage.Session, error) {
	if m == nil {
		return nil, errors.New("state manager is required")
	}

	if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
		return nil, err
	}

	return m.store.List(ctx)
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

	return m.store.Delete(ctx, id)
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
	}

	return m.store.SetCurrent(ctx, id)
}

func (m *Manager) CurrentSession(ctx context.Context) (string, error) {
	if m == nil {
		return "", errors.New("state manager is required")
	}

	id, ok, err := m.store.Current(ctx)
	if err != nil {
		return "", err
	}

	if ok {
		return id, nil
	}

	return storage.DefaultSessionID, nil
}

func (m *Manager) resolveDefaultSession(ctx context.Context, now time.Time) (storage.Session, error) {
	session, ok, err := m.store.Get(ctx, storage.DefaultSessionID)
	if err != nil {
		return storage.Session{}, err
	}

	if !ok {
		session = storage.Session{ID: storage.DefaultSessionID, UpdatedAt: now}
		if err := m.store.Save(ctx, session); err != nil {
			return storage.Session{}, err
		}

		return session, nil
	}

	return session, nil
}

func (m *Manager) clearIdleDefaultSession(ctx context.Context, now time.Time) error {
	session, ok, err := m.store.Get(ctx, storage.DefaultSessionID)
	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	messages, err := m.store.GetMessages(ctx, session.ID, storage.MessageQueryOptions{})
	if err != nil {
		return err
	}

	if len(messages) > 0 && !session.UpdatedAt.IsZero() && !session.UpdatedAt.Add(m.defaultIdleExpiry).After(now) {
		archiveID, err := generateArchiveID()
		if err != nil {
			return err
		}

		archive := storage.ArchivedSession{
			ID:              archiveID,
			SourceSessionID: session.ID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(m.archiveRetention),
		}

		if err := m.store.CreateArchive(ctx, archive); err != nil {
			return err
		}

		if err := m.store.ClearMessages(ctx, session.ID, storage.MessageQueryOptions{}); err != nil {
			return err
		}

		session.Compaction = storage.SessionCompaction{}
		session.UpdatedAt = now

		if err := m.store.Save(ctx, session); err != nil {
			return err
		}
	}

	return nil
}
