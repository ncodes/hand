package storage

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type Message struct {
	Role      string
	Content   string
	CreatedAt time.Time
}

type Session struct {
	ID        string
	Messages  []Message
	UpdatedAt time.Time
}

type SessionStore interface {
	Save(context.Context, Session) error
	Get(context.Context, string) (Session, bool, error)
	List(context.Context) ([]Session, error)
}

type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewSessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]Session),
	}
}

func (s *InMemorySessionStore) Save(_ context.Context, session Session) error {
	if s == nil {
		return errors.New("session store is required")
	}

	session.ID = strings.TrimSpace(session.ID)
	if session.ID == "" {
		return errors.New("session id is required")
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *InMemorySessionStore) Get(_ context.Context, id string) (Session, bool, error) {
	if s == nil {
		return Session{}, false, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[strings.TrimSpace(id)]
	return session, ok, nil
}

func (s *InMemorySessionStore) List(context.Context) ([]Session, error) {
	if s == nil {
		return nil, errors.New("session store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}
