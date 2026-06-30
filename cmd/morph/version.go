package main

import (
	"context"
	"fmt"
	"io"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/pkg/stringx"
)

func newVersionCommand(output io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print Morph version information",
		Action: func(_ context.Context, _ *cli.Command) error {
			_, err := fmt.Fprint(output, formatVersionCommandOutput())
			return err
		},
	}
}

func formatRootVersion() string {
	version := getAppVersion()
	commit := getCommitHash()
	if commit == "" {
		return version
	}

	return version + " (commit " + commit + ")"
}

func formatVersionCommandOutput() string {
	return fmt.Sprintf("morph version %s\ncommit %s\n", getAppVersion(), getCommitHash())
}

func getAppVersion() string {
	version := stringx.String(constants.AppVersion).Trim()
	if version == "" {
		return "dev"
	}

	return version
}

func getCommitHash() string {
	commit := stringx.String(constants.CommitHash).Trim()
	if commit == "" {
		return "unknown"
	}

	return commit
}
