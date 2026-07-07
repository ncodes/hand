package automation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/state/storememory"
)

func TestNewService_Validation(t *testing.T) {
	_, err := NewService(ServiceOptions{})
	require.EqualError(t, err, "automation store is required")

	_, err = NewService(ServiceOptions{Store: storememory.NewStore()})
	require.EqualError(t, err, "automation runner is required")

	called := false
	result, err := RunnerFunc(func(context.Context, Job) (RunResult, error) {
		called = true
		return RunResult{Output: "ok"}, nil
	}).RunAutomation(context.Background(), Job{})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "ok", result.Output)

	_, err = RunnerFunc(nil).RunAutomation(context.Background(), Job{})
	require.EqualError(t, err, "automation runner is required")

	service, err := NewService(ServiceOptions{
		Store:  storememory.NewStore(),
		Runner: &automationRunnerStub{},
	})
	require.NoError(t, err)
	require.NotNil(t, service.now)
	require.Equal(t, defaultMaxTimerSleep, service.maxTimerSleep)
	require.Equal(t, defaultStaleRunningAfter, service.staleRunningAfter)
	require.False(t, service.getNow().IsZero())
}

func TestService_ControlValidationAndNoops(t *testing.T) {
	ctx := context.Background()
	var nilService *Service

	require.EqualError(t, nilService.Start(ctx), "automation service is required")
	require.NoError(t, nilService.Stop())
	_, err := nilService.Status(ctx)
	require.EqualError(t, err, "automation service is required")
	_, err = nilService.List(ctx, JobQuery{})
	require.EqualError(t, err, "automation service is required")
	_, err = nilService.Add(ctx, Job{})
	require.EqualError(t, err, "automation service is required")
	_, err = nilService.Update(ctx, JobPatch{})
	require.EqualError(t, err, "automation service is required")
	require.EqualError(t, nilService.Remove(ctx, testServiceJobA), "automation service is required")
	_, err = nilService.Run(ctx, testServiceJobA)
	require.EqualError(t, err, "automation service is required")

	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, storememory.NewStore(), clock, &automationRunnerStub{})

	require.NoError(t, service.Start(ctx))
	require.NoError(t, service.Stop())
	require.NoError(t, service.Stop())
}

func TestService_AddListUpdateRemove(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, RunnerFunc(func(context.Context, Job) (RunResult, error) {
		return RunResult{}, nil
	}))

	job, err := service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)
	require.Equal(t, clock.Now().Add(time.Hour), job.State.NextRunAt)

	list, err := service.List(ctx, JobQuery{})
	require.NoError(t, err)
	require.Equal(t, []string{testServiceJobA}, automationTestJobIDs(list.Jobs))

	nextSchedule := Schedule{Kind: ScheduleEvery, Every: 2 * time.Hour}
	updated, err := service.Update(ctx, JobPatch{
		ID:       testServiceJobA,
		Schedule: &nextSchedule,
	})
	require.NoError(t, err)
	require.Equal(t, clock.Now().Add(2*time.Hour), updated.State.NextRunAt)

	require.NoError(t, service.Remove(ctx, testServiceJobA))
	list, err = service.List(ctx, JobQuery{IncludeDisabled: true})
	require.NoError(t, err)
	require.Empty(t, list.Jobs)

	_, err = service.Add(ctx, Job{Enabled: true})
	require.EqualError(t, err, "automation schedule kind is required")

	createErr := errors.New("create job failed")
	_, err = newAutomationTestService(t, automationStoreStub{
		Store:        storememory.NewStore(),
		createJobErr: createErr,
	}, clock, &automationRunnerStub{}).Add(ctx, Job{
		ID:      testServiceJobB,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.ErrorIs(t, err, createErr)

	_, err = service.Update(ctx, JobPatch{ID: "bad"})
	require.EqualError(t, err, "automation job id must be a valid auto_ nanoid")

	_, err = service.Update(ctx, JobPatch{
		ID:      testServiceJobB,
		Enabled: new(true),
	})
	require.EqualError(t, err, "automation job not found")

	getErr := errors.New("get job failed")
	_, err = newAutomationTestService(t, automationStoreStub{
		Store:  store,
		getErr: getErr,
	}, clock, &automationRunnerStub{}).Update(ctx, JobPatch{
		ID:      testServiceJobA,
		Enabled: new(true),
	})
	require.ErrorIs(t, err, getErr)

	err = service.Remove(ctx, "bad")
	require.EqualError(t, err, "automation job id must be a valid auto_ nanoid")
}

func TestService_UpdateRejectsInvalidScheduleBeforePersisting(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	_, err := service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)

	badSchedule := Schedule{Kind: ScheduleCron}
	_, err = service.Update(ctx, JobPatch{
		ID:       testServiceJobA,
		Schedule: &badSchedule,
	})
	require.EqualError(t, err, "automation cron schedule expression is required")

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, ScheduleEvery, job.Schedule.Kind)
	require.Equal(t, time.Hour, job.Schedule.Every)
}

func TestService_RunExecutesJobAndUpdatesState(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	runner := &automationRunnerStub{results: []RunResult{{
		Output:    "done",
		SessionID: "ses_projectaprojectaproje",
		Model:     "gpt-test",
		Provider:  "openai",
		Usage:     Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
	}}}
	service := newAutomationTestService(t, store, clock, runner)

	_, err := service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
		Delivery: Delivery{Mode: DeliveryLocal},
	})
	require.NoError(t, err)

	run, err := service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	require.Equal(t, RunStatusOK, run.Status)
	require.Equal(t, "done", run.Output)
	require.Equal(t, "ses_projectaprojectaproje", run.SessionID)
	require.Equal(t, DeliveryStatusDelivered, run.DeliveryStatus)
	require.Equal(t, Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}, run.Usage)

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, job.State.RunningAt)
	require.Equal(t, RunStatusOK, job.State.LastStatus)
	require.Zero(t, job.State.NextRunAt)
	require.Equal(t, 1, runner.CallCount())
}

func TestService_RunDeliversSuccessfulOutputToGateway(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	sink := &automationDeliverySinkStub{}
	service, err := NewService(ServiceOptions{
		Store:        store,
		Runner:       &automationRunnerStub{results: []RunResult{{Output: "delivered", SessionID: "ses_projectaprojectaproje"}}},
		DeliverySink: sink,
		Now:          clock.Now,
	})
	require.NoError(t, err)

	_, err = service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
		Delivery: Delivery{
			Mode:     DeliveryGateway,
			Channel:  " slack ",
			Target:   " C1 ",
			ThreadID: " 123.456 ",
		},
	})
	require.NoError(t, err)

	run, err := service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	require.Equal(t, DeliveryStatusDelivered, run.DeliveryStatus)
	require.Empty(t, run.DeliveryError)

	requests := sink.Requests()
	require.Len(t, requests, 1)
	require.Equal(t, testServiceJobA, requests[0].JobID)
	require.Equal(t, run.ID, requests[0].RunID)
	require.Equal(t, RunStatusOK, requests[0].Status)
	require.Equal(t, "delivered", requests[0].Output)
	require.Equal(t, DeliveryGateway, requests[0].Target.Mode)
	require.Equal(t, "slack", requests[0].Target.Channel)
	require.Equal(t, "C1", requests[0].Target.Target)
	require.Equal(t, "123.456", requests[0].Target.ThreadID)
}

func TestService_RunTracksDeliveryFailure(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	deliveryErr := errors.New("gateway unavailable")
	service, err := NewService(ServiceOptions{
		Store:        store,
		Runner:       &automationRunnerStub{results: []RunResult{{Output: "done"}}},
		DeliverySink: &automationDeliverySinkStub{err: deliveryErr},
		Now:          clock.Now,
	})
	require.NoError(t, err)

	_, err = service.Add(ctx, Job{
		ID:       testServiceJobA,
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleAt, At: clock.Now()},
		Delivery: Delivery{Mode: DeliveryGateway, Target: "ops"},
	})
	require.NoError(t, err)

	run, err := service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, deliveryErr)
	require.Equal(t, RunStatusOK, run.Status)
	require.Equal(t, DeliveryStatusNotDelivered, run.DeliveryStatus)
	require.Equal(t, "gateway unavailable", run.DeliveryError)

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunStatusOK, job.State.LastStatus)
	require.Zero(t, job.State.ConsecutiveErrors)
}

func TestService_RunAllowsBestEffortDeliveryFailure(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service, err := NewService(ServiceOptions{
		Store:        store,
		Runner:       &automationRunnerStub{results: []RunResult{{Output: "done"}}},
		DeliverySink: &automationDeliverySinkStub{err: errors.New("gateway unavailable")},
		Now:          clock.Now,
	})
	require.NoError(t, err)

	_, err = service.Add(ctx, Job{
		ID:       testServiceJobA,
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleAt, At: clock.Now()},
		Delivery: Delivery{Mode: DeliveryGateway, Target: "ops", BestEffort: true},
	})
	require.NoError(t, err)

	run, err := service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	require.Equal(t, DeliveryStatusNotDelivered, run.DeliveryStatus)
	require.Equal(t, "gateway unavailable", run.DeliveryError)
}

func TestService_RunSendsFailureNoticeAfterThreshold(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	runnerErr := errors.New("agent failed")
	sink := &automationDeliverySinkStub{}
	service, err := NewService(ServiceOptions{
		Store:        store,
		Runner:       &automationRunnerStub{errs: []error{runnerErr}},
		DeliverySink: sink,
		Now:          clock.Now,
	})
	require.NoError(t, err)

	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		Delivery: Delivery{
			Mode:            DeliveryGateway,
			Channel:         "slack",
			Target:          "primary",
			FailureTarget:   "ops",
			FailureAfter:    2,
			FailureCooldown: time.Hour,
		},
		State: JobState{ConsecutiveErrors: 1},
	})
	require.NoError(t, err)

	_, err = service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, runnerErr)

	requests := sink.Requests()
	require.Len(t, requests, 1)
	require.Equal(t, RunStatusError, requests[0].Status)
	require.Equal(t, "agent failed", requests[0].Error)
	require.Equal(t, "ops", requests[0].Target.Target)

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, job.State.ConsecutiveErrors)
	require.Equal(t, clock.Now(), job.State.LastFailureNoticeAt)
}

func TestService_RunSkipsFailureNoticeDuringCooldown(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	now := time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC)
	clock := newAutomationTestClock(now)
	sink := &automationDeliverySinkStub{}
	runnerErr := errors.New("agent failed")
	service, err := NewService(ServiceOptions{
		Store:        store,
		Runner:       &automationRunnerStub{errs: []error{runnerErr}},
		DeliverySink: sink,
		Now:          clock.Now,
	})
	require.NoError(t, err)

	_, err = store.CreateJob(ctx, Job{
		ID:       testServiceJobA,
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
		Delivery: Delivery{
			Mode:            DeliveryGateway,
			Target:          "ops",
			FailureAfter:    1,
			FailureCooldown: time.Hour,
		},
		State: JobState{LastFailureNoticeAt: now.Add(-30 * time.Minute)},
	})
	require.NoError(t, err)

	_, err = service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, runnerErr)
	require.Empty(t, sink.Requests())

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, now.Add(-30*time.Minute), job.State.LastFailureNoticeAt)
}

func TestService_RunSetsLocalFailureNoticeCooldown(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	runnerErr := errors.New("agent failed")
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{errs: []error{runnerErr}})

	_, err := store.CreateJob(ctx, Job{
		ID:       testServiceJobA,
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
		Delivery: Delivery{Mode: DeliveryLocal, FailureAfter: 1, FailureCooldown: time.Hour},
	})
	require.NoError(t, err)

	_, err = service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, runnerErr)

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, clock.Now(), job.State.LastFailureNoticeAt)
}

func TestService_RunDeliversOriginFromMetadata(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	sink := &automationDeliverySinkStub{}
	service, err := NewService(ServiceOptions{
		Store:        store,
		Runner:       &automationRunnerStub{results: []RunResult{{Output: "origin body"}}},
		DeliverySink: sink,
		Now:          clock.Now,
	})
	require.NoError(t, err)

	_, err = service.Add(ctx, Job{
		ID:       testServiceJobA,
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleAt, At: clock.Now()},
		Payload: Payload{Metadata: map[string]string{
			metadataOriginChannel: " Slack ",
			metadataOriginTarget:  " C1 ",
			metadataOriginThread:  " 123.456 ",
		}},
		Delivery: Delivery{Mode: DeliveryOrigin},
	})
	require.NoError(t, err)

	run, err := service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	require.Equal(t, DeliveryStatusDelivered, run.DeliveryStatus)

	requests := sink.Requests()
	require.Len(t, requests, 1)
	require.Equal(t, DeliveryOrigin, requests[0].Target.Mode)
	require.Equal(t, "slack", requests[0].Target.Channel)
	require.Equal(t, "C1", requests[0].Target.Target)
	require.Equal(t, "123.456", requests[0].Target.ThreadID)
}

func TestService_RunDeliversWebhook(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	var request DeliveryRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	service := newAutomationTestService(
		t,
		store,
		clock,
		&automationRunnerStub{results: []RunResult{{Output: "webhook body"}}},
	)

	_, err := service.Add(ctx, Job{
		ID:       testServiceJobA,
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleAt, At: clock.Now()},
		Delivery: Delivery{Mode: DeliveryWebhook, WebhookURL: server.URL},
	})
	require.NoError(t, err)

	run, err := service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	require.Equal(t, DeliveryStatusDelivered, run.DeliveryStatus)
	require.Equal(t, testServiceJobA, request.JobID)
	require.Equal(t, run.ID, request.RunID)
	require.Equal(t, "webhook body", request.Output)
}

func TestService_RunTracksWebhookDeliveryFailure(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad hook", http.StatusBadGateway)
	}))
	defer server.Close()
	service := newAutomationTestService(
		t,
		store,
		clock,
		&automationRunnerStub{results: []RunResult{{Output: "webhook body"}}},
	)

	_, err := service.Add(ctx, Job{
		ID:       testServiceJobA,
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleAt, At: clock.Now()},
		Delivery: Delivery{Mode: DeliveryWebhook, WebhookURL: server.URL, BestEffort: true},
	})
	require.NoError(t, err)

	run, err := service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	require.Equal(t, DeliveryStatusNotDelivered, run.DeliveryStatus)
	require.Equal(t, "automation webhook delivery failed: 502 Bad Gateway: bad hook", run.DeliveryError)
}

func TestService_DeliverRunBranches(t *testing.T) {
	now := time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC)
	service := &Service{}

	t.Run("skipped run does not request output delivery", func(t *testing.T) {
		result, err := service.deliverRun(
			context.Background(),
			Job{Delivery: Delivery{Mode: DeliveryLocal}},
			testServiceJobA,
			RunStatusSkipped,
			RunResult{},
			nil,
			now,
		)
		require.NoError(t, err)
		require.Equal(t, DeliveryStatusNotRequested, result.Status)
	})

	t.Run("unsupported delivery mode is recorded as not delivered", func(t *testing.T) {
		result, err := service.deliver(
			context.Background(),
			Job{},
			testServiceJobA,
			RunStatusOK,
			RunResult{},
			nil,
			Delivery{Mode: DeliveryMode("unknown")},
			DeliveryTarget{Mode: DeliveryMode("unknown")},
			now,
			false,
		)
		require.EqualError(t, err, "unsupported automation delivery mode")
		require.Equal(t, DeliveryStatusNotDelivered, result.Status)
	})

	t.Run("gateway delivery requires configured sink", func(t *testing.T) {
		result, err := service.deliver(
			context.Background(),
			Job{},
			testServiceJobA,
			RunStatusOK,
			RunResult{},
			nil,
			Delivery{Mode: DeliveryGateway},
			DeliveryTarget{Mode: DeliveryGateway},
			now,
			false,
		)
		require.EqualError(t, err, "automation delivery sink is required")
		require.Equal(t, DeliveryStatusNotDelivered, result.Status)
	})

	t.Run("gateway sink failure is recorded as not delivered", func(t *testing.T) {
		expected := errors.New("sink failed")
		service.deliverySink = DeliverySinkFunc(func(context.Context, DeliveryRequest) error {
			return expected
		})
		result, err := service.deliver(
			context.Background(),
			Job{},
			testServiceJobA,
			RunStatusOK,
			RunResult{},
			nil,
			Delivery{Mode: DeliveryGateway},
			DeliveryTarget{Mode: DeliveryGateway},
			now,
			false,
		)
		require.ErrorIs(t, err, expected)
		require.Equal(t, DeliveryStatusNotDelivered, result.Status)
	})
}

func TestService_RunReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, storememory.NewStore(), clock, &automationRunnerStub{})

	_, err := service.Run(ctx, testServiceJobA)
	require.EqualError(t, err, "automation job not found")

	getErr := errors.New("get job failed")
	service = newAutomationTestService(t, automationStoreStub{
		Store:  storememory.NewStore(),
		getErr: getErr,
	}, clock, &automationRunnerStub{})
	_, err = service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, getErr)
}

func TestService_RunFailureRecordsErrorAndNextRun(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	runnerErr := errors.New("runner failed")
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{errs: []error{runnerErr}})

	_, err := service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)

	_, err = service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, runnerErr)

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunStatusError, job.State.LastStatus)
	require.Equal(t, "runner failed", job.State.LastError)
	require.Equal(t, 1, job.State.ConsecutiveErrors)
	require.Equal(t, clock.Now().Add(time.Hour), job.State.NextRunAt)
}

func TestService_RunHandlesPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	baseStore := storememory.NewStore()
	_, err := baseStore.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)

	createRunErr := errors.New("create run failed")
	service := newAutomationTestService(t, automationStoreStub{
		Store:        baseStore,
		createRunErr: createRunErr,
	}, clock, &automationRunnerStub{})
	_, err = service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, createRunErr)
	job, ok, err := baseStore.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, job.State.RunningAt)

	finishRunErr := errors.New("finish run failed")
	service = newAutomationTestService(t, automationStoreStub{
		Store:        baseStore,
		finishRunErr: finishRunErr,
	}, clock, &automationRunnerStub{})
	_, err = service.Run(ctx, testServiceJobA)
	require.ErrorIs(t, err, finishRunErr)
	job, ok, err = baseStore.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, job.State.RunningAt)

	getErr := errors.New("get failed")
	err = newAutomationTestService(t, automationStoreStub{
		Store:  baseStore,
		getErr: getErr,
	}, clock, &automationRunnerStub{}).clearJobRunning(ctx, Job{ID: testServiceJobA}, createRunErr)
	require.ErrorIs(t, err, getErr)
}

func TestService_RunHandlesMissingJobAfterExecution(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, RunnerFunc(func(ctx context.Context, job Job) (RunResult, error) {
		require.NoError(t, store.DeleteJob(ctx, job.ID))
		return RunResult{}, nil
	}))

	_, err := service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)

	_, err = service.Run(ctx, testServiceJobA)
	require.EqualError(t, err, "automation job not found")
}

func TestService_RunDeletesOneShotJobAfterSuccess(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	_, err := service.Add(ctx, Job{
		ID:             testServiceJobA,
		Enabled:        true,
		DeleteAfterRun: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
	})
	require.NoError(t, err)

	_, err = service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	_, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.False(t, ok)

	deleteErr := errors.New("delete failed")
	store = storememory.NewStore()
	service = newAutomationTestService(t, automationStoreStub{
		Store:     store,
		deleteErr: deleteErr,
	}, clock, &automationRunnerStub{})
	_, err = service.Add(ctx, Job{
		ID:             testServiceJobB,
		Enabled:        true,
		DeleteAfterRun: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
	})
	require.NoError(t, err)
	_, err = service.Run(ctx, testServiceJobB)
	require.ErrorIs(t, err, deleteErr)
}

func TestService_RunPreventsDuplicateExecution(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	block := make(chan struct{})
	runner := &automationRunnerStub{block: block, started: make(chan Job, 1)}
	service := newAutomationTestService(t, store, clock, runner)

	_, err := service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := service.Run(ctx, testServiceJobA)
		done <- err
	}()
	require.Eventually(t, func() bool {
		select {
		case <-runner.started:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	_, err = service.Run(ctx, testServiceJobA)
	require.EqualError(t, err, "automation job is already running")

	close(block)
	require.NoError(t, <-done)
}

func TestService_StartExecutesDueJobOnce(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	runner := &automationRunnerStub{started: make(chan Job, 1)}
	service := newAutomationTestService(t, store, clock, runner)

	require.NoError(t, service.Start(ctx))
	defer func() { require.NoError(t, service.Stop()) }()

	_, err := service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return runner.CallCount() == 1
	}, time.Second, 10*time.Millisecond)

	runs, err := store.ListRuns(ctx, RunQuery{JobID: testServiceJobA})
	require.NoError(t, err)
	require.Len(t, runs.Runs, 1)

	status, err := service.Status(ctx)
	require.NoError(t, err)
	require.True(t, status.Running)
	require.Equal(t, 1, status.JobCount)
}

func TestService_StartupRecoverySkipsMissedAndClearsStaleRunning(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	_, err := store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		State: JobState{
			NextRunAt: clock.Now().Add(-time.Hour),
			RunningAt: clock.Now().Add(-2 * time.Hour),
		},
	})
	require.NoError(t, err)
	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobB,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now().Add(-time.Hour),
		},
		State: JobState{NextRunAt: clock.Now().Add(-time.Hour)},
	})
	require.NoError(t, err)

	require.NoError(t, service.Start(ctx))
	require.NoError(t, service.Stop())

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, job.State.RunningAt)
	require.Equal(t, RunStatusSkipped, job.State.LastStatus)
	require.Equal(t, clock.Now().Add(time.Hour), job.State.NextRunAt)

	job, ok, err = store.GetJob(ctx, testServiceJobB)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunStatusSkipped, job.State.LastStatus)
	require.Zero(t, job.State.NextRunAt)
}

func TestService_StartupRecoveryRepairsMissingStateAndIgnoresDisabled(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	_, err := store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)
	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobB,
		Enabled: false,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
	})
	require.NoError(t, err)

	require.NoError(t, service.Start(ctx))
	require.NoError(t, service.Stop())

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, clock.Now().Add(time.Hour), job.State.NextRunAt)

	job, ok, err = store.GetJob(ctx, testServiceJobB)
	require.NoError(t, err)
	require.True(t, ok)
	require.Zero(t, job.State.NextRunAt)
	require.False(t, job.Enabled)
}

func TestService_StartupRecoveryFailure(t *testing.T) {
	ctx := context.Background()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	listErr := errors.New("list failed")
	service := newAutomationTestService(t, automationStoreStub{
		Store:   storememory.NewStore(),
		listErr: listErr,
	}, clock, &automationRunnerStub{})

	require.ErrorIs(t, service.Start(ctx), listErr)

	baseStore := storememory.NewStore()
	_, err := baseStore.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		State: JobState{
			RunningAt: clock.Now().Add(-2 * time.Hour),
		},
	})
	require.NoError(t, err)
	patchErr := errors.New("patch failed")
	service = newAutomationTestService(t, automationStoreStub{
		Store:    baseStore,
		patchErr: patchErr,
	}, clock, &automationRunnerStub{})
	require.ErrorIs(t, service.Start(ctx), patchErr)
	require.False(t, service.started)
	require.Nil(t, service.ctx)
	require.Nil(t, service.cancel)
	require.Nil(t, service.done)
}

func TestService_StartupRecoveryDisablesRepeatedBadSchedules(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service, err := NewService(ServiceOptions{
		Store:                      store,
		Runner:                     &automationRunnerStub{},
		Now:                        clock.Now,
		DisableAfterScheduleErrors: 1,
	})
	require.NoError(t, err)

	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
	})
	require.NoError(t, err)

	require.NoError(t, service.Start(ctx))
	require.NoError(t, service.Stop())

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, job.Enabled)
	require.Equal(t, "automation cron schedule expression is required", job.State.LastError)
}

func TestService_StartRecordsSchedulePatchFailure(t *testing.T) {
	ctx := context.Background()
	baseStore := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	_, err := baseStore.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
	})
	require.NoError(t, err)
	patchErr := errors.New("patch failed")
	service := newAutomationTestService(t, automationStoreStub{
		Store:    baseStore,
		patchErr: patchErr,
	}, clock, &automationRunnerStub{})

	require.NoError(t, service.Start(ctx))
	require.NoError(t, service.Stop())
}

func TestService_ObservabilityRecordsLifecycleEvents(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	logger := &automationLoggerStub{}
	tracer := &automationTracerStub{}
	service, err := NewService(ServiceOptions{
		Store:  store,
		Runner: &automationRunnerStub{},
		Logger: logger,
		Tracer: tracer,
		Now:    clock.Now,
	})
	require.NoError(t, err)

	_, err = service.Add(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
	})
	require.NoError(t, err)
	_, err = service.Run(ctx, testServiceJobA)
	require.NoError(t, err)
	require.NoError(t, service.Start(ctx))
	require.NoError(t, service.Stop())

	require.Contains(t, logger.Messages(), "automation job started")
	require.Contains(t, logger.Messages(), "automation job finished")
	require.Contains(t, logger.Messages(), "automation scheduler started")
	require.Contains(t, logger.Messages(), "automation scheduler stopped")
	require.Contains(t, tracer.EventNames(), automationEventStarted)
	require.Contains(t, tracer.EventNames(), automationEventFinished)
	require.Contains(t, tracer.EventNames(), automationEventSvcStarted)
	require.Contains(t, tracer.EventNames(), automationEventSvcStopped)
}

func TestService_StatusAndTimerHelpers(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	_, err := store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		State: JobState{NextRunAt: clock.Now().Add(5 * time.Millisecond)},
	})
	require.NoError(t, err)
	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobB,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		State: JobState{RunningAt: clock.Now(), NextRunAt: clock.Now().Add(-time.Hour)},
	})
	require.NoError(t, err)

	sleep := service.nextSleep(ctx)
	require.Equal(t, 5*time.Millisecond, sleep)

	status, err := service.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, status.JobCount)
	require.Equal(t, 1, status.RunningCount)

	job, ok, err := store.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	job.State.NextRunAt = clock.Now().Add(-time.Second)
	_, err = store.PatchJob(ctx, JobPatch{ID: job.ID, State: &job.State})
	require.NoError(t, err)
	require.Zero(t, service.nextSleep(ctx))

	logger := &automationLoggerStub{}
	tracer := &automationTracerStub{}
	service.logger = logger
	service.tracer = tracer
	service.record(ctx, "debug", "debug message", "debug.event", nil)
	service.record(ctx, "warn", "warn message", "warn.event", nil)
	service.record(ctx, "error", "error message", "error.event", nil)
	require.Contains(t, logger.Messages(), "debug message")
	require.Contains(t, logger.Messages(), "warn message")
	require.Contains(t, logger.Messages(), "error message")
	require.Contains(t, tracer.EventNames(), "debug.event")
	require.Contains(t, tracer.EventNames(), "warn.event")
	require.Contains(t, tracer.EventNames(), "error.event")
	service.notifyWake()
	(*Service)(nil).notifyWake()
	require.False(t, (*Service)(nil).getNow().IsZero())

	stopTimer(nil)
	timer := time.NewTimer(time.Nanosecond)
	<-timer.C
	stopTimer(timer)
}

func TestService_StoreControlErrors(t *testing.T) {
	ctx := context.Background()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	listErr := errors.New("list failed")
	service := newAutomationTestService(t, automationStoreStub{
		Store:   storememory.NewStore(),
		listErr: listErr,
	}, clock, &automationRunnerStub{})

	_, err := service.Status(ctx)
	require.ErrorIs(t, err, listErr)

	_, err = service.List(ctx, JobQuery{})
	require.ErrorIs(t, err, listErr)

	deleteErr := errors.New("delete failed")
	service = newAutomationTestService(t, automationStoreStub{
		Store:     storememory.NewStore(),
		deleteErr: deleteErr,
	}, clock, &automationRunnerStub{})
	require.ErrorIs(t, service.Remove(ctx, testServiceJobA), deleteErr)
}

func TestService_ExecuteDueJobsHandlesFailuresAndConflicts(t *testing.T) {
	ctx := context.Background()
	baseStore := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	logger := &automationLoggerStub{}
	service, err := NewService(ServiceOptions{
		Store:  automationStoreStub{Store: baseStore, listErr: errors.New("list failed")},
		Runner: &automationRunnerStub{},
		Logger: logger,
		Now:    clock.Now,
	})
	require.NoError(t, err)
	service.executeDueJobs(ctx)
	require.Contains(t, logger.Messages(), "automation scheduler failed to list jobs")

	_, err = baseStore.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
	})
	require.NoError(t, err)
	service = newAutomationTestService(t, baseStore, clock, &automationRunnerStub{})
	service.executeDueJobs(ctx)
	job, ok, err := baseStore.GetJob(ctx, testServiceJobA)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "automation cron schedule expression is required", job.State.LastError)

	_, err = baseStore.CreateJob(ctx, Job{
		ID:      testServiceJobB,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
		State: JobState{
			NextRunAt: clock.Now(),
			RunningAt: clock.Now(),
		},
	})
	require.NoError(t, err)
	service = newAutomationTestService(t, baseStore, clock, &automationRunnerStub{})
	service.executeDueJobs(ctx)
	runs, err := baseStore.ListRuns(ctx, RunQuery{JobID: testServiceJobB})
	require.NoError(t, err)
	require.Empty(t, runs.Runs)

	_, err = baseStore.CreateJob(ctx, Job{
		ID:      testServiceJobC,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleAt,
			At:   clock.Now(),
		},
		State: JobState{NextRunAt: clock.Now()},
	})
	require.NoError(t, err)
	logger = &automationLoggerStub{}
	service, err = NewService(ServiceOptions{
		Store:  automationStoreStub{Store: baseStore, patchErr: errors.New("patch failed")},
		Runner: &automationRunnerStub{},
		Logger: logger,
		Now:    clock.Now,
	})
	require.NoError(t, err)
	service.executeDueJobs(ctx)
	require.Contains(t, logger.Messages(), "automation scheduler skipped running job")
}

func TestService_PrivateScheduleAndRunningBranches(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	disabled := Job{
		ID:      testServiceJobA,
		Enabled: false,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
	}
	repaired, err := service.repairJobSchedule(ctx, disabled, clock.Now(), false, false)
	require.NoError(t, err)
	require.False(t, repaired.Enabled)

	future := Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		State: JobState{NextRunAt: clock.Now().Add(time.Hour)},
	}
	repaired, err = service.repairJobSchedule(ctx, future, clock.Now(), false, false)
	require.NoError(t, err)
	require.Equal(t, future.State.NextRunAt, repaired.State.NextRunAt)

	prepared, err := service.prepareJobSchedule(disabled, clock.Now())
	require.NoError(t, err)
	require.False(t, prepared.Enabled)

	_, err = service.skipMissedJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
		State: JobState{NextRunAt: clock.Now().Add(-time.Minute)},
	}, clock.Now())
	require.EqualError(t, err, "automation cron schedule expression is required")

	_, err = newAutomationTestService(t, automationStoreStub{
		Store:    store,
		patchErr: errors.New("patch failed"),
	}, clock, &automationRunnerStub{}).skipMissedJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		State: JobState{NextRunAt: clock.Now().Add(-time.Minute)},
	}, clock.Now())
	require.EqualError(t, err, "patch failed")
}

func TestService_MarkJobRunningBranches(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	getErr := errors.New("get failed")
	err := newAutomationTestService(t, automationStoreStub{
		Store:  store,
		getErr: getErr,
	}, clock, &automationRunnerStub{}).markJobRunning(ctx, Job{ID: testServiceJobA}, clock.Now())
	require.ErrorIs(t, err, getErr)

	err = service.markJobRunning(ctx, Job{ID: testServiceJobA}, clock.Now())
	require.EqualError(t, err, "automation job not found")

	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
		State: JobState{RunningAt: clock.Now()},
	})
	require.NoError(t, err)
	err = service.markJobRunning(ctx, Job{ID: testServiceJobA}, clock.Now())
	require.EqualError(t, err, "automation job is already running")

	patchErr := errors.New("patch failed")
	err = newAutomationTestService(t, automationStoreStub{
		Store:    store,
		patchErr: patchErr,
	}, clock, &automationRunnerStub{}).markJobRunning(ctx, Job{ID: testServiceJobA}, clock.Now())
	require.EqualError(t, err, "automation job is already running")

	store = storememory.NewStore()
	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobB,
		Enabled: true,
		Schedule: Schedule{
			Kind:  ScheduleEvery,
			Every: time.Hour,
		},
	})
	require.NoError(t, err)
	err = newAutomationTestService(t, automationStoreStub{
		Store:    store,
		patchErr: patchErr,
	}, clock, &automationRunnerStub{}).markJobRunning(ctx, Job{ID: testServiceJobB}, clock.Now())
	require.ErrorIs(t, err, patchErr)
}

func TestService_FinishJobRunBranches(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	_, err := service.finishJobRun(ctx, Job{ID: testServiceJobA}, Run{}, nil, time.Time{})
	require.EqualError(t, err, "automation job not found")

	getErr := errors.New("get failed")
	_, err = newAutomationTestService(t, automationStoreStub{
		Store:  store,
		getErr: getErr,
	}, clock, &automationRunnerStub{}).finishJobRun(ctx, Job{ID: testServiceJobA}, Run{}, nil, time.Time{})
	require.ErrorIs(t, err, getErr)

	_, err = store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
		State: JobState{RunningAt: clock.Now()},
	})
	require.NoError(t, err)
	updated, err := service.finishJobRun(ctx, Job{ID: testServiceJobA}, Run{
		EndedAt:  clock.Now(),
		Status:   RunStatusOK,
		Duration: time.Second,
	}, nil, time.Time{})
	require.NoError(t, err)
	require.Zero(t, updated.State.RunningAt)
	require.Equal(t, RunStatusOK, updated.State.LastStatus)
	require.Zero(t, updated.State.NextRunAt)
}

func TestService_UpdateCanDisableInvalidSchedule(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	clock := newAutomationTestClock(time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC))
	service := newAutomationTestService(t, store, clock, &automationRunnerStub{})

	_, err := store.CreateJob(ctx, Job{
		ID:      testServiceJobA,
		Enabled: true,
		Schedule: Schedule{
			Kind: ScheduleCron,
		},
	})
	require.NoError(t, err)

	disabled := false
	state := JobState{LastError: "manual"}
	updated, err := service.Update(ctx, JobPatch{
		ID:      testServiceJobA,
		Enabled: &disabled,
		State:   &state,
	})
	require.NoError(t, err)
	require.False(t, updated.Enabled)
	require.Equal(t, "manual", updated.State.LastError)
}
