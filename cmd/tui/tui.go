// Package tui provides Hand's interactive terminal UI command.
package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	cli "github.com/urfave/cli/v3"
)

type programRunner interface {
	Run() (tea.Model, error)
}

var newProgram = func(model tea.Model) programRunner {
	return tea.NewProgram(model)
}

// NewCommand returns the interactive TUI command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "tui",
		Usage: "Start the interactive Hand terminal UI",
		Action: func(context.Context, *cli.Command) error {
			_, err := newProgram(newModel()).Run()
			return err
		},
	}
}
