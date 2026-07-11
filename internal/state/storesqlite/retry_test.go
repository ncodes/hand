package storesqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunSQLiteWriteWithRetry_RetriesLockErrors(t *testing.T) {
	attempts := 0
	err := runSQLiteWriteWithRetry(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked")
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestRunSQLiteWriteWithRetry_DoesNotRetryOtherErrors(t *testing.T) {
	wantErr := errors.New("write failed")
	attempts := 0
	err := runSQLiteWriteWithRetry(context.Background(), func(context.Context) error {
		attempts++
		return wantErr
	})

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, 1, attempts)
}

func TestRunSQLiteWriteWithRetry_StopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	err := runSQLiteWriteWithRetry(ctx, func(context.Context) error {
		attempts++
		cancel()
		return errors.New("database table is locked")
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}

func TestRunSQLiteWriteWithRetry_ReturnsFinalLockError(t *testing.T) {
	wantErr := errors.New("database is locked")
	attempts := 0
	err := runSQLiteWriteWithRetry(context.Background(), func(context.Context) error {
		attempts++
		return wantErr
	})

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, sqliteWriteAttempts, attempts)
}

func TestIsSQLiteLockError_ClassifiesSupportedErrors(t *testing.T) {
	require.False(t, isSQLiteLockError(nil))
	require.False(t, isSQLiteLockError(errors.New("write failed")))
	require.True(t, isSQLiteLockError(errors.New("database is locked")))
	require.True(t, isSQLiteLockError(errors.New("database table is locked")))
	require.True(t, isSQLiteLockError(errors.New("SQLITE_BUSY")))
	require.True(t, isSQLiteLockError(errors.New("SQLITE_LOCKED")))
}

func TestRunSQLiteWriteWithRetry_ProvidesBoundedContext(t *testing.T) {
	var remaining time.Duration
	err := runSQLiteWriteWithRetry(nil, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		remaining = time.Until(deadline)
		return nil
	})

	require.NoError(t, err)
	require.Positive(t, remaining)
	require.LessOrEqual(t, remaining, sqliteWriteRetryBudget)
}
