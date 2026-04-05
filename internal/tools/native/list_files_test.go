package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

func TestListFiles_ToolListsFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0o755))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":true}`})

	require.NoError(t, err)
	var payload struct {
		Root    string `json:"root"`
		Path    string `json:"path"`
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, root, payload.Root)
	require.Equal(t, ".", payload.Path)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
	require.Equal(t, "file", payload.Entries[0].Type)
	require.Equal(t, int64(5), payload.Entries[0].Size)
	require.Equal(t, "nested", payload.Entries[1].Path)
	require.Equal(t, "dir", payload.Entries[1].Type)
	require.Equal(t, int64(0), payload.Entries[1].Size)
}

func TestListFiles_ToolListsDirectoryNonRecursivelyAndSkipsHiddenEntries(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".secret"), []byte("hidden"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested", "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "child", "beta.txt"), []byte("beta"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":false}`})

	require.NoError(t, err)
	var payload struct {
		Path    string `json:"path"`
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, ".", payload.Path)
	require.Len(t, payload.Entries, 2)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
	require.Equal(t, "file", payload.Entries[0].Type)
	require.Equal(t, "nested", payload.Entries[1].Path)
	require.Equal(t, "dir", payload.Entries[1].Type)
}

func TestListFiles_ToolDefaultsToNonRecursiveListing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested", "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "child", "beta.txt"), []byte("beta"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":"."}`})

	require.NoError(t, err)
	var payload struct {
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Entries, 2)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
	require.Equal(t, "file", payload.Entries[0].Type)
	require.Equal(t, "nested", payload.Entries[1].Path)
	require.Equal(t, "dir", payload.Entries[1].Type)
}

func TestListFiles_ToolIncludesHiddenEntriesWhenRequested(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".hidden", "secret.txt"), []byte("secret"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":true,"include_hidden":true}`})

	require.NoError(t, err)
	var payload struct {
		Entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Entries, 2)
	require.Equal(t, ".hidden", payload.Entries[0].Path)
	require.Equal(t, "dir", payload.Entries[0].Type)
	require.Equal(t, ".hidden/secret.txt", payload.Entries[1].Path)
	require.Equal(t, "file", payload.Entries[1].Type)
}

func TestListFiles_ToolAppliesEntryLimit(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "beta.txt"), []byte("beta"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "list_files", Input: `{"path":".","recursive":false,"max_entries":1}`})

	require.NoError(t, err)
	var payload struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Entries, 1)
	require.Equal(t, "alpha.txt", payload.Entries[0].Path)
}

func TestListFiles_DefinitionDeclaresStrictRequiredSchema(t *testing.T) {
	root := t.TempDir()
	definition := ListFilesDefinition(&testDependencies{
		filePolicy: guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots([]string{root})},
	})

	required, ok := definition.InputSchema["required"].([]string)
	require.True(t, ok)
	require.Equal(t, []string{"path", "recursive", "include_hidden", "max_entries"}, required)
}
