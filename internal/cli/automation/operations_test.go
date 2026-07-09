package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreautomation "github.com/wandxy/morph/internal/automation"
	"github.com/wandxy/morph/internal/config"
)

type failAfterWriter struct {
	writes    int
	failAfter int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.failAfter {
		return 0, errors.New("write failed")
	}

	return len(p), nil
}

func TestNewCommand_DiagnoseInspectAndRecoverCallRPC(t *testing.T) {
	api, output := setupAutomationCommandTest(t)
	runningAt := time.Now().UTC().Add(-20 * time.Minute)
	api.jobs = []coreautomation.Job{{
		ID:       testAutomationCommandJobID,
		Enabled:  true,
		Schedule: coreautomation.Schedule{Kind: coreautomation.ScheduleEvery, Every: time.Hour},
		Delivery: coreautomation.Delivery{Mode: coreautomation.DeliveryWebhook},
		State:    coreautomation.JobState{RunningAt: runningAt},
	}}

	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "diagnose", "--all"}))
	require.True(t, api.jobQuery.IncludeDisabled)
	require.Contains(t, output.String(), "delivery_webhook_url_missing")

	output.Reset()
	api.runs = []coreautomation.Run{{
		ID:             testAutomationCommandRunID,
		JobID:          testAutomationCommandJobID,
		Status:         coreautomation.RunStatusError,
		Error:          "failed",
		SessionID:      "ses_projectaprojectaproje",
		DeliveryStatus: coreautomation.DeliveryStatusNotDelivered,
		DeliveryError:  "delivery failed",
	}}
	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "inspect", testAutomationCommandJobID,
	}))
	require.Equal(t, testAutomationCommandJobID, api.jobQuery.IDs[0])
	require.Contains(t, output.String(), "trace_session=ses_projectaprojectaproje")
	require.Contains(t, output.String(), "failure="+testAutomationCommandRunID)

	output.Reset()
	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "recover", "recompute-schedules",
	}))
	require.Contains(t, output.String(), "recomputed=1")

	output.Reset()
	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "recover", "clear-running", testAutomationCommandJobID,
	}))
	require.NotNil(t, api.patch.State)
	require.True(t, api.patch.State.RunningAt.IsZero())
	require.Contains(t, output.String(), "running=false")

	output.Reset()
	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "recover", "rerun-failed", testAutomationCommandJobID,
	}))
	require.Equal(t, []coreautomation.RunStatus{coreautomation.RunStatusError}, api.runQuery.Status)
	require.Contains(t, output.String(), testAutomationCommandRunID)
}

func TestNewCommand_DiagnoseReportsHealthyState(t *testing.T) {
	api, output := setupAutomationCommandTest(t)
	api.jobs = []coreautomation.Job{{
		ID:       testAutomationCommandJobID,
		Enabled:  true,
		Schedule: coreautomation.Schedule{Kind: coreautomation.ScheduleEvery, Every: time.Hour},
		Delivery: coreautomation.Delivery{Mode: coreautomation.DeliveryLocal},
	}}

	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "diagnose"}))

	require.Equal(t, "automation diagnostics passed\n", output.String())
}

func TestNewCommand_PropagatesOperationActionErrors(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*automationCommandAPIStub)
	}{
		{
			name: "diagnose rpc",
			args: []string{"automation", "diagnose"},
			mutate: func(api *automationCommandAPIStub) {
				api.listErr = errors.New("diagnose failed")
			},
		},
		{
			name: "diagnose write pass",
			args: []string{"automation", "diagnose"},
			mutate: func(*automationCommandAPIStub) {
				automationOutput = errorWriter{}
			},
		},
		{
			name: "diagnose write finding",
			args: []string{"automation", "diagnose"},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{{
					ID:       testAutomationCommandJobID,
					Enabled:  true,
					Schedule: coreautomation.Schedule{Kind: coreautomation.ScheduleEvery, Every: time.Hour},
					Delivery: coreautomation.Delivery{Mode: coreautomation.DeliveryWebhook},
				}}
				automationOutput = errorWriter{}
			},
		},
		{
			name: "inspect missing id",
			args: []string{"automation", "inspect"},
		},
		{
			name: "inspect list rpc",
			args: []string{"automation", "inspect", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.listErr = errors.New("inspect failed")
			},
		},
		{
			name: "inspect missing job",
			args: []string{"automation", "inspect", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{}
				api.added = coreautomation.Job{}
			},
		},
		{
			name: "inspect runs rpc",
			args: []string{"automation", "inspect", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{{ID: testAutomationCommandJobID}}
				api.runsErr = errors.New("runs failed")
			},
		},
		{
			name: "inspect write",
			args: []string{"automation", "inspect", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{{ID: testAutomationCommandJobID}}
				automationOutput = errorWriter{}
			},
		},
		{
			name: "recover recompute list rpc",
			args: []string{"automation", "recover", "recompute-schedules"},
			mutate: func(api *automationCommandAPIStub) {
				api.listErr = errors.New("list failed")
			},
		},
		{
			name: "recover recompute update rpc",
			args: []string{"automation", "recover", "recompute-schedules"},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{{ID: testAutomationCommandJobID, Enabled: true}}
				api.updateErr = errors.New("update failed")
			},
		},
		{
			name: "recover recompute write",
			args: []string{"automation", "recover", "recompute-schedules"},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{{ID: testAutomationCommandJobID, Enabled: false}}
				automationOutput = errorWriter{}
			},
		},
		{
			name: "recover clear missing id",
			args: []string{"automation", "recover", "clear-running"},
		},
		{
			name: "recover clear list rpc",
			args: []string{"automation", "recover", "clear-running", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.listErr = errors.New("list failed")
			},
		},
		{
			name: "recover clear missing job",
			args: []string{"automation", "recover", "clear-running", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{}
				api.added = coreautomation.Job{}
			},
		},
		{
			name: "recover clear update rpc",
			args: []string{"automation", "recover", "clear-running", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{{ID: testAutomationCommandJobID}}
				api.updateErr = errors.New("update failed")
			},
		},
		{
			name: "recover clear write",
			args: []string{"automation", "recover", "clear-running", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.jobs = []coreautomation.Job{{ID: testAutomationCommandJobID}}
				automationOutput = errorWriter{}
			},
		},
		{
			name: "recover rerun missing id",
			args: []string{"automation", "recover", "rerun-failed"},
		},
		{
			name: "recover rerun runs rpc",
			args: []string{"automation", "recover", "rerun-failed", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.runsErr = errors.New("runs failed")
			},
		},
		{
			name: "recover rerun no failures",
			args: []string{"automation", "recover", "rerun-failed", testAutomationCommandJobID},
		},
		{
			name: "recover rerun rpc",
			args: []string{"automation", "recover", "rerun-failed", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.runs = []coreautomation.Run{{ID: testAutomationCommandRunID}}
				api.runErr = errors.New("run failed")
			},
		},
		{
			name: "recover rerun write",
			args: []string{"automation", "recover", "rerun-failed", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.runs = []coreautomation.Run{{ID: testAutomationCommandRunID}}
				automationOutput = errorWriter{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api, _ := setupAutomationCommandTest(t)
			if test.mutate != nil {
				test.mutate(api)
			}

			require.Error(t, newTestCommand().Run(context.Background(), test.args))
		})
	}
}

func TestNewCommand_PropagatesOperationClientCreationErrors(t *testing.T) {
	tests := [][]string{
		{"automation", "diagnose"},
		{"automation", "inspect", testAutomationCommandJobID},
		{"automation", "recover", "recompute-schedules"},
		{"automation", "recover", "clear-running", testAutomationCommandJobID},
		{"automation", "recover", "rerun-failed", testAutomationCommandJobID},
	}

	for _, args := range tests {
		t.Run(args[1], func(t *testing.T) {
			_, _ = setupAutomationCommandTest(t)
			expected := errors.New("client failed")
			newClient = func(context.Context, *config.Config) (automationClient, error) {
				return nil, expected
			}

			err := newTestCommand().Run(context.Background(), args)

			require.ErrorIs(t, err, expected)
		})
	}
}

func TestNewRecoverCommand_ShowsHelpForMissingSubcommand(t *testing.T) {
	_, _ = setupAutomationCommandTest(t)

	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "recover"}))
}

func TestWriteInspection_CoversNoRunAndWriteErrors(t *testing.T) {
	_, output := setupAutomationCommandTest(t)
	require.NoError(t, writeInspection(coreautomation.RunInspection{
		Job: coreautomation.Job{ID: testAutomationCommandJobID},
	}))
	require.Contains(t, output.String(), "last_run=-")

	automationOutput = &failAfterWriter{failAfter: 1}
	err := writeInspection(coreautomation.RunInspection{
		Job:     coreautomation.Job{ID: testAutomationCommandJobID},
		LastRun: coreautomation.Run{ID: testAutomationCommandRunID},
	})
	require.Error(t, err)

	automationOutput = &failAfterWriter{failAfter: 2}
	err = writeInspection(coreautomation.RunInspection{
		Job:     coreautomation.Job{ID: testAutomationCommandJobID},
		LastRun: coreautomation.Run{ID: testAutomationCommandRunID},
	})
	require.Error(t, err)

	automationOutput = &failAfterWriter{failAfter: 3}
	err = writeInspection(coreautomation.RunInspection{
		Job:            coreautomation.Job{ID: testAutomationCommandJobID},
		LastRun:        coreautomation.Run{ID: testAutomationCommandRunID},
		RecentFailures: []coreautomation.Run{{ID: testAutomationCommandRunID}},
	})
	require.Error(t, err)
}
