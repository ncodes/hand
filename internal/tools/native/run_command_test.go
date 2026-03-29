package native

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

func TestRunCommand_ToolRunsCommand(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"printf","args":["hello"]}`})

	require.NoError(t, err)
	var payload struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		TimedOut bool   `json:"timed_out"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 0, payload.ExitCode)
	require.Equal(t, "hello", payload.Stdout)
	require.Empty(t, payload.Stderr)
	require.False(t, payload.TimedOut)
}

func TestRunCommand_ToolRejectsInvalidJSONInput(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "invalid tool input", toolErr.Message)
}

func TestRunCommand_ToolRequiresCommand(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

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
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{Ask: []string{"git push"}})

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
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

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
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{Deny: []string{"git push"}})

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
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "run_command",
		Input: `{"command":"printf","args":["hello"],"cwd":` + quoteJSON(outside) + `}`,
	})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "path_outside_roots", toolErr.Code)
}

func TestRunCommand_ToolTimesOut(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"sleep","args":["2"],"timeout_seconds":1}`})

	require.NoError(t, err)
	var payload struct {
		ExitCode int  `json:"exit_code"`
		TimedOut bool `json:"timed_out"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, -1, payload.ExitCode)
	require.True(t, payload.TimedOut)
}

func TestRunCommand_ToolPassesEnvironmentVariables(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "run_command",
		Input: `{"command":"printf %s \"$HAND_TEST_VAR\"","env":{"HAND_TEST_VAR":"visible"}}`,
	})

	require.NoError(t, err)
	var payload struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		TimedOut bool   `json:"timed_out"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 0, payload.ExitCode)
	require.Equal(t, "visible", payload.Stdout)
	require.False(t, payload.TimedOut)
}

func TestRunCommand_ToolReturnsNonZeroExitCode(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "run_command", Input: `{"command":"false"}`})

	require.NoError(t, err)
	var payload struct {
		ExitCode int  `json:"exit_code"`
		TimedOut bool `json:"timed_out"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 1, payload.ExitCode)
	require.False(t, payload.TimedOut)
}

func TestRunCommand_ToolSupportsContextCancellation(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})
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
