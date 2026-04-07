package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
)

func TestBuild_ReturnsPassingReportForValidConfig(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("MODEL_KEY=test-key\n"), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte("name: test-agent\n"), 0o600))

	report := Build(envPath, configPath, &config.Config{
		Name:          "test-agent",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openrouter",
		ModelKey:      "test-key",
		LogLevel:      "info",
	}, nil)

	require.False(t, report.HasFailures())
	require.Contains(t, report.Checks, Check{Name: "config validation", Status: StatusPass, Message: "configuration is valid"})
	require.Contains(t, report.Checks, Check{Name: "model auth", Status: StatusPass, Message: `resolved auth for provider "openrouter"`})
	require.Contains(t, report.Checks, Check{Name: "model base URL", Status: StatusPass, Message: `using "https://openrouter.ai/api/v1"`})
}

func TestBuild_ReturnsLoadFailureWhenConfigLoadFails(t *testing.T) {
	report := Build(".env", "config.yaml", nil, os.ErrPermission)
	require.True(t, report.HasFailures())
	require.Contains(t, report.Summary(), "config load")
	require.Contains(t, report.Summary(), "permission denied")
}

func TestBuild_ReturnsValidationFailureForInvalidConfig(t *testing.T) {
	// config error: model provider must be one of: openai, openrouter
	report := Build(".env", "config.yaml", &config.Config{
		Name:          "test-agent",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "anthropic",
		ModelKey:      "test-key",
		LogLevel:      "info",
	}, nil)

	require.True(t, report.HasFailures())
	require.Contains(t, report.Summary(), "config validation")
}

func TestBuild_ReturnsBaseURLFailureForInvalidURL(t *testing.T) {
	report := Build(".env", "config.yaml", &config.Config{
		Name:          "test-agent",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openai",
		ModelKey:      "test-key",
		ModelBaseURL:  "://bad-url",
		LogLevel:      "info",
	}, nil)

	require.True(t, report.HasFailures())
	require.Contains(t, report.Summary(), "model base URL")
}

func TestBuild_ReturnsValidationFailureWhileAuthStillPasses(t *testing.T) {
	report := Build(".env", "config.yaml", &config.Config{
		Name:          "test-agent",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openrouter",
		ModelKey:      "test-key",
		LogLevel:      "trace",
	}, nil)

	require.True(t, report.HasFailures())
	require.Contains(t, report.Checks, Check{
		Name:    "config validation",
		Status:  StatusFail,
		Message: "log level must be one of debug, info, warn, or error; use --log.level",
	})
	require.Contains(t, report.Checks, Check{
		Name:    "model auth",
		Status:  StatusPass,
		Message: `resolved auth for provider "openrouter"`,
	})
}

func TestBuild_ReturnsModelAuthFailureWhenKeyIsMissing(t *testing.T) {
	report := Build(".env", "config.yaml", &config.Config{
		Name:          "test-agent",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openrouter",
		LogLevel:      "info",
	}, nil)

	require.True(t, report.HasFailures())
	require.Contains(t, report.Checks, Check{
		Name:    "model auth",
		Status:  StatusFail,
		Message: "model key is required; set MODEL_KEY, provide it in config, or use --model.key",
	})
}

func TestBuild_IncludesSummaryModelChecksWhenSummaryAuthDiffersFromMain(t *testing.T) {
	report := Build(".env", "config.yaml", &config.Config{
		Name:                "test-agent",
		Model:               "openai/gpt-4o-mini",
		ModelProvider:       "openrouter",
		ModelKey:            "test-key",
		LogLevel:            "info",
		SummaryProvider:     "openai",
		SummaryModelBaseURL: "https://api.example/v1",
		VerifyModel:         new(false),
	}, nil)

	require.False(t, report.HasFailures())
	require.Contains(t, report.Checks, Check{
		Name:    "summary model auth",
		Status:  StatusPass,
		Message: `resolved summary auth for provider "openai"`,
	})
	require.Contains(t, report.Checks, Check{
		Name:    "summary model base URL",
		Status:  StatusPass,
		Message: `using "https://api.example/v1"`,
	})
}

func TestBuild_ReturnsSummaryBaseURLFailureWhenSummaryURLInvalid(t *testing.T) {
	report := Build(".env", "config.yaml", &config.Config{
		Name:                "test-agent",
		Model:               "openai/gpt-4o-mini",
		ModelProvider:       "openrouter",
		ModelKey:            "test-key",
		LogLevel:            "info",
		SummaryProvider:     "openai",
		SummaryModelBaseURL: "://bad",
		VerifyModel:         new(false),
	}, nil)

	require.True(t, report.HasFailures())
	require.Contains(t, report.Summary(), "summary model base URL")
	require.Contains(t, report.Summary(), `"://bad" is not a valid absolute URL`)
}

func TestBuild_ReturnsSummaryModelAuthFailureWhenSummaryResolveFails(t *testing.T) {
	orig := resolveSummaryModelAuth
	t.Cleanup(func() { resolveSummaryModelAuth = orig })
	resolveSummaryModelAuth = func(*config.Config) (config.ModelAuth, error) {
		return config.ModelAuth{}, errors.New("summary resolve failed")
	}

	report := Build(".env", "config.yaml", &config.Config{
		Name:          "test-agent",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openrouter",
		ModelKey:      "test-key",
		LogLevel:      "info",
		VerifyModel:   new(false),
	}, nil)

	require.True(t, report.HasFailures())
	require.Contains(t, report.Checks, Check{
		Name:    "summary model auth",
		Status:  StatusFail,
		Message: "summary resolve failed",
	})
}

func TestBuild_WarnsForMissingOptionalFiles(t *testing.T) {
	report := Build("missing.env", "missing.yaml", &config.Config{
		Name:          "test-agent",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openai",
		ModelKey:      "test-key",
		LogLevel:      "info",
	}, nil)

	require.False(t, report.HasFailures())
	require.Contains(t, report.Checks, Check{Name: "env file", Status: StatusWarn, Message: `"missing.env" not found; continuing without it`})
	require.Contains(t, report.Checks, Check{Name: "config file", Status: StatusWarn, Message: `"missing.yaml" not found; continuing without file values`})
}

func TestBuild_ReturnsFailureWhenConfigIsNil(t *testing.T) {
	report := Build(".env", "config.yaml", nil, nil)
	require.True(t, report.HasFailures())
	require.Equal(t, "config is required", report.FirstFailure())
	require.Contains(t, report.Summary(), "config load: config is required")
}

func TestReport_SummaryReturnsSuccessWhenNoFailures(t *testing.T) {
	report := Report{
		Checks: []Check{
			{Name: "env file", Status: StatusWarn, Message: "not set"},
			{Name: "config validation", Status: StatusPass, Message: "configuration is valid"},
		},
	}

	require.False(t, report.HasFailures())
	require.Equal(t, "startup diagnostics passed", report.Summary())
	require.Empty(t, report.FirstFailure())
}

func TestReport_FirstFailureReturnsFirstFailureOnly(t *testing.T) {
	report := Report{
		Checks: []Check{
			{Name: "env file", Status: StatusWarn, Message: "not set"},
			{Name: "config validation", Status: StatusFail, Message: "first failure"},
			{Name: "model auth", Status: StatusFail, Message: "second failure"},
		},
	}

	require.True(t, report.HasFailures())
	require.Equal(t, "first failure", report.FirstFailure())
	require.Equal(t, "config validation: first failure; model auth: second failure", report.Summary())
}

func TestFileCheck_WarnsWhenPathNotSet(t *testing.T) {
	check := fileCheck("env file", "   ", true)
	require.Equal(t, Check{
		Name:    "env file",
		Status:  StatusWarn,
		Message: "not set",
	}, check)
}

func TestFileCheck_FailsWhenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()

	check := fileCheck("config file", dir, false)

	require.Equal(t, Check{
		Name:    "config file",
		Status:  StatusFail,
		Message: `"` + dir + `" is a directory`,
	}, check)
}

func TestFileCheck_FailsForUnexpectedStatError(t *testing.T) {
	originalStat := osStat
	t.Cleanup(func() {
		osStat = originalStat
	})

	osStat = func(string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}

	check := fileCheck("config file", "config.yaml", false)

	require.Equal(t, Check{
		Name:    "config file",
		Status:  StatusFail,
		Message: os.ErrPermission.Error(),
	}, check)
}

func TestBaseURLCheck_PassesWhenEmpty(t *testing.T) {
	check := baseURLCheck("model base URL", "   ")
	require.Equal(t, Check{
		Name:    "model base URL",
		Status:  StatusPass,
		Message: "using provider default base URL",
	}, check)
}

func TestBaseURLCheck_PassesForValidAbsoluteURL(t *testing.T) {
	check := baseURLCheck("model base URL", "https://example.com/v1")
	require.Equal(t, Check{
		Name:    "model base URL",
		Status:  StatusPass,
		Message: `using "https://example.com/v1"`,
	}, check)
}
