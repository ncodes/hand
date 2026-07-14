package process

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/agent/runcontext"
	processenv "github.com/wandxy/morph/internal/environment/process"
	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/permissions"
	storage "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/internal/tools"
	toolmocks "github.com/wandxy/morph/internal/tools/mocks"
	"github.com/wandxy/morph/pkg/nanoid"
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

func TestProcess_EnforcementDeniesStartBeforeRuntimeMutation(t *testing.T) {
	called := false
	runtime := &toolmocks.Runtime{StartProcessFunc: func(
		context.Context,
		string,
		processenv.StartRequest,
	) (processenv.Info, error) {
		called = true
		return processenv.Info{}, nil
	}}
	registry := tools.NewInMemoryRegistry(tools.RegistryOptions{PermissionPolicy: permissions.Policy{
		Mode: permissions.ModeEnforce,
		Rules: []permissions.Rule{{
			Name: "deny process start", Actions: []permissions.Action{permissions.ActionStart}, Decision: permissions.DecisionDeny,
		}},
	}})
	require.NoError(t, registry.Register(Definition(runtime)))
	ctx := permissions.WithContext(context.Background(), permissions.AuthorizationContext{
		Actor: permissions.Actor{Kind: permissions.ActorLocalOwner}, Surface: permissions.SurfaceCLI,
	})

	result, err := registry.Invoke(ctx, tools.Call{
		Name: "process", Input: `{"action":"start","command":"printf","args":["hello"]}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, permissions.ErrorCodeDenied, "permission denied")
	require.False(t, called)
}

func TestProcess_ResolvePermissionClassifiesActions(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})
	tests := []struct {
		name    string
		input   string
		action  permissions.Action
		effects []permissions.Effect
		target  string
	}{
		{name: "start", input: `{"action":"start","command":"git","args":["status"]}`, action: permissions.ActionStart, effects: []permissions.Effect{permissions.EffectExecution, permissions.EffectWrite}, target: "git status"},
		{name: "status", input: `{"action":"status","process_id":"proc-1"}`, action: permissions.ActionRead, effects: []permissions.Effect{permissions.EffectRead}, target: "proc-1"},
		{name: "read", input: `{"action":"read","process_id":"proc-1"}`, action: permissions.ActionRead, effects: []permissions.Effect{permissions.EffectRead}, target: "proc-1"},
		{name: "stop", input: `{"action":"stop","process_id":"proc-1"}`, action: permissions.ActionStop, effects: []permissions.Effect{permissions.EffectDestructive, permissions.EffectExecution, permissions.EffectWrite}, target: "proc-1"},
		{name: "list", input: `{"action":"list"}`, action: permissions.ActionList, effects: []permissions.Effect{permissions.EffectRead}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := definition.ResolvePermission(context.Background(), tools.Call{Input: test.input})

			require.NoError(t, err)
			require.Len(t, inputs, 1)
			require.Equal(t, test.action, inputs[0].Operation.Action)
			require.Equal(t, test.effects, inputs[0].Operation.Effects)
			require.Equal(t, test.target, inputs[0].Operation.Target)
		})
	}
}

func TestProcess_ResolvePermissionPreservesCommandConstraints(t *testing.T) {
	tests := []struct {
		name               string
		policy             guardrails.CommandPolicy
		wantHardDeny       bool
		wantApprovalReason string
	}{
		{name: "deny", policy: guardrails.CommandPolicy{Deny: []string{"git push"}}, wantHardDeny: true},
		{name: "ask", policy: guardrails.CommandPolicy{Ask: []string{"git push"}}, wantApprovalReason: "command requires approval: git push"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := Definition(toolmocks.NewRuntime(t.TempDir(), test.policy))
			inputs, err := definition.ResolvePermission(context.Background(), tools.Call{
				Input: `{"action":"start","command":"git","args":["push","origin","main"]}`,
			})

			require.NoError(t, err)
			require.Equal(t, test.wantHardDeny, inputs[0].HardDenyReason != "")
			require.Equal(t, test.wantApprovalReason, inputs[0].ApprovalReason)
		})
	}
}

func TestProcess_ResolvePermissionRejectsInvalidCalls(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})
	tests := []struct {
		name    string
		input   string
		message string
	}{
		{name: "malformed", input: `{"action":`, message: "invalid tool input"},
		{name: "missing action", input: `{}`, message: "action is required"},
		{name: "unknown action", input: `{"action":"unknown"}`, message: `unsupported action "unknown"`},
		{name: "missing command", input: `{"action":"start"}`, message: "command is required for start"},
		{name: "invalid fields", input: `{"action":"list","stdout_bytes":1}`, message: "invalid process list request: stdout_bytes is only valid for action=read"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := definition.ResolvePermission(context.Background(), tools.Call{Input: test.input})

			require.EqualError(t, err, test.message)
			require.Nil(t, inputs)
		})
	}
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
			input:   `{"action":"start","command":"printf","stdout_cursor":1}`,
			message: "invalid process start request: stdout_cursor is only valid for action=read; use output_buffer_bytes to configure retained output",
		},
		{
			name:    "start multiple read fields",
			input:   `{"action":"start","command":"printf","stdout_bytes":16,"stderr_cursor":1}`,
			message: "invalid process start request: stderr_cursor, stdout_bytes are only valid for action=read; use output_buffer_bytes to configure retained output",
		},
		{
			name:    "status next stderr cursor",
			input:   `{"action":"status","process_id":"proc_1","stderr_cursor":1}`,
			message: "invalid process status request: stderr_cursor is only valid for action=read",
		},
		{
			name:    "stop stdout bytes",
			input:   `{"action":"stop","process_id":"proc_1","stdout_bytes":16}`,
			message: "invalid process stop request: stdout_bytes is only valid for action=read",
		},
		{
			name:    "list stderr bytes",
			input:   `{"action":"list","stderr_bytes":16}`,
			message: "invalid process list request: stderr_bytes is only valid for action=read",
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

func TestProcess_ToolIgnoresZeroReadOnlyFieldsForNonReadActions(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{
		StartProcessFunc: func(context.Context, string, processenv.StartRequest) (processenv.Info, error) {
			return processenv.Info{ID: "proc_1", Status: processenv.StatusRunning}, nil
		},
		GetProcessFunc: func(string, string) (processenv.Info, error) {
			return processenv.Info{ID: "proc_1", Status: processenv.StatusRunning}, nil
		},
		StopProcessFunc: func(context.Context, string, string) (processenv.Info, error) {
			return processenv.Info{ID: "proc_1", Status: processenv.StatusStopped}, nil
		},
		ListProcessesFunc: func(string) []processenv.Info {
			return nil
		},
	})

	testCases := []string{
		`{"action":"start","command":"printf","stdout_cursor":0}`,
		`{"action":"status","process_id":"proc_1","stderr_cursor":0}`,
		`{"action":"stop","process_id":"proc_1","stdout_bytes":0}`,
		`{"action":"list","stderr_bytes":0}`,
	}

	for _, input := range testCases {
		t.Run(input, func(t *testing.T) {
			result, err := definition.Handler.Invoke(context.Background(), tools.Call{
				Name:  "process",
				Input: input,
			})

			require.NoError(t, err)
			require.Empty(t, result.Error)
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
			require.Equal(t, "printer", req.Label)
			require.Equal(t, 32, req.OutputBufferBytes)
			return processenv.Info{
				ID:        "proc_1",
				Label:     req.Label,
				Command:   req.Command,
				Args:      req.Args,
				CWD:       req.CWD,
				Status:    processenv.StatusRunning,
				StartedAt: startedAt,
			}, nil
		},
	}).Handler.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":" printf ","args":["hello"],"cwd":" workspace ","env":{"KEY":"value"},"label":" printer ","output_buffer_bytes":32}`,
	})

	require.NoError(t, err)
	var payload struct {
		Process processenv.Info `json:"process"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "proc_1", payload.Process.ID)
	require.Equal(t, "printer", payload.Process.Label)
	require.Equal(t, processenv.StatusRunning, payload.Process.Status)
}

func TestProcess_ToolStatusAcceptsLabel(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "sleep_5min", processID)
			return processenv.Info{ID: "proc_1", Label: "sleep_5min", Status: processenv.StatusRunning}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"status","process_id":"sleep_5min"}`,
	})

	require.NoError(t, err)
	var payload struct {
		Process processenv.Info `json:"process"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "proc_1", payload.Process.ID)
	require.Equal(t, "sleep_5min", payload.Process.Label)
}

func TestProcess_ToolUsesChildSessionIDForChildState(t *testing.T) {
	parentID := nanoid.MustFromSeed(storage.SessionIDPrefix, "parent", "ProcessToolLineageTestSeed")
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "ProcessToolLineageTestSeed")
	parent, err := runcontext.NewParent(parentID)
	require.NoError(t, err)
	child, err := parent.NewChild(runcontext.ChildOptions{
		ChildSessionID: childID,
		RunID:          "run_process",
	})
	require.NoError(t, err)

	result, err := Definition(&toolmocks.Runtime{
		StartProcessFunc: func(_ context.Context, sessionID string, req processenv.StartRequest) (processenv.Info, error) {
			require.Equal(t, childID, sessionID)
			require.Equal(t, "printf", req.Command)
			return processenv.Info{ID: "proc_1", Status: processenv.StatusRunning, StartedAt: time.Now().UTC()}, nil
		},
	}).Handler.Invoke(tools.WithRunContext(context.Background(), child), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"printf"}`,
	})

	require.NoError(t, err)
	require.Empty(t, result.Error)
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

func TestProcess_ToolAllowsZeroOutputBufferBytesForStart(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		StartProcessFunc: func(_ context.Context, _ string, req processenv.StartRequest) (processenv.Info, error) {
			require.Zero(t, req.OutputBufferBytes)
			return processenv.Info{ID: "proc_1", Status: processenv.StatusRunning}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"printf","output_buffer_bytes":0}`,
	})

	require.NoError(t, err)
	require.Empty(t, result.Error)
}

func TestProcess_ToolValidatesOutputBufferBytesForStart(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"start","command":"printf","output_buffer_bytes":-1}`,
	})

	require.NoError(t, err)
	requireToolError(t, result.Error, "invalid_input", "output_buffer_bytes must be greater than or equal to zero")
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
			require.NotNil(t, req.StdoutCursor)
			require.Equal(t, 0, *req.StdoutCursor)
			require.NotNil(t, req.StderrCursor)
			require.Equal(t, 0, *req.StderrCursor)
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
	require.Equal(t, "abc", payload.Output.Stdout)
	require.Equal(t, "uv", payload.Output.Stderr)
	require.Equal(t, 3, payload.Output.NextStdoutCursor)
	require.Equal(t, 2, payload.Output.NextStderrCursor)
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
			require.NotNil(t, req.StdoutCursor)
			require.Equal(t, 0, *req.StdoutCursor)
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
	require.Equal(t, "A", payload.Output.Stdout)
	require.Equal(t, 1, payload.Output.NextStdoutCursor)
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
			require.NotNil(t, req.StderrCursor)
			require.Equal(t, 0, *req.StderrCursor)
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

func TestProcess_ToolReadSupportsCursorAndLimit(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{
		GetProcessFunc: func(sessionID string, processID string) (processenv.Info, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", processID)
			return processenv.Info{ID: processID, Status: processenv.StatusRunning}, nil
		},
		ReadProcessFunc: func(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
			require.Equal(t, "default", sessionID)
			require.Equal(t, "proc_1", req.ProcessID)
			require.NotNil(t, req.StdoutCursor)
			require.Equal(t, 2, *req.StdoutCursor)
			require.NotNil(t, req.StderrCursor)
			require.Equal(t, 1, *req.StderrCursor)
			return processenv.Output{
				Stdout:           "cdef",
				Stderr:           "vwxyz",
				StdoutBytes:      6,
				StderrBytes:      6,
				NextStdoutCursor: 6,
				NextStderrCursor: 6,
			}, nil
		},
	}).Handler.Invoke(context.Background(), tools.Call{
		Name:  "process",
		Input: `{"action":"read","process_id":"proc_1","stdout_cursor":2,"stdout_bytes":3,"stderr_cursor":1,"stderr_bytes":2}`,
	})

	require.NoError(t, err)
	var payload struct {
		Output processenv.Output `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, "cde", payload.Output.Stdout)
	require.Equal(t, "vw", payload.Output.Stderr)
	require.Equal(t, 5, payload.Output.NextStdoutCursor)
	require.Equal(t, 3, payload.Output.NextStderrCursor)
}

func TestProcess_ToolReadRejectsInvalidCursors(t *testing.T) {
	result, err := Definition(&toolmocks.Runtime{}).Handler.Invoke(context.Background(), tools.Call{
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
			require.NotNil(t, req.StdoutCursor)
			require.Equal(t, 0, *req.StdoutCursor)
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
	require.Equal(t, "abc", payload.Output.Stdout)
	require.Equal(t, "xyz", payload.Output.Stderr)
	require.Equal(t, 3, payload.Output.NextStdoutCursor)
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
