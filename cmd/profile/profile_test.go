package profilecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
)

func TestCommandUseStoresCurrentProfile(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".hand", "profiles", "work"), 0o700))

	var output bytes.Buffer
	SetOutput(&output)
	err := NewCommand().Run(context.Background(), []string{"profile", "use", "Work"})
	require.NoError(t, err)

	path := filepath.Join(home, ".hand", "state.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{"current_profile":"work"}`, string(data))
	require.Equal(t, "work\n", output.String())
}

func TestCommandUseRejectsUnknownProfile(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	err := NewCommand().Run(context.Background(), []string{"profile", "use", "Work"})
	require.EqualError(t, err, `profile "work" does not exist; run `+"`hand profile init work` first")

	path := filepath.Join(home, ".hand", "state.json")
	require.NoFileExists(t, path)
}

func TestCommandUseRequiresName(t *testing.T) {
	resetProfileCommand(t)

	err := NewCommand().Run(context.Background(), []string{"profile", "use"})
	require.EqualError(t, err, "profile name is required")
}

func TestCommandCurrentUsesStoredProfile(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, err := profile.StoreCurrentName("Work", home)
	require.NoError(t, err)
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{
		Name:    "override",
		HomeDir: filepath.Join(home, ".hand", "profiles", "override"),
	}))

	var output bytes.Buffer
	SetOutput(&output)
	err = NewCommand().Run(context.Background(), []string{"profile", "current"})
	require.NoError(t, err)

	require.Equal(t, "work\n", output.String())
}

func TestCommandCurrentDefaultsWhenStoredProfileMissing(t *testing.T) {
	resetProfileCommand(t)
	t.Setenv("HOME", t.TempDir())

	var output bytes.Buffer
	SetOutput(&output)
	err := NewCommand().Run(context.Background(), []string{"profile", "current"})
	require.NoError(t, err)

	require.Equal(t, "default\n", output.String())
}

func TestCommandCurrentReturnsInvalidStoredProfileError(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	currentPath := filepath.Join(home, ".hand", "state.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(currentPath), 0o700))
	require.NoError(t, os.WriteFile(currentPath, []byte(`{"current_profile":"work/team"}`+"\n"), 0o600))

	err := NewCommand().Run(context.Background(), []string{"profile", "current"})
	require.EqualError(t, err, `invalid profile name "work/team": must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,63}`)
}

func TestCommandInitBareCreatesProfileDirIdempotently(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	var output bytes.Buffer
	SetOutput(&output)
	cmd := NewCommand()
	err := cmd.Run(context.Background(), []string{"profile", "init", "Work", "--bare"})
	require.NoError(t, err)
	err = cmd.Run(context.Background(), []string{"profile", "init", "Work", "--bare"})
	require.NoError(t, err)

	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.DirExists(t, profileHome)
	require.NoFileExists(t, filepath.Join(profileHome, "config.yaml"))
	require.Equal(t, profileHome+"\n"+profileHome+"\n", output.String())
}

func TestCommandInitCreatesStarterConfig(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	var output bytes.Buffer
	SetOutput(&output)
	err := NewCommand().Run(context.Background(), []string{"profile", "init", "Alpha"})
	require.NoError(t, err)

	profileHome := filepath.Join(home, ".hand", "profiles", "alpha")
	configPath := filepath.Join(profileHome, "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "name: alpha\n")
	require.Contains(t, string(data), "models:\n")
	require.Contains(t, string(data), "name: \"\"\n")
	require.Contains(t, string(data), "provider: \"\"\n")
	require.NotContains(t, string(data), "gpt-")
	require.NotContains(t, string(data), "openrouter")
	cfg, err := config.Load("", configPath)
	require.NoError(t, err)
	require.Equal(t, "alpha", cfg.Name)
	require.Empty(t, cfg.Web.Provider)
	require.Equal(t, profileHome+"\n", output.String())
}

func TestCommandInitRefusesConfigOverwrite(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	profileHome := filepath.Join(home, ".hand", "profiles", "alpha")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	configPath := filepath.Join(profileHome, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: existing\n"), 0o600))

	err := NewCommand().Run(context.Background(), []string{"profile", "init", "Alpha"})
	require.EqualError(t, err, "config file already exists: "+configPath)
	data, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	require.Equal(t, "name: existing\n", string(data))
}

func TestCommandInitRequiresName(t *testing.T) {
	resetProfileCommand(t)

	err := NewCommand().Run(context.Background(), []string{"profile", "init"})
	require.EqualError(t, err, "profile name is required")
}

func TestCommandListPrintsExistingProfileDirs(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	profilesDir := filepath.Join(home, ".hand", "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "work"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "Personal"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "ignored"), []byte("file"), 0o600))

	var output bytes.Buffer
	SetOutput(&output)
	err := NewCommand().Run(context.Background(), []string{"profile", "list"})
	require.NoError(t, err)

	require.Equal(t, "personal\nwork\n", output.String())
}

func TestCommandPathPrintsExplicitProfilePath(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	var output bytes.Buffer
	SetOutput(&output)
	err := NewCommand().Run(context.Background(), []string{"profile", "path", "Work"})
	require.NoError(t, err)

	require.Equal(t, filepath.Join(home, ".hand", "profiles", "work")+"\n", output.String())
}

func TestCommandPathPrintsActiveProfilePath(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{
		Name:    "desk",
		HomeDir: filepath.Join(home, ".hand", "profiles", "desk"),
	}))

	var output bytes.Buffer
	SetOutput(&output)
	err := NewCommand().Run(context.Background(), []string{"profile", "path"})
	require.NoError(t, err)

	require.Equal(t, filepath.Join(home, ".hand", "profiles", "desk")+"\n", output.String())
}

func TestCommandPathUsesStoredCurrentProfile(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, err := profile.StoreCurrentName("Work", home)
	require.NoError(t, err)

	var output bytes.Buffer
	SetOutput(&output)
	err = NewCommand().Run(context.Background(), []string{"profile", "path"})
	require.NoError(t, err)

	require.Equal(t, filepath.Join(home, ".hand", "profiles", "work")+"\n", output.String())
}

func TestCommandDoctorPrintsProfilePathsAndStatuses(t *testing.T) {
	resetProfileCommand(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte("name: test\n"), 0o600))

	var output bytes.Buffer
	SetOutput(&output)
	err := NewCommand().Run(context.Background(), []string{"profile", "doctor", "Work"})
	require.NoError(t, err)

	got := output.String()
	require.Contains(t, got, "name=work\n")
	require.Contains(t, got, "home="+profileHome+"\n")
	require.Contains(t, got, "config="+filepath.Join(profileHome, "config.yaml")+"\n")
	require.Contains(t, got, "env="+filepath.Join(profileHome, ".env")+"\n")
	require.Contains(t, got, "runtime="+filepath.Join(profileHome, "runtime.json")+"\n")
	require.Contains(t, got, "pid="+filepath.Join(profileHome, "hand.pid")+"\n")
	require.Contains(t, got, "home_exists=true\n")
	require.Contains(t, got, "config_exists=true\n")
	require.Contains(t, got, "env_exists=false\n")
	require.Contains(t, got, "runtime_exists=false\n")
}

func resetProfileCommand(t *testing.T) {
	t.Helper()
	originalOutput := SetOutput(nil)
	originalProfile := profile.Active()
	t.Cleanup(func() {
		SetOutput(originalOutput)
		profile.SetActive(originalProfile)
	})
	profile.SetActive(profile.Profile{})
}
