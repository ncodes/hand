//go:build !windows

package native

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

func TestRunCommand_ToolKillsShellChildrenOnTimeout(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "run_command",
		Input: `{"command":"sleep 30 & child=$!; echo $child > child.pid; wait","timeout_seconds":1}`,
	})

	require.NoError(t, err)

	var payload struct {
		ExitCode int  `json:"exit_code"`
		TimedOut bool `json:"timed_out"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, -1, payload.ExitCode)
	require.True(t, payload.TimedOut)

	rawPID, readErr := os.ReadFile(filepath.Join(root, "child.pid"))
	require.NoError(t, readErr)
	childPID, parseErr := strconv.Atoi(string(bytesTrimSpace(rawPID)))
	require.NoError(t, parseErr)

	require.Eventually(t, func() bool {
		err := syscall.Kill(childPID, 0)
		return errors.Is(err, syscall.ESRCH)
	}, 3*time.Second, 50*time.Millisecond)
}

func bytesTrimSpace(value []byte) string {
	start := 0
	for start < len(value) && (value[start] == ' ' || value[start] == '\n' || value[start] == '\t' || value[start] == '\r') {
		start++
	}
	end := len(value)
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\t' || value[end-1] == '\r') {
		end--
	}

	return string(value[start:end])
}
