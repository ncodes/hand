package storesqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	statememory "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/pkg/nanoid"
	"gorm.io/gorm"
)

type memoryItemModel struct {
	ID              string    `gorm:"column:id;primaryKey"`
	Kind            string    `gorm:"column:kind;not null;default:'';index:idx_memory_items_kind;index:idx_memory_items_kind_status,priority:1"`
	Status          string    `gorm:"column:status;not null;index:idx_memory_items_status;index:idx_memory_items_kind_status,priority:2"`
	Title           string    `gorm:"column:title;not null;default:''"`
	Text            string    `gorm:"column:text;not null;default:''"`
	TagsJSON        string    `gorm:"column:tags_json;type:TEXT;not null;default:'null'"`
	MetadataJSON    string    `gorm:"column:metadata_json;type:TEXT;not null;default:'null'"`
	SourceLinksJSON string    `gorm:"column:source_links_json;type:TEXT;not null;default:'null'"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime:false;index:idx_memory_items_updated_at"`
}

func (memoryItemModel) TableName() string {
	return "memory_items"
}

type memoryItemTagModel struct {
	MemoryID string `gorm:"column:memory_id;primaryKey"`
	Tag      string `gorm:"column:tag;primaryKey;index:idx_memory_item_tags_tag"`
}

func (memoryItemTagModel) TableName() string {
	return "memory_item_tags"
}

func (s *Store) SearchMemory(ctx context.Context, query statememory.MemorySearchQuery) (statememory.MemorySearchResult, error) {
	if s == nil || s.db == nil {
		return statememory.MemorySearchResult{}, errors.New("store is required")
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	records, err := s.searchMemoryRecords(ctx, query, limit)
	if err != nil {
		return statememory.MemorySearchResult{}, err
	}

	hits := make([]statememory.MemorySearchHit, 0, len(records))
	for _, record := range records {
		item, err := memoryModelToItem(record)
		if err != nil {
			return statememory.MemorySearchResult{}, err
		}

		hits = append(hits, statememory.MemorySearchHit{
			Item:  item.Clone(),
			Score: statememory.SimpleMemoryScore(item, query.Text),
		})
	}

	return statememory.MemorySearchResult{Hits: hits}, nil
}

func (s *Store) UpsertMemory(ctx context.Context, item statememory.MemoryItem) (statememory.MemoryItem, error) {
	if s == nil || s.db == nil {
		return statememory.MemoryItem{}, errors.New("store is required")
	}

	item = item.Clone()
	now := time.Now().UTC()
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = nanoid.MustGenerate("mem_")
	}
	if item.Status == "" {
		item.Status = statememory.MemoryStatusCandidate
	}

	var existing memoryItemModel
	err := s.db.WithContext(ctx).
		Select("created_at").
		First(&existing, "id = ?", item.ID).
		Error
	switch {
	case err == nil:
		item.CreatedAt = existing.CreatedAt.UTC()
	case errors.Is(err, gorm.ErrRecordNotFound):
		if item.CreatedAt.IsZero() {
			item.CreatedAt = now
		} else {
			item.CreatedAt = item.CreatedAt.UTC()
		}
	default:
		return statememory.MemoryItem{}, err
	}

	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	record := itemToMemoryModel(item)

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		if err := tx.Where("memory_id = ?", item.ID).Delete(&memoryItemTagModel{}).Error; err != nil {
			return err
		}
		for _, tag := range statememory.NormalizeMemoryTags(item.Tags) {
			if err := tx.Create(&memoryItemTagModel{MemoryID: item.ID, Tag: tag}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return statememory.MemoryItem{}, err
	}

	return item.Clone(), nil
}

func (s *Store) DeleteMemory(ctx context.Context, req statememory.MemoryDeleteRequest) error {
	if s == nil || s.db == nil {
		return errors.New("store is required")
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		return errors.New("memory id is required")
	}

	result := s.db.WithContext(ctx).
		Model(&memoryItemModel{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": string(statememory.MemoryStatusDeleted), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (s *Store) searchMemoryRecords(ctx context.Context, query statememory.MemorySearchQuery, limit int) ([]memoryItemModel, error) {
	statuses := query.Statuses
	if len(statuses) == 0 {
		statuses = []statememory.MemoryStatus{statememory.MemoryStatusActive}
	}

	db := s.db.WithContext(ctx).Model(&memoryItemModel{})
	if len(query.Kinds) > 0 {
		db = db.Where("kind IN ?", statememory.MemoryKindStrings(query.Kinds))
	}
	if len(statuses) > 0 {
		db = db.Where("status IN ?", statememory.MemoryStatusStrings(statuses))
	}
	if tags := statememory.NormalizeMemoryTags(query.Tags); len(tags) > 0 {
		subquery := s.db.WithContext(ctx).
			Model(&memoryItemTagModel{}).
			Select("memory_id").
			Where("tag IN ?", tags).
			Group("memory_id").
			Having("COUNT(DISTINCT tag) = ?", len(tags))
		db = db.Where("id IN (?)", subquery)
	}

	text := strings.TrimSpace(strings.ToLower(query.Text))
	if text != "" {
		pattern := statememory.MemoryLikePattern(text)
		matchSQL := `LOWER(title) LIKE ? ESCAPE '\' OR LOWER(text) LIKE ? ESCAPE '\'`
		scoreSQL := `((CASE WHEN LOWER(title) LIKE ? ESCAPE '\' THEN 2 ELSE 0 END) + (CASE WHEN LOWER(text) LIKE ? ESCAPE '\' THEN 1 ELSE 0 END)) DESC`
		db = db.Where(matchSQL, pattern, pattern).
			Order(gorm.Expr(scoreSQL, pattern, pattern))
	}

	var records []memoryItemModel
	if err := db.Order("updated_at DESC").Order("id ASC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func ensureMemoryStorage(db *gorm.DB) error {
	if db == nil {
		return errors.New("memory db is required")
	}

	if err := db.AutoMigrate(&memoryItemModel{}, &memoryItemTagModel{}); err != nil {
		return fmt.Errorf("failed to migrate memory db: %w", err)
	}

	return nil
}

func itemToMemoryModel(item statememory.MemoryItem) memoryItemModel {
	return memoryItemModel{
		ID:              item.ID,
		Kind:            string(item.Kind),
		Status:          string(item.Status),
		Title:           item.Title,
		Text:            item.Text,
		TagsJSON:        memoryJSONString(item.Tags),
		MetadataJSON:    memoryJSONString(item.Metadata),
		SourceLinksJSON: memoryJSONString(item.SourceLinks),
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
}

func memoryModelToItem(record memoryItemModel) (statememory.MemoryItem, error) {
	item := statememory.MemoryItem{
		ID:        record.ID,
		Kind:      statememory.MemoryKind(record.Kind),
		Status:    statememory.MemoryStatus(record.Status),
		Title:     record.Title,
		Text:      record.Text,
		CreatedAt: record.CreatedAt.UTC(),
		UpdatedAt: record.UpdatedAt.UTC(),
	}
	if err := memoryDecodeJSON(record.TagsJSON, &item.Tags); err != nil {
		return statememory.MemoryItem{}, err
	}
	if err := memoryDecodeJSON(record.MetadataJSON, &item.Metadata); err != nil {
		return statememory.MemoryItem{}, err
	}
	if err := memoryDecodeJSON(record.SourceLinksJSON, &item.SourceLinks); err != nil {
		return statememory.MemoryItem{}, err
	}
	return item, nil
}

func memoryJSONString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
}

func memoryDecodeJSON(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "null"
	}
	return json.Unmarshal([]byte(raw), target)
}
