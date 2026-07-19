package browser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverChromiumExecutable_UsesConfiguredPathOrPATH(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "custom-browser")
	require.NoError(t, os.WriteFile(executable, []byte("test"), 0o700))

	resolved, err := discoverChromiumExecutable(executable)
	require.NoError(t, err)
	require.Equal(t, executable, resolved)

	t.Setenv("PATH", directory)
	resolved, err = discoverChromiumExecutable("custom-browser")
	require.NoError(t, err)
	require.Equal(t, executable, resolved)

	_, err = discoverChromiumExecutable(filepath.Join(directory, "missing"))
	require.EqualError(t, err, "browser executable is unavailable")
}
