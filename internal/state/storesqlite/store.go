package storesqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	handdb "github.com/wandxy/hand/internal/db"
	base "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store persists sessions, messages, memory, and traces in SQLite.
type Store struct {
	vectors        *vectorConfig
	memoryReranker search.Reranker
	db             *gorm.DB
}

// NewStore returns a store backed by the supplied dependencies.
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

// NewStoreFromDB returns a store using an existing database handle.
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

func (s *Store) Session() base.SessionStore {
	return s
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

func gormOpenSQLite(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("session sqlite path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create session db directory: %w", err)
	}

	db, err := handdb.OpenSQLite(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open session db: %w", err)
	}

	return NewStoreFromDB(db)
}
