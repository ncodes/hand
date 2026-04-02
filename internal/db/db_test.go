package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
)

func TestOpen_ValidatesConfigAndBackend(t *testing.T) {
	db, err := Open(nil)
	require.Nil(t, db)
	require.EqualError(t, err, "config is required")
}

func TestOpenSQLite_ValidatesPath(t *testing.T) {
	db, err := OpenSQLite("")

	require.Nil(t, db)
	require.EqualError(t, err, "sqlite path is required")
}

func TestOpen_OpensSQLiteAtStateDBPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HAND_HOME", homeDir)

	db, err := Open(&config.Config{StorageBackend: "sqlite"})
	require.NoError(t, err)
	require.NotNil(t, db)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	require.FileExists(t, datadir.StateDBPath())
	require.Equal(t, filepath.Join(homeDir, "data", "state.db"), datadir.StateDBPath())
}
