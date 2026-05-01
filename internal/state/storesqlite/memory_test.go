package storesqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	statememory "github.com/wandxy/hand/internal/state/core"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSQLiteMemoryStore_MigrationSearchWriteDeleteAndSourceLinks(t *testing.T) {
	db := openMemoryTestDB(t)
	store, err := NewStoreFromDB(db)
	require.NoError(t, err)
	require.True(t, db.Migrator().HasTable(&memoryItemModel{}))
	require.True(t, db.Migrator().HasTable(&memoryItemTagModel{}))
	require.True(t, db.Migrator().HasIndex(&memoryItemModel{}, "idx_memory_items_kind"))
	require.True(t, db.Migrator().HasIndex(&memoryItemModel{}, "idx_memory_items_status"))
	require.True(t, db.Migrator().HasIndex(&memoryItemModel{}, "idx_memory_items_kind_status"))
	require.True(t, db.Migrator().HasIndex(&memoryItemModel{}, "idx_memory_items_updated_at"))
	require.True(t, db.Migrator().HasIndex(&memoryItemTagModel{}, "idx_memory_item_tags_tag"))

	createdAt := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	item, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{
		ID:        "  mem_one  ",
		Kind:      statememory.MemoryKindSemantic,
		Status:    statememory.MemoryStatusActive,
		Title:     "Go preference",
		Text:      "Use focused tests",
		Tags:      []string{"Go", "Style"},
		CreatedAt: createdAt,
		Metadata:  map[string]string{"project": "hand"},
		SourceLinks: []statememory.MemorySourceLink{{
			SessionID:     "session",
			MessageIDs:    []uint{1},
			Offsets:       []int{2},
			SummaryID:     "summary",
			CreatedBy:     "reflection",
			CreatedReason: "preference",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, "mem_one", item.ID)
	require.Equal(t, createdAt, item.CreatedAt)

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text: "focused",
		Tags: []string{"go", "style"},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, 1.0, result.Hits[0].Score)
	require.Equal(t, []uint{1}, result.Hits[0].Item.SourceLinks[0].MessageIDs)
	require.Equal(t, []int{2}, result.Hits[0].Item.SourceLinks[0].Offsets)
	require.Equal(t, "summary", result.Hits[0].Item.SourceLinks[0].SummaryID)

	require.NoError(t, store.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{ID: item.ID}))

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{Text: "focused"})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text:     "focused",
		Statuses: []statememory.MemoryStatus{statememory.MemoryStatusDeleted},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, statememory.MemoryStatusDeleted, result.Hits[0].Item.Status)
}

func TestSQLiteMemoryStore_DefaultsToCandidateAndActiveOnlySearch(t *testing.T) {
	store, err := NewStoreFromDB(openMemoryTestDB(t))
	require.NoError(t, err)

	item, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{Text: "candidate"})
	require.NoError(t, err)
	require.NotEmpty(t, item.ID)
	require.Equal(t, statememory.MemoryStatusCandidate, item.Status)

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{Text: "candidate"})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text:     "candidate",
		Statuses: []statememory.MemoryStatus{statememory.MemoryStatusCandidate},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
}

func TestSQLiteMemoryStore_UpdatePreservesCreatedAtAndReplacesTags(t *testing.T) {
	store, err := NewStoreFromDB(openMemoryTestDB(t))
	require.NoError(t, err)

	createdAt := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	first, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{
		ID:        "mem_update",
		Status:    statememory.MemoryStatusActive,
		Title:     "First title",
		Text:      "old text",
		Tags:      []string{"old"},
		CreatedAt: createdAt,
	})
	require.NoError(t, err)
	require.Equal(t, createdAt, first.CreatedAt)

	second, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{
		ID:     "mem_update",
		Status: statememory.MemoryStatusActive,
		Title:  "Second title",
		Text:   "new text",
		Tags:   []string{"new"},
	})
	require.NoError(t, err)
	require.Equal(t, createdAt, second.CreatedAt)
	require.True(t, second.UpdatedAt.After(second.CreatedAt))

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text: "second",
		Tags: []string{"new"},
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)

	result, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{Tags: []string{"old"}})
	require.NoError(t, err)
	require.Empty(t, result.Hits)

	require.NoError(t, store.db.Create(&memoryItemModel{
		ID:              "mem_zero_created",
		Status:          string(statememory.MemoryStatusActive),
		TagsJSON:        "null",
		MetadataJSON:    "null",
		SourceLinksJSON: "null",
	}).Error)
	zeroCreated, err := store.UpsertMemory(context.Background(), statememory.MemoryItem{
		ID:     "mem_zero_created",
		Status: statememory.MemoryStatusActive,
		Text:   "zero created",
	})
	require.NoError(t, err)
	require.False(t, zeroCreated.CreatedAt.IsZero())
}

func TestSQLiteMemoryStore_SearchFiltersKindsStatusesTagsAndLimit(t *testing.T) {
	store, err := NewStoreFromDB(openMemoryTestDB(t))
	require.NoError(t, err)

	now := time.Date(2026, 4, 30, 8, 0, 0, 0, time.UTC)
	items := []statememory.MemoryItem{
		{
			ID:        "mem_a",
			Kind:      statememory.MemoryKindSemantic,
			Status:    statememory.MemoryStatusActive,
			Title:     "Plan preference",
			Text:      "Use phased plans",
			Tags:      []string{"plan", "go"},
			UpdatedAt: now,
		},
		{
			ID:        "mem_b",
			Kind:      statememory.MemoryKindProcedural,
			Status:    statememory.MemoryStatusActive,
			Title:     "Plan procedure",
			Text:      "Review before commit",
			Tags:      []string{"plan", "workflow"},
			UpdatedAt: now.Add(time.Minute),
		},
		{
			ID:        "mem_c",
			Kind:      statememory.MemoryKindSemantic,
			Status:    statememory.MemoryStatusSuperseded,
			Title:     "Old plan preference",
			Text:      "Superseded text",
			Tags:      []string{"plan"},
			UpdatedAt: now.Add(2 * time.Minute),
		},
	}
	for _, item := range items {
		_, err := store.UpsertMemory(context.Background(), item)
		require.NoError(t, err)
	}

	result, err := store.SearchMemory(context.Background(), statememory.MemorySearchQuery{
		Text:     "plan",
		Kinds:    []statememory.MemoryKind{statememory.MemoryKindSemantic},
		Statuses: []statememory.MemoryStatus{statememory.MemoryStatusActive, statememory.MemoryStatusSuperseded},
		Tags:     []string{"plan"},
		Limit:    1,
	})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.Equal(t, "mem_c", result.Hits[0].Item.ID)
}

func TestSQLiteMemoryStore_ValidationAndDatabaseErrors(t *testing.T) {
	var nilStore *Store
	_, err := nilStore.SearchMemory(context.Background(), statememory.MemorySearchQuery{})
	require.EqualError(t, err, "store is required")
	_, err = (&Store{}).UpsertMemory(context.Background(), statememory.MemoryItem{})
	require.EqualError(t, err, "store is required")
	require.EqualError(t, nilStore.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{}), "store is required")

	store, err := NewStoreFromDB(openMemoryTestDB(t))
	require.NoError(t, err)
	require.EqualError(t, store.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{}), "memory id is required")
	require.NoError(t, store.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{ID: "missing"}))

	searchErr := errors.New("memory search failed")
	require.NoError(t, store.db.Callback().Query().Before("gorm:query").Register("test:memory-search-error", func(tx *gorm.DB) {
		tx.AddError(searchErr)
	}))
	_, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{})
	require.ErrorIs(t, err, searchErr)
	require.NoError(t, store.db.Callback().Query().Remove("test:memory-search-error"))

	upsertErr := errors.New("memory existing lookup failed")
	require.NoError(t, store.db.Callback().Query().Before("gorm:query").Register("test:memory-upsert-query-error", func(tx *gorm.DB) {
		tx.AddError(upsertErr)
	}))
	_, err = store.UpsertMemory(context.Background(), statememory.MemoryItem{ID: "mem_error"})
	require.ErrorIs(t, err, upsertErr)
	require.NoError(t, store.db.Callback().Query().Remove("test:memory-upsert-query-error"))

	deleteErr := errors.New("memory delete failed")
	require.NoError(t, store.db.Callback().Update().Before("gorm:update").Register("test:memory-delete-error", func(tx *gorm.DB) {
		tx.AddError(deleteErr)
	}))
	err = store.DeleteMemory(context.Background(), statememory.MemoryDeleteRequest{ID: "mem_error"})
	require.ErrorIs(t, err, deleteErr)
	require.NoError(t, store.db.Callback().Update().Remove("test:memory-delete-error"))
}

func TestSQLiteMemoryStore_UpsertTransactionErrors(t *testing.T) {
	store, err := NewStoreFromDB(openMemoryTestDB(t))
	require.NoError(t, err)

	saveErr := errors.New("memory save failed")
	require.NoError(t, store.db.Callback().Create().Before("gorm:create").Register("test:memory-save-error", func(tx *gorm.DB) {
		if callbackTable(tx) == "memory_items" {
			tx.AddError(saveErr)
		}
	}))
	_, err = store.UpsertMemory(context.Background(), statememory.MemoryItem{ID: "mem_save_error"})
	require.ErrorIs(t, err, saveErr)
	require.NoError(t, store.db.Callback().Create().Remove("test:memory-save-error"))

	tagDeleteErr := errors.New("memory tag delete failed")
	require.NoError(t, store.db.Callback().Delete().Before("gorm:delete").Register("test:memory-tag-delete-error", func(tx *gorm.DB) {
		if callbackTable(tx) == "memory_item_tags" {
			tx.AddError(tagDeleteErr)
		}
	}))
	_, err = store.UpsertMemory(context.Background(), statememory.MemoryItem{ID: "mem_tag_delete", Tags: []string{"tag"}})
	require.ErrorIs(t, err, tagDeleteErr)
	require.NoError(t, store.db.Callback().Delete().Remove("test:memory-tag-delete-error"))

	tagCreateErr := errors.New("memory tag create failed")
	require.NoError(t, store.db.Callback().Create().Before("gorm:create").Register("test:memory-tag-create-error", func(tx *gorm.DB) {
		if callbackTable(tx) == "memory_item_tags" {
			tx.AddError(tagCreateErr)
		}
	}))
	_, err = store.UpsertMemory(context.Background(), statememory.MemoryItem{ID: "mem_tag_create", Tags: []string{"tag"}})
	require.ErrorIs(t, err, tagCreateErr)
	require.NoError(t, store.db.Callback().Create().Remove("test:memory-tag-create-error"))
}

func TestSQLiteMemoryStore_MalformedJSONReturnsDecodeErrors(t *testing.T) {
	store, err := NewStoreFromDB(openMemoryTestDB(t))
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, store.db.Create(&memoryItemModel{
		ID:              "mem_bad_json",
		Status:          string(statememory.MemoryStatusActive),
		TagsJSON:        "not-json",
		MetadataJSON:    "null",
		SourceLinksJSON: "null",
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	_, err = store.SearchMemory(context.Background(), statememory.MemorySearchQuery{})
	require.Error(t, err)

	_, err = memoryModelToItem(memoryItemModel{
		TagsJSON:        "[]",
		MetadataJSON:    "not-json",
		SourceLinksJSON: "null",
	})
	require.Error(t, err)

	_, err = memoryModelToItem(memoryItemModel{
		TagsJSON:        "[]",
		MetadataJSON:    "null",
		SourceLinksJSON: "not-json",
	})
	require.Error(t, err)
}

func TestSQLiteMemoryStore_StorageAndJSONHelpers(t *testing.T) {
	require.EqualError(t, ensureMemoryStorage(nil), "memory db is required")

	emptyPath := filepath.Join(t.TempDir(), "empty.db")
	require.NoError(t, os.WriteFile(emptyPath, nil, 0o600))
	readonlyEmptyDB, err := gorm.Open(sqlite.Open("file:"+emptyPath+"?mode=ro"), &gorm.Config{})
	require.NoError(t, err)
	err = ensureMemoryStorage(readonlyEmptyDB)
	require.ErrorContains(t, err, "failed to migrate memory db")

	require.Equal(t, "null", memoryJSONString(make(chan int)))

	var metadata map[string]string
	require.NoError(t, memoryDecodeJSON("", &metadata))
	require.Nil(t, metadata)
}

func openMemoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "memory.db")), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	return db
}

func callbackTable(tx *gorm.DB) string {
	if tx == nil || tx.Statement == nil {
		return ""
	}
	if tx.Statement.Table != "" {
		return tx.Statement.Table
	}
	if tx.Statement.Schema != nil {
		return tx.Statement.Schema.Table
	}
	return ""
}
