package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	storage "github.com/wandxy/hand/internal/state/core"
	tui "github.com/wandxy/hand/internal/tui/app"
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

func TestNewCommand_StartsProgram(t *testing.T) {
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

	err := NewCommand().Run(context.Background(), []string{"tui"})

	require.NoError(t, err)
	require.True(t, program.ran)
	require.IsType(t, fakeModel{}, program.model)
}

func TestNewCommand_ReturnsProgramError(t *testing.T) {
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

	err := NewCommand().Run(context.Background(), []string{"tui"})

	require.ErrorIs(t, err, expectedErr)
}

func TestNewCommand_ReturnsModelLoadError(t *testing.T) {
	originalLoadCommandModel := loadCommandModel
	t.Cleanup(func() {
		loadCommandModel = originalLoadCommandModel
	})

	expectedErr := errors.New("rpc unavailable")
	loadCommandModel = func(context.Context, *cli.Command) (tea.Model, func(), error) {
		return nil, nil, expectedErr
	}

	err := NewCommand().Run(context.Background(), []string{"tui"})

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
	t.Cleanup(func() {
		newTUIChatClient = originalNewTUIChatClient
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
	t.Cleanup(func() {
		newTUIChatClient = originalNewTUIChatClient
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
