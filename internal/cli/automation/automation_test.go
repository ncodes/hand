package automation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	coreautomation "github.com/wandxy/morph/internal/automation"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/profile"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
)

type automationCommandClientStub struct {
	api *automationCommandAPIStub
}

func (s *automationCommandClientStub) Close() error {
	return nil
}

func (s *automationCommandClientStub) AutomationAPI() rpcclient.AutomationAPI {
	return s.api
}

type automationCommandAPIStub struct {
	status    coreautomation.Status
	added     coreautomation.Job
	patch     coreautomation.JobPatch
	removedID string
	run       coreautomation.Run
	runQuery  coreautomation.RunQuery
	runs      []coreautomation.Run
	statusErr error
	listErr   error
	addErr    error
	updateErr error
	removeErr error
	runErr    error
	runsErr   error
}

func (s *automationCommandAPIStub) Status(context.Context) (coreautomation.Status, error) {
	if s.statusErr != nil {
		return coreautomation.Status{}, s.statusErr
	}
	return s.status, nil
}

func (s *automationCommandAPIStub) List(context.Context, coreautomation.JobQuery) (coreautomation.JobList, error) {
	if s.listErr != nil {
		return coreautomation.JobList{}, s.listErr
	}
	return coreautomation.JobList{Jobs: []coreautomation.Job{s.added}}, nil
}

func (s *automationCommandAPIStub) Add(_ context.Context, job coreautomation.Job) (coreautomation.Job, error) {
	if s.addErr != nil {
		return coreautomation.Job{}, s.addErr
	}
	s.added = job
	if s.added.ID == "" {
		s.added.ID = testAutomationCommandJobID
	}
	return s.added, nil
}

func (s *automationCommandAPIStub) Update(_ context.Context, patch coreautomation.JobPatch) (coreautomation.Job, error) {
	if s.updateErr != nil {
		return coreautomation.Job{}, s.updateErr
	}
	s.patch = patch
	enabled := false
	if patch.Enabled != nil {
		enabled = *patch.Enabled
	}
	return coreautomation.Job{ID: patch.ID, Enabled: enabled}, nil
}

func (s *automationCommandAPIStub) Remove(_ context.Context, id string) error {
	if s.removeErr != nil {
		return s.removeErr
	}
	s.removedID = id
	return nil
}

func (s *automationCommandAPIStub) Run(_ context.Context, id string) (coreautomation.Run, error) {
	if s.runErr != nil {
		return coreautomation.Run{}, s.runErr
	}
	if s.run.ID == "" {
		s.run = coreautomation.Run{ID: testAutomationCommandRunID, JobID: id, Status: coreautomation.RunStatusOK}
	}
	return s.run, nil
}

func (s *automationCommandAPIStub) Runs(_ context.Context, query coreautomation.RunQuery) (coreautomation.RunList, error) {
	if s.runsErr != nil {
		return coreautomation.RunList{}, s.runsErr
	}
	s.runQuery = query
	return coreautomation.RunList{Runs: s.runs}, nil
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

var (
	testAutomationCommandJobID = "auto_projectaprojectaproje"
	testAutomationCommandRunID = "autorun_projectaprojectaproj"
)

func newTestCommand() *cli.Command {
	return &cli.Command{
		Name: "automation",
		Commands: []*cli.Command{
			NewStatusCommand(),
			NewListCommand(),
			NewAddCommand(),
			NewUpdateCommand(),
			NewPauseCommand(),
			NewResumeCommand(),
			NewRunCommand(),
			NewRemoveCommand(),
			NewRunsCommand(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func TestNewCommand_StatusCallsRPC(t *testing.T) {
	api, output := setupAutomationCommandTest(t)
	api.status = coreautomation.Status{
		Running:      true,
		JobCount:     2,
		RunningCount: 1,
		StartedAt:    time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC),
		NextWakeAt:   time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC),
	}

	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "status"}))

	require.Equal(t, "running=true jobs=2 running_jobs=1 started_at=2026-07-05T08:00:00Z next_wake_at=2026-07-05T09:00:00Z\n", output.String())
}

func TestNewCommand_AddParsesJobFlags(t *testing.T) {
	api, output := setupAutomationCommandTest(t)

	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "add",
		"--name", "Daily",
		"--schedule", "every 1h",
		"--prompt", "summarize",
		"--tool-group", "memory",
		"--delivery", "local",
	}))

	require.Equal(t, testAutomationCommandJobID+"\n", output.String())
	require.Equal(t, "Daily", api.added.Name)
	require.True(t, api.added.Enabled)
	require.Equal(t, time.Hour, api.added.Schedule.Every)
	require.Equal(t, "summarize", api.added.Payload.Prompt)
	require.Equal(t, []string{"memory"}, api.added.Payload.ToolGroups)
	require.Equal(t, coreautomation.DeliveryLocal, api.added.Delivery.Mode)
}

func TestNewCommand_ListAndUpdateCallRPC(t *testing.T) {
	api, output := setupAutomationCommandTest(t)
	api.added = coreautomation.Job{
		ID:      testAutomationCommandJobID,
		Name:    "Daily",
		Enabled: true,
		Schedule: coreautomation.Schedule{
			Kind:  coreautomation.ScheduleEvery,
			Every: time.Hour,
		},
		State: coreautomation.JobState{NextRunAt: time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)},
	}

	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "list", "--all"}))
	require.Contains(t, output.String(), `name="Daily"`)
	require.Contains(t, output.String(), "schedule=every 1h0m0s")

	output.Reset()
	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "update", testAutomationCommandJobID,
		"--name", "Renamed",
		"--schedule", "every 2h",
		"--prompt", "next",
	}))
	require.Equal(t, testAutomationCommandJobID+"\n", output.String())
	require.Equal(t, testAutomationCommandJobID, api.patch.ID)
	require.NotNil(t, api.patch.Name)
	require.Equal(t, "Renamed", *api.patch.Name)
	require.NotNil(t, api.patch.Schedule)
	require.Equal(t, 2*time.Hour, api.patch.Schedule.Every)
	require.NotNil(t, api.patch.Payload)
	require.Equal(t, "next", api.patch.Payload.Prompt)
}

func TestNewCommand_PauseResumeRunRemoveAndRunsCallRPC(t *testing.T) {
	api, output := setupAutomationCommandTest(t)

	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "pause", testAutomationCommandJobID}))
	require.NotNil(t, api.patch.Enabled)
	require.False(t, *api.patch.Enabled)
	require.Contains(t, output.String(), "enabled=false")

	output.Reset()
	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "resume", testAutomationCommandJobID}))
	require.NotNil(t, api.patch.Enabled)
	require.True(t, *api.patch.Enabled)
	require.Contains(t, output.String(), "enabled=true")

	output.Reset()
	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "run", testAutomationCommandJobID}))
	require.Contains(t, output.String(), testAutomationCommandRunID)

	output.Reset()
	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation", "remove", testAutomationCommandJobID}))
	require.Equal(t, testAutomationCommandJobID, api.removedID)
	require.Equal(t, testAutomationCommandJobID+"\n", output.String())

	output.Reset()
	api.runs = []coreautomation.Run{{ID: testAutomationCommandRunID, JobID: testAutomationCommandJobID, Status: coreautomation.RunStatusError}}
	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "runs",
		"--job", testAutomationCommandJobID,
		"--status", "error",
	}))
	require.Equal(t, testAutomationCommandJobID, api.runQuery.JobID)
	require.Equal(t, []coreautomation.RunStatus{coreautomation.RunStatusError}, api.runQuery.Status)
	require.Contains(t, output.String(), "status=error")
}

func TestCommandHelpersCoverScheduleAndArgumentBranches(t *testing.T) {
	require.Equal(t, "at 2026-07-05T08:00:00Z", formatSchedule(coreautomation.Schedule{
		Kind: coreautomation.ScheduleAt,
		At:   time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC),
	}))
	require.Equal(t, "cron 0 8 * * *", formatSchedule(coreautomation.Schedule{
		Kind: coreautomation.ScheduleCron,
		Cron: "0 8 * * *",
	}))
	require.Equal(t, "", formatSchedule(coreautomation.Schedule{}))
	require.Nil(t, parseRunStatuses(""))
}

func TestSetOutputHandlesNilAndWriter(t *testing.T) {
	previous := SetOutput(nil)
	t.Cleanup(func() { SetOutput(previous) })
	require.Equal(t, io.Discard, automationOutput)

	output := &bytes.Buffer{}
	require.Equal(t, io.Discard, SetOutput(output))
	require.Same(t, output, automationOutput)
}

func TestTestCommand_ShowsHelpForMissingSubcommand(t *testing.T) {
	_, _ = setupAutomationCommandTest(t)

	require.NoError(t, newTestCommand().Run(context.Background(), []string{"automation"}))
}

func TestNewCommand_PropagatesActionErrors(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*automationCommandAPIStub)
	}{
		{
			name: "status rpc",
			args: []string{"automation", "status"},
			mutate: func(api *automationCommandAPIStub) {
				api.statusErr = errors.New("status failed")
			},
		},
		{
			name: "status write",
			args: []string{"automation", "status"},
			mutate: func(*automationCommandAPIStub) {
				automationOutput = errorWriter{}
			},
		},
		{
			name: "list rpc",
			args: []string{"automation", "list"},
			mutate: func(api *automationCommandAPIStub) {
				api.listErr = errors.New("list failed")
			},
		},
		{
			name: "list write",
			args: []string{"automation", "list"},
			mutate: func(api *automationCommandAPIStub) {
				api.added = coreautomation.Job{ID: testAutomationCommandJobID}
				automationOutput = errorWriter{}
			},
		},
		{
			name: "add parse",
			args: []string{"automation", "add", "--prompt", "summarize"},
		},
		{
			name: "add rpc",
			args: []string{"automation", "add", "--schedule", "every 1h", "--prompt", "summarize"},
			mutate: func(api *automationCommandAPIStub) {
				api.addErr = errors.New("add failed")
			},
		},
		{
			name: "add write",
			args: []string{"automation", "add", "--schedule", "every 1h", "--prompt", "summarize"},
			mutate: func(*automationCommandAPIStub) {
				automationOutput = errorWriter{}
			},
		},
		{
			name: "update missing id",
			args: []string{"automation", "update"},
		},
		{
			name: "update parse",
			args: []string{"automation", "update", testAutomationCommandJobID, "--schedule", "not-a-schedule"},
		},
		{
			name: "update rpc",
			args: []string{"automation", "update", testAutomationCommandJobID, "--name", "Renamed"},
			mutate: func(api *automationCommandAPIStub) {
				api.updateErr = errors.New("update failed")
			},
		},
		{
			name: "update write",
			args: []string{"automation", "update", testAutomationCommandJobID, "--name", "Renamed"},
			mutate: func(*automationCommandAPIStub) {
				automationOutput = errorWriter{}
			},
		},
		{
			name: "pause missing id",
			args: []string{"automation", "pause"},
		},
		{
			name: "pause rpc",
			args: []string{"automation", "pause", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.updateErr = errors.New("pause failed")
			},
		},
		{
			name: "pause write",
			args: []string{"automation", "pause", testAutomationCommandJobID},
			mutate: func(*automationCommandAPIStub) {
				automationOutput = errorWriter{}
			},
		},
		{
			name: "run missing id",
			args: []string{"automation", "run"},
		},
		{
			name: "run rpc",
			args: []string{"automation", "run", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.runErr = errors.New("run failed")
			},
		},
		{
			name: "run write",
			args: []string{"automation", "run", testAutomationCommandJobID},
			mutate: func(*automationCommandAPIStub) {
				automationOutput = errorWriter{}
			},
		},
		{
			name: "remove missing id",
			args: []string{"automation", "remove"},
		},
		{
			name: "remove rpc",
			args: []string{"automation", "remove", testAutomationCommandJobID},
			mutate: func(api *automationCommandAPIStub) {
				api.removeErr = errors.New("remove failed")
			},
		},
		{
			name: "remove write",
			args: []string{"automation", "remove", testAutomationCommandJobID},
			mutate: func(*automationCommandAPIStub) {
				automationOutput = errorWriter{}
			},
		},
		{
			name: "runs rpc",
			args: []string{"automation", "runs"},
			mutate: func(api *automationCommandAPIStub) {
				api.runsErr = errors.New("runs failed")
			},
		},
		{
			name: "runs write",
			args: []string{"automation", "runs"},
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

func TestNewCommand_PropagatesClientCreationError(t *testing.T) {
	_, _ = setupAutomationCommandTest(t)
	expected := errors.New("client failed")
	newClient = func(context.Context, *config.Config) (automationClient, error) {
		return nil, expected
	}

	err := newTestCommand().Run(context.Background(), []string{"automation", "status"})

	require.ErrorIs(t, err, expected)
}

func TestNewCommand_PropagatesClientCreationErrorForEveryAction(t *testing.T) {
	tests := [][]string{
		{"automation", "list"},
		{"automation", "add", "--schedule", "every 1h", "--prompt", "summarize"},
		{"automation", "update", testAutomationCommandJobID, "--name", "Renamed"},
		{"automation", "pause", testAutomationCommandJobID},
		{"automation", "resume", testAutomationCommandJobID},
		{"automation", "run", testAutomationCommandJobID},
		{"automation", "remove", testAutomationCommandJobID},
		{"automation", "runs"},
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

func TestLoadAutomationConfig_ReturnsLoadError(t *testing.T) {
	originalProfile := profile.Active()
	t.Cleanup(func() { profile.SetActive(originalProfile) })
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte("name: ["), 0o600))
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "test", HomeDir: home}))

	_, err := loadAutomationConfig(nil)

	require.ErrorContains(t, err, "failed to parse config file")
}

func TestGetAutomationAPI_ReturnsLoadError(t *testing.T) {
	originalProfile := profile.Active()
	t.Cleanup(func() { profile.SetActive(originalProfile) })
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte("name: ["), 0o600))
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "test", HomeDir: home}))

	api, closeClient, err := getAutomationAPI(context.Background(), nil)

	require.Nil(t, api)
	require.NotNil(t, closeClient)
	require.ErrorContains(t, err, "failed to parse config file")
}

func TestPatchFromCommand_CoversOptionalFlags(t *testing.T) {
	_, _ = setupAutomationCommandTest(t)
	cmd := newTestCommand()
	require.NoError(t, cmd.Run(context.Background(), []string{
		"automation", "update", testAutomationCommandJobID,
		"--description", "desc",
		"--system-event", "wake",
		"--delivery", "local",
		"--profile", "work",
		"--session-target", "main",
		"--delete-after-run",
	}))
}

func TestJobFromCommand_CoversDisabledAndSystemEvent(t *testing.T) {
	api, _ := setupAutomationCommandTest(t)

	require.NoError(t, newTestCommand().Run(context.Background(), []string{
		"automation", "add",
		"--disabled",
		"--schedule", "every 1h",
		"--system-event", "wake",
		"--no-timeout",
		"--max-runtime", "2m",
		"--max-iterations", "3",
		"--retry-attempts", "2",
		"--retry-backoff", "1s",
		"--retry-max-delay", "5s",
		"--base-url", "https://example.test",
		"--provider", "openai",
		"--model", "gpt-test",
		"--channel", "telegram",
		"--target", "user",
		"--thread", "thread",
		"--webhook-url", "https://hook.test",
		"--best-effort",
		"--delete-after-run",
	}))

	require.False(t, api.added.Enabled)
	require.Equal(t, coreautomation.PayloadSystemEvent, api.added.Payload.Kind)
	require.Equal(t, "wake", api.added.Payload.SystemEvent)
	require.True(t, api.added.Payload.NoTimeout)
	require.True(t, api.added.Delivery.BestEffort)
	require.True(t, api.added.DeleteAfterRun)
}

func setupAutomationCommandTest(t *testing.T) (*automationCommandAPIStub, *bytes.Buffer) {
	t.Helper()
	t.Setenv("MORPH_RPC_ADDRESS", "")
	t.Setenv("MORPH_RPC_PORT", "")

	originalProfile := profile.Active()
	originalNewClient := newClient
	originalOutput := automationOutput
	t.Cleanup(func() {
		profile.SetActive(originalProfile)
		newClient = originalNewClient
		automationOutput = originalOutput
	})
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "test", HomeDir: t.TempDir()}))

	output := &bytes.Buffer{}
	automationOutput = output
	api := &automationCommandAPIStub{}
	newClient = func(context.Context, *config.Config) (automationClient, error) {
		return &automationCommandClientStub{api: api}, nil
	}

	return api, output
}
