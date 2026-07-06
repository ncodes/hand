package automation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/pkg/nanoid"
)

var (
	testServiceJobA = nanoid.MustFromSeed(JobIDPrefix, "service-a", "AutomationServiceJobSeed")
	testServiceJobB = nanoid.MustFromSeed(JobIDPrefix, "service-b", "AutomationServiceJobSeed")
	testServiceJobC = nanoid.MustFromSeed(JobIDPrefix, "service-c", "AutomationServiceJobSeed")
)

type automationTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newAutomationTestClock(now time.Time) *automationTestClock {
	return &automationTestClock{now: now}
}

func (c *automationTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *automationTestClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

type automationRunnerStub struct {
	mu      sync.Mutex
	calls   []Job
	results []RunResult
	errs    []error
	block   chan struct{}
	started chan Job
}

func (r *automationRunnerStub) RunAutomation(ctx context.Context, job Job) (RunResult, error) {
	r.mu.Lock()
	r.calls = append(r.calls, job.Clone())
	if r.started != nil {
		select {
		case r.started <- job.Clone():
		default:
		}
	}
	var result RunResult
	if len(r.results) > 0 {
		result = r.results[0]
		r.results = r.results[1:]
	}
	var err error
	if len(r.errs) > 0 {
		err = r.errs[0]
		r.errs = r.errs[1:]
	}
	block := r.block
	r.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return RunResult{}, ctx.Err()
		}
	}

	return result, err
}

func (r *automationRunnerStub) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.calls)
}

type automationLogEvent struct {
	level   string
	message string
	fields  map[string]any
}

type automationLoggerStub struct {
	mu     sync.Mutex
	events []automationLogEvent
}

func (l *automationLoggerStub) Debug(message string, fields map[string]any) {
	l.add("debug", message, fields)
}

func (l *automationLoggerStub) Info(message string, fields map[string]any) {
	l.add("info", message, fields)
}

func (l *automationLoggerStub) Warn(message string, fields map[string]any) {
	l.add("warn", message, fields)
}

func (l *automationLoggerStub) Error(message string, fields map[string]any) {
	l.add("error", message, fields)
}

func (l *automationLoggerStub) add(level string, message string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, automationLogEvent{level: level, message: message, fields: fields})
}

func (l *automationLoggerStub) Messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	values := make([]string, 0, len(l.events))
	for _, event := range l.events {
		values = append(values, event.message)
	}

	return values
}

type automationTraceEvent struct {
	event   string
	payload any
}

type automationTracerStub struct {
	mu     sync.Mutex
	events []automationTraceEvent
}

func (t *automationTracerStub) Record(_ context.Context, event string, payload any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.events = append(t.events, automationTraceEvent{event: event, payload: payload})
}

func (t *automationTracerStub) EventNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	values := make([]string, 0, len(t.events))
	for _, event := range t.events {
		values = append(values, event.event)
	}

	return values
}

type automationStoreStub struct {
	Store
	getErr       error
	listErr      error
	patchErr     error
	deleteErr    error
	createJobErr error
	createRunErr error
	finishRunErr error
}

func (s automationStoreStub) GetJob(ctx context.Context, id string) (Job, bool, error) {
	if s.getErr != nil {
		return Job{}, false, s.getErr
	}

	return s.Store.GetJob(ctx, id)
}

func (s automationStoreStub) ListJobs(ctx context.Context, query JobQuery) (JobList, error) {
	if s.listErr != nil {
		return JobList{}, s.listErr
	}

	return s.Store.ListJobs(ctx, query)
}

func (s automationStoreStub) CreateJob(ctx context.Context, job Job) (Job, error) {
	if s.createJobErr != nil {
		return Job{}, s.createJobErr
	}

	return s.Store.CreateJob(ctx, job)
}

func (s automationStoreStub) PatchJob(ctx context.Context, patch JobPatch) (Job, error) {
	if s.patchErr != nil {
		return Job{}, s.patchErr
	}

	return s.Store.PatchJob(ctx, patch)
}

func (s automationStoreStub) DeleteJob(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}

	return s.Store.DeleteJob(ctx, id)
}

func (s automationStoreStub) CreateRun(ctx context.Context, run Run) (Run, error) {
	if s.createRunErr != nil {
		return Run{}, s.createRunErr
	}

	return s.Store.CreateRun(ctx, run)
}

func (s automationStoreStub) FinishRun(ctx context.Context, patch RunPatch) (Run, error) {
	if s.finishRunErr != nil {
		return Run{}, s.finishRunErr
	}

	return s.Store.FinishRun(ctx, patch)
}

func newAutomationTestService(t *testing.T, store Store, clock *automationTestClock, runner Runner) *Service {
	t.Helper()

	service, err := NewService(ServiceOptions{
		Store:             store,
		Runner:            runner,
		Now:               clock.Now,
		MaxTimerSleep:     10 * time.Millisecond,
		StaleRunningAfter: time.Minute,
		DefaultTimezone:   "UTC",
	})
	require.NoError(t, err)

	return service
}

func automationTestJobIDs(jobs []Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	return ids
}
