package readiness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/profile"
)

func TestReport_HasFailuresAndSummary(t *testing.T) {
	report := Report{Groups: []Group{
		{
			Name: "models",
			Checks: []Check{
				check("main", StatusPass, "ready"),
				check("summary", StatusFail, "missing auth"),
			},
		},
	}}

	require.True(t, report.HasFailures())
	require.Equal(t, "models summary: missing auth", report.Summary())

	report.Groups[0].Checks[1].Status = StatusWarn
	require.False(t, report.HasFailures())
	require.Equal(t, "readiness checks passed", report.Summary())
}

func TestBuild_ReportsProfileAndMissingDaemonWithoutFailure(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: test\n"), 0o600))
	active := profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: home})
	cfg := readyConfig()

	report := Build(context.Background(), Options{
		Config:     cfg,
		Profile:    active,
		ConfigPath: configPath,
		EnvPath:    filepath.Join(home, ".env"),
	})

	require.False(t, report.HasFailures())
	require.Equal(t, StatusPass, findReadinessCheck(t, report, "profile", "home").Status)
	daemon := findReadinessCheck(t, report, "daemon", "runtime")
	require.Equal(t, StatusWarn, daemon.Status)
	require.Contains(t, daemon.Message, "runtime metadata is not present")
	require.Equal(t, "hand up", daemon.Actions[0].Command)
}

func TestBuild_ReportsModelAuthWithoutLeakingCredentials(t *testing.T) {
	cfg := readyConfig()
	cfg.Models.Providers[constants.ModelProviderOpenRouter] = config.ProviderModelConfig{APIKey: "secret-openrouter-key"}

	report := Build(context.Background(), Options{
		Config:  cfg,
		Profile: profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: t.TempDir()}),
	})

	main := findReadinessCheck(t, report, "models", "main")
	require.Equal(t, StatusPass, main.Status)
	require.Contains(t, main.Message, "provider-config auth")
	embedding := findReadinessCheck(t, report, "models", "embedding")
	require.Equal(t, StatusWarn, embedding.Status)
	require.Contains(t, embedding.Message, "embedding model")
	require.Contains(t, embedding.Message, "vector search is disabled")
	require.NotContains(t, report.Summary()+main.Message, "secret-openrouter-key")
}

func TestBuild_ReportsMissingWebCredentialAsWarning(t *testing.T) {
	clearWebCredentialEnv(t)
	original := resolveWebAPIKeySource
	t.Cleanup(func() {
		resolveWebAPIKeySource = original
	})
	resolveWebAPIKeySource = func(*config.Config) (config.WebCredentialSource, error) {
		return config.WebCredentialSource{}, nil
	}
	cfg := readyConfig()
	cfg.Web.Provider = constants.WebProviderExa

	report := Build(context.Background(), Options{
		Config:  cfg,
		Profile: profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: t.TempDir()}),
	})

	web := findReadinessCheck(t, report, "capabilities", "web tools")
	require.Equal(t, StatusWarn, web.Status)
	require.Contains(t, web.Message, "exa web credentials are not configured")
	require.Equal(t, "hand config set web.provider exa && hand config set web.apiKey <api-key>", web.Actions[0].Command)
}

func TestBuild_DoesNotEmitAnsi(t *testing.T) {
	report := Build(context.Background(), Options{
		Config:  readyConfig(),
		Profile: profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: t.TempDir()}),
	})

	require.NotRegexp(t, regexp.MustCompile(`\x1b\[[0-9;]*m`), report.Summary())
	for _, group := range report.Groups {
		for _, check := range group.Checks {
			require.NotRegexp(t, regexp.MustCompile(`\x1b\[[0-9;]*m`), check.Message)
		}
	}
}

func TestBuild_CoversModelAndCapabilityBranches(t *testing.T) {
	cfg := readyConfig()
	cfg.Search.Vector.Enabled = true
	cfg.Search.Vector.Required = true
	cfg.Models.Embedding.Provider = constants.ModelProviderOpenAI
	cfg.Reranker.Enabled = new(bool)
	cfg.Cap.Network = new(bool)
	cfg.Memory.Enabled = new(bool)
	cfg.Web.Provider = "native"

	report := Build(context.Background(), Options{
		Config:  cfg,
		Profile: profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: t.TempDir()}),
	})

	require.True(t, report.HasFailures())
	embedding := findReadinessCheck(t, report, "models", "embedding")
	require.Equal(t, StatusFail, embedding.Status)
	require.Equal(t, "hand auth login openai --api-key <api-key>", embedding.Actions[0].Command)
	require.Equal(t, "hand config set models.providers.openai.apiKey <api-key>", embedding.Actions[1].Command)
	vector := findReadinessCheck(t, report, "capabilities", "vector search")
	require.Equal(t, StatusFail, vector.Status)
	require.Equal(t, `required vector search cannot resolve embedding auth for provider "openai"`, vector.Message)
	require.Equal(t, "hand auth login openai --api-key <api-key>", vector.Actions[0].Command)
	require.Equal(t, "hand config set models.providers.openai.apiKey <api-key>", vector.Actions[1].Command)
	require.Equal(t, StatusWarn, findReadinessCheck(t, report, "capabilities", "memory").Status)
	require.Equal(t, StatusWarn, findReadinessCheck(t, report, "capabilities", "reranker").Status)
	require.Equal(t, StatusWarn, findReadinessCheck(t, report, "capabilities", "web tools").Status)
}

func TestBuild_CoversWebCredentialBranches(t *testing.T) {
	original := resolveWebAPIKeySource
	t.Cleanup(func() {
		resolveWebAPIKeySource = original
	})

	cfg := readyConfig()
	cfg.Web.Provider = constants.WebProviderExa
	resolveWebAPIKeySource = func(*config.Config) (config.WebCredentialSource, error) {
		return config.WebCredentialSource{Configured: true, Source: "environment"}, nil
	}
	report := Build(context.Background(), Options{Config: cfg})
	require.Equal(t, StatusPass, findReadinessCheck(t, report, "capabilities", "web tools").Status)

	resolveWebAPIKeySource = func(*config.Config) (config.WebCredentialSource, error) {
		return config.WebCredentialSource{}, errors.New("stored failed")
	}
	report = Build(context.Background(), Options{Config: cfg})
	web := findReadinessCheck(t, report, "capabilities", "web tools")
	require.Equal(t, StatusWarn, web.Status)
	require.Contains(t, web.Message, "stored failed")

	cfg.Web.Provider = "custom"
	report = Build(context.Background(), Options{Config: cfg})
	require.Equal(t, StatusWarn, findReadinessCheck(t, report, "capabilities", "web tools").Status)

	cfg.Web.Provider = "native"
	report = Build(context.Background(), Options{Config: cfg})
	require.Equal(t, StatusPass, findReadinessCheck(t, report, "capabilities", "web tools").Status)

	require.Equal(t, "hand config set web.provider exa && hand config set web.apiKey <api-key>", webAuthAction("").Command)
}

func TestBuild_CoversProfilePathBranches(t *testing.T) {
	home := t.TempDir()
	filePath := filepath.Join(home, "file")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))
	dirPath := filepath.Join(home, "dir")
	require.NoError(t, os.Mkdir(dirPath, 0o700))

	report := Build(context.Background(), Options{
		Config: readyConfig(),
		Profile: profile.Profile{
			Name:        "",
			HomeDir:     filePath,
			ConfigPath:  dirPath,
			EnvPath:     "",
			RuntimePath: filepath.Join(home, "missing-runtime.json"),
		},
	})

	require.Equal(t, StatusFail, findReadinessCheck(t, report, "profile", "home").Status)
	require.Equal(t, StatusFail, findReadinessCheck(t, report, "profile", "config").Status)
	require.Equal(t, StatusFail, findReadinessCheck(t, report, "profile", "env").Status)
	require.Equal(t, StatusWarn, findReadinessCheck(t, report, "profile", "runtime").Status)
}

func TestBuild_CoversReadyDaemon(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, listener.Close())
	})
	accepted := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
		close(accepted)
	}()

	home := t.TempDir()
	active := profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: home})
	require.NoError(t, os.WriteFile(active.RuntimePath, []byte(`{
  "profile": "work",
  "pid": `+fmt.Sprint(os.Getpid())+`,
  "rpc": {
    "address": "127.0.0.1",
    "port": `+fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)+`
  },
  "started_at": "2026-06-03T00:00:00Z"
}`), 0o600))

	report := Build(context.Background(), Options{
		Config:  readyConfig(),
		Profile: active,
	})

	require.Equal(t, StatusPass, findReadinessCheck(t, report, "daemon", "runtime").Status)
	select {
	case <-accepted:
	case <-time.After(time.Second):
		require.Fail(t, "runtime probe did not dial test listener")
	}
}

func TestBuild_CoversNilConfig(t *testing.T) {
	report := Build(context.Background(), Options{})

	require.True(t, report.HasFailures())
	require.Equal(t, StatusFail, findReadinessCheck(t, report, "models", "config").Status)
	require.Equal(t, StatusFail, findReadinessCheck(t, report, "capabilities", "config").Status)
}

func TestBuild_CoversRerankDisabledBySearch(t *testing.T) {
	cfg := readyConfig()
	enabled := true
	disabled := false
	cfg.Reranker.Enabled = &enabled
	cfg.Search.EnableRerank = &disabled

	report := Build(context.Background(), Options{Config: cfg})

	require.Equal(t, StatusWarn, findReadinessCheck(t, report, "capabilities", "reranker").Status)
}

func TestMissingAuthActionAndCredentialSourceFormatting(t *testing.T) {
	modelMissingAuthActions := modelErrorActions(constants.ModelProviderOpenRouter, errors.New("model API key is required for provider"))
	require.Equal(t, "hand auth login openrouter --api-key <api-key>", modelMissingAuthActions[0].Command)
	require.Equal(t, "hand config set models.providers.openrouter.apiKey <api-key>", modelMissingAuthActions[1].Command)

	embeddingMissingAuthActions := embeddingModelErrorActions(constants.ModelProviderOpenAI, errors.New("embedding API key is required for provider"))
	require.Equal(t, "hand auth login openai --api-key <api-key>", embeddingMissingAuthActions[0].Command)
	require.Equal(t, "hand config set models.providers.openai.apiKey <api-key>", embeddingMissingAuthActions[1].Command)

	modelSelectionActions := modelErrorActions(constants.ModelProviderOpenRouter, errors.New("model provider must be one of: openrouter"))
	require.Len(t, modelSelectionActions, 2)
	require.Equal(t, "/providers", modelSelectionActions[0].Command)
	require.Equal(t, "/models", modelSelectionActions[1].Command)

	require.False(t, isMissingAuthError(nil))
	require.Equal(t, "hand auth login openai", missingAuthActions(constants.ModelProviderOpenAI)[0].Command)
	require.Equal(
		t,
		"hand auth login openrouter --api-key <api-key>",
		missingAuthActions(constants.ModelProviderOpenRouter)[0].Command,
	)
	require.Equal(
		t,
		"hand config set models.providers.openrouter.apiKey <api-key>",
		missingAuthActions(constants.ModelProviderOpenRouter)[1].Command,
	)
	require.Equal(
		t,
		"hand auth login openrouter --api-key <api-key>",
		missingAuthActions("")[0].Command,
	)

	require.Equal(t, "role-config", formatCredentialSource(config.ModelAuth{
		CredentialSource: config.ModelCredentialSource{Kind: config.ModelCredentialSourceRoleConfig},
	}))
	require.Equal(t, "oauth env", formatCredentialSource(config.ModelAuth{
		CredentialSource: config.ModelCredentialSource{
			Kind: config.ModelCredentialSourceProviderEnv,
			Type: "oauth",
		},
	}))
	require.Equal(t, "environment", formatCredentialSource(config.ModelAuth{
		CredentialSource: config.ModelCredentialSource{Kind: config.ModelCredentialSourceProviderEnv},
	}))
	require.Equal(t, "token-store oauth refreshable", formatCredentialSource(config.ModelAuth{
		CredentialSource: config.ModelCredentialSource{
			Kind:      config.ModelCredentialSourceTokenStore,
			Type:      "oauth",
			HasExpiry: true,
		},
	}))
	require.Equal(t, "api-key", formatCredentialSource(config.ModelAuth{APIKey: "key"}))
}

func findReadinessCheck(t *testing.T, report Report, groupName string, checkName string) Check {
	t.Helper()

	for _, group := range report.Groups {
		if group.Name != groupName {
			continue
		}
		for _, check := range group.Checks {
			if check.Name == checkName {
				return check
			}
		}
	}

	require.Failf(t, "missing readiness check", "%s/%s", groupName, checkName)
	return Check{}
}

func readyConfig() *config.Config {
	cfg := config.NewDefaultConfig()
	cfg.Name = "test"
	cfg.Models.Main.Provider = constants.ModelProviderOpenRouter
	cfg.Models.Main.Name = "gpt-4o-mini"
	cfg.Models.Providers = map[string]config.ProviderModelConfig{
		constants.ModelProviderOpenRouter: {APIKey: "model-key"},
	}
	cfg.Search.Vector.Enabled = false
	cfg.Web.Provider = ""

	return cfg
}

func clearWebCredentialEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"HAND_EXA_API_KEY",
		"EXA_API_KEY",
		"HAND_FIRECRAWL_API_KEY",
		"FIRECRAWL_API_KEY",
		"HAND_PARALLEL_API_KEY",
		"PARALLEL_API_KEY",
		"HAND_TAVILY_API_KEY",
		"TAVILY_API_KEY",
		"HAND_WEB_API_KEY",
	} {
		t.Setenv(key, "")
	}
}
