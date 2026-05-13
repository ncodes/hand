package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
)

func TestResolveConfigInputs_UsesProfileDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, profile.EnvName, "HAND_ENV_FILE", "HAND_CONFIG")

	var got ConfigInputs
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		var err error
		got, err = ResolveConfigInputs(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "Work"})

	require.NoError(t, err)
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.Equal(t, "work", got.Profile.Name)
	require.Equal(t, filepath.Join(profileHome, ".env"), got.EnvPath)
	require.Equal(t, filepath.Join(profileHome, "config.yaml"), got.ConfigPath)
	require.Equal(t, got.Profile, profile.Active())
}

func TestResolveConfigInputs_UsesProfileShorthand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, profile.EnvName, "HAND_ENV_FILE", "HAND_CONFIG")

	var got ConfigInputs
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		var err error
		got, err = ResolveConfigInputs(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "-p", "Work"})

	require.NoError(t, err)
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.Equal(t, "work", got.Profile.Name)
	require.Equal(t, filepath.Join(profileHome, ".env"), got.EnvPath)
	require.Equal(t, filepath.Join(profileHome, "config.yaml"), got.ConfigPath)
}

func TestResolveConfigInputs_KeepsExplicitPathOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, profile.EnvName, "HAND_ENV_FILE", "HAND_CONFIG")

	envPath := filepath.Join(t.TempDir(), "custom.env")
	configPath := filepath.Join(t.TempDir(), "custom.yaml")

	var got ConfigInputs
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		var err error
		got, err = ResolveConfigInputs(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--profile", "Work",
		"--env-file", envPath,
		"--config", configPath,
	})

	require.NoError(t, err)
	require.Equal(t, "work", got.Profile.Name)
	require.Equal(t, envPath, got.EnvPath)
	require.Equal(t, configPath, got.ConfigPath)
}

func TestResolveConfigInputs_KeepsEnvironmentPathOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, profile.EnvName)
	envPath := filepath.Join(t.TempDir(), "custom.env")
	configPath := filepath.Join(t.TempDir(), "custom.yaml")
	t.Setenv("HAND_ENV_FILE", envPath)
	t.Setenv("HAND_CONFIG", configPath)

	var got ConfigInputs
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		var err error
		got, err = ResolveConfigInputs(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "Work"})

	require.NoError(t, err)
	require.Equal(t, "work", got.Profile.Name)
	require.Equal(t, envPath, got.EnvPath)
	require.Equal(t, configPath, got.ConfigPath)
}

func TestResolveConfigInputs_UsesProfileEnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, "HAND_ENV_FILE", "HAND_CONFIG")
	t.Setenv(profile.EnvName, "Desk")

	var got ConfigInputs
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		var err error
		got, err = ResolveConfigInputs(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand"})

	require.NoError(t, err)
	profileHome := filepath.Join(home, ".hand", "profiles", "desk")
	require.Equal(t, "desk", got.Profile.Name)
	require.Equal(t, filepath.Join(profileHome, ".env"), got.EnvPath)
	require.Equal(t, filepath.Join(profileHome, "config.yaml"), got.ConfigPath)
}

func TestLoadConfig_UsesProfileConfigAndEnvDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, profile.EnvName, "HAND_ENV_FILE", "HAND_CONFIG", "HAND_LOG_LEVEL")

	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: profile-agent
models:
  verify: false
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), []byte("HAND_LOG_LEVEL=debug\n"), 0o600))

	var got *config.Config
	var inputs ConfigInputs
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		var err error
		got, inputs, err = LoadConfig(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "Work"})

	require.NoError(t, err)
	require.Equal(t, filepath.Join(profileHome, ".env"), inputs.EnvPath)
	require.Equal(t, filepath.Join(profileHome, "config.yaml"), inputs.ConfigPath)
	require.NotNil(t, got)
	require.Equal(t, "profile-agent", got.Name)
	require.Equal(t, "debug", got.Log.Level)
}

func TestLoadConfig_ReturnsConfigLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, profile.EnvName, "HAND_ENV_FILE", "HAND_CONFIG")

	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte("name: ["), 0o600))

	var inputs ConfigInputs
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		var err error
		_, inputs, err = LoadConfig(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "Work"})

	require.ErrorContains(t, err, "failed to parse config file")
	require.Equal(t, filepath.Join(profileHome, ".env"), inputs.EnvPath)
	require.Equal(t, filepath.Join(profileHome, "config.yaml"), inputs.ConfigPath)
}

func TestLoadConfig_ReturnsProfileResolutionError(t *testing.T) {
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		_, _, err := LoadConfig(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work/team"})

	require.EqualError(t, err, `invalid profile name "work/team": must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,63}`)
}

func TestResolveConfigInputs_UsesDefaultProfileWhenCommandNil(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t, profile.EnvName, "HAND_ENV_FILE", "HAND_CONFIG")

	inputs, err := ResolveConfigInputs(nil)

	require.NoError(t, err)
	profileHome := filepath.Join(home, ".hand", "profiles", "default")
	require.Equal(t, profile.DefaultName, inputs.Profile.Name)
	require.Equal(t, filepath.Join(profileHome, ".env"), inputs.EnvPath)
	require.Equal(t, filepath.Join(profileHome, "config.yaml"), inputs.ConfigPath)
}

func TestResolveConfigInputs_ReturnsInvalidProfileError(t *testing.T) {
	cmd := newProfileInputCommand(t, func(cmd *cli.Command) error {
		_, err := ResolveConfigInputs(cmd)
		return err
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "work/team"})

	require.EqualError(t, err, `invalid profile name "work/team": must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,63}`)
}

func newProfileInputCommand(t *testing.T, action func(*cli.Command) error) *cli.Command {
	t.Helper()

	active := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(active)
	})

	envFile := ".env"
	configFile := "config.yaml"
	return &cli.Command{
		Flags: RootFlags(&envFile, &configFile),
		Action: func(_ context.Context, cmd *cli.Command) error {
			return action(cmd)
		},
	}
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
