package process

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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

func TestProcess_ToolRejectsReadOnlyFieldsForNonReadActions(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})

	testCases := []struct {
		name    string
		input   string
		message string
	}{
		{
			name:    "start next stdout cursor",
			input:   `{"action":"start","command":"printf","stdout_cursor":0}`,
			message: "stdout_cursor is only supported for read",
		},
		{
			name:    "status next stderr cursor",
			input:   `{"action":"status","process_id":"proc_1","stderr_cursor":0}`,
			message: "stderr_cursor is only supported for read",
		},
		{
			name:    "stop stdout bytes",
			input:   `{"action":"stop","process_id":"proc_1","stdout_bytes":16}`,
			message: "stdout_bytes is only supported for read",
		},
		{
			name:    "list stderr bytes",
			input:   `{"action":"list","stderr_bytes":16}`,
			message: "stderr_bytes is only supported for read",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := definition.Handler.Invoke(context.Background(), tools.Call{
				Name:  "process",
				Input: tc.input,
			})

			require.NoError(t, err)
			requireToolError(t, result.Error, "invalid_input", tc.message)
		})
	}
}

func TestProcess_ToolStartDelegatesToRuntime(t *testing.T) {
	startedAt := time.Now().UTC()
	result, err := Definition(&toolmocks.Runtime{
		StartProcessFunc: func(_ context.Context, sessionID string, req processenv.StartRequest) (processenv.Info, error) {
			require.Equal(t, "session-1", sessionID)
			require.Equal(t, "printf", req.Command)
			require.Equal(t, []string{"hello"}, req.Args)
			require.Equal(t, "workspace", req.CWD)
			require.Equal(t, map[string]string{"KEY": "value"}, req.Env)
			require.Equal(t, 32, req.OutputBufferBytes)
			return processenv.Info{ID: "proc_1", Command: req.Command, Args: req.Args, CWD: req.CWD, Status: processenv.StatusRunning, StartedAt: startedAt}, nil
		},
	}).Handler.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
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
		StartProcessFunc: func(_ context.Context, sessionID string, req processenv.StartRequest) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
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
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
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
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", req.ProcessID)
			require.Nil(t, req.StdoutCursor)
			require.Nil(t, req.StderrCursor)
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
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", req.ProcessID)
			require.Nil(t, req.StdoutCursor)
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
		GetProcessFunc: func(sessionID string, _ string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
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
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(sessionID string, _ processenv.ReadRequest) (processenv.Output, error) {
			require.Equal(t, "default", sessionID)
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

func TestProcess_ToolReadSupportsCursorSemantics(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "session-42", sessionID)
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
			require.Equal(t, "session-42", sessionID)
			require.Equal(t, "proc_1", req.ProcessID)
			require.NotNil(t, req.StdoutCursor)
			require.Equal(t, 3, *req.StdoutCursor)
			require.Nil(t, req.StderrCursor)
			return processenv.Output{
				Stdout:              "def",
				StdoutBytes:         6,
				NextStdoutCursor:    6,
				StdoutCursorExpired: true,
			}, nil
		},
	}).Handler.Invoke(tools.WithSessionID(context.Background(), "session-42"), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stdout_cursor":3}`,
	})

	require.NoError(t, err)
	var payload struct {
		Output processenv.Output `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "def", payload.Output.Stdout)
	require.Equal(t, 6, payload.Output.NextStdoutCursor)
	require.True(t, payload.Output.StdoutCursorExpired)
}

func TestProcess_ToolReadRejectsInvalidCursorCombinations(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stdout_cursor":0,"stdout_bytes":4}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "stdout_cursor cannot be combined with stdout_bytes")

	result, err = Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stderr_cursor":0,"stderr_bytes":4}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "stderr_cursor cannot be combined with stderr_bytes")

	result, err = Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stdout_cursor":-1}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "stdout_cursor must be greater than or equal to zero")

	result, err = Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stderr_cursor":-1}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "stderr_cursor must be greater than or equal to zero")
}

func TestProcess_ToolReadSupportsStderrCursorSemantics(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", req.ProcessID)
			require.Nil(t, req.StdoutCursor)
			require.NotNil(t, req.StderrCursor)
			require.Equal(t, 2, *req.StderrCursor)
			return processenv.Output{
				Stdout:           "abcdef",
				Stderr:           "xyz",
				StdoutBytes:      6,
				StderrBytes:      3,
				NextStderrCursor: 3,
				NextStdoutCursor: 6,
			}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stderr_cursor":2,"stdout_bytes":3}`,
	})

	require.NoError(t, err)
	var payload struct {
		Output processenv.Output `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "def", payload.Output.Stdout)
	require.Equal(t, "xyz", payload.Output.Stderr)
	require.Equal(t, 3, payload.Output.NextStderrCursor)
}

func TestProcess_ToolListReturnsProcesses(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		ListProcessesFunc: func(sessionID string) []processenv.Info {
			require.Equal(t, "default", sessionID)
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
		StartProcessFunc: func(context.Context, string, processenv.StartRequest) (processenv.Info, error) {
			return processenv.Info{}, errors.New("start failed")
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"printf"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "start failed")

	result, err = Definition(&toolmocks.Runtime{
		GetProcessFunc: func(string, string) (processenv.Info, error) {
			return processenv.Info{}, errors.New("process not found")
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"status","process_id":"proc_1"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "process not found")

	result, err = Definition(&toolmocks.Runtime{
		StopProcessFunc: func(context.Context, string, string) (processenv.Info, error) {
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
		StopProcessFunc: func(_ context.Context, sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
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

func TestProcess_ToolPropagatesExplicitSessionIDForNonStartActions(t *testing.T) {
	ctx := tools.WithSessionID(context.Background(), "session-42")

	t.Run("status", func(t *testing.T) {
		result, err := Definition(&toolmocks.Runtime{
			GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
				require.Equal(t, "session-42", sessionID)
				require.Equal(t, "proc_1", processID)
				return processenv.Info{ID: processID, Status: processenv.StatusExited}, nil
			},
		}).Handler.Invoke(ctx, tools.Call{
			Name:  "process",
			Input: `{"action":"status","process_id":"proc_1"}`,
		})

		require.NoError(t, err)
		require.Empty(t, result.Error)
	})

	t.Run("read", func(t *testing.T) {
		result, err := Definition(&toolmocks.Runtime{
			GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
				require.Equal(t, "session-42", sessionID)
				require.Equal(t, "proc_1", processID)
				return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
			},
			ReadProcessFunc: func(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
				require.Equal(t, "session-42", sessionID)
				require.Equal(t, "proc_1", req.ProcessID)
				return processenv.Output{Stdout: "hello"}, nil
			},
		}).Handler.Invoke(ctx, tools.Call{
			Name:  "process",
			Input: `{"action":"read","process_id":"proc_1"}`,
		})

		require.NoError(t, err)
		require.Empty(t, result.Error)
	})

	t.Run("stop", func(t *testing.T) {
		result, err := Definition(&toolmocks.Runtime{
			StopProcessFunc: func(_ context.Context, sessionID string, processID string) (processenv.Info, error) {
				require.Equal(t, "session-42", sessionID)
				require.Equal(t, "proc_1", processID)
				return processenv.Info{ID: processID, Status: processenv.StatusStopped}, nil
			},
		}).Handler.Invoke(ctx, tools.Call{
			Name:  "process",
			Input: `{"action":"stop","process_id":"proc_1"}`,
		})

		require.NoError(t, err)
		require.Empty(t, result.Error)
	})

	t.Run("list", func(t *testing.T) {
		result, err := Definition(&toolmocks.Runtime{
			ListProcessesFunc: func(sessionID string) []processenv.Info {
				require.Equal(t, "session-42", sessionID)
				return []processenv.Info{{ID: "proc_1", Status: processenv.StatusRunning}}
			},
		}).Handler.Invoke(ctx, tools.Call{
			Name:  "process",
			Input: `{"action":"list"}`,
		})

		require.NoError(t, err)
		require.Empty(t, result.Error)
	})
}

func TestProcess_ToolRequiresRuntime(t *testing.T) {
	result, err := Definition(nil).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"list"}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "tool_error", "process manager is not configured")
}

func TestProcess_EncodeProcessOutputReturnsInternalErrorOnMarshalFailure(t *testing.T) {
	result := encodeProcessOutput(map[string]any{
		"bad": math.NaN(),
	})

	requireToolError(t, result.Error, "internal_error", "failed to encode tool output")
}

func requireToolError(t *testing.T, raw, code, message string) {
	t.Helper()
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(raw), &toolErr))
	require.Equal(t, code, toolErr.Code)
	require.Equal(t, message, toolErr.Message)
}
