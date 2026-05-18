// Package tui provides Hand's interactive terminal UI command.
package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	handruntime "github.com/wandxy/hand/internal/runtime"
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
	sessionTimelineLoader
	Close() error
}

var newTUIChatClient = func(ctx context.Context, cfg *config.Config) (tuiClient, error) {
	return rpcclient.NewClient(ctx, rpcclient.Options{
		Address: cfg.RPC.Address,
		Port:    cfg.RPC.Port,
	})
}

var loadCommandModel = loadTUICommandModel

// NewCommand returns the interactive TUI command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "tui",
		Usage: "Start the interactive Hand terminal UI",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			model, cleanup, err := loadCommandModel(ctx, cmd)
			if err != nil {
				return err
			}
			if cleanup != nil {
				defer cleanup()
			}

			_, err = newProgram(model).Run()
			return err
		},
	}
}

func loadTUICommandModel(ctx context.Context, cmd *cli.Command) (model, func(), error) {
	cfg, _, err := handcli.LoadConfig(cmd)
	if err != nil {
		return model{}, nil, err
	}

	handcli.ApplyConfigOverrides(cmd, cfg)

	endpoint, err := handruntime.ResolveRPC(ctx, cmd, cfg)
	if err != nil {
		return model{}, nil, err
	}
	cfg.RPC = endpoint

	config.Set(cfg)
	_ = logutils.ConfigureLogger("hand", cfg.Log.NoColor)
	logutils.SetLogLevel(cfg.Log.Level)

	client, err := newTUIChatClient(ctx, cfg)
	if err != nil {
		return model{}, nil, err
	}

	cleanup := func() {
		_ = client.Close()
	}

	return newModelWithClientContext(ctx, client), cleanup, nil
}
