package storesqlite

import (
	"context"
	"errors"
	"strings"
	"time"
)

const sqliteWriteAttempts = 3
const sqliteWriteRetryBudget = 5 * time.Second

func runSQLiteWriteWithRetry(ctx context.Context, write func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	retryCtx, cancel := context.WithTimeout(ctx, sqliteWriteRetryBudget)
	defer cancel()

	var err error
	for attempt := range sqliteWriteAttempts {
		if err = write(retryCtx); err == nil || !isSQLiteLockError(err) {
			return err
		}
		if attempt == sqliteWriteAttempts-1 {
			break
		}

		delay := time.Duration(20*(1<<attempt))*time.Millisecond +
			time.Duration(time.Now().UnixNano()%int64(10*time.Millisecond))
		timer := time.NewTimer(delay)
		select {
		case <-retryCtx.Done():
			timer.Stop()
			return errors.Join(retryCtx.Err(), err)
		case <-timer.C:
		}
	}

	return err
}

func isSQLiteLockError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked")
}
