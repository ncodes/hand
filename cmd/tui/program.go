package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	handruntime "github.com/wandxy/hand/internal/runtime"
	tui "github.com/wandxy/hand/internal/tui/app"
	"github.com/wandxy/hand/pkg/logutils"
)

type programRunner interface {
	Run() (tea.Model, error)
}

var newProgram = func(model tea.Model) programRunner {
	return tea.NewProgram(model)
}

type tuiClient interface {
	rpcclient.ChatAPI
	SessionAPI() rpcclient.SessionAPI
	Close() error
}

var newTUIChatClient = func(ctx context.Context, cfg *config.Config) (tuiClient, error) {
	return rpcclient.NewClient(ctx, rpcclient.Options{
		Address: cfg.RPC.Address,
		Port:    cfg.RPC.Port,
	})
}

var loadCommandModel = loadTUICommandModel

func loadTUICommandModel(ctx context.Context, cmd *cli.Command) (tea.Model, func(), error) {
	cfg, inputs, err := handcli.LoadConfig(cmd)
	if err != nil {
		return nil, nil, err
	}

	handcli.ApplyConfigOverrides(cmd, cfg)
	handcli.AddStartupFilesystemRoots(cfg, inputs)

	endpoint, err := handruntime.ResolveRPC(ctx, cmd, cfg)
	if err != nil {
		return nil, nil, err
	}
	cfg.RPC = endpoint

	config.Set(cfg)
	_ = logutils.ConfigureLogger("hand", cfg.Log.NoColor)
	logutils.SetLogLevel(cfg.Log.Level)

	client, err := newTUIChatClient(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_ = client.Close()
	}

	return tui.NewModelWithClientContextAndConfig(ctx, client, cfg), cleanup, nil
}
