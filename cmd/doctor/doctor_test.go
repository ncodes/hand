package doctor

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/profile"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestNewCommand_PrintsPassingReport(t *testing.T) {
	isolateProfile(t)
	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	cmd := newRootCommandForTest()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--models.verify=false",
		"doctor",
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "[\x1b[32mPASS\x1b[0m] config validation: configuration is valid")
	require.Contains(t, output.String(), "safety: input=enabled, output=enabled, pii=disabled")
	require.Contains(t, output.String(), "doctor checks passed")
}

func TestNewCommand_PrintsSafetyModeFromConfig(t *testing.T) {
	isolateProfile(t)
	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: flag-agent
models:
  key: flag-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
  verify: false
safety:
  input: false
  output: true
  pii: true
search:
  vector:
    enabled: false
`), 0o600))

	cmd := newRootCommandForTest()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--config", configPath,
		"doctor",
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "safety: input=disabled, output=enabled, pii=enabled")
}

func TestNewCommand_PrintsFailureReport(t *testing.T) {
	isolateProfile(t)
	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
search:
  vector:
    enabled: false
`), 0o600))

	cmd := newRootCommandForTest()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--config", configPath,
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"doctor",
	})
	require.EqualError(t, err, "doctor checks failed: config validation: model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key; model auth: model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
	require.Contains(t, output.String(), "[\x1b[31mFAIL\x1b[0m] config validation")
	require.Contains(t, output.String(), "[\x1b[31mFAIL\x1b[0m] model auth")
	require.Contains(t, output.String(), "safety: input=enabled, output=enabled, pii=disabled")
}

func TestNewCommand_DisablesColorWhenRequested(t *testing.T) {
	isolateProfile(t)
	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	cmd := newRootCommandForTest()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--log.no-color",
		"--models.verify=false",
		"doctor",
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "[PASS] config validation: configuration is valid")
	require.Contains(t, output.String(), "safety: input=enabled, output=enabled, pii=disabled")
	require.NotRegexp(t, regexp.MustCompile(`\x1b\[[0-9;]*m`), output.String())
}

func newRootCommandForTest() *cli.Command {
	envFile := ".env"
	configFile := "config.yaml"

	return &cli.Command{
		Name:  "hand",
		Flags: handcli.RootFlags(&envFile, &configFile),
		Commands: []*cli.Command{
			NewCommand(),
		},
	}
}

func isolateProfile(t *testing.T) {
	t.Helper()

	originalProfile := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(originalProfile)
	})
	profile.SetActive(profile.Profile{})
	t.Setenv("HOME", t.TempDir())
	clearEnv(t, profile.EnvName, "HAND_ENV_FILE", "HAND_CONFIG", "HAND_SAFETY_INPUT", "HAND_SAFETY_OUTPUT", "HAND_SAFETY_PII")
}

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		original, ok := os.LookupEnv(key)
		if ok {
			t.Cleanup(func() {
				require.NoError(t, os.Setenv(key, original))
			})
		} else {
			t.Cleanup(func() {
				require.NoError(t, os.Unsetenv(key))
			})
		}
		require.NoError(t, os.Unsetenv(key))
	}
}
