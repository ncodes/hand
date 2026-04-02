package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestApplyConfigOverrides_AppliesPlatformAndCapabilities(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{"hand", "--platform", "desktop", "--cap.fs=false", "--cap.browser"})

	require.NoError(t, err)
	cfg.Normalize()
	require.Equal(t, "desktop", cfg.Platform)
	require.False(t, boolValue(cfg.CapFilesystem))
	require.True(t, boolValue(cfg.CapNetwork))
	require.True(t, boolValue(cfg.CapExec))
	require.True(t, boolValue(cfg.CapMemory))
	require.True(t, boolValue(cfg.CapBrowser))
}

func TestApplyConfigOverrides_AppliesFilesystemRootsAndExecRules(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--fs.roots", "./workspace,./nested",
		"--exec.allow", "git status",
		"--exec.ask", "git push",
		"--exec.deny", "git reset --hard",
	})

	require.NoError(t, err)
	cfg.Normalize()
	require.Equal(t, []string{
		filepath.Join(dir, "workspace"),
		filepath.Join(dir, "nested"),
	}, cfg.FSRoots)
	require.Equal(t, []string{"git status"}, cfg.ExecAllow)
	require.Equal(t, []string{"git push"}, cfg.ExecAsk)
	require.Equal(t, []string{"git reset --hard"}, cfg.ExecDeny)
}

func TestApplyConfigOverrides_AppliesSessionSettings(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--storage.backend", "memory",
		"--session.default-idle-expiry", "2h",
		"--session.archive-retention", "72h",
	})

	require.NoError(t, err)
	require.Equal(t, "memory", cfg.StorageBackend)
	require.Equal(t, 2*time.Hour, cfg.SessionDefaultIdleExpiry)
	require.Equal(t, 72*time.Hour, cfg.SessionArchiveRetention)
}
