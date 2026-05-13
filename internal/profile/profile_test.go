package profile

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolve_DefaultProfile(t *testing.T) {
	resolved, err := Resolve(ResolveOptions{UserHomeDir: "/Users/me"})
	require.NoError(t, err)

	home := filepath.Join("/Users/me", ".hand", "profiles", "default")
	require.Equal(t, Profile{
		Name:        DefaultName,
		HomeDir:     home,
		ConfigPath:  filepath.Join(home, "config.yaml"),
		EnvPath:     filepath.Join(home, ".env"),
		RuntimePath: filepath.Join(home, "runtime.json"),
		PIDPath:     filepath.Join(home, "hand.pid"),
	}, resolved)
}

func TestActive(t *testing.T) {
	original := Active()
	t.Cleanup(func() {
		SetActive(original)
	})

	profile := Profile{Name: "work", HomeDir: "/Users/me/.hand/profiles/work"}

	SetActive(profile)

	require.Equal(t, profile, Active())
}

func TestWithMetadataPaths_FillsEmptyPathsFromHomeDir(t *testing.T) {
	home := filepath.Join("/Users/me", ".hand", "profiles", "work")

	resolved := WithMetadataPaths(Profile{Name: "work", HomeDir: home})

	require.Equal(t, filepath.Join(home, "config.yaml"), resolved.ConfigPath)
	require.Equal(t, filepath.Join(home, ".env"), resolved.EnvPath)
	require.Equal(t, filepath.Join(home, "runtime.json"), resolved.RuntimePath)
	require.Equal(t, filepath.Join(home, "hand.pid"), resolved.PIDPath)
}

func TestWithMetadataPaths_KeepsExplicitPaths(t *testing.T) {
	home := filepath.Join("/Users/me", ".hand", "profiles", "work")
	resolved := WithMetadataPaths(Profile{
		Name:        "work",
		HomeDir:     home,
		ConfigPath:  "/tmp/config.yaml",
		EnvPath:     "/tmp/.env",
		RuntimePath: "/tmp/runtime.json",
		PIDPath:     "/tmp/hand.pid",
	})

	require.Equal(t, "/tmp/config.yaml", resolved.ConfigPath)
	require.Equal(t, "/tmp/.env", resolved.EnvPath)
	require.Equal(t, "/tmp/runtime.json", resolved.RuntimePath)
	require.Equal(t, "/tmp/hand.pid", resolved.PIDPath)
}

func TestResolve_UsesExplicitProfileBeforeEnv(t *testing.T) {
	resolved, err := Resolve(ResolveOptions{
		Name:        "Work",
		Env:         map[string]string{EnvName: "personal"},
		UserHomeDir: "/Users/me",
	})
	require.NoError(t, err)

	require.Equal(t, "work", resolved.Name)
	require.Equal(t, filepath.Join("/Users/me", ".hand", "profiles", "work"), resolved.HomeDir)
}

func TestResolve_UsesEnvProfile(t *testing.T) {
	resolved, err := Resolve(ResolveOptions{
		Env:         map[string]string{EnvName: "Research_01"},
		UserHomeDir: "/Users/me",
	})
	require.NoError(t, err)

	require.Equal(t, "research_01", resolved.Name)
	require.Equal(t, filepath.Join("/Users/me", ".hand", "profiles", "research_01"), resolved.HomeDir)
}

func TestResolve_UsesProcessEnvProfile(t *testing.T) {
	t.Setenv(EnvName, "Desk")

	resolved, err := Resolve(ResolveOptions{UserHomeDir: "/Users/me"})
	require.NoError(t, err)

	require.Equal(t, "desk", resolved.Name)
	require.Equal(t, filepath.Join("/Users/me", ".hand", "profiles", "desk"), resolved.HomeDir)
}

func TestResolve_ReturnsInvalidProfileNameError(t *testing.T) {
	_, err := Resolve(ResolveOptions{
		Name:        "work/team",
		UserHomeDir: "/Users/me",
	})
	require.EqualError(t, err, `invalid profile name "work/team": must match `+namePattern)
}

func TestResolve_UsesUserHomeDirWhenHomeDirEmpty(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "/Users/from-home", nil
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	resolved, err := Resolve(ResolveOptions{Name: "desk"})
	require.NoError(t, err)

	require.Equal(t, filepath.Join("/Users/from-home", ".hand", "profiles", "desk"), resolved.HomeDir)
}

func TestResolve_ReturnsUserHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, err := Resolve(ResolveOptions{Name: "desk"})
	require.EqualError(t, err, "resolve user home dir: home unavailable")
}

func TestResolve_ReturnsEmptyHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "   ", nil
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, err := Resolve(ResolveOptions{Name: "desk"})
	require.EqualError(t, err, "home directory is required")
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "", want: DefaultName},
		{name: "   ", want: DefaultName},
		{name: "default", want: "default"},
		{name: "Work", want: "work"},
		{name: "Research_01", want: "research_01"},
		{name: "desk-agent", want: "desk-agent"},
		{name: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijkl", want: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijkl"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeName(tc.name)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeName_RejectsInvalidNames(t *testing.T) {
	tests := []string{
		"-work",
		"_work",
		"work team",
		"work/team",
		"work.team",
		"work:team",
		"日本",
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklm",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeName(name)
			require.EqualError(t, err, `invalid profile name "`+name+`": must match `+namePattern)
			require.False(t, IsValidName(name))
		})
	}
}

func TestIsValidName(t *testing.T) {
	require.True(t, IsValidName("default"))
	require.True(t, IsValidName("Work_01"))
	require.False(t, IsValidName(""))
	require.False(t, IsValidName("   "))
	require.False(t, IsValidName("work/team"))
}
