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

func TestApplyConfigOverrides_AppliesModelVerifyModel(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{"hand", "--model.verify-model=false"})

	require.NoError(t, err)
	cfg.Normalize()
	require.False(t, boolValue(cfg.VerifyModel))
}

func TestApplyConfigOverrides_AppliesModelStream(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{"hand", "--model.stream=false"})

	require.NoError(t, err)
	cfg.Normalize()
	require.False(t, cfg.StreamEnabled())
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

func TestApplyConfigOverrides_AppliesWebSettings(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--web.provider", " exa ",
		"--web.key", " web-key ",
		"--web.base-url", " https://example.test ",
		"--web.max-char-per-result", "1300",
		"--web.max-extract-char-per-result", "51000",
		"--web.max-extract-response-bytes", "2097152",
		"--web.extract-min-summarize-chars", "12000",
		"--web.extract-max-summary-chars", "3000",
		"--web.extract-max-summary-chunk-chars", "70000",
		"--web.extract-refusal-threshold-chars", "190000",
	})

	require.NoError(t, err)
	cfg.Normalize()
	require.Equal(t, "exa", cfg.WebProvider)
	require.Equal(t, "web-key", cfg.WebAPIKey)
	require.Equal(t, "https://example.test", cfg.WebBaseURL)
	require.Equal(t, 1300, cfg.WebMaxCharPerResult)
	require.Equal(t, 51000, cfg.WebMaxExtractCharPerResult)
	require.Equal(t, 2097152, cfg.WebMaxExtractResponseBytes)
	require.Equal(t, 12000, cfg.WebExtractMinSummarizeChars)
	require.Equal(t, 3000, cfg.WebExtractMaxSummaryChars)
	require.Equal(t, 70000, cfg.WebExtractMaxSummaryChunkChars)
	require.Equal(t, 190000, cfg.WebExtractRefusalThresholdChars)
}
