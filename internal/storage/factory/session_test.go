package factory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	storagememory "github.com/wandxy/hand/internal/storage/memory"
	storagesqlite "github.com/wandxy/hand/internal/storage/sqlite"
)

func TestOpenSessionStore_ValidatesConfigAndBackend(t *testing.T) {
	store, err := OpenSessionStore(nil)
	require.Nil(t, store)
	require.EqualError(t, err, "config is required")

	store, err = OpenSessionStore(&config.Config{StorageBackend: "bogus"})
	require.Nil(t, store)
	require.EqualError(t, err, "storage backend must be one of: memory, sqlite")
}

func TestOpenSessionStore_ReturnsMemoryStore(t *testing.T) {
	store, err := OpenSessionStore(&config.Config{StorageBackend: "memory"})

	require.NoError(t, err)
	require.IsType(t, &storagememory.SessionStore{}, store)
}

func TestOpenSessionStore_ReturnsSQLiteStore(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	store, err := OpenSessionStore(&config.Config{StorageBackend: "sqlite"})

	require.NoError(t, err)
	require.IsType(t, &storagesqlite.SessionStore{}, store)
	require.FileExists(t, datadir.StateDBPath())
	require.Equal(t, filepath.Join(homeDir, "data", "state.db"), datadir.StateDBPath())
}

func TestOpenSessionStore_DefaultsToSQLite(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	store, err := OpenSessionStore(&config.Config{})

	require.NoError(t, err)
	require.IsType(t, &storagesqlite.SessionStore{}, store)
	require.FileExists(t, datadir.StateDBPath())
	require.Equal(t, filepath.Join(homeDir, "data", "state.db"), datadir.StateDBPath())
}

func TestOpenSessionStore_ReturnsSQLiteOpenError(t *testing.T) {
	homePath := filepath.Join(t.TempDir(), "hand-home")
	require.NoError(t, os.WriteFile(homePath, []byte("not-a-directory"), 0o600))
	t.Setenv("HAND_HOME", homePath)

	store, err := OpenSessionStore(&config.Config{StorageBackend: "sqlite"})

	require.Nil(t, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create sqlite db directory")
}
