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

func TestReadFile_ToolReadsText(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "read_file", Input: `{"path":"file.txt"}`})
	require.NoError(t, err)

	var payload struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Bytes   int    `json:"bytes"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "file.txt", payload.Path)
	require.Equal(t, "hello", payload.Content)
	require.Equal(t, 5, payload.Bytes)
}

func TestReadFile_ToolRejectsInvalidJSONInput(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "read_file", Input: `{"path":`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "invalid tool input", toolErr.Message)
}

func TestReadFile_ToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "read_file", Input: `{"path":` + quoteJSON(outside) + `}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "path_outside_roots", toolErr.Code)
}

func TestReadFile_ToolRejectsDirectories(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0o755))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "read_file", Input: `{"path":"nested"}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
}
