package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHelpers_SplitAndDedupeCSVAndBools(t *testing.T) {
	require.Nil(t, splitAndTrimCSV(""))
	require.Equal(t, []string{"a", "b"}, splitAndTrimCSV(" a, ,b ,,"))

	require.Nil(t, dedupeAndTrim(nil))
	require.Equal(t, []string{"a", "b"}, dedupeAndTrim([]string{" a ", "", "b", "a"}))

	require.False(t, getBoolValue(nil))
	require.True(t, getBoolValue(new(true)))
	require.True(t, getBoolValueDefault(nil, true))
	require.False(t, getBoolValueDefault(new(false), true))
}

func TestResolvePathsFromBase_HandlesEmptyAndAbsolute(t *testing.T) {
	require.Nil(t, getPathsFromBase(nil, "/tmp"))
	require.Equal(t, []string{"a", "b"}, getPathsFromBase([]string{"a", "b"}, ""))

	abs := filepath.Join(string(os.PathSeparator), "tmp", "x")
	require.Equal(t, []string{abs, filepath.Join("/base", "rel")},
		getPathsFromBase([]string{abs, "rel"}, "/base"))
}

func TestDefaultFSRootsAndNormalizeFSRootsFallbackWhenGetwdFails(t *testing.T) {
	originalGetwd := getwd
	t.Cleanup(func() {
		getwd = originalGetwd
	})

	getwd = func() (string, error) {
		return "", errors.New("cwd missing")
	}

	require.Equal(t, []string{"."}, getDefaultFSRoots())
	require.Equal(t, []string{"."}, normalizeFSRoots([]string{"."}))
}

func TestNormalizeFSRoots_PreservesAbsoluteRoots(t *testing.T) {
	abs := filepath.Join(string(os.PathSeparator), "tmp", "workspace")
	require.Equal(t, []string{abs}, normalizeFSRoots([]string{abs}))
}
