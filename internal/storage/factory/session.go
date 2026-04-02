package factory

import (
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	handdb "github.com/wandxy/hand/internal/db"
	"github.com/wandxy/hand/internal/storage"
	storagememory "github.com/wandxy/hand/internal/storage/memory"
	storagesqlite "github.com/wandxy/hand/internal/storage/sqlite"
)

// OpenSessionStore opens a session store based on the configuration.
//
// It supports the following storage backends:
// - sqlite: uses a SQLite database
// - memory: uses a memory-based store
//
// The configuration is used to determine the storage backend to use.
//
// The function returns a SessionStore interface that can be used to store and retrieve sessions.
func OpenSessionStore(cfg *config.Config) (storage.SessionStore, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	switch strings.TrimSpace(strings.ToLower(cfg.StorageBackend)) {
	case "", "sqlite":
		db, err := handdb.Open(cfg)
		if err != nil {
			return nil, err
		}

		return storagesqlite.NewSessionStoreFromDB(db)
	case "memory":
		return storagememory.NewSessionStore(), nil
	default:
		return nil, errors.New("storage backend must be one of: memory, sqlite")
	}
}
