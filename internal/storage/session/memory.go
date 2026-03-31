package session

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	handctx "github.com/wandxy/hand/internal/context"
)

// MemoryStore implements the SessionStore interface using in-memory storage.
type MemoryStore struct {
	mu              sync.RWMutex
	sessions        map[string]Session
	messages        map[string][]handctx.Message
	archives        map[string]ArchivedSession
	archiveMessages map[string][]handctx.Message
	currentSession  string
}

// NewStore creates a new in-memory session store.
func NewStore() *MemoryStore {
	return &MemoryStore{
		sessions:        make(map[string]Session),
		messages:        make(map[string][]handctx.Message),
		archives:        make(map[string]ArchivedSession),
		archiveMessages: make(map[string][]handctx.Message),
	}
}

func (s *MemoryStore) Save(_ context.Context, session Session) error {
	if s == nil {
		return errors.New("session store is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[strings.TrimSpace(session.ID)]; ok && session.CreatedAt.IsZero() {
		session.CreatedAt = existing.CreatedAt
	}

	session, err := normalizeSession(session)
	if err != nil {
		return err
	}

	s.sessions[session.ID] = session

	return nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Session, bool, error) {
	if s == nil {
		return Session{}, false, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[strings.TrimSpace(id)]
	return cloneSession(session), ok, nil
}

func (s *MemoryStore) List(context.Context) ([]Session, error) {
	if s == nil {
		return nil, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, cloneSession(session))
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

func (s *MemoryStore) AppendMessages(_ context.Context, id string, messages []handctx.Message) error {
	if s == nil {
		return errors.New("session store is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}
	if len(messages) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return errors.New("session not found")
	}
	copied := cloneMessages(messages)
	s.messages[id] = append(s.messages[id], copied...)
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	return nil
}

func (s *MemoryStore) GetMessages(_ context.Context, id string, opts MessageQueryOptions) ([]handctx.Message, error) {
	if s == nil {
		return nil, errors.New("session store is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if opts.Archived {
		return cloneMessages(s.archiveMessages[id]), nil
	}
	return cloneMessages(s.messages[id]), nil
}

func (s *MemoryStore) GetMessage(_ context.Context, id string, index int, opts MessageQueryOptions) (handctx.Message, bool, error) {
	if s == nil {
		return handctx.Message{}, false, errors.New("session store is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return handctx.Message{}, false, nil
	}
	if index < 0 {
		return handctx.Message{}, false, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var messages []handctx.Message
	if opts.Archived {
		messages = s.archiveMessages[id]
	} else {
		messages = s.messages[id]
	}
	if index >= len(messages) {
		return handctx.Message{}, false, nil
	}
	return cloneMessages(messages[index : index+1])[0], true, nil
}

func (s *MemoryStore) CreateArchive(_ context.Context, archive ArchivedSession) error {
	if s == nil {
		return errors.New("session store is required")
	}

	normalized, err := normalizeCreateArchive(archive)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if normalized.SourceSessionID != "" {
		s.archiveMessages[normalized.ID] = cloneMessages(s.messages[normalized.SourceSessionID])
	}
	s.archives[normalized.ID] = normalized
	return nil
}

func (s *MemoryStore) GetArchives(_ context.Context, sourceSessionID string) ([]ArchivedSession, error) {
	if s == nil {
		return nil, errors.New("session store is required")
	}

	sourceSessionID = strings.TrimSpace(sourceSessionID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	archives := make([]ArchivedSession, 0, len(s.archives))
	for _, archive := range s.archives {
		if sourceSessionID != "" && archive.SourceSessionID != sourceSessionID {
			continue
		}
		archives = append(archives, cloneCreateArchive(archive))
	}

	sort.Slice(archives, func(i, j int) bool {
		if archives[i].ArchivedAt.Equal(archives[j].ArchivedAt) {
			return archives[i].ID < archives[j].ID
		}
		return archives[i].ArchivedAt.After(archives[j].ArchivedAt)
	})

	return archives, nil
}

func (s *MemoryStore) DeleteExpiredArchives(_ context.Context, now time.Time) error {
	if s == nil {
		return errors.New("session store is required")
	}

	now = now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, archive := range s.archives {
		if !archive.ExpiresAt.IsZero() && !archive.ExpiresAt.After(now) {
			delete(s.archives, id)
			delete(s.archiveMessages, id)
		}
	}

	return nil
}

func (s *MemoryStore) ClearMessages(_ context.Context, id string, opts MessageQueryOptions) error {
	if s == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if opts.Archived {
		if _, ok := s.archives[id]; !ok {
			return errors.New("archive not found")
		}
		delete(s.archiveMessages, id)
		return nil
	}

	session, ok := s.sessions[id]
	if !ok {
		return errors.New("session not found")
	}
	delete(s.messages, id)
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	return nil
}

func (s *MemoryStore) SetCurrent(_ context.Context, id string) error {
	if s == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return errors.New("session not found")
	}
	s.currentSession = id
	return nil
}

func (s *MemoryStore) Current(_ context.Context) (string, bool, error) {
	if s == nil {
		return "", false, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(s.currentSession) == "" {
		return "", false, nil
	}
	return s.currentSession, true, nil
}
