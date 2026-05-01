package storememory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	statememory "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/pkg/nanoid"
)

func (s *Store) SearchMemory(_ context.Context, query statememory.MemorySearchQuery) (statememory.MemorySearchResult, error) {
	if s == nil {
		return statememory.MemorySearchResult{}, errors.New("store is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	hits := make([]statememory.MemorySearchHit, 0, len(s.memoryItems))
	for _, item := range s.memoryItems {
		if !statememory.MemoryMatchesQuery(item, query) {
			continue
		}

		hits = append(hits, statememory.MemorySearchHit{
			Item:  item.Clone(),
			Score: statememory.SimpleMemoryScore(item, query.Text),
		})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if !hits[i].Item.UpdatedAt.Equal(hits[j].Item.UpdatedAt) {
			return hits[i].Item.UpdatedAt.After(hits[j].Item.UpdatedAt)
		}
		return hits[i].Item.ID < hits[j].Item.ID
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}

	return statememory.MemorySearchResult{Hits: hits}, nil
}

func (s *Store) UpsertMemory(_ context.Context, item statememory.MemoryItem) (statememory.MemoryItem, error) {
	if s == nil {
		return statememory.MemoryItem{}, errors.New("store is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.memoryItems == nil {
		s.memoryItems = make(map[string]statememory.MemoryItem)
	}

	now := time.Now().UTC()
	item = item.Clone()
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = nanoid.MustGenerate("mem_")
	}
	if item.Status == "" {
		item.Status = statememory.MemoryStatusCandidate
	}
	if existing, ok := s.memoryItems[item.ID]; ok {
		item.CreatedAt = existing.CreatedAt
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	} else {
		item.CreatedAt = item.CreatedAt.UTC()
	}
	item.UpdatedAt = now

	s.memoryItems[item.ID] = item.Clone()
	return item.Clone(), nil
}

func (s *Store) DeleteMemory(_ context.Context, req statememory.MemoryDeleteRequest) error {
	if s == nil {
		return errors.New("store is required")
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		return errors.New("memory id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.memoryItems[id]
	if !ok {
		return nil
	}
	item.Status = statememory.MemoryStatusDeleted
	item.UpdatedAt = time.Now().UTC()
	s.memoryItems[id] = item
	return nil
}
