package process

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	processenv "github.com/wandxy/hand/internal/environment/process"
	"github.com/wandxy/hand/internal/tools"
	toolmocks "github.com/wandxy/hand/internal/tools/mocks"
)

func TestProcess_ToolRejectsInvalidJSON(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "invalid tool input")
}

func TestProcess_ToolValidatesAction(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})

	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Name: "process", Input: `{}`})
	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "action is required")

	result, err = definition.Handler.Invoke(context.Background(), tools.Call{Name: "process", Input: `{"action":"unknown"}`})
	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", `unsupported action "unknown"`)
}

func TestProcess_ToolStartDelegatesToRuntime(t *testing.T) {
	startedAt := time.Now().UTC()
	result, err := Definition(&toolmocks.Runtime{
		StartProcessFunc: func(_ context.Context, req processenv.StartRequest) (processenv.Info, error) {
			require.Equal(t, "printf", req.Command)
			require.Equal(t, []string{"hello"}, req.Args)
			require.Equal(t, "workspace", req.CWD)
			require.Equal(t, map[string]string{"KEY": "value"}, req.Env)
			require.Equal(t, 32, req.OutputBufferBytes)
			return processenv.Info{ID: "proc_1", Command: req.Command, Args: req.Args, CWD: req.CWD, Status: processenv.StatusRunning, StartedAt: startedAt}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":" printf ","args":["hello"],"cwd":" workspace ","env":{"KEY":"value"},"output_buffer_bytes":32}`,
	})

	require.NoError(t, err)
	var payload struct {
		Process processenv.Info `json:"process"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "proc_1", payload.Process.ID)
	require.Equal(t, processenv.StatusRunning, payload.Process.Status)
}

func TestProcess_ToolStartPassesNilEnvWhenEmpty(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		StartProcessFunc: func(_ context.Context, req processenv.StartRequest) (processenv.Info, error) {
			require.Nil(t, req.Env)
			return processenv.Info{ID: "proc_1", Status: processenv.StatusRunning, StartedAt: time.Now().UTC()}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"printf"}`,
	})

	require.NoError(t, err)
	require.Empty(t, result.Error)
}

func TestProcess_ToolRequiresCommandForStart(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"   "}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "command is required for start")
}

func TestProcess_ToolValidatesOutputBufferBytesForStart(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"printf","output_buffer_bytes":0}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "output_buffer_bytes must be greater than zero")
}

func TestProcess_ToolStatusReturnsProcess(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(processID string) (processenv.Info, error) {
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusExited}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"status","process_id":"proc_1"}`,
	})

	require.NoError(t, err)
	var payload struct {
		Process processenv.Info `json:"process"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "proc_1", payload.Process.ID)
	require.Equal(t, processenv.StatusExited, payload.Process.Status)
}

func TestProcess_ToolStatusReadAndStopRequireProcessID(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})

	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Name: "process", Input: `{"action":"status"}`})
	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "process_id is required for status")

	result, err = definition.Handler.Invoke(context.Background(), tools.Call{Name: "process", Input: `{"action":"read"}`})
	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "process_id is required for read")

	result, err = definition.Handler.Invoke(context.Background(), tools.Call{Name: "process", Input: `{"action":"stop"}`})
	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "process_id is required for stop")
}

func TestProcess_ToolReadReturnsTrimmedOutput(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(processID string) (processenv.Info, error) {
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(processID string) (processenv.Output, error) {
			require.Equal(t, "proc_1", processID)
			return processenv.Output{Stdout: "abcdef", Stderr: "uvwxyz", StdoutBytes: 6, StderrBytes: 6}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stdout_bytes":3,"stderr_bytes":2}`,
	})

	require.NoError(t, err)
	var payload struct {
		Process processenv.Info   `json:"process"`
		Output  processenv.Output `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "def", payload.Output.Stdout)
	require.Equal(t, "yz", payload.Output.Stderr)
}

func TestProcess_ToolReadTrimPreservesUTF8(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(processID string) (processenv.Info, error) {
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(processID string) (processenv.Output, error) {
			require.Equal(t, "proc_1", processID)
			return processenv.Output{Stdout: "AéB", StdoutBytes: len("AéB")}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stdout_bytes":2}`,
	})

	require.NoError(t, err)
	var payload struct {
		Output processenv.Output `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "B", payload.Output.Stdout)
}

func TestProcess_ToolReadReturnsRuntimeGetError(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(string) (processenv.Info, error) {
			return processenv.Info{}, errors.New("process not found")
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "process not found")
}

func TestProcess_ToolReadReturnsRuntimeReadError(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(processID string) (processenv.Info, error) {
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(string) (processenv.Output, error) {
			return processenv.Output{}, errors.New("read failed")
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "read failed")
}

func TestProcess_ToolReadValidatesLimits(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stdout_bytes":0}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "stdout_bytes must be greater than zero")

	result, err = Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stderr_bytes":0}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "stderr_bytes must be greater than zero")
}

func TestProcess_ToolListReturnsProcesses(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		ListProcessesFunc: func() []processenv.Info {
			return []processenv.Info{
				{ID: "proc_1", Status: processenv.StatusRunning},
				{ID: "proc_2", Status: processenv.StatusExited}}
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"list"}`,
	})

	require.NoError(t, err)
	var payload struct {
		Processes []processenv.Info `json:"processes"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Processes, 2)
}

func TestProcess_ToolReturnsRuntimeErrors(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		StartProcessFunc: func(context.Context, processenv.StartRequest) (processenv.Info, error) {
			return processenv.Info{}, errors.New("start failed")
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"printf"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "start failed")

	result, err = Definition(&toolmocks.Runtime{
		GetProcessFunc: func(string) (processenv.Info, error) {
			return processenv.Info{}, errors.New("process not found")
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"status","process_id":"proc_1"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "process not found")

	result, err = Definition(&toolmocks.Runtime{
		StopProcessFunc: func(context.Context, string) (processenv.Info, error) {
			return processenv.Info{}, errors.New("process not found")
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"stop","process_id":"proc_1"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "process not found")
}

func TestProcess_ToolStopReturnsProcess(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		StopProcessFunc: func(_ context.Context, processID string) (processenv.Info, error) {
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusStopped}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"stop","process_id":"proc_1"}`,
	})

	require.NoError(t, err)
	var payload struct {
		Process processenv.Info `json:"process"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "proc_1", payload.Process.ID)
	require.Equal(t, processenv.StatusStopped, payload.Process.Status)
}

func TestProcess_ToolRequiresRuntime(t *testing.T) {
	result, err := Definition(nil).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"list"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "process manager is not configured")
}

func requireToolError(t *testing.T, raw, code, message string) {
	t.Helper()
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(raw), &toolErr))
	require.Equal(t, code, toolErr.Code)
	require.Equal(t, message, toolErr.Message)
}
