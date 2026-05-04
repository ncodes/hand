package storesqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/state/search"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	vectors        *vectorConfig
	memoryReranker search.Reranker
	db             *gorm.DB
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("session sqlite path is required")
	}

	backend, err := gormOpenSQLite(path)
	if err != nil {
		return nil, err
	}

	return backend, nil
}

func NewStoreFromDB(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("session db is required")
	}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})

	if err := db.AutoMigrate(
		&sessionModel{},
		&archiveModel{},
		&stateModel{},
		&summaryModel{},
		&messageModel{},
		&archivedMessageModel{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate session db: %w", err)
	}

	if err := ensureMemoryStorage(db); err != nil {
		return nil, err
	}

	if err := ensureSearchIndex(db); err != nil {
		return nil, err
	}

	if err := ensureTraceStorage(db); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

func gormOpenSQLite(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("session sqlite path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create session db directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open session db: %w", err)
	}

	return NewStoreFromDB(db)
}
