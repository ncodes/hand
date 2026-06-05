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
	require.Equal(t, []string{"/tmp/Hand.md", "./custom.md", "/tmp/CLAUDE.md"}, cfg.Rules.Files)
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
	require.Equal(t, "be terse", cfg.Session.Instruct)
}

func TestChatFlag_AcceptsLongAndShortForms(t *testing.T) {
	for _, args := range [][]string{
		{"hand", "--chat", "hello"},
		{"hand", "-c", "hello"},
	} {
		var gotChat bool
		var gotArgs []string
		cmd := &cli.Command{
			Flags: []cli.Flag{ChatFlag()},
			Action: func(_ context.Context, cmd *cli.Command) error {
				gotChat = cmd.Bool("chat")
				gotArgs = cmd.Args().Slice()
				return nil
			},
		}

		err := cmd.Run(context.Background(), args)

		require.NoError(t, err)
		require.True(t, gotChat)
		require.Equal(t, []string{"hello"}, gotArgs)
	}
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
	require.False(t, getBoolValue(cfg.Cap.Filesystem))
	require.True(t, getBoolValue(cfg.Cap.Network))
	require.True(t, getBoolValue(cfg.Cap.Exec))
	require.True(t, getBoolValue(cfg.Cap.Memory))
	require.True(t, getBoolValue(cfg.Cap.Browser))
}

func TestApplyConfigOverrides_AppliesModelMaxRetries(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{"hand", "--model.max-retries", "0"})

	require.NoError(t, err)
	require.Equal(t, 0, cfg.ModelMaxRetriesEffective())
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

func TestApplyConfigOverrides_AppliesTUIThinkingComposer(t *testing.T) {
	cfg := &config.Config{}
	var cmd *cli.Command
	cmd = &cli.Command{Flags: RootFlags(nil, nil)}
	cmd.Action = func(context.Context, *cli.Command) error {
		ApplyConfigOverrides(cmd, cfg)
		return nil
	}

	err := cmd.Run(context.Background(), []string{"hand", "--tui.thinking-composer=false"})

	require.NoError(t, err)
	cfg.Normalize()
	require.False(t, cfg.TUIThinkingComposerEnabled())
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
	}, cfg.FS.Roots)
	require.Equal(t, []string{"git status"}, cfg.Exec.Allow)
	require.Equal(t, []string{"git push"}, cfg.Exec.Ask)
	require.Equal(t, []string{"git reset --hard"}, cfg.Exec.Deny)
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
		"--memory.backend", "sqlite",
		"--session.default-idle-expiry", "2h",
		"--session.archive-retention", "72h",
	})

	require.NoError(t, err)
	require.Equal(t, "memory", cfg.Storage.Backend)
	require.Equal(t, "sqlite", cfg.Memory.Backend)
	require.Equal(t, 2*time.Hour, cfg.Session.DefaultIdleExpiry)
	require.Equal(t, 72*time.Hour, cfg.Session.ArchiveRetention)
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
		"--web.cache-ttl", "15m",
		"--web.blocked-domains-enabled",
		"--web.blocked-domains", " blocked.example , ads.example ",
		"--web.blocked-domain-files", " blocked.txt , shared.txt ",
		"--web.native-allowed-hosts", " allowed.example , docs.example ",
		"--web.native-blocked-hosts", " blocked.example , raw.example ",
		"--web.native-allowed-host-files", " allow.txt , safe.txt ",
		"--web.native-blocked-host-files", " deny.txt , banned.txt ",
		"--web.extract-min-summarize-chars", "12000",
		"--web.extract-max-summary-chars", "3000",
		"--web.extract-max-summary-chunk-chars", "70000",
		"--web.extract-refusal-threshold-chars", "190000",
	})

	require.NoError(t, err)
	cfg.Normalize()
	require.Equal(t, "exa", cfg.Web.Provider)
	require.Equal(t, "web-key", cfg.Web.APIKey)
	require.Equal(t, "https://example.test", cfg.Web.BaseURL)
	require.Equal(t, 1300, cfg.Web.MaxCharPerResult)
	require.Equal(t, 51000, cfg.Web.MaxExtractCharPerResult)
	require.Equal(t, 2097152, cfg.Web.MaxExtractResponseBytes)
	require.Equal(t, 15*time.Minute, cfg.Web.CacheTTL)
	require.True(t, cfg.Web.BlockedDomainsEnabled)
	require.Equal(t, []string{"blocked.example", "ads.example"}, cfg.Web.BlockedDomains)
	require.Equal(t, []string{"blocked.txt", "shared.txt"}, cfg.Web.BlockedDomainFiles)
	require.Equal(t, []string{"allowed.example", "docs.example"}, cfg.Web.NativeAllowedHosts)
	require.Equal(t, []string{"blocked.example", "raw.example"}, cfg.Web.NativeBlockedHosts)
	require.Equal(t, []string{"allow.txt", "safe.txt"}, cfg.Web.NativeAllowedHostFiles)
	require.Equal(t, []string{"deny.txt", "banned.txt"}, cfg.Web.NativeBlockedHostFiles)
	require.Equal(t, 12000, cfg.Web.ExtractMinSummarizeChars)
	require.Equal(t, 3000, cfg.Web.ExtractMaxSummaryChars)
	require.Equal(t, 70000, cfg.Web.ExtractMaxSummaryChunkChars)
	require.Equal(t, 190000, cfg.Web.ExtractRefusalThresholdChars)
}
