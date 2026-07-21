package browser

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errBrowserActionTimedOut = errors.New("browser action timed out")

type actionBudgetContextKey struct{}

type actionBudget struct {
	mu        sync.Mutex
	cancel    context.CancelCauseFunc
	timer     *time.Timer
	remaining time.Duration
	startedAt time.Time
	pauses    int
	closed    bool
}

func newActionBudget(parent context.Context, duration time.Duration) (context.Context, *actionBudget) {
	ctx, cancel := context.WithCancelCause(parent)
	budget := &actionBudget{cancel: cancel, remaining: duration, startedAt: time.Now()}
	ctx = context.WithValue(ctx, actionBudgetContextKey{}, budget)
	budget.timer = time.AfterFunc(duration, budget.expire)
	return ctx, budget
}

func actionBudgetFromContext(ctx context.Context) *actionBudget {
	if ctx == nil {
		return nil
	}
	budget, _ := ctx.Value(actionBudgetContextKey{}).(*actionBudget)
	return budget
}

func (b *actionBudget) Pause() func() {
	if b == nil {
		return func() {}
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	if b.pauses == 0 {
		if b.timer.Stop() {
			b.remaining -= time.Since(b.startedAt)
			if b.remaining < 0 {
				b.remaining = 0
			}
		}
	}
	b.pauses++
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(b.resume)
	}
}

func (b *actionBudget) resume() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.pauses == 0 {
		return
	}
	b.pauses--
	if b.pauses != 0 {
		return
	}
	b.startedAt = time.Now()
	b.timer = time.AfterFunc(b.remaining, b.expire)
}

func (b *actionBudget) expire() {
	b.mu.Lock()
	if b.closed || b.pauses > 0 {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()
	b.cancel(errBrowserActionTimedOut)
}

func (b *actionBudget) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.timer.Stop()
	b.mu.Unlock()
	b.cancel(context.Canceled)
}
