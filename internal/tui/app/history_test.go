package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptHistory_LoadMissingFileReturnsEmptyHistory(t *testing.T) {
	withPromptHistoryPath(t, filepath.Join(t.TempDir(), "missing.json"))

	history, err := loadPromptHistory()

	require.NoError(t, err)
	require.Empty(t, history)
}

func TestPromptHistory_SaveAndLoadRoundTripsNormalizedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "tui-history.json")
	withPromptHistoryPath(t, path)

	err := savePromptHistory([]string{" first ", "", "second", "second", "multi\nline", `path\to\thing`})
	require.NoError(t, err)

	history, err := loadPromptHistory()

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

func TestPromptHistory_LoadRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui-history.json")
	withPromptHistoryPath(t, path)
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

	_, err := loadPromptHistory()

	require.Error(t, err)
}

func TestPromptHistory_LoadReturnsReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-dir")
	require.NoError(t, os.MkdirAll(path, 0o700))
	withPromptHistoryPath(t, path)

	_, err := loadPromptHistory()

	require.Error(t, err)
}

func TestPromptHistory_SaveReturnsWriteError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-dir")
	require.NoError(t, os.MkdirAll(path, 0o700))
	withPromptHistoryPath(t, path)

	err := savePromptHistory([]string{"prompt"})

	require.Error(t, err)
}

func TestPromptHistory_SaveReturnsCreateDirError(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(parent, []byte("not a dir"), 0o600))
	withPromptHistoryPath(t, filepath.Join(parent, "tui-history.json"))

	err := savePromptHistory([]string{"prompt"})

	require.Error(t, err)
}

func TestPromptHistory_SaveSkipsEmptyPath(t *testing.T) {
	withPromptHistoryPath(t, "")

	err := savePromptHistory([]string{"prompt"})

	require.NoError(t, err)
}

func TestNormalizePromptHistory_BoundsHistory(t *testing.T) {
	entries := make([]string, 0, maxPromptHistory+5)
	for index := range maxPromptHistory + 5 {
		entries = append(entries, "prompt "+strconv.Itoa(index))
	}

	history := normalizePromptHistory(entries)

	require.Len(t, history, maxPromptHistory)
	require.Equal(t, "prompt 5", history[0])
	require.Equal(t, "prompt 104", history[len(history)-1])
}

func TestModel_NewModelLoadsPromptHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui-history.json")
	withPromptHistoryPath(t, path)
	require.NoError(t, savePromptHistory([]string{"first", "second"}))

	runModel := newModel()

	require.Equal(t, []string{"first", "second"}, runModel.history)
	require.Equal(t, 2, runModel.historyAt)
}

func TestModel_NewModelReportsPromptHistoryLoadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-dir")
	require.NoError(t, os.MkdirAll(path, 0o700))
	withPromptHistoryPath(t, path)

	runModel := newModel()

	require.Empty(t, runModel.history)
	require.Equal(t, "prompt history unavailable", runModel.status.Text())
}

func TestModel_SubmitPromptPersistsHistoryForRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui-history.json")
	withPromptHistoryPath(t, path)

	runModel := newModel()
	runModel.input.SetValue("remember this")
	require.Nil(t, runModel.submitPrompt())

	restarted := newModel()

	require.Equal(t, []string{"remember this"}, restarted.history)
	require.Equal(t, 1, restarted.historyAt)
}

func TestModel_SubmitSlashCommandDoesNotPersistPromptHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui-history.json")
	withPromptHistoryPath(t, path)

	runModel := newModel()
	runModel.input.SetValue("/clear")
	require.NotNil(t, runModel.submitPrompt())

	require.Empty(t, runModel.history)
	restarted := newModel()
	require.Empty(t, restarted.history)
}

func TestModel_AddPromptHistoryReportsSaveFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-dir")
	require.NoError(t, os.MkdirAll(path, 0o700))
	withPromptHistoryPath(t, path)

	runModel := newModel()
	runModel.addPromptHistory("remember this")

	require.Equal(t, []string{"remember this"}, runModel.history)
	require.Equal(t, "prompt history unavailable", runModel.status.Text())
}

func withPromptHistoryPath(t *testing.T, path string) {
	t.Helper()

	original := promptHistoryPath
	promptHistoryPath = func() string {
		return path
	}
	t.Cleanup(func() {
		promptHistoryPath = original
	})
}
