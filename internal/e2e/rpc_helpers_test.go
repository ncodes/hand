package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

func TestNewDefaultRPCHarness_UsesDefaultSpecAndConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "hand-home")

	h, err := NewDefaultRPCHarness(context.Background(), home, NewTextClient("hello"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})

	assert.Equal(t, "127.0.0.1", h.Address())
	assert.NotZero(t, h.Port())
}

func TestWriteRPCConfigFile_WritesExpectedContent(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteRPCConfigFile(dir, "127.0.0.1", 8123, RPCConfigOptions{
		Name:     "rpc-agent",
		Stream:   true,
		Instruct: "be brief",
		NoColor:  true,
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "config.yaml"), path)
	assert.Contains(t, string(raw), "name: rpc-agent")
	assert.Contains(t, string(raw), "stream: true")
	assert.Contains(t, string(raw), "address: 127.0.0.1")
	assert.Contains(t, string(raw), "port: 8123")
	assert.Contains(t, string(raw), "noColor: true")
	assert.Contains(t, string(raw), "instruct: be brief")
}

func TestWriteRPCConfigFile_DefaultsName(t *testing.T) {
	path, err := WriteRPCConfigFile(t.TempDir(), "127.0.0.1", 9000, RPCConfigOptions{})
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "name: yaml-agent")
	assert.NotContains(t, string(raw), "instruct:")
}

func TestWriteRPCConfigFile_ReturnsWriteError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	_, err := WriteRPCConfigFile(file, "127.0.0.1", 9000, RPCConfigOptions{})
	require.Error(t, err)
}

func TestMissingTools(t *testing.T) {
	assert.NoError(t, MissingTools("run_command")(models.Request{
		Tools: []models.ToolDefinition{{Name: "read_file"}},
	}))

	err := MissingTools("read_file")(models.Request{
		Tools: []models.ToolDefinition{{Name: "read_file"}},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool "read_file" to be unavailable, got tools [read_file]`)
}

func TestToolMessagePresent(t *testing.T) {
	assert.NoError(t, ToolMessagePresent("call-1", "time")(models.Request{
		Messages: []handmsg.Message{{Role: handmsg.RoleTool, Name: "time", ToolCallID: "call-1"}},
	}))

	err := ToolMessagePresent("call-1", "time")(models.Request{
		Messages: []handmsg.Message{{Role: handmsg.RoleTool, Name: "clock", ToolCallID: "call-1"}},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool message name "time"`)

	err = ToolMessagePresent("call-1", "time")(models.Request{})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool message for tool call "call-1"`)

	err = ToolMessagePresent("call-1", "time")(models.Request{
		Messages: []handmsg.Message{
			{Role: handmsg.RoleAssistant, Name: "time", ToolCallID: "call-1"},
			{Role: handmsg.RoleTool, Name: "time", ToolCallID: "other-call"},
		},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool message for tool call "call-1"`)
}

func TestToolError(t *testing.T) {
	assert.NoError(t, ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{
		Messages: []handmsg.Message{{
			Role:       handmsg.RoleTool,
			Name:       "read_file",
			ToolCallID: "call-1",
			Content:    `{"name":"read_file","error":{"code":"path_outside_roots","message":"path is outside allowed roots"}}`,
		}},
	}))

	err := ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{
		Messages: []handmsg.Message{{
			Role:       handmsg.RoleTool,
			Name:       "clock",
			ToolCallID: "call-1",
			Content:    `{"name":"clock","error":{"code":"path_outside_roots","message":"path is outside allowed roots"}}`,
		}},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool message name "read_file"`)

	err = ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{
		Messages: []handmsg.Message{{
			Role:       handmsg.RoleTool,
			Name:       "read_file",
			ToolCallID: "call-1",
			Content:    `{`,
		}},
	})
	require.Error(t, err)

	err = ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{
		Messages: []handmsg.Message{{
			Role:       handmsg.RoleTool,
			Name:       "read_file",
			ToolCallID: "call-1",
			Content:    `{"name":"wrong","error":{"code":"path_outside_roots","message":"path is outside allowed roots"}}`,
		}},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool payload name "read_file"`)

	err = ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{
		Messages: []handmsg.Message{{
			Role:       handmsg.RoleTool,
			Name:       "read_file",
			ToolCallID: "call-1",
			Content:    `{"name":"read_file","error":{"code":"wrong","message":"path is outside allowed roots"}}`,
		}},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool error code "path_outside_roots", got "wrong"`)

	err = ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{
		Messages: []handmsg.Message{{
			Role:       handmsg.RoleTool,
			Name:       "read_file",
			ToolCallID: "call-1",
			Content:    `{"name":"read_file","error":{"code":"path_outside_roots","message":"wrong"}}`,
		}},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool error message "path is outside allowed roots", got "wrong"`)

	err = ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool error for tool call "call-1"`)

	err = ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots")(models.Request{
		Messages: []handmsg.Message{
			{Role: handmsg.RoleAssistant, Name: "read_file", ToolCallID: "call-1"},
			{Role: handmsg.RoleTool, Name: "read_file", ToolCallID: "other-call", Content: `{"name":"read_file","error":{"code":"path_outside_roots","message":"path is outside allowed roots"}}`},
		},
	})
	require.Error(t, err)
	assert.EqualError(t, err, `expected tool error for tool call "call-1"`)
}
