package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
)

func TestApplyConfigOverrides_AppliesRulesFiles(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{"hand", "--rules.files", "/tmp/Hand.md, ./custom.md ,/tmp/CLAUDE.md"})

	require.NoError(t, err)
	require.Equal(t, []string{"/tmp/Hand.md", "./custom.md", "/tmp/CLAUDE.md"}, cfg.RulesFiles)
}

func TestApplyConfigOverrides_AppliesInstruct(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: []cli.Flag{RequestInstructFlag()}}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{"hand", "--instruct", " be terse "})

	require.NoError(t, err)
	require.Equal(t, "be terse", cfg.Instruct)
}
