package datadir

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/profile"
)

var (
	getenv      = os.Getenv
	userHomeDir = os.UserHomeDir
)

// ProjectHomeDir returns the per-project Hand data directory.
func ProjectHomeDir() string {
	return HomeDir()
}

// HomeDir returns the configured Hand home directory.
func HomeDir() string {
	if active := profile.Active(); strings.TrimSpace(active.HomeDir) != "" {
		return active.HomeDir
	}

	userHome := loadUserHomeDir()
	if userHome == "" {
		return filepath.Join(".hand", "profiles", profile.DefaultName)
	}

	resolved, err := profile.Resolve(profile.ResolveOptions{
		Env:         map[string]string{profile.EnvName: getenv(profile.EnvName)},
		UserHomeDir: userHome,
	})
	if err != nil {
		return filepath.Join(".hand", "profiles", profile.DefaultName)
	}

	profile.SetActive(resolved)
	return resolved.HomeDir
}

func loadUserHomeDir() string {
	home, err := userHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}

	return home
}

// DataDir returns the directory used for persistent Hand data.
func DataDir() string {
	return filepath.Join(HomeDir(), "data")
}

// DebugTraceDir returns the directory used for debug trace files.
func DebugTraceDir() string {
	return filepath.Join(HomeDir(), "traces")
}

// StateDBPath returns the path to the project state database.
func StateDBPath() string {
	return filepath.Join(DataDir(), "state.db")
}

// SessionDBPath returns the path to the project session database.
func SessionDBPath() string {
	return filepath.Join(DataDir(), "session.db")
}
