// Package tui provides Hand's interactive terminal UI command.
package tui

import (
	"context"

	cli "github.com/urfave/cli/v3"
)

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
