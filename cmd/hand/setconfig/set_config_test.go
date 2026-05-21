package setconfig

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
)

func TestCommand_UpdatesSelectedProfileConfig(t *testing.T) {
	clearSetConfigEnv(t, "HAND_CONFIG", "HAND_ENV_FILE", "HAND_PROFILE", "HAND_MODEL_KEY")
	resetSetConfigProfileState(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := writeCommandProfileConfig(t, home, "work")

	var output bytes.Buffer
	cmd := newTestRootCommand(&output)

	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--profile", "work",
		"set-config",
		"search.enableRank",
		"true",
	}))

	cfg, err := config.Load("", configPath)
	require.NoError(t, err)
	require.True(t, *cfg.Search.EnableRerank)
	require.Equal(t, "search.enableRerank=true\n", output.String())
}

func TestCommand_UpdatesSelectedProfileConfigWithInlineValueAndLocalProfileFlag(t *testing.T) {
	clearSetConfigEnv(t, "HAND_CONFIG", "HAND_ENV_FILE", "HAND_PROFILE", "HAND_MODEL_KEY")
	resetSetConfigProfileState(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := writeCommandProfileConfig(t, home, "safety-manual")

	var output bytes.Buffer
	cmd := newTestRootCommand(&output)

	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"set-config",
		"-p", "safety-manual",
		"safety.pii=true",
	}))

	cfg, err := config.Load("", configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Safety.PII)
	require.True(t, *cfg.Safety.PII)
	require.Equal(t, "safety.pii=true\n", output.String())
}

func TestCommand_UpdatesMultipleSelectedProfileConfigValues(t *testing.T) {
	clearSetConfigEnv(t, "HAND_CONFIG", "HAND_ENV_FILE", "HAND_PROFILE", "HAND_MODEL_KEY")
	resetSetConfigProfileState(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := writeCommandProfileConfig(t, home, "work")

	var output bytes.Buffer
	cmd := newTestRootCommand(&output)

	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--profile", "work",
		"set-config",
		"search.enableRank=true",
		"safety.pii=true",
	}))

	cfg, err := config.Load("", configPath)
	require.NoError(t, err)
	require.True(t, *cfg.Search.EnableRerank)
	require.NotNil(t, cfg.Safety.PII)
	require.True(t, *cfg.Safety.PII)
	require.Equal(t, "search.enableRerank=true\nsafety.pii=true\n", output.String())
}

func TestCommand_UpdatesMultipleSelectedProfileConfigValuesWithSpacedPairs(t *testing.T) {
	clearSetConfigEnv(t, "HAND_CONFIG", "HAND_ENV_FILE", "HAND_PROFILE", "HAND_MODEL_KEY")
	resetSetConfigProfileState(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := writeCommandProfileConfig(t, home, "work")

	var output bytes.Buffer
	cmd := newTestRootCommand(&output)

	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--profile", "work",
		"set-config",
		"search.enableRank", "true",
		"safety.pii", "true",
	}))

	cfg, err := config.Load("", configPath)
	require.NoError(t, err)
	require.True(t, *cfg.Search.EnableRerank)
	require.NotNil(t, cfg.Safety.PII)
	require.True(t, *cfg.Safety.PII)
	require.Equal(t, "search.enableRerank=true\nsafety.pii=true\n", output.String())
}

func TestCommand_RequiresPathAndValue(t *testing.T) {
	cmd := newTestRootCommand(nil)
	err := cmd.Run(context.Background(), []string{"hand", "set-config", "search.enableRerank"})

	require.EqualError(t, err, "config path and value are required")
}

func newTestRootCommand(output io.Writer) *cli.Command {
	envFile := ".env"
	configFile := "config.yaml"
	return &cli.Command{
		Name:     "hand",
		Flags:    handcli.RootFlags(&envFile, &configFile),
		Commands: []*cli.Command{NewCommand(output)},
	}
}

func clearSetConfigEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		original, ok := os.LookupEnv(key)
		if ok {
			t.Cleanup(func() {
				_ = os.Setenv(key, original)
			})
		} else {
			t.Cleanup(func() {
				_ = os.Unsetenv(key)
			})
		}
		_ = os.Unsetenv(key)
	}
}

func resetSetConfigProfileState(t *testing.T) {
	t.Helper()

	originalProfile := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(originalProfile)
	})
	profile.SetActive(profile.Profile{})
}

func writeCommandProfileConfig(t *testing.T, home string, name string) string {
	t.Helper()

	profileHome := filepath.Join(home, ".hand", "profiles", name)
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	configPath := filepath.Join(profileHome, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: test-agent
models:
  verify: false
  key: test-key
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
search:
  enableRerank: false
  vector:
    enabled: false
storage:
  backend: memory
safety:
  pii: false
`), 0o600))

	return configPath
}
