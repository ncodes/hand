package profile

import (
	"errors"
	"os"
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

func TestWithMetadataPaths_ReturnsProfileWhenHomeDirEmpty(t *testing.T) {
	resolved := Profile{Name: "work"}
	require.Equal(t, resolved, WithMetadataPaths(resolved))
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

func TestResolve_UsesStoredCurrentProfile(t *testing.T) {
	home := t.TempDir()
	_, err := StoreCurrentName("Work", home)
	require.NoError(t, err)

	resolved, err := Resolve(ResolveOptions{UserHomeDir: home})
	require.NoError(t, err)

	require.Equal(t, "work", resolved.Name)
	require.Equal(t, filepath.Join(home, ".hand", "profiles", "work"), resolved.HomeDir)
}

func TestResolve_ExplicitProfileOverridesStoredCurrentProfile(t *testing.T) {
	home := t.TempDir()
	_, err := StoreCurrentName("Work", home)
	require.NoError(t, err)

	resolved, err := Resolve(ResolveOptions{Name: "Desk", UserHomeDir: home})
	require.NoError(t, err)

	require.Equal(t, "desk", resolved.Name)
	require.Equal(t, filepath.Join(home, ".hand", "profiles", "desk"), resolved.HomeDir)
}

func TestResolve_EnvProfileOverridesStoredCurrentProfile(t *testing.T) {
	home := t.TempDir()
	_, err := StoreCurrentName("Work", home)
	require.NoError(t, err)

	resolved, err := Resolve(ResolveOptions{
		Env:         map[string]string{EnvName: "Desk"},
		UserHomeDir: home,
	})
	require.NoError(t, err)

	require.Equal(t, "desk", resolved.Name)
	require.Equal(t, filepath.Join(home, ".hand", "profiles", "desk"), resolved.HomeDir)
}

func TestResolve_ReturnsInvalidStoredCurrentProfileError(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"current_profile":"work/team"}`+"\n"), 0o600))

	_, err = Resolve(ResolveOptions{UserHomeDir: home})
	require.EqualError(t, err, `invalid profile name "work/team": must match `+namePattern)
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

func TestRootDir_ReturnsHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, err := RootDir("")
	require.EqualError(t, err, "resolve user home dir: home unavailable")
}

func TestProfilesDir_ReturnsHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, err := ProfilesDir("")
	require.EqualError(t, err, "resolve user home dir: home unavailable")
}

func TestCurrentPath(t *testing.T) {
	home := t.TempDir()

	path, err := CurrentPath(home)
	require.NoError(t, err)

	require.Equal(t, filepath.Join(home, ".hand", "state.json"), path)
}

func TestCurrentPath_ReturnsHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, err := CurrentPath("")
	require.EqualError(t, err, "resolve user home dir: home unavailable")
}

func TestLoadCurrentName_ReturnsFalseWhenMissing(t *testing.T) {
	name, ok, err := LoadCurrentName(t.TempDir())
	require.NoError(t, err)

	require.Empty(t, name)
	require.False(t, ok)
}

func TestLoadCurrentName_ReturnsFalseWhenStateFileEmpty(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("  \n"), 0o600))

	name, ok, err := LoadCurrentName(home)

	require.NoError(t, err)
	require.Empty(t, name)
	require.False(t, ok)
}

func TestLoadCurrentName_ReturnsFalseWhenStateFileNull(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("null\n"), 0o600))

	name, ok, err := LoadCurrentName(home)

	require.NoError(t, err)
	require.Empty(t, name)
	require.False(t, ok)
}

func TestLoadCurrentName_IgnoresLegacyCurrentProfileFile(t *testing.T) {
	home := t.TempDir()
	legacyPath := filepath.Join(home, ".hand", "current-profile")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o700))
	require.NoError(t, os.WriteFile(legacyPath, []byte("Work\n"), 0o600))

	name, ok, err := LoadCurrentName(home)

	require.NoError(t, err)
	require.Empty(t, name)
	require.False(t, ok)
}

func TestLoadCurrentName_ReturnsHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, _, err := LoadCurrentName("")
	require.EqualError(t, err, "resolve user home dir: home unavailable")
}

func TestLoadCurrentName_ReturnsReadError(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(path, 0o700))

	_, _, err = LoadCurrentName(home)
	require.ErrorContains(t, err, "read current profile:")
}

func TestLoadCurrentName_ReturnsInvalidStoredNameError(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"current_profile":"work/team"}`+"\n"), 0o600))

	_, _, err = LoadCurrentName(home)
	require.EqualError(t, err, `invalid profile name "work/team": must match `+namePattern)
}

func TestLoadCurrentName_ReturnsParseStateError(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

	_, _, err = LoadCurrentName(home)

	require.ErrorContains(t, err, "parse profile state:")
}

func TestStoreCurrentNameAndLoadCurrentName(t *testing.T) {
	home := t.TempDir()

	stored, err := StoreCurrentName("Work", home)
	require.NoError(t, err)
	name, ok, err := LoadCurrentName(home)
	require.NoError(t, err)

	require.Equal(t, "work", stored)
	require.True(t, ok)
	require.Equal(t, "work", name)
}

func TestStoreCurrentName_PreservesExistingStateFields(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"last_sessions":{"default":"session-a"}}`+"\n"), 0o600))

	_, err = StoreCurrentName("Work", home)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"current_profile": "work",
		"last_sessions": {
			"default": "session-a"
		}
	}`, string(data))
}

func TestStoreCurrentName_ReturnsLoadStateError(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

	_, err = StoreCurrentName("Work", home)

	require.ErrorContains(t, err, "parse profile state:")
}

func TestStoreCurrentName_RejectsInvalidName(t *testing.T) {
	_, err := StoreCurrentName("work/team", t.TempDir())
	require.EqualError(t, err, `invalid profile name "work/team": must match `+namePattern)
}

func TestStoreCurrentName_ReturnsCreateDirError(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".hand"), []byte("file"), 0o600))

	_, err := StoreCurrentName("work", home)
	require.ErrorContains(t, err, "create profile selector dir:")
}

func TestStoreCurrentName_ReturnsWriteError(t *testing.T) {
	home := t.TempDir()
	path, err := CurrentPath(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o500))
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Dir(path), 0o700)
	})

	_, err = StoreCurrentName("work", home)
	require.ErrorContains(t, err, "write current profile:")
}

func TestStoreCurrentName_ReturnsHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, err := StoreCurrentName("work", "")
	require.EqualError(t, err, "resolve user home dir: home unavailable")
}

func TestInit_CreatesProfileDir(t *testing.T) {
	home := t.TempDir()

	resolved, err := Init("Work", home)
	require.NoError(t, err)

	require.Equal(t, "work", resolved.Name)
	require.DirExists(t, filepath.Join(home, ".hand", "profiles", "work"))
}

func TestInit_IsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Init("Work", home)
	require.NoError(t, err)
	second, err := Init("Work", home)
	require.NoError(t, err)

	require.Equal(t, first, second)
}

func TestInit_ReturnsCreateProfileDirError(t *testing.T) {
	home := t.TempDir()
	profilesDir, err := ProfilesDir(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(profilesDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "work"), []byte("file"), 0o600))

	_, err = Init("Work", home)
	require.ErrorContains(t, err, "create profile dir:")
}

func TestInit_ReturnsResolveError(t *testing.T) {
	_, err := Init("work/team", t.TempDir())
	require.EqualError(t, err, `invalid profile name "work/team": must match `+namePattern)
}

func TestList_ReturnsSortedValidProfileDirs(t *testing.T) {
	home := t.TempDir()
	profilesDir, err := ProfilesDir(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "zeta"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "Alpha"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "work.team"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "file"), []byte("ignored"), 0o600))

	names, err := List(home)
	require.NoError(t, err)

	require.Equal(t, []string{"alpha", "zeta"}, names)
}

func TestList_ReturnsEmptyWhenProfilesDirMissing(t *testing.T) {
	names, err := List(t.TempDir())
	require.NoError(t, err)

	require.Empty(t, names)
}

func TestList_ReturnsReadDirError(t *testing.T) {
	home := t.TempDir()
	profilesDir, err := ProfilesDir(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(profilesDir), 0o700))
	require.NoError(t, os.WriteFile(profilesDir, []byte("file"), 0o600))

	_, err = List(home)
	require.ErrorContains(t, err, "read profiles dir:")
}

func TestList_ReturnsHomeDirError(t *testing.T) {
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = originalUserHomeDir
	})

	_, err := List("")
	require.EqualError(t, err, "resolve user home dir: home unavailable")
}

func TestResolveName(t *testing.T) {
	tests := []struct {
		name         string
		explicitName string
		env          map[string]string
		want         string
	}{
		{name: "default", want: DefaultName},
		{name: "env", env: map[string]string{EnvName: "Work"}, want: "work"},
		{name: "explicit", explicitName: "Desk", env: map[string]string{EnvName: "Work"}, want: "desk"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveName(tc.explicitName, tc.env)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestResolveName_ReturnsInvalidNameError(t *testing.T) {
	_, err := ResolveName("work/team", nil)
	require.EqualError(t, err, `invalid profile name "work/team": must match `+namePattern)
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
