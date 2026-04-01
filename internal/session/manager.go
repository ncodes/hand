package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
)

type Manager struct {
	store             storage.SessionStore
	defaultIdleExpiry time.Duration
	archiveRetention  time.Duration
	now               func() time.Time
	workerOnce        sync.Once
}

func OpenStore(cfg *config.Config) (storage.SessionStore, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	backend := strings.TrimSpace(strings.ToLower(cfg.SessionBackend))
	if backend == "" {
		backend = "sqlite"
	}

	switch backend {
	case "sqlite":
		return NewSQLiteStore(datadir.SessionDBPath())
	case "memory":
		return NewStore(), nil
	default:
		return nil, errors.New("session backend must be one of: memory, sqlite")
	}
}

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

func (m *Manager) ResolveSession(ctx context.Context, requestedID string) (Session, error) {
	if m == nil {
		return Session{}, errors.New("session manager is required")
	}

	now := m.now().UTC()

	id := strings.TrimSpace(requestedID)
	if id == "" {
		id = DefaultSessionID
	}

	if id == DefaultSessionID {
		return m.resolveDefaultSession(ctx, now)
	}

	if err := validateSessionID(id); err != nil {
		return Session{}, err
	}

	session, ok, err := m.store.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}

	if !ok {
		return Session{}, errors.New("session not found")
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

	return m.store.AppendMessages(ctx, id, cloneMessages(messages))
}

func (m *Manager) GetMessages(ctx context.Context, id string, opts MessageQueryOptions) ([]handmsg.Message, error) {
	if m == nil {
		return nil, errors.New("session manager is required")
	}

	return m.store.GetMessages(ctx, strings.TrimSpace(id), opts)
}

func (m *Manager) GetMessage(ctx context.Context, id string, index int, opts MessageQueryOptions) (handmsg.Message, bool, error) {
	if m == nil {
		return handmsg.Message{}, false, errors.New("session manager is required")
	}

	return m.store.GetMessage(ctx, strings.TrimSpace(id), index, opts)
}

func (m *Manager) CreateSession(ctx context.Context, id string) (Session, error) {
	if m == nil {
		return Session{}, errors.New("session manager is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		generatedID, err := NewSessionID()
		if err != nil {
			return Session{}, err
		}
		id = generatedID
	} else if err := validateSessionID(id); err != nil {
		return Session{}, err
	}

	if _, ok, err := m.store.Get(ctx, id); err != nil {
		return Session{}, err
	} else if ok {
		return Session{}, errors.New("session already exists")
	}

	now := m.now().UTC()
	session := Session{CreatedAt: now, ID: id, UpdatedAt: now}
	if err := m.store.Save(ctx, session); err != nil {
		return Session{}, err
	}

	return session, nil
}

func (m *Manager) ListSessions(ctx context.Context) ([]Session, error) {
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
		if err := validateSessionID(id); err != nil {
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
	if id == DefaultSessionID {
		if _, err := m.resolveDefaultSession(ctx, m.now().UTC()); err != nil {
			return err
		}
	} else if err := validateSessionID(id); err != nil {
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

	return DefaultSessionID, nil
}

func (m *Manager) resolveDefaultSession(ctx context.Context, now time.Time) (Session, error) {
	session, ok, err := m.store.Get(ctx, DefaultSessionID)
	if err != nil {
		return Session{}, err
	}

	if !ok {
		session = Session{ID: DefaultSessionID, UpdatedAt: now}
		if err := m.store.Save(ctx, session); err != nil {
			return Session{}, err
		}

		return session, nil
	}

	return session, nil
}

func (m *Manager) clearIdleDefaultSession(ctx context.Context, now time.Time) error {
	session, ok, err := m.store.Get(ctx, DefaultSessionID)
	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	messages, err := m.store.GetMessages(ctx, session.ID, MessageQueryOptions{})
	if err != nil {
		return err
	}

	if len(messages) > 0 && !session.UpdatedAt.IsZero() && !session.UpdatedAt.Add(m.defaultIdleExpiry).After(now) {
		archive := ArchivedSession{
			ID:              fmt.Sprintf("%s-%s", session.ID, now.Format("20060102T150405.000000000Z")),
			SourceSessionID: session.ID,
			ArchivedAt:      now,
			ExpiresAt:       now.Add(m.archiveRetention),
		}

		if err := m.store.CreateArchive(ctx, archive); err != nil {
			return err
		}

		if err := m.store.ClearMessages(ctx, session.ID, MessageQueryOptions{}); err != nil {
			return err
		}

		session.UpdatedAt = now
		if err := m.store.Save(ctx, session); err != nil {
			return err
		}
	}

	return nil
}
