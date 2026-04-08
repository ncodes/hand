package runcommand

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
	nativemocks "github.com/wandxy/hand/internal/tools/mocks"
)

type runCommandPayload struct {
	ExitCode         int     `json:"exit_code"`
	Stdout           string  `json:"stdout"`
	Stderr           string  `json:"stderr"`
	TimedOut         bool    `json:"timed_out"`
	TimeoutSeconds   int     `json:"timeout_seconds"`
	ElapsedSeconds   float64 `json:"elapsed_seconds"`
	RemainingSeconds float64 `json:"remaining_seconds"`
}

func TestRunCommand_ToolRunsCommand(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"printf","args":["hello"]}`})

	require.NoError(t, err)
	var payload runCommandPayload
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 0, payload.ExitCode)
	require.Equal(t, "hello", payload.Stdout)
	require.Empty(t, payload.Stderr)
	require.False(t, payload.TimedOut)
	require.Equal(t, 30, payload.TimeoutSeconds)
	require.GreaterOrEqual(t, payload.ElapsedSeconds, 0.0)
	require.GreaterOrEqual(t, payload.RemainingSeconds, 0.0)
	require.LessOrEqual(t, payload.RemainingSeconds, float64(payload.TimeoutSeconds))
}

func TestRunCommand_ToolRejectsInvalidJSONInput(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "invalid tool input", toolErr.Message)
}

func TestRunCommand_ToolRequiresCommand(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"   "}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "command is required", toolErr.Message)
}

func TestRunCommand_ToolReturnsApprovalRequiredWithoutExecution(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() {
		commandContext = originalCommandContext
	})
	called := false
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		called = true
		return exec.CommandContext(ctx, name, args...)
	}

	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{Ask: []string{"git push"}}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"git","args":["push","origin","main"]}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "approval_required", toolErr.Code)
	require.Contains(t, toolErr.Message, "git push")
	require.False(t, called)
}

func TestRunCommand_ToolReturnsBuiltInApprovalMessageWithoutRule(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() {
		commandContext = originalCommandContext
	})
	called := false
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		called = true
		return nil
	}

	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"rm","args":["-rf","/"]}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "approval_required", toolErr.Code)
	require.Equal(t, "command requires approval", toolErr.Message)
	require.False(t, called)
}

func TestRunCommand_ToolReturnsDeniedWithoutExecution(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() {
		commandContext = originalCommandContext
	})
	called := false
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		called = true
		return exec.CommandContext(ctx, name, args...)
	}

	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{Deny: []string{"git push"}}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"git","args":["push","origin","main"]}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "command_denied", toolErr.Code)
	require.Contains(t, toolErr.Message, "matched deny rule")
	require.False(t, called)
}

func TestRunCommand_ToolRejectsOutsideWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "run_command",
		Input: `{"command":"printf","args":["hello"],"cwd":` + nativemocks.QuoteJSON(outside) + `}`,
	})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "path_outside_roots", toolErr.Code)
}

func TestRunCommand_ToolTimesOut(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"sleep","args":["2"],"timeout_seconds":1}`})

	require.NoError(t, err)
	var payload runCommandPayload
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, -1, payload.ExitCode)
	require.True(t, payload.TimedOut)
	require.Equal(t, 1, payload.TimeoutSeconds)
	require.GreaterOrEqual(t, payload.ElapsedSeconds, 0.0)
	require.Equal(t, 0.0, payload.RemainingSeconds)
}

func TestRunCommand_ToolPassesEnvironmentVariables(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "run_command",
		Input: `{"command":"printf %s \"$HAND_TEST_VAR\"","env":{"HAND_TEST_VAR":"visible"}}`,
	})

	require.NoError(t, err)
	var payload runCommandPayload
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 0, payload.ExitCode)
	require.Equal(t, "visible", payload.Stdout)
	require.False(t, payload.TimedOut)
	require.Equal(t, 30, payload.TimeoutSeconds)
	require.GreaterOrEqual(t, payload.ElapsedSeconds, 0.0)
	require.GreaterOrEqual(t, payload.RemainingSeconds, 0.0)
}

func TestRunCommand_ToolReturnsNonZeroExitCode(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"false"}`})

	require.NoError(t, err)
	var payload runCommandPayload
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 1, payload.ExitCode)
	require.False(t, payload.TimedOut)
	require.Equal(t, 30, payload.TimeoutSeconds)
	require.GreaterOrEqual(t, payload.ElapsedSeconds, 0.0)
	require.GreaterOrEqual(t, payload.RemainingSeconds, 0.0)
}

func TestRunCommand_ToolReportsClampedTimeout(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"printf","args":["hello"],"timeout_seconds":999}`})

	require.NoError(t, err)
	var payload runCommandPayload
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 120, payload.TimeoutSeconds)
	require.False(t, payload.TimedOut)
	require.GreaterOrEqual(t, payload.ElapsedSeconds, 0.0)
	require.GreaterOrEqual(t, payload.RemainingSeconds, 0.0)
	require.LessOrEqual(t, payload.RemainingSeconds, float64(payload.TimeoutSeconds))
}

func TestRunCommand_ToolSupportsContextCancellation(t *testing.T) {
	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := registry.Invoke(ctx, tools.Call{Name: "run_command", Input: `{"command":"printf","args":["hello"]}`})

	require.NoError(t, err)
	require.Empty(t, result.Output)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "command_failed", toolErr.Code)
	require.Contains(t, toolErr.Message, "context canceled")
}

func TestBuildCommand_UsesDirectExecutionWhenArgsAreProvided(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() {
		commandContext = originalCommandContext
	})

	var gotName string
	var gotArgs []string
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return exec.CommandContext(ctx, "printf", "")
	}

	_ = buildCommand(context.Background(), "git", []string{"status", "--short"})

	require.Equal(t, "git", gotName)
	require.Equal(t, []string{"status", "--short"}, gotArgs)
}

func TestBuildCommand_UsesShellWhenNoArgsAreProvided(t *testing.T) {
	originalCommandContext := commandContext
	originalGOOS := currentGOOS
	t.Cleanup(func() {
		commandContext = originalCommandContext
		currentGOOS = originalGOOS
	})

	var gotName string
	var gotArgs []string
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return exec.CommandContext(ctx, "printf", "")
	}
	currentGOOS = "linux"

	_ = buildCommand(context.Background(), "echo hello", nil)

	require.Equal(t, "sh", gotName)
	require.Equal(t, []string{"-lc", "echo hello"}, gotArgs)
}

func TestBuildCommand_UsesCmdOnWindowsWhenNoArgsAreProvided(t *testing.T) {
	originalCommandContext := commandContext
	originalGOOS := currentGOOS
	t.Cleanup(func() {
		commandContext = originalCommandContext
		currentGOOS = originalGOOS
	})

	var gotName string
	var gotArgs []string
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return exec.CommandContext(ctx, "printf", "")
	}
	currentGOOS = "windows"

	_ = buildCommand(context.Background(), "dir", nil)

	require.Equal(t, "cmd", gotName)
	require.Equal(t, []string{"/C", "dir"}, gotArgs)
}

func TestRunCommand_ToolKillsShellChildrenOnTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group assertions are unix-only")
	}

	root := t.TempDir()
	registry := nativemocks.RegisterRuntime(t, root, guardrails.CommandPolicy{}, Definition)

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
	childPID, parseErr := strconv.Atoi(bytesTrimSpace(rawPID))
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
