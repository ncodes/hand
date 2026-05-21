package composer

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistory_LoadMissingFileReturnsEmptyHistory(t *testing.T) {
	history, err := LoadHistory(filepath.Join(t.TempDir(), "missing.json"))

	require.NoError(t, err)
	require.Empty(t, history)
}

func TestHistory_SaveAndLoadRoundTripsNormalizedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "tui-history.json")

	err := SaveHistory(path, []string{" first ", "", "second", "second", "multi\nline", `path\to\thing`})
	require.NoError(t, err)

	history, err := LoadHistory(path)

	require.NoError(t, err)
	require.Equal(t, []string{"first", "second", "multi\nline", `path\to\thing`}, history)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"entries": ["first", "second", "multi\nline", "path\\to\\thing"]
	}`, string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestHistory_LoadRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui-history.json")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

	_, err := LoadHistory(path)

	require.Error(t, err)
}

func TestHistory_LoadReturnsReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-dir")
	require.NoError(t, os.MkdirAll(path, 0o700))

	_, err := LoadHistory(path)

	require.Error(t, err)
}

func TestHistory_SaveReturnsWriteError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-dir")
	require.NoError(t, os.MkdirAll(path, 0o700))

	err := SaveHistory(path, []string{"prompt"})

	require.Error(t, err)
}

func TestHistory_SaveReturnsCreateDirError(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(parent, []byte("not a dir"), 0o600))

	err := SaveHistory(filepath.Join(parent, "tui-history.json"), []string{"prompt"})

	require.Error(t, err)
}

func TestHistory_SaveSkipsEmptyPath(t *testing.T) {
	err := SaveHistory("", []string{"prompt"})

	require.NoError(t, err)
}

func TestNormalizeHistory_BoundsHistory(t *testing.T) {
	entries := make([]string, 0, MaxPromptHistory+5)
	for index := range MaxPromptHistory + 5 {
		entries = append(entries, "prompt "+strconv.Itoa(index))
	}

	history := NormalizeHistory(entries)

	require.Len(t, history, MaxPromptHistory)
	require.Equal(t, "prompt 5", history[0])
	require.Equal(t, "prompt 104", history[len(history)-1])
}
