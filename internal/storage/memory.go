package storage

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryEntry struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

type MemoryStore interface {
	Save(context.Context, MemoryEntry) error
	Get(context.Context, string) (MemoryEntry, bool, error)
	List(context.Context) ([]MemoryEntry, error)
}

type InMemoryMemoryStore struct {
	mu       sync.RWMutex
	memories map[string]MemoryEntry
}

func NewMemoryStore() *InMemoryMemoryStore {
	return &InMemoryMemoryStore{
		memories: make(map[string]MemoryEntry),
	}
}

func (s *InMemoryMemoryStore) Upsert(_ context.Context, entry MemoryEntry) error {
	if s == nil {
		return errors.New("memory store is required")
	}

	entry.Key = strings.TrimSpace(entry.Key)
	if entry.Key == "" {
		return errors.New("memory key is required")
	}

	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.memories[entry.Key] = entry
	return nil
}

func (s *InMemoryMemoryStore) Get(_ context.Context, key string) (MemoryEntry, bool, error) {
	if s == nil {
		return MemoryEntry{}, false, errors.New("memory store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.memories[strings.TrimSpace(key)]
	return entry, ok, nil
}

func (s *InMemoryMemoryStore) List(context.Context) ([]MemoryEntry, error) {
	if s == nil {
		return nil, errors.New("memory store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	memories := make([]MemoryEntry, 0, len(s.memories))
	for _, entry := range s.memories {
		memories = append(memories, entry)
	}

	sort.Slice(memories, func(i, j int) bool {
		if memories[i].UpdatedAt.Equal(memories[j].UpdatedAt) {
			return memories[i].Key < memories[j].Key
		}

		return memories[i].UpdatedAt.After(memories[j].UpdatedAt)
	})

	return memories, nil
}
