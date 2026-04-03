package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
	common "github.com/wandxy/hand/internal/storage/common"
)

type Manager struct {
	store             storage.SessionStore
	defaultIdleExpiry time.Duration
	archiveRetention  time.Duration
	now               func() time.Time
	workerOnce        sync.Once
}

var generateSessionID = storage.NewSessionID
var generateArchiveID = storage.NewArchiveID

func NewManager(store storage.SessionStore, defaultIdleExpiry, archiveRetention time.Duration) (*Manager, error) {
	if store == nil {
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

func (m *Manager) Resolve(ctx context.Context, id string) (storage.Session, error) {
	if m == nil {
		return storage.Session{}, errors.New("session manager is required")
	}

	now := m.now().UTC()

	id = strings.TrimSpace(id)
	if id == "" {
		id = storage.DefaultSessionID
	}

	if id == storage.DefaultSessionID {
		return m.resolveDefaultSession(ctx, now)
	}

	if err := common.ValidateSessionID(id); err != nil {
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
		return errors.New("session manager is required")
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
		return errors.New("session manager is required")
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
		return errors.New("session manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}

	return m.store.AppendMessages(ctx, id, common.CloneMessages(messages))
}

func (m *Manager) Save(ctx context.Context, session storage.Session) error {
	if m == nil {
		return errors.New("session manager is required")
	}

	return m.store.Save(ctx, session)
}

func (m *Manager) Get(ctx context.Context, id string) (storage.Session, bool, error) {
	if m == nil {
		return storage.Session{}, false, errors.New("session manager is required")
	}

	return m.store.Get(ctx, strings.TrimSpace(id))
}

func (m *Manager) GetMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
	if m == nil {
		return nil, errors.New("session manager is required")
	}

	return m.store.GetMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) CountMessages(ctx context.Context, id string, opts storage.MessageQueryOptions) (int, error) {
	if m == nil {
		return 0, errors.New("session manager is required")
	}

	return m.store.CountMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) GetMessage(ctx context.Context, id string, index int, opts storage.MessageQueryOptions) (handmsg.Message, bool, error) {
	if m == nil {
		return handmsg.Message{}, false, errors.New("session manager is required")
	}

	return m.store.GetMessage(ctx, strings.TrimSpace(id), index, opts)
}

func (m *Manager) SaveSummary(ctx context.Context, summary storage.SessionSummary) error {
	if m == nil {
		return errors.New("session manager is required")
	}

	return m.store.SaveSummary(ctx, summary)
}

func (m *Manager) GetSummary(ctx context.Context, sessionID string) (storage.SessionSummary, bool, error) {
	if m == nil {
		return storage.SessionSummary{}, false, errors.New("session manager is required")
	}

	return m.store.GetSummary(ctx, strings.TrimSpace(sessionID))
}

func (m *Manager) DeleteSummary(ctx context.Context, sessionID string) error {
	if m == nil {
		return errors.New("session manager is required")
	}

	return m.store.DeleteSummary(ctx, strings.TrimSpace(sessionID))
}

func (m *Manager) UpdateLastPromptTokens(ctx context.Context, id string, promptTokens int) error {
	if m == nil {
		return errors.New("session manager is required")
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
		return storage.Session{}, errors.New("session manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		generatedID, err := generateSessionID()
		if err != nil {
			return storage.Session{}, err
		}
		id = generatedID
	} else if err := common.ValidateSessionID(id); err != nil {
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
		return nil, errors.New("session manager is required")
	}

	if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
		return nil, err
	}

	return m.store.List(ctx)
}

func (m *Manager) DeleteSession(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("session manager is required")
	}

	id = strings.TrimSpace(id)
	if id != "" {
		if err := common.ValidateSessionID(id); err != nil {
			return err
		}
	}

	return m.store.Delete(ctx, id)
}

func (m *Manager) UseSession(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("session manager is required")
	}

	id = strings.TrimSpace(id)
	if id == storage.DefaultSessionID {
		if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
			return err
		}
	} else if err := common.ValidateSessionID(id); err != nil {
		return err
	}

	return m.store.SetCurrent(ctx, id)
}

func (m *Manager) CurrentSession(ctx context.Context) (string, error) {
	if m == nil {
		return "", errors.New("session manager is required")
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
