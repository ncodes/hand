package dispatch

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDispatcher_EnqueueRunsJobAndReportsCompletion(t *testing.T) {
	ctx := t.Context()
	dispatcher := New(Options{Capacity: 2, Workers: 1})
	dispatcher.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, dispatcher.Shutdown(context.Background()))
	})
	done := make(chan struct{})

	enqueued, err := dispatcher.Enqueue(Job{
		ID: "job-1",
		Run: func(context.Context) error {
			close(done)
			return nil
		},
	})

	require.NoError(t, err)
	require.True(t, enqueued)
	require.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	status := dispatcher.Status()
	require.Equal(t, uint64(1), status.Completed)
	require.Equal(t, 0, status.InFlight)
	require.False(t, status.Degraded)
}

func TestDispatcher_NilReceiverIsSafe(t *testing.T) {
	var dispatcher *Dispatcher
	dispatcher.Start(context.Background())

	enqueued, err := dispatcher.Enqueue(Job{ID: "nil", Run: func(context.Context) error { return nil }})

	require.False(t, enqueued)
	require.ErrorIs(t, err, ErrDispatcherClosed)
	require.NoError(t, dispatcher.Shutdown(context.Background()))
	dispatcher.Close()
	require.Equal(t, Status{}, dispatcher.Status())
}

func TestDispatcher_RejectsInvalidJobs(t *testing.T) {
	dispatcher := New(Options{})

	enqueued, err := dispatcher.Enqueue(Job{ID: " "})

	require.False(t, enqueued)
	require.ErrorIs(t, err, ErrJobIDRequired)

	enqueued, err = dispatcher.Enqueue(Job{ID: "job"})

	require.False(t, enqueued)
	require.ErrorIs(t, err, ErrJobRunRequired)
}

func TestDispatcher_DeduplicatesQueuedJobs(t *testing.T) {
	dispatcher := New(Options{Capacity: 2})
	var calls atomic.Int64

	enqueued, err := dispatcher.Enqueue(Job{
		ID: "same",
		Run: func(context.Context) error {
			calls.Add(1)
			return nil
		},
	})
	require.NoError(t, err)
	require.True(t, enqueued)

	enqueued, err = dispatcher.Enqueue(Job{
		ID: "same",
		Run: func(context.Context) error {
			calls.Add(1)
			return nil
		},
	})

	require.NoError(t, err)
	require.False(t, enqueued)
	require.Equal(t, uint64(1), dispatcher.Status().Duplicates)
	require.Equal(t, int64(0), calls.Load())
}

func TestDispatcher_EvictsOldIdempotencyKeysAtLimit(t *testing.T) {
	dispatcher := New(Options{Capacity: 3, IdempotencyLimit: 1})
	_, err := dispatcher.Enqueue(Job{ID: "first", Run: func(context.Context) error { return nil }})
	require.NoError(t, err)
	_, err = dispatcher.Enqueue(Job{ID: "second", Run: func(context.Context) error { return nil }})
	require.NoError(t, err)

	enqueued, err := dispatcher.Enqueue(Job{ID: "first", Run: func(context.Context) error { return nil }})

	require.NoError(t, err)
	require.True(t, enqueued)
	require.Equal(t, uint64(0), dispatcher.Status().Duplicates)
}

func TestDispatcher_ReturnsQueueFullAndReportsDegradedStatus(t *testing.T) {
	dispatcher := New(Options{Capacity: 1})
	_, err := dispatcher.Enqueue(Job{ID: "first", Run: func(context.Context) error { return nil }})
	require.NoError(t, err)

	enqueued, err := dispatcher.Enqueue(Job{ID: "second", Run: func(context.Context) error { return nil }})

	require.False(t, enqueued)
	require.ErrorIs(t, err, ErrQueueFull)
	status := dispatcher.Status()
	require.Equal(t, uint64(1), status.Dropped)
	require.True(t, status.Degraded)
	require.Equal(t, ErrQueueFull.Error(), status.LastError)
}

func TestDispatcher_RetriesFailedJobsWithBackoff(t *testing.T) {
	ctx := t.Context()
	dispatcher := New(Options{
		Capacity:       1,
		Workers:        1,
		MaxAttempts:    3,
		RetryBaseDelay: time.Nanosecond,
		RetryMaxDelay:  time.Nanosecond,
	})
	dispatcher.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, dispatcher.Shutdown(context.Background()))
	})
	var calls atomic.Int64

	_, err := dispatcher.Enqueue(Job{
		ID: "retry",
		Run: func(context.Context) error {
			if calls.Add(1) < 3 {
				return errors.New("temporary")
			}

			return nil
		},
	})

	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return calls.Load() == 3 && dispatcher.Status().Completed == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, uint64(0), dispatcher.Status().Failed)
}

func TestDispatcher_UsesJobAttemptOverride(t *testing.T) {
	ctx := t.Context()
	dispatcher := New(Options{
		Capacity:       1,
		Workers:        1,
		MaxAttempts:    3,
		RetryBaseDelay: time.Nanosecond,
		RetryMaxDelay:  time.Nanosecond,
	})
	dispatcher.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, dispatcher.Shutdown(context.Background()))
	})
	var calls atomic.Int64

	_, err := dispatcher.Enqueue(Job{
		ID:          "single-attempt",
		MaxAttempts: 1,
		Run: func(context.Context) error {
			calls.Add(1)
			return errors.New("permanent")
		},
	})

	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return dispatcher.Status().Failed == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int64(1), calls.Load())
}

func TestDispatcher_AppliesJobTimeout(t *testing.T) {
	ctx := t.Context()
	dispatcher := New(Options{Capacity: 1, Workers: 1})
	dispatcher.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, dispatcher.Shutdown(context.Background()))
	})
	errCh := make(chan error, 1)

	_, err := dispatcher.Enqueue(Job{
		ID:          "timeout",
		MaxAttempts: 1,
		Timeout:     time.Nanosecond,
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			errCh <- ctx.Err()
			return ctx.Err()
		},
	})

	require.NoError(t, err)
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("job did not observe timeout")
	}
	require.Eventually(t, func() bool {
		return dispatcher.Status().Failed == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, context.DeadlineExceeded.Error(), dispatcher.Status().LastError)
}

func TestDispatcher_UsesDefaultTimeout(t *testing.T) {
	ctx := t.Context()
	dispatcher := New(Options{Capacity: 1, Workers: 1, Timeout: time.Nanosecond})
	dispatcher.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, dispatcher.Shutdown(context.Background()))
	})

	_, err := dispatcher.Enqueue(Job{
		ID:          "default-timeout",
		MaxAttempts: 1,
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return dispatcher.Status().Failed == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, context.DeadlineExceeded.Error(), dispatcher.Status().LastError)
}

func TestDispatcher_JobTimeoutOverridesDefaultTimeout(t *testing.T) {
	ctx := t.Context()
	dispatcher := New(Options{Capacity: 1, Workers: 1, Timeout: time.Hour})
	dispatcher.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, dispatcher.Shutdown(context.Background()))
	})

	_, err := dispatcher.Enqueue(Job{
		ID:          "override-timeout",
		MaxAttempts: 1,
		Timeout:     time.Nanosecond,
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return dispatcher.Status().Failed == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, context.DeadlineExceeded.Error(), dispatcher.Status().LastError)
}

func TestDispatcher_RunWithRetryReturnsContextErrorBeforeRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dispatcher := New(Options{})

	err := dispatcher.runWithRetry(ctx, queuedJob{
		id:  "canceled",
		run: func(context.Context) error { return nil },
	})

	require.ErrorIs(t, err, context.Canceled)
}

func TestDispatcher_StopsRetryingWhenContextIsCanceledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher := New(Options{
		Capacity:       1,
		Workers:        1,
		MaxAttempts:    3,
		RetryBaseDelay: time.Hour,
		RetryMaxDelay:  time.Hour,
	})
	dispatcher.Start(ctx)
	var calls atomic.Int64
	_, err := dispatcher.Enqueue(Job{
		ID: "cancel-backoff",
		Run: func(context.Context) error {
			calls.Add(1)
			cancel()
			return errors.New("temporary")
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return dispatcher.Status().Failed == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, context.Canceled.Error(), dispatcher.Status().LastError)
}

func TestDispatcher_RecordsFailureAfterRetryExhaustion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatcher := New(Options{
		Capacity:       1,
		Workers:        1,
		MaxAttempts:    2,
		RetryBaseDelay: time.Nanosecond,
		RetryMaxDelay:  time.Nanosecond,
	})
	dispatcher.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, dispatcher.Shutdown(context.Background()))
	})

	_, err := dispatcher.Enqueue(Job{
		ID: "fail",
		Run: func(context.Context) error {
			return errors.New("permanent")
		},
	})

	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return dispatcher.Status().Failed == 1
	}, time.Second, 10*time.Millisecond)
	status := dispatcher.Status()
	require.True(t, status.Degraded)
	require.Equal(t, "permanent", status.LastError)
}

func TestDispatcher_ShutdownDrainsQueuedJobs(t *testing.T) {
	ctx := t.Context()
	dispatcher := New(Options{Capacity: 1, Workers: 1})
	dispatcher.Start(ctx)
	release := make(chan struct{})
	var ran atomic.Bool
	_, err := dispatcher.Enqueue(Job{
		ID: "slow",
		Run: func(context.Context) error {
			<-release
			ran.Store(true)
			return nil
		},
	})
	require.NoError(t, err)
	done := make(chan error, 1)

	go func() {
		done <- dispatcher.Shutdown(context.Background())
	}()

	require.Never(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, 50*time.Millisecond, 10*time.Millisecond)
	close(release)
	require.NoError(t, <-done)
	require.True(t, ran.Load())
}

func TestDispatcher_ShutdownReturnsContextErrorWhenDrainTimesOut(t *testing.T) {
	runCtx, stop := context.WithCancel(context.Background())
	dispatcher := New(Options{Capacity: 1, Workers: 1})
	dispatcher.Start(runCtx)
	_, err := dispatcher.Enqueue(Job{
		ID: "blocked",
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	err = dispatcher.Shutdown(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	stop()
	dispatcher.Close()
}

func TestDispatcher_RejectsEnqueueAfterClose(t *testing.T) {
	dispatcher := New(Options{})
	dispatcher.Close()

	enqueued, err := dispatcher.Enqueue(Job{ID: "closed", Run: func(context.Context) error { return nil }})

	require.False(t, enqueued)
	require.ErrorIs(t, err, ErrDispatcherClosed)
}

func TestDispatcher_CloseIsSafeBeforeInitialization(t *testing.T) {
	var dispatcher *Dispatcher

	dispatcher.Close()
}
