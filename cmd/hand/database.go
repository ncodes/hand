package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/datadir"
)

func newDatabaseCommand() *cli.Command {
	return &cli.Command{
		Name:  "db",
		Usage: "Manage the configured local database",
		Commands: []*cli.Command{
			{
				Name:  "reset",
				Usage: "Delete the configured SQLite database for a fresh start",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "force",
						Usage: "Confirm deletion of the configured database",
					},
				},
				Action: resetConfiguredDatabase,
			},
		},
	}
}

func resetConfiguredDatabase(_ context.Context, cmd *cli.Command) error {
	if !cmd.Bool("force") {
		return errors.New("database reset requires --force")
	}

	cfg, inputs, err := handcli.LoadConfig(cmd)
	if err != nil {
		return err
	}

	handcli.ApplyConfigOverrides(cmd, cfg)
	handcli.AddStartupFilesystemRoots(cfg, inputs)

	if strings.TrimSpace(strings.ToLower(cfg.Storage.Backend)) != "sqlite" {
		return errors.New("database reset requires sqlite storage backend")
	}

	paths := getConfiguredDatabasePaths()
	for _, path := range paths {
		if err := removeDatabasePath(path); err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(rootOutput, "Reset database: %s\n", paths[0])
	return err
}

func getConfiguredDatabasePaths() []string {
	path := datadir.StateDBPath()
	return []string{
		path,
		path + "-wal",
		path + "-shm",
		path + "-journal",
	}
}

func removeDatabasePath(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove database file %q: %w", path, err)
	}

	return nil
}
