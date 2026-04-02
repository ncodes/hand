package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	return OpenSQLite(datadir.StateDBPath())
}

func OpenSQLite(path string) (*gorm.DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create sqlite db directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	return db, nil
}
