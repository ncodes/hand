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

func TestWriteFile_ToolCreatesFile(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "write_file", Input: `{"path":"nested/file.txt","content":"hello"}`})

	require.NoError(t, err)
	var payload struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytes_written"`
		Created      bool   `json:"created"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "nested/file.txt", payload.Path)
	require.Equal(t, 5, payload.BytesWritten)
	require.True(t, payload.Created)
	content, readErr := os.ReadFile(filepath.Join(root, "nested", "file.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "hello", string(content))
}

func TestWriteFile_ToolOverwritesExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("before"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "write_file", Input: `{"path":"file.txt","content":"after"}`})

	require.NoError(t, err)
	var payload struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytes_written"`
		Created      bool   `json:"created"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "file.txt", payload.Path)
	require.Equal(t, 5, payload.BytesWritten)
	require.False(t, payload.Created)
	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "after", string(content))
}
