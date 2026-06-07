package daemon

import (
	"context"
	"io"

	urfavecli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
)

func SetOutput(w io.Writer) io.Writer {
	return handcli.SetDaemonOutput(w)
}

func NewCommand() *urfavecli.Command {
	return &urfavecli.Command{
		Name:  "daemon",
		Usage: "Manage the Hand daemon",
		Flags: []urfavecli.Flag{handcli.PersistentInstructFlag()},
		Commands: []*urfavecli.Command{
			{
				Name:  "start",
				Usage: "Start the Hand daemon",
				Action: func(ctx context.Context, cmd *urfavecli.Command) error {
					return handcli.RunDaemonWithConfigRestarts(ctx, cmd)
				},
			},
		},
		Action: func(_ context.Context, cmd *urfavecli.Command) error {
			return urfavecli.ShowSubcommandHelp(cmd)
		},
	}
}
