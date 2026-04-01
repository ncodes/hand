package datadir

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectHomeDir_UsesHANDHOME(t *testing.T) {
	originalGetenv := getenv
	originalUserHomeDir := userHomeDir
	getenv = func(key string) string {
		if key == "HAND_HOME" {
			return "/tmp/custom-hand"
		}

		return ""
	}
	userHomeDir = func() (string, error) {
		return "/Users/ignored", nil
	}
	defer func() {
		getenv = originalGetenv
		userHomeDir = originalUserHomeDir
	}()

	require.Equal(t, "/tmp/custom-hand", ProjectHomeDir())
}

func TestProjectHomeDir_UsesUserHomeDir(t *testing.T) {
	originalGetenv := getenv
	originalUserHomeDir := userHomeDir
	getenv = func(string) string { return "" }
	userHomeDir = func() (string, error) {
		return "/Users/me", nil
	}
	defer func() {
		getenv = originalGetenv
		userHomeDir = originalUserHomeDir
	}()

	require.Equal(t, filepath.Join("/Users/me", ".hand"), ProjectHomeDir())
}

func TestProjectHomeDir_FallsBackWhenUserHomeFails(t *testing.T) {
	originalGetenv := getenv
	originalUserHomeDir := userHomeDir
	getenv = func(string) string { return "" }
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	defer func() {
		getenv = originalGetenv
		userHomeDir = originalUserHomeDir
	}()

	require.Equal(t, ".hand", ProjectHomeDir())
}

func TestProjectPaths_DeriveFromProjectHomeDir(t *testing.T) {
	originalGetenv := getenv
	originalUserHomeDir := userHomeDir
	getenv = func(string) string { return "" }
	userHomeDir = func() (string, error) {
		return "/Users/me", nil
	}
	defer func() {
		getenv = originalGetenv
		userHomeDir = originalUserHomeDir
	}()

	require.Equal(t, filepath.Join("/Users/me", ".hand"), HomeDir())
	require.Equal(t, filepath.Join("/Users/me", ".hand", "data"), DataDir())
	require.Equal(t, filepath.Join("/Users/me", ".hand", "traces"), DebugTraceDir())
	require.Equal(t, filepath.Join("/Users/me", ".hand", "data", "state.db"), StateDBPath())
	require.Equal(t, filepath.Join("/Users/me", ".hand", "data", "session.db"), SessionDBPath())
}
