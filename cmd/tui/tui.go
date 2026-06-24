// Package tui provides Morph's interactive terminal UI command.
package tui

import (
	"context"

	cli "github.com/urfave/cli/v3"
)

func Run(ctx context.Context, cmd *cli.Command) error {
	model, cleanup, err := loadCommandModel(ctx, cmd)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	_, err = newProgram(model).Run()
	return err
}
