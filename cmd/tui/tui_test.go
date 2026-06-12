package tui

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	handpb "github.com/wandxy/hand/internal/rpc/proto"
	storage "github.com/wandxy/hand/internal/state/core"
	tui "github.com/wandxy/hand/internal/tui/app"
	"google.golang.org/grpc"
)

type fakeProgram struct {
	model tea.Model
	err   error
	ran   bool
}

func (program *fakeProgram) Run() (tea.Model, error) {
	program.ran = true
	return program.model, program.err
}

type fakeModel struct{}

func (fakeModel) Init() tea.Cmd {
	return nil
}

func (fakeModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return fakeModel{}, nil
}

func (fakeModel) View() tea.View {
	return tea.NewView("")
}

func TestRun_StartsProgram(t *testing.T) {
	originalNewProgram := newProgram
	originalLoadCommandModel := loadCommandModel
	t.Cleanup(func() {
		newProgram = originalNewProgram
		loadCommandModel = originalLoadCommandModel
	})

	program := &fakeProgram{}
	newProgram = func(model tea.Model) programRunner {
		program.model = model
		return program
	}
	loadCommandModel = func(context.Context, *cli.Command) (tea.Model, func(), error) {
		return fakeModel{}, func() {}, nil
	}

	err := Run(context.Background(), &cli.Command{})

	require.NoError(t, err)
	require.True(t, program.ran)
	require.IsType(t, fakeModel{}, program.model)
}

func TestRun_ReturnsProgramError(t *testing.T) {
	originalNewProgram := newProgram
	originalLoadCommandModel := loadCommandModel
	t.Cleanup(func() {
		newProgram = originalNewProgram
		loadCommandModel = originalLoadCommandModel
	})

	expectedErr := errors.New("terminal unavailable")
	newProgram = func(model tea.Model) programRunner {
		return &fakeProgram{model: model, err: expectedErr}
	}
	loadCommandModel = func(context.Context, *cli.Command) (tea.Model, func(), error) {
		return fakeModel{}, func() {}, nil
	}

	err := Run(context.Background(), &cli.Command{})

	require.ErrorIs(t, err, expectedErr)
}

func TestRun_ReturnsModelLoadError(t *testing.T) {
	originalLoadCommandModel := loadCommandModel
	t.Cleanup(func() {
		loadCommandModel = originalLoadCommandModel
	})

	expectedErr := errors.New("rpc unavailable")
	loadCommandModel = func(context.Context, *cli.Command) (tea.Model, func(), error) {
		return nil, nil, expectedErr
	}

	err := Run(context.Background(), &cli.Command{})

	require.ErrorIs(t, err, expectedErr)
}

func TestDefaultTUIFactories_ConstructProgramAndClient(t *testing.T) {
	runner := newProgram(fakeModel{})
	require.NotNil(t, runner)

	client, err := newTUIChatClient(context.Background(), &config.Config{
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 1},
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NoError(t, client.Close())
}

func TestLoadTUICommandModel_UsesConfiguredRPCClientAndCleanup(t *testing.T) {
	originalNewTUIChatClient := newTUIChatClient
	originalEnsureTUIDaemonRunning := ensureTUIDaemonRunning
	t.Cleanup(func() {
		newTUIChatClient = originalNewTUIChatClient
		ensureTUIDaemonRunning = originalEnsureTUIDaemonRunning
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: tui-agent
rpc:
  address: 127.0.0.2
  port: 45678
models:
tui:
  thinkingComposer: false
`), 0o600))

	client := &fakeTUIChatClient{}
	var gotRPC config.RPCConfig
	var gotEnsureRPC config.RPCConfig
	var gotEnsureProfile string
	ensureTUIDaemonRunning = func(
		_ context.Context,
		_ *cli.Command,
		cfg *config.Config,
		inputs handcli.ConfigInputs,
	) error {
		gotEnsureRPC = cfg.RPC
		gotEnsureProfile = inputs.Profile.Name
		return nil
	}
	newTUIChatClient = func(_ context.Context, cfg *config.Config) (tuiClient, error) {
		gotRPC = cfg.RPC
		return client, nil
	}

	var cleanup func()
	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		var err error
		_, cleanup, err = loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work"})

	require.NoError(t, err)
	require.Equal(t, config.RPCConfig{Address: "127.0.0.2", Port: 45678}, gotEnsureRPC)
	require.Equal(t, "work", gotEnsureProfile)
	require.Equal(t, config.RPCConfig{Address: "127.0.0.2", Port: 45678}, gotRPC)
	require.NotNil(t, cleanup)
	cleanup()
	require.True(t, client.closed)
}

func TestLoadTUICommandModel_ReturnsConfigLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, "bad-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("{"), 0o600))

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		_, _, err := loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--config", configPath})

	require.Error(t, err)
	require.ErrorContains(t, err, "yaml")
}

func TestLoadTUICommandModel_ReturnsRPCResolutionError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "runtime.json"), []byte("{"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: tui-agent
models:
`), 0o600))

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		_, _, err := loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work"})

	require.Error(t, err)
	require.ErrorContains(t, err, "parse runtime metadata")
}

func TestLoadTUICommandModel_ReturnsClientCreationError(t *testing.T) {
	originalNewTUIChatClient := newTUIChatClient
	originalEnsureTUIDaemonRunning := ensureTUIDaemonRunning
	t.Cleanup(func() {
		newTUIChatClient = originalNewTUIChatClient
		ensureTUIDaemonRunning = originalEnsureTUIDaemonRunning
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: tui-agent
rpc:
  address: 127.0.0.2
  port: 45678
models:
`), 0o600))

	expectedErr := errors.New("client unavailable")
	ensureTUIDaemonRunning = func(context.Context, *cli.Command, *config.Config, handcli.ConfigInputs) error {
		return nil
	}
	newTUIChatClient = func(context.Context, *config.Config) (tuiClient, error) {
		return nil, expectedErr
	}

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		_, _, err := loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work"})

	require.ErrorIs(t, err, expectedErr)
}

func TestLoadTUICommandModel_ReturnsDaemonBootstrapError(t *testing.T) {
	originalNewTUIChatClient := newTUIChatClient
	originalEnsureTUIDaemonRunning := ensureTUIDaemonRunning
	t.Cleanup(func() {
		newTUIChatClient = originalNewTUIChatClient
		ensureTUIDaemonRunning = originalEnsureTUIDaemonRunning
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: tui-agent
rpc:
  address: 127.0.0.2
  port: 45678
models:
`), 0o600))

	expectedErr := errors.New("daemon unavailable")
	ensureTUIDaemonRunning = func(context.Context, *cli.Command, *config.Config, handcli.ConfigInputs) error {
		return expectedErr
	}
	newTUIChatClient = func(context.Context, *config.Config) (tuiClient, error) {
		t.Fatal("client should not be created when daemon bootstrap fails")
		return nil, nil
	}

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		_, _, err := loadTUICommandModel(ctx, cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work"})

	require.ErrorIs(t, err, expectedErr)
}

func TestEnsureTUIDaemonRunning_ReturnsWhenRPCIsAlreadyReady(t *testing.T) {
	restore := replaceTUIDaemonBootstrapHooks(t)
	defer restore()

	checks := 0
	checkTUIDaemonRPC = func(context.Context, *config.Config) error {
		checks++
		return nil
	}
	startTUIDaemonProcess = func([]string) error {
		t.Fatal("daemon should not start when RPC is already ready")
		return nil
	}

	err := ensureTUIDaemonRunningImpl(
		context.Background(),
		nil,
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
		handcli.ConfigInputs{},
	)

	require.NoError(t, err)
	require.Equal(t, 1, checks)
}

func TestEnsureTUIDaemonRunning_ReturnsConfigError(t *testing.T) {
	err := ensureTUIDaemonRunningImpl(context.Background(), nil, nil, handcli.ConfigInputs{})

	require.EqualError(t, err, "config is required")
}

func TestEnsureTUIDaemonRunning_StartsDaemonAndWaitsForRPC(t *testing.T) {
	restore := replaceTUIDaemonBootstrapHooks(t)
	defer restore()

	unavailableErr := errors.New("connection refused")
	checks := 0
	checkTUIDaemonRPC = func(context.Context, *config.Config) error {
		checks++
		if checks < 3 {
			return unavailableErr
		}
		return nil
	}
	var gotArgs []string
	startTUIDaemonProcess = func(args []string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}

	inputs := handcli.ConfigInputs{
		Profile:    testProfile("work"),
		EnvPath:    "/tmp/work.env",
		ConfigPath: "/tmp/work.yaml",
	}
	err := ensureTUIDaemonRunningImpl(
		context.Background(),
		nil,
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
		inputs,
	)

	require.NoError(t, err)
	require.Equal(t, []string{
		"--profile", "work",
		"--env-file", "/tmp/work.env",
		"--config", "/tmp/work.yaml",
		"daemon", "start",
	}, gotArgs)
	require.Equal(t, 3, checks)
}

func TestEnsureTUIDaemonRunning_ForwardsExplicitConfigOverrides(t *testing.T) {
	restore := replaceTUIDaemonBootstrapHooks(t)
	defer restore()

	checks := 0
	checkTUIDaemonRPC = func(context.Context, *config.Config) error {
		checks++
		if checks == 1 {
			return errors.New("connection refused")
		}
		return nil
	}
	var gotArgs []string
	startTUIDaemonProcess = func(args []string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}

	cmd := newTUITestRootCommand(func(ctx context.Context, cmd *cli.Command) error {
		return ensureTUIDaemonRunningImpl(
			ctx,
			cmd,
			&config.Config{RPC: config.RPCConfig{Address: "127.0.0.9", Port: 50099}},
			handcli.ConfigInputs{
				Profile:    testProfile("work"),
				EnvPath:    "/tmp/work.env",
				ConfigPath: "/tmp/work.yaml",
			},
		)
	})
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--profile", "work",
		"--model", "gpt-5.5",
		"--model.stream=false",
		"--rpc.address", "127.0.0.9",
		"--rpc.port", "50099",
		"--web.cache-ttl", "2s",
		"--gateway.enabled",
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"--profile", "work",
		"--env-file", "/tmp/work.env",
		"--config", "/tmp/work.yaml",
		"--model=gpt-5.5",
		"--model.stream=false",
		"--rpc.address=127.0.0.9",
		"--rpc.port=50099",
		"--gateway.enabled=true",
		"--web.cache-ttl=2s",
		"daemon", "start",
	}, gotArgs)
}

func TestEnsureTUIDaemonRunning_ReturnsStartError(t *testing.T) {
	restore := replaceTUIDaemonBootstrapHooks(t)
	defer restore()

	checkTUIDaemonRPC = func(context.Context, *config.Config) error {
		return errors.New("connection refused")
	}
	expectedErr := errors.New("spawn failed")
	startTUIDaemonProcess = func([]string) error {
		return expectedErr
	}

	err := ensureTUIDaemonRunningImpl(
		context.Background(),
		nil,
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
		handcli.ConfigInputs{},
	)

	require.ErrorIs(t, err, expectedErr)
}

func TestEnsureTUIDaemonRunning_ReturnsReadinessError(t *testing.T) {
	restore := replaceTUIDaemonBootstrapHooks(t)
	defer restore()

	checkTUIDaemonRPC = func(context.Context, *config.Config) error {
		return errors.New("connection refused")
	}
	startTUIDaemonProcess = func([]string) error {
		return nil
	}

	err := ensureTUIDaemonRunningImpl(
		context.Background(),
		nil,
		&config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051}},
		handcli.ConfigInputs{},
	)

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "RPC did not become ready at 127.0.0.1:50051"))
	require.True(t, strings.Contains(err.Error(), "connection refused"))
}

func TestWaitForTUIDaemonRPC_UsesSingleCheckWhenTimeoutIsNotPositive(t *testing.T) {
	restore := replaceTUIDaemonBootstrapHooks(t)
	defer restore()

	expectedErr := errors.New("connection refused")
	checks := 0
	checkTUIDaemonRPC = func(context.Context, *config.Config) error {
		checks++
		return expectedErr
	}

	err := waitForTUIDaemonRPC(context.Background(), &config.Config{}, 0)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 1, checks)
}

func TestWaitForTUIDaemonRPC_ReturnsContextCancellation(t *testing.T) {
	restore := replaceTUIDaemonBootstrapHooks(t)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	checkTUIDaemonRPC = func(context.Context, *config.Config) error {
		cancel()
		return errors.New("connection refused")
	}

	err := waitForTUIDaemonRPC(ctx, &config.Config{}, time.Second)

	require.ErrorIs(t, err, context.Canceled)
}

func TestCheckTUIDaemonRPCImpl_ReturnsConfigError(t *testing.T) {
	err := checkTUIDaemonRPCImpl(context.Background(), nil)

	require.EqualError(t, err, "config is required")
}

func TestCheckTUIDaemonRPCImpl_ReturnsClientConfigError(t *testing.T) {
	err := checkTUIDaemonRPCImpl(context.Background(), &config.Config{
		RPC: config.RPCConfig{Address: "127.0.0.1"},
	})

	require.EqualError(t, err, "rpc port must be greater than zero")
}

func TestCheckTUIDaemonRPCImpl_CallsGatewayStatus(t *testing.T) {
	server := grpc.NewServer()
	stub := &gatewayStatusServerStub{}
	handpb.RegisterGatewayServiceServer(server, stub)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	go func() {
		_ = server.Serve(listener)
	}()

	cfg := &config.Config{
		RPC: config.RPCConfig{
			Address: "127.0.0.1",
			Port:    listener.Addr().(*net.TCPAddr).Port,
		},
	}

	err = checkTUIDaemonRPCImpl(context.Background(), cfg)

	require.NoError(t, err)
	require.Equal(t, 1, stub.calls)
}

func TestStartTUIDaemonProcessImpl_ReturnsExecutableError(t *testing.T) {
	originalGetTUIDaemonExecutable := getTUIDaemonExecutable
	t.Cleanup(func() { getTUIDaemonExecutable = originalGetTUIDaemonExecutable })

	expectedErr := errors.New("not found")
	getTUIDaemonExecutable = func() (string, error) {
		return "", expectedErr
	}

	err := startTUIDaemonProcessImpl(nil)

	require.ErrorIs(t, err, expectedErr)
	require.ErrorContains(t, err, "resolve hand executable")
}

func TestStartTUIDaemonProcessImpl_ReturnsStartError(t *testing.T) {
	originalGetTUIDaemonExecutable := getTUIDaemonExecutable
	t.Cleanup(func() { getTUIDaemonExecutable = originalGetTUIDaemonExecutable })

	getTUIDaemonExecutable = func() (string, error) {
		return filepath.Join(t.TempDir(), "missing-hand"), nil
	}

	err := startTUIDaemonProcessImpl(nil)

	require.Error(t, err)
	require.ErrorContains(t, err, "start Hand daemon")
}

func TestStartTUIDaemonProcessImpl_StartsAndReleasesProcess(t *testing.T) {
	originalGetTUIDaemonExecutable := getTUIDaemonExecutable
	t.Cleanup(func() { getTUIDaemonExecutable = originalGetTUIDaemonExecutable })

	scriptPath := filepath.Join(t.TempDir(), "hand-test-daemon")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	getTUIDaemonExecutable = func() (string, error) {
		return scriptPath, nil
	}

	err := startTUIDaemonProcessImpl([]string{"daemon", "start"})

	require.NoError(t, err)
}

func TestStartTUIDaemonProcessImpl_ReturnsReleaseError(t *testing.T) {
	originalGetTUIDaemonExecutable := getTUIDaemonExecutable
	originalReleaseTUIDaemonProcess := releaseTUIDaemonProcess
	t.Cleanup(func() {
		getTUIDaemonExecutable = originalGetTUIDaemonExecutable
		releaseTUIDaemonProcess = originalReleaseTUIDaemonProcess
	})

	scriptPath := filepath.Join(t.TempDir(), "hand-test-daemon")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 1\n"), 0o700))
	getTUIDaemonExecutable = func() (string, error) {
		return scriptPath, nil
	}
	expectedErr := errors.New("release failed")
	releaseTUIDaemonProcess = func(process *os.Process) error {
		_ = process.Kill()
		_, _ = process.Wait()
		return expectedErr
	}

	err := startTUIDaemonProcessImpl([]string{"daemon", "start"})

	require.ErrorIs(t, err, expectedErr)
	require.ErrorContains(t, err, "release Hand daemon process")
}

func newTUITestRootCommand(action func(context.Context, *cli.Command) error) *cli.Command {
	envFile := ".env"
	configFile := "config.yaml"

	return &cli.Command{
		Flags: handcli.RootFlags(&envFile, &configFile),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return action(ctx, cmd)
		},
	}
}

func replaceTUIDaemonBootstrapHooks(t *testing.T) func() {
	t.Helper()

	originalCheckTUIDaemonRPC := checkTUIDaemonRPC
	originalStartTUIDaemonProcess := startTUIDaemonProcess
	originalReleaseTUIDaemonProcess := releaseTUIDaemonProcess
	originalInitialTimeout := daemonBootstrapInitialTimeout
	originalReadyTimeout := daemonBootstrapReadyTimeout
	originalPollInterval := daemonBootstrapPollInterval
	daemonBootstrapInitialTimeout = time.Millisecond
	daemonBootstrapReadyTimeout = 5 * time.Millisecond
	daemonBootstrapPollInterval = time.Millisecond

	return func() {
		checkTUIDaemonRPC = originalCheckTUIDaemonRPC
		startTUIDaemonProcess = originalStartTUIDaemonProcess
		releaseTUIDaemonProcess = originalReleaseTUIDaemonProcess
		daemonBootstrapInitialTimeout = originalInitialTimeout
		daemonBootstrapReadyTimeout = originalReadyTimeout
		daemonBootstrapPollInterval = originalPollInterval
	}
}

func testProfile(name string) profile.Profile {
	return profile.Profile{Name: name}
}

type gatewayStatusServerStub struct {
	handpb.UnimplementedGatewayServiceServer
	calls int
}

func (s *gatewayStatusServerStub) GatewayStatus(
	context.Context,
	*handpb.GetGatewayStatusRequest,
) (*handpb.GetGatewayStatusResponse, error) {
	s.calls++
	return &handpb.GetGatewayStatusResponse{
		Status: &handpb.GatewayStatus{State: "running"},
	}, nil
}

type fakeTUIChatClient struct {
	closed bool
}

func (c *fakeTUIChatClient) Respond(
	context.Context,
	string,
	rpcclient.RespondOptions,
) (string, error) {
	return "", nil
}

func (c *fakeTUIChatClient) SessionAPI() rpcclient.SessionAPI {
	return c
}

func (c *fakeTUIChatClient) ModelAPI() rpcclient.ModelAPI {
	return c
}

func (c *fakeTUIChatClient) ListProviders(context.Context) (rpcclient.ProviderList, error) {
	return rpcclient.ProviderList{}, nil
}

func (c *fakeTUIChatClient) ListModels(context.Context, ...rpcclient.ModelListOptions) (rpcclient.ModelList, error) {
	return rpcclient.ModelList{}, nil
}

func (c *fakeTUIChatClient) SelectModel(context.Context, string, ...rpcclient.ModelSelectOptions) (rpcclient.ModelOption, error) {
	return rpcclient.ModelOption{}, nil
}

func (c *fakeTUIChatClient) SetProviderAPIKey(context.Context, string, string) error {
	return nil
}

func (c *fakeTUIChatClient) Timeline(
	context.Context,
	rpcclient.SessionTimelineOptions,
) (rpcclient.SessionTimeline, error) {
	return rpcclient.SessionTimeline{}, nil
}

func (c *fakeTUIChatClient) Compact(context.Context, string) (rpcclient.CompactSessionResult, error) {
	return rpcclient.CompactSessionResult{}, nil
}

func (c *fakeTUIChatClient) Create(context.Context, string) (storage.Session, error) {
	return storage.Session{}, nil
}

func (c *fakeTUIChatClient) CreateWithOptions(
	context.Context,
	rpcclient.CreateSessionOptions,
) (storage.Session, error) {
	return storage.Session{}, nil
}

func (c *fakeTUIChatClient) List(context.Context, ...rpcclient.SessionListOptions) ([]storage.Session, error) {
	return nil, nil
}

func (c *fakeTUIChatClient) Use(context.Context, string) error {
	return nil
}

func (c *fakeTUIChatClient) Archive(context.Context, string) error {
	return nil
}

func (c *fakeTUIChatClient) Unarchive(context.Context, string) (storage.Session, error) {
	return storage.Session{}, nil
}

func (c *fakeTUIChatClient) Rename(context.Context, string, string) (storage.Session, error) {
	return storage.Session{}, nil
}

func (c *fakeTUIChatClient) Current(context.Context) (storage.Session, error) {
	return storage.Session{}, nil
}

func (c *fakeTUIChatClient) Repair(
	context.Context,
	rpcclient.RepairSessionOptions,
) (rpcclient.RepairSessionResult, error) {
	return rpcclient.RepairSessionResult{}, nil
}

func (c *fakeTUIChatClient) Status(context.Context, string) (rpcclient.ContextStatus, error) {
	return rpcclient.ContextStatus{}, nil
}

func (c *fakeTUIChatClient) Close() error {
	c.closed = true
	return nil
}

var _ tui.SessionTimelineLoader = (*fakeTUIChatClient)(nil)
