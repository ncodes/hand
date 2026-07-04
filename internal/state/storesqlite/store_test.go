package storesqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSQLiteStore_NewStoreValidationAndSchema(t *testing.T) {
	_, err := NewStore("")
	require.EqualError(t, err, "session sqlite path is required")

	blockerPath := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blockerPath, []byte("x"), 0o600))

	_, err = NewStore(filepath.Join(blockerPath, "session.db"))
	require.ErrorContains(t, err, "failed to create session db directory")

	_, err = NewStore(t.TempDir())
	require.ErrorContains(t, err, "failed to open session db")

	store, err := NewStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.Equal(t, "sessions", sessionModel{}.TableName())
	require.Equal(t, "session_state", stateModel{}.TableName())
	require.Equal(t, "session_summaries", summaryModel{}.TableName())
	require.Equal(t, "session_messages", messageModel{}.TableName())
	require.True(t, store.db.Migrator().HasTable(&sessionModel{}))
	require.True(t, store.db.Migrator().HasTable(&stateModel{}))
	require.True(t, store.db.Migrator().HasTable(&summaryModel{}))
	require.True(t, store.db.Migrator().HasTable(&messageModel{}))
	require.True(t, store.db.Migrator().HasTable(&automationJobModel{}))
	require.True(t, store.db.Migrator().HasTable(&automationRunModel{}))
	require.True(t, store.db.Migrator().HasTable(&memoryItemModel{}))
	require.True(t, store.db.Migrator().HasTable(&memoryItemTagModel{}))
	require.True(t, store.db.Migrator().HasTable(&traceEventModel{}))
	require.True(t, store.db.Migrator().HasTable(&gatewayPairingRequestModel{}))
	require.True(t, store.db.Migrator().HasTable(&gatewayPairedSenderModel{}))
	require.True(t, store.db.Migrator().HasColumn(&sessionModel{}, "episodic_checkpoint_offset"))
	require.True(t, store.db.Migrator().HasColumn(&sessionModel{}, "reflection_checkpoint_offset"))
	require.True(t, store.db.Migrator().HasColumn(&sessionModel{}, "title"))
	require.True(t, store.db.Migrator().HasColumn(&sessionModel{}, "title_source"))
	require.False(t, store.db.Migrator().HasColumn(&sessionModel{}, "messages"))
}

func TestSQLiteStore_AggregateCapabilities(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)

	sessionStore := store.Session()
	require.Same(t, store, sessionStore)

	automationStore, ok := store.Automation()
	require.True(t, ok)
	require.Same(t, store, automationStore)

	memoryStore, ok := store.Memory()
	require.True(t, ok)
	require.Same(t, store, memoryStore)

	traceStore, ok := store.Trace()
	require.True(t, ok)
	require.Same(t, store, traceStore)
	require.False(t, store.SupportsVectorSearch())

	vectorStore, _ := sqliteVectorStoreTestStore(t)
	require.True(t, vectorStore.SupportsVectorSearch())

	var nilStore *Store
	require.Nil(t, nilStore.Session())
	automationStore, ok = nilStore.Automation()
	require.False(t, ok)
	require.Nil(t, automationStore)

	memoryStore, ok = nilStore.Memory()
	require.False(t, ok)
	require.Nil(t, memoryStore)

	traceStore, ok = nilStore.Trace()
	require.False(t, ok)
	require.Nil(t, traceStore)
	require.False(t, nilStore.SupportsVectorSearch())
}

func TestSQLiteStore_MigrationFailsOnReadOnlyDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.db")
	store, err := NewStore(path)
	require.NoError(t, err)
	require.NoError(t, store.db.Migrator().DropTable(&stateModel{}))
	sqlDB, err := store.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	_, err = NewStore("file:session.db?mode=ro")
	require.ErrorContains(t, err, "failed to migrate session db")
}

func TestSQLiteStore_ConstructorsValidateInputs(t *testing.T) {
	_, err := NewStoreFromDB(nil)
	require.EqualError(t, err, "session db is required")

	_, err = gormOpenSQLite("")
	require.EqualError(t, err, "session sqlite path is required")
}

func TestSQLiteStore_Close(t *testing.T) {
	var nilStore *Store
	require.NoError(t, nilStore.Close())
	require.NoError(t, (&Store{}).Close())

	store, err := NewStore(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, store.Close())

	err = (&Store{db: &gorm.DB{Config: &gorm.Config{}}}).Close()
	require.ErrorIs(t, err, gorm.ErrInvalidDB)
}

func TestSQLiteStore_StorageInitializationErrors(t *testing.T) {
	t.Run("memory storage", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "session.db")

		writableDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, writableDB.AutoMigrate(
			&sessionModel{},
			&stateModel{},
			&summaryModel{},
			&messageModel{},
			&automationJobModel{},
			&automationRunModel{},
			&gatewayBindingModel{},
			&traceEventModel{},
			&gatewayPairingRequestModel{},
			&gatewayPairedSenderModel{},
		))

		readonlyDB, err := gorm.Open(sqlite.Open("file:"+path+"?mode=ro"), &gorm.Config{})
		require.NoError(t, err)

		_, err = NewStoreFromDB(readonlyDB)
		require.ErrorContains(t, err, "failed to migrate memory db")
	})

	t.Run("session search index", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "session.db")

		writableDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, writableDB.AutoMigrate(
			&sessionModel{},
			&stateModel{},
			&summaryModel{},
			&messageModel{},
			&automationJobModel{},
			&automationRunModel{},
			&gatewayBindingModel{},
			&traceEventModel{},
			&gatewayPairingRequestModel{},
			&gatewayPairedSenderModel{},
		))
		require.NoError(t, ensureMemoryStorage(writableDB))

		readonlyDB, err := gorm.Open(sqlite.Open("file:"+path+"?mode=ro"), &gorm.Config{})
		require.NoError(t, err)

		_, err = NewStoreFromDB(readonlyDB)
		require.ErrorContains(t, err, "failed to create session message search index")
	})
}
