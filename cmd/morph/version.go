package main

import (
	"context"
	"fmt"
	"io"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(constants.AppVersion)
	version := stringValue1.Trim()
	if version == "" {
		return "dev"
	}

	return version
}

func getCommitHash() string {
	stringValue2 := str.String(constants.CommitHash)
	commit := stringValue2.Trim()
	if commit == "" {
		return "unknown"
	}

	return commit
}
