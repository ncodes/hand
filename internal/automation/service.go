package automation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/wandxy/morph/pkg/stringx"
)

const (
	defaultMaxTimerSleep     = time.Minute
	defaultStaleRunningAfter = 10 * time.Minute
)

type Runner interface {
	RunAutomation(context.Context, Job) (RunResult, error)
}

type RunnerFunc func(context.Context, Job) (RunResult, error)

func (fn RunnerFunc) RunAutomation(ctx context.Context, job Job) (RunResult, error) {
	if fn == nil {
		return RunResult{}, errors.New("automation runner is required")
	}

	return fn(ctx, job)
}

type RunResult struct {
	Status         RunStatus
	Output         string
	SessionID      string
	DeliveryStatus DeliveryStatus
	DeliveryError  string
	Model          string
	Provider       string
	Usage          Usage
}

type ServiceOptions struct {
	Store                      Store
	Runner                     Runner
	Logger                     Logger
	Tracer                     Tracer
	Now                        func() time.Time
	Location                   *time.Location
	DefaultTimezone            string
	MaxTimerSleep              time.Duration
	StaleRunningAfter          time.Duration
	DisableAfterScheduleErrors int
}

type Status struct {
	Running      bool
	StartedAt    time.Time
	JobCount     int
	RunningCount int
	NextWakeAt   time.Time
}

type Service struct {
	store                      Store
	runner                     Runner
	logger                     Logger
	tracer                     Tracer
	now                        func() time.Time
	location                   *time.Location
	defaultTimezone            string
	maxTimerSleep              time.Duration
	staleRunningAfter          time.Duration
	disableAfterScheduleErrors int

	mu        sync.Mutex
	started   bool
	startedAt time.Time
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	wake      chan struct{}
	running   map[string]struct{}
	nextWake  time.Time
}

func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Store == nil {
		return nil, errors.New("automation store is required")
	}
	if opts.Runner == nil {
		return nil, errors.New("automation runner is required")
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	maxTimerSleep := opts.MaxTimerSleep
	if maxTimerSleep <= 0 {
		maxTimerSleep = defaultMaxTimerSleep
	}
	staleRunningAfter := opts.StaleRunningAfter
	if staleRunningAfter <= 0 {
		staleRunningAfter = defaultStaleRunningAfter
	}

	return &Service{
		store:                      opts.Store,
		runner:                     opts.Runner,
		logger:                     opts.Logger,
		tracer:                     opts.Tracer,
		now:                        now,
		location:                   opts.Location,
		defaultTimezone:            stringx.String(opts.DefaultTimezone).Trim(),
		maxTimerSleep:              maxTimerSleep,
		staleRunningAfter:          staleRunningAfter,
		disableAfterScheduleErrors: opts.DisableAfterScheduleErrors,
		wake:                       make(chan struct{}, 1),
		running:                    make(map[string]struct{}),
	}, nil
}

func (s *Service) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("automation service is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	s.ctx = serviceCtx
	s.cancel = cancel
	s.done = make(chan struct{})
	s.started = true
	s.startedAt = s.getNow()
	s.mu.Unlock()

	if err := s.recoverStartup(serviceCtx); err != nil {
		cancel()
		s.mu.Lock()
		s.started = false
		s.cancel = nil
		s.ctx = nil
		close(s.done)
		s.done = nil
		s.mu.Unlock()
		return err
	}

	s.record(serviceCtx, "info", "automation scheduler started", automationEventSvcStarted, nil)
	go s.runLoop(serviceCtx, s.done)
	s.notifyWake()

	return nil
}

func (s *Service) Stop() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	done := s.done
	s.started = false
	s.cancel = nil
	s.ctx = nil
	s.done = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	s.record(context.Background(), "info", "automation scheduler stopped", automationEventSvcStopped, nil)

	return nil
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	if s == nil {
		return Status{}, errors.New("automation service is required")
	}

	list, err := s.store.ListJobs(ctx, JobQuery{IncludeDisabled: true})
	if err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	status := Status{
		Running:    s.started,
		StartedAt:  s.startedAt,
		JobCount:   len(list.Jobs),
		NextWakeAt: s.nextWake,
	}
	for _, job := range list.Jobs {
		if !job.State.RunningAt.IsZero() {
			status.RunningCount++
		}
	}
	s.mu.Unlock()

	return status, nil
}

func (s *Service) List(ctx context.Context, query JobQuery) (JobList, error) {
	if s == nil {
		return JobList{}, errors.New("automation service is required")
	}

	return s.store.ListJobs(ctx, query)
}

func (s *Service) Add(ctx context.Context, job Job) (Job, error) {
	if s == nil {
		return Job{}, errors.New("automation service is required")
	}

	prepared, err := s.prepareJobSchedule(job, s.getNow())
	if err != nil {
		return Job{}, err
	}
	created, err := s.store.CreateJob(ctx, prepared)
	if err != nil {
		return Job{}, err
	}
	s.notifyWake()

	return created, nil
}

func (s *Service) Update(ctx context.Context, patch JobPatch) (Job, error) {
	if s == nil {
		return Job{}, errors.New("automation service is required")
	}

	if checkJobPatchNeedsScheduleRepair(patch) {
		current, ok, err := s.store.GetJob(ctx, patch.ID)
		if err != nil {
			return Job{}, err
		}
		if !ok {
			return Job{}, errors.New("automation job not found")
		}

		candidate := applyServiceJobPatchForScheduleCheck(current, patch)
		prepared, err := s.prepareJobSchedule(candidate, s.getNow())
		if err != nil {
			return Job{}, err
		}
		patch.State = &prepared.State
	}

	updated, err := s.store.PatchJob(ctx, patch)
	if err != nil {
		return Job{}, err
	}
	s.notifyWake()

	return updated, nil
}

func (s *Service) Remove(ctx context.Context, id string) error {
	if s == nil {
		return errors.New("automation service is required")
	}
	if err := s.store.DeleteJob(ctx, id); err != nil {
		return err
	}
	s.notifyWake()

	return nil
}

func (s *Service) Run(ctx context.Context, id string) (Run, error) {
	if s == nil {
		return Run{}, errors.New("automation service is required")
	}

	job, ok, err := s.store.GetJob(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if !ok {
		return Run{}, errors.New("automation job not found")
	}
	if err := s.markJobRunning(ctx, job, s.getNow()); err != nil {
		return Run{}, err
	}

	return s.executeJob(ctx, job)
}

func (s *Service) runLoop(ctx context.Context, done chan struct{}) {
	defer close(done)

	for {
		s.executeDueJobs(ctx)
		sleep := s.nextSleep(ctx)
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return
		case <-s.wake:
			stopTimer(timer)
		case <-timer.C:
		}
	}
}

func (s *Service) executeDueJobs(ctx context.Context) {
	now := s.getNow()
	list, err := s.store.ListJobs(ctx, JobQuery{})
	if err != nil {
		s.record(ctx, "error", "automation scheduler failed to list jobs", automationEventFailed, map[string]any{
			"error": err.Error(),
		})
		return
	}

	for _, job := range list.Jobs {
		if ctx.Err() != nil {
			return
		}
		if job.State.RunningAt.IsZero() && job.State.NextRunAt.IsZero() {
			repaired, err := s.repairJobSchedule(ctx, job, now, false, false)
			if err != nil {
				s.handleScheduleError(ctx, job, err, now)
				continue
			}
			job = repaired
		}
		if job.State.RunningAt.IsZero() &&
			!job.State.NextRunAt.IsZero() &&
			!job.State.NextRunAt.After(now) {
			if err := s.markJobRunning(ctx, job, now); err != nil {
				s.record(ctx, "warn", "automation scheduler skipped running job", automationEventSkipped, map[string]any{
					"job_id": job.ID,
					"error":  err.Error(),
				})
				continue
			}
			go func(job Job) {
				_, _ = s.executeJob(ctx, job)
			}(job)
		}
	}
}

func (s *Service) executeJob(ctx context.Context, job Job) (Run, error) {
	started := s.getNow()
	run, err := s.store.CreateRun(ctx, Run{
		JobID:     job.ID,
		Status:    RunStatusRunning,
		StartedAt: started,
	})
	if err != nil {
		_ = s.clearJobRunning(ctx, job, err)
		s.clearRunningLocal(job.ID)
		return Run{}, err
	}

	s.record(ctx, "info", "automation job started", automationEventStarted, map[string]any{
		"job_id": job.ID,
		"run_id": run.ID,
	})
	result, runErr := s.runner.RunAutomation(ctx, job.Clone())
	finished := s.getNow()
	status := result.Status
	if status == "" {
		status = RunStatusOK
	}
	if runErr != nil {
		status = RunStatusError
	}
	usage := result.Usage
	patch := RunPatch{
		ID:             run.ID,
		Status:         status,
		EndedAt:        finished,
		Output:         result.Output,
		SessionID:      result.SessionID,
		DeliveryStatus: result.DeliveryStatus,
		DeliveryError:  result.DeliveryError,
		Model:          result.Model,
		Provider:       result.Provider,
		Usage:          &usage,
	}
	if runErr != nil {
		patch.Error = runErr.Error()
	}
	finishedRun, finishErr := s.store.FinishRun(ctx, patch)
	if finishErr != nil {
		_ = s.clearJobRunning(ctx, job, finishErr)
		s.clearRunningLocal(job.ID)
		return Run{}, finishErr
	}

	updatedJob, jobErr := s.finishJobRun(ctx, job, finishedRun, runErr)
	s.clearRunningLocal(job.ID)
	s.notifyWake()
	fields := map[string]any{
		"job_id":      job.ID,
		"run_id":      finishedRun.ID,
		"status":      string(finishedRun.Status),
		"duration_ms": finishedRun.Duration.Milliseconds(),
	}
	if runErr != nil {
		fields["error"] = runErr.Error()
		s.record(ctx, "error", "automation job failed", automationEventFailed, fields)
		return finishedRun, runErr
	}
	if jobErr != nil {
		fields["error"] = jobErr.Error()
		s.record(ctx, "error", "automation job state update failed", automationEventFailed, fields)
		return finishedRun, jobErr
	}
	if updatedJob.DeleteAfterRun && finishedRun.Status == RunStatusOK {
		if err := s.store.DeleteJob(ctx, updatedJob.ID); err != nil {
			return finishedRun, err
		}
	}
	s.record(ctx, "info", "automation job finished", automationEventFinished, fields)

	return finishedRun, nil
}

func (s *Service) recoverStartup(ctx context.Context) error {
	now := s.getNow()
	list, err := s.store.ListJobs(ctx, JobQuery{IncludeDisabled: true})
	if err != nil {
		return err
	}

	for _, job := range list.Jobs {
		if !job.State.RunningAt.IsZero() && now.Sub(job.State.RunningAt) >= s.staleRunningAfter {
			job.State.RunningAt = time.Time{}
			updated, err := s.patchJobState(ctx, job.ID, job.State)
			if err != nil {
				return err
			}
			job = updated
		}
		if !job.Enabled {
			continue
		}
		_, err := s.repairJobSchedule(ctx, job, now, true, false)
		if err != nil {
			s.handleScheduleError(ctx, job, err, now)
		}
	}

	return nil
}

func (s *Service) repairJobSchedule(
	ctx context.Context,
	job Job,
	now time.Time,
	skipMissed bool,
	force bool,
) (Job, error) {
	if !job.Enabled {
		return job.Clone(), nil
	}
	if !force && !job.State.NextRunAt.IsZero() && job.State.NextRunAt.After(now) {
		return job.Clone(), nil
	}
	if skipMissed && !job.State.NextRunAt.IsZero() && !job.State.NextRunAt.After(now) {
		return s.skipMissedJob(ctx, job, now)
	}

	prepared, err := s.prepareJobSchedule(job, now)
	if err != nil {
		return Job{}, err
	}

	return s.patchJobState(ctx, job.ID, prepared.State)
}

func (s *Service) prepareJobSchedule(job Job, now time.Time) (Job, error) {
	if !job.Enabled {
		return job.Clone(), nil
	}

	evaluation, err := EvaluateJob(job, NextRunOptions{
		Now:             now,
		LastRunAt:       job.State.LastRunAt,
		Location:        s.location,
		DefaultTimezone: s.defaultTimezone,
	})
	if err != nil {
		return Job{}, err
	}

	return evaluation.Job, nil
}

func (s *Service) skipMissedJob(ctx context.Context, job Job, now time.Time) (Job, error) {
	state := job.State
	state.RunningAt = time.Time{}
	state.LastRunAt = now.UTC()
	state.LastStatus = RunStatusSkipped
	state.LastError = "missed scheduled run skipped during startup"
	state.LastDuration = 0
	if job.Schedule.Kind == ScheduleAt {
		state.NextRunAt = time.Time{}
	} else {
		result, err := NextRun(job.Schedule, NextRunOptions{
			Now:             now,
			LastRunAt:       now,
			Location:        s.location,
			DefaultTimezone: s.defaultTimezone,
		})
		if err != nil {
			return Job{}, err
		}
		state.NextRunAt = result.NextRunAt
	}

	updated, err := s.patchJobState(ctx, job.ID, state)
	if err != nil {
		return Job{}, err
	}
	s.record(ctx, "info", "automation job skipped during startup recovery", automationEventSkipped, map[string]any{
		"job_id": job.ID,
	})

	return updated, nil
}

func (s *Service) markJobRunning(ctx context.Context, job Job, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.running[job.ID]; ok {
		return errors.New("automation job is already running")
	}
	loaded, ok, err := s.store.GetJob(ctx, job.ID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("automation job not found")
	}
	if !loaded.State.RunningAt.IsZero() {
		return errors.New("automation job is already running")
	}
	loaded.State.RunningAt = now.UTC()
	if _, err := s.patchJobState(ctx, loaded.ID, loaded.State); err != nil {
		return err
	}
	s.running[job.ID] = struct{}{}

	return nil
}

func (s *Service) finishJobRun(ctx context.Context, job Job, run Run, runErr error) (Job, error) {
	loaded, ok, err := s.store.GetJob(ctx, job.ID)
	if err != nil {
		return Job{}, err
	}
	if !ok {
		return Job{}, errors.New("automation job not found")
	}
	state := loaded.State
	state.RunningAt = time.Time{}
	state.LastRunAt = run.EndedAt
	state.LastStatus = run.Status
	state.LastDuration = run.Duration
	if runErr != nil {
		state.LastError = runErr.Error()
		state.ConsecutiveErrors++
	} else {
		state.LastError = ""
		state.ConsecutiveErrors = 0
	}
	result, err := NextRun(loaded.Schedule, NextRunOptions{
		Now:             run.EndedAt,
		LastRunAt:       run.EndedAt,
		Location:        s.location,
		DefaultTimezone: s.defaultTimezone,
	})
	if err != nil {
		return s.patchJobState(ctx, loaded.ID, state)
	}
	if result.Done {
		state.NextRunAt = time.Time{}
	} else {
		state.NextRunAt = result.NextRunAt
	}

	return s.patchJobState(ctx, loaded.ID, state)
}

func (s *Service) clearJobRunning(ctx context.Context, job Job, cause error) error {
	state := job.State
	if loaded, ok, err := s.store.GetJob(ctx, job.ID); err != nil {
		return err
	} else if ok {
		state = loaded.State
	}
	state.RunningAt = time.Time{}
	if cause != nil {
		state.LastError = cause.Error()
	}

	_, err := s.patchJobState(ctx, job.ID, state)
	return err
}

func (s *Service) handleScheduleError(ctx context.Context, job Job, scheduleErr error, now time.Time) {
	updated := ApplyScheduleError(job, scheduleErr, ScheduleErrorOptions{
		Now:          now,
		DisableAfter: s.disableAfterScheduleErrors,
	})
	if _, err := s.store.PatchJob(ctx, JobPatch{
		ID:      job.ID,
		Enabled: &updated.Enabled,
		State:   &updated.State,
	}); err != nil {
		s.record(ctx, "error", "automation schedule error patch failed", automationEventFailed, map[string]any{
			"job_id": job.ID,
			"error":  err.Error(),
		})
		return
	}
	s.record(ctx, "warn", "automation schedule evaluation failed", automationEventFailed, map[string]any{
		"job_id": job.ID,
		"error":  scheduleErr.Error(),
	})
}

func (s *Service) patchJobState(ctx context.Context, id string, state JobState) (Job, error) {
	return s.store.PatchJob(ctx, JobPatch{
		ID:    id,
		State: &state,
	})
}

func (s *Service) nextSleep(ctx context.Context) time.Duration {
	now := s.getNow()
	sleep := s.maxTimerSleep
	nextWake := now.Add(sleep)

	list, err := s.store.ListJobs(ctx, JobQuery{})
	if err == nil {
		for _, job := range list.Jobs {
			if !job.State.RunningAt.IsZero() || job.State.NextRunAt.IsZero() {
				continue
			}
			until := job.State.NextRunAt.Sub(now)
			if until <= 0 {
				sleep = 0
				nextWake = now
				break
			}
			if until < sleep {
				sleep = until
				nextWake = job.State.NextRunAt
			}
		}
	}

	s.mu.Lock()
	s.nextWake = nextWake.UTC()
	s.mu.Unlock()

	return sleep
}

func (s *Service) getNow() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

func (s *Service) notifyWake() {
	if s == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) clearRunningLocal(jobID string) {
	s.mu.Lock()
	delete(s.running, jobID)
	s.mu.Unlock()
}

func checkJobPatchNeedsScheduleRepair(patch JobPatch) bool {
	return patch.Schedule != nil ||
		patch.Enabled != nil ||
		patch.State != nil
}

func applyServiceJobPatchForScheduleCheck(job Job, patch JobPatch) Job {
	job = job.Clone()
	if patch.Enabled != nil {
		job.Enabled = *patch.Enabled
	}
	if patch.Schedule != nil {
		job.Schedule = *patch.Schedule
	}
	if patch.State != nil {
		job.State = *patch.State
	}

	return job
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
