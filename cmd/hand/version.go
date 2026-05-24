package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/constants"
)

func newVersionCommand(output io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print Hand version information",
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
	return fmt.Sprintf("hand version %s\ncommit %s\n", getAppVersion(), getCommitHash())
}

func getAppVersion() string {
	version := strings.TrimSpace(constants.AppVersion)
	if version == "" {
		return "dev"
	}

	return version
}

func getCommitHash() string {
	commit := strings.TrimSpace(constants.CommitHash)
	if commit == "" {
		return "unknown"
	}

	return commit
}
