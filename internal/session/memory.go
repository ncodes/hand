package session

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
)

// MemoryStore implements the SessionStore interface using in-memory storage.
type MemoryStore struct {
	mu              sync.RWMutex
	sessions        map[string]Session
	messages        map[string][]handmsg.Message
	archives        map[string]ArchivedSession
	archiveMessages map[string][]handmsg.Message
	currentSession  string
}

// NewStore creates a new in-memory session store.
func NewStore() *MemoryStore {
	return &MemoryStore{
		sessions:        make(map[string]Session),
		messages:        make(map[string][]handmsg.Message),
		archives:        make(map[string]ArchivedSession),
		archiveMessages: make(map[string][]handmsg.Message),
	}
}

func (s *MemoryStore) Save(_ context.Context, session Session) error {
	if s == nil {
		return errors.New("session store is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session.ID = strings.TrimSpace(session.ID)
	if err := validateSessionID(session.ID); err != nil {
		return err
	}

	if existing, ok := s.sessions[session.ID]; ok {
		session.CreatedAt = existing.CreatedAt
		session.UpdatedAt = time.Now().UTC()
	}

	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	} else {
		session.CreatedAt = session.CreatedAt.UTC()
	}

	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	} else {
		session.UpdatedAt = session.UpdatedAt.UTC()
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

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	if s == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := validateSessionID(id); err != nil {
		return err
	}

	if id == DefaultSessionID {
		return errors.New("default session cannot be deleted")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return errors.New("session not found")
	}

	delete(s.sessions, id)
	delete(s.messages, id)
	if s.currentSession == id {
		s.currentSession = ""
	}

	return nil
}

func (s *MemoryStore) AppendMessages(_ context.Context, id string, messages []handmsg.Message) error {
	if s == nil {
		return errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if err := validateSessionID(id); err != nil {
		return err
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

func (s *MemoryStore) GetMessages(_ context.Context, id string, opts MessageQueryOptions) ([]handmsg.Message, error) {
	if s == nil {
		return nil, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}

	if !opts.Archived {
		if err := validateSessionID(id); err != nil {
			return nil, err
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if opts.Archived {
		return cloneMessages(s.archiveMessages[id]), nil
	}

	return cloneMessages(s.messages[id]), nil
}

func (s *MemoryStore) GetMessage(_ context.Context, id string, index int, opts MessageQueryOptions) (handmsg.Message, bool, error) {
	if s == nil {
		return handmsg.Message{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return handmsg.Message{}, false, nil
	}

	if index < 0 {
		return handmsg.Message{}, false, nil
	}

	if !opts.Archived {
		if err := validateSessionID(id); err != nil {
			return handmsg.Message{}, false, err
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var messages []handmsg.Message
	if opts.Archived {
		messages = s.archiveMessages[id]
	} else {
		messages = s.messages[id]
	}

	if index >= len(messages) {
		return handmsg.Message{}, false, nil
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

	sourceMessages := s.messages[normalized.SourceSessionID]
	if len(sourceMessages) == 0 {
		return errors.New("source session has no messages")
	}

	s.archiveMessages[normalized.ID] = cloneMessages(sourceMessages)
	s.archives[normalized.ID] = normalized

	delete(s.messages, normalized.SourceSessionID)
	if normalized.SourceSessionID != DefaultSessionID {
		delete(s.sessions, normalized.SourceSessionID)
		if s.currentSession == normalized.SourceSessionID {
			s.currentSession = ""
		}
	}

	return nil
}

func (s *MemoryStore) GetArchive(_ context.Context, id string) (ArchivedSession, bool, error) {
	if s == nil {
		return ArchivedSession{}, false, errors.New("session store is required")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ArchivedSession{}, false, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	archive, ok := s.archives[id]
	return cloneCreateArchive(archive), ok, nil
}

func (s *MemoryStore) ListArchives(_ context.Context, sourceSessionID string) ([]ArchivedSession, error) {
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

func (s *MemoryStore) DeleteArchives(_ context.Context, archiveID string) error {
	if s == nil {
		return errors.New("session store is required")
	}

	archiveID = strings.TrimSpace(archiveID)
	if archiveID == "" {
		return errors.New("archive id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.archives[archiveID]; !ok {
		return errors.New("archive not found")
	}

	delete(s.archives, archiveID)
	delete(s.archiveMessages, archiveID)

	return nil
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
	if !opts.Archived {
		if err := validateSessionID(id); err != nil {
			return err
		}
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
	if err := validateSessionID(id); err != nil {
		return err
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
