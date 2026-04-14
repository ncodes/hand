package process

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog"
	processenv "github.com/wandxy/hand/internal/environment/process"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
	"github.com/wandxy/hand/pkg/logutils"
)

var processLog = logutils.InitLogger("tools.process")

type input struct {
	Action      string            `json:"action"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Cwd         string            `json:"cwd"`
	Env         map[string]string `json:"env"`
	OutputBytes *int              `json:"output_buffer_bytes"`
	ProcessID   string            `json:"process_id"`
	StdoutBytes *int              `json:"stdout_bytes"`
	StderrBytes *int              `json:"stderr_bytes"`
}

func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:        "process",
		Description: "Start, inspect, read, stop, or list tracked background processes.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Exec: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Process action to perform.",
				"enum":        []string{"start", "status", "read", "stop", "list"},
			},
			"command": common.StringSchema("Command to start. Required for action=start."),
			"args": map[string]any{
				"type":        "array",
				"description": "Arguments passed directly to the command for action=start.",
				"items":       common.StringSchema("Command argument."),
			},
			"cwd": common.StringSchema("Optional working directory for action=start."),
			"env": map[string]any{
				"type":        "object",
				"description": "Optional environment variable overrides for action=start.",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
			"output_buffer_bytes": common.IntegerSchema(
				"Optional maximum bytes to retain per stdout/stderr buffer for action=start.",
			),
			"process_id": common.StringSchema("Tracked process identifier. Required for status, read, and stop."),
			"stdout_bytes": common.IntegerSchema(
				"Optional maximum stdout bytes to return for action=read.",
			),
			"stderr_bytes": common.IntegerSchema(
				"Optional maximum stderr bytes to return for action=read.",
			),
		}, "action"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}
			if runtime == nil {
				return common.ToolError("tool_error", "process manager is not configured"), nil
			}

			action := strings.TrimSpace(strings.ToLower(req.Action))
			if action == "" {
				return common.ToolError("invalid_input", "action is required"), nil
			}

			logEvent := processLog.Info().
				Str("tool", "process").
				Str("phase", "start").
				Str("action", action)

			switch action {
			case "start":
				return handleStart(ctx, runtime, action, req, logEvent), nil

			case "status":
				return handleStatus(runtime, action, req, logEvent), nil

			case "read":
				return handleRead(runtime, action, req, logEvent), nil

			case "stop":
				return handleStop(ctx, runtime, action, req, logEvent), nil

			case "list":
				return handleList(runtime, action, logEvent), nil

			default:
				return common.ToolError("invalid_input", fmt.Sprintf("unsupported action %q", action)), nil
			}
		}),
	}
}

func handleStart(
	ctx context.Context,
	runtime envtypes.Runtime,
	action string,
	req input,
	logEvent anyLogEvent,
) tools.Result {
	logEvent.
		Bool("cwd_provided", strings.TrimSpace(req.Cwd) != "").
		Int("args_count", len(req.Args)).
		Int("env_overrides", len(req.Env)).
		Bool("output_buffer_limit", req.OutputBytes != nil).
		Msg("tool call started")

	command := strings.TrimSpace(req.Command)
	if command == "" {
		return common.ToolError("invalid_input", "command is required for start")
	}

	outputBufferBytes, err := resolveBufferLimit(req.OutputBytes, "output_buffer_bytes")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}

	info, err := runtime.StartProcess(ctx, processenv.StartRequest{
		Command:           command,
		Args:              append([]string(nil), req.Args...),
		CWD:               strings.TrimSpace(req.Cwd),
		Env:               cloneEnv(req.Env),
		OutputBufferBytes: outputBufferBytes,
	})
	if err != nil {
		logProcessError(action, "start_failed")
		return common.ToolError("tool_error", err.Error())
	}

	logProcessComplete(action, info.Status, info.ID, 0, 0, false)

	return encodeProcessOutput(map[string]any{"process": info})
}

func handleStatus(runtime envtypes.Runtime, action string, req input, logEvent anyLogEvent) tools.Result {
	logEvent.
		Bool("has_process_id", strings.TrimSpace(req.ProcessID) != "").
		Msg("tool call started")

	processID := strings.TrimSpace(req.ProcessID)
	if processID == "" {
		return common.ToolError("invalid_input", "process_id is required for status")
	}

	info, err := runtime.GetProcess(processID)
	if err != nil {
		logProcessError(action, "status_failed")
		return common.ToolError("tool_error", err.Error())
	}

	logProcessComplete(action, info.Status, info.ID, 0, 0, false)

	return encodeProcessOutput(map[string]any{"process": info})
}

func handleRead(runtime envtypes.Runtime, action string, req input, logEvent anyLogEvent) tools.Result {
	logEvent.
		Bool("has_process_id", strings.TrimSpace(req.ProcessID) != "").
		Bool("stdout_limit", req.StdoutBytes != nil).
		Bool("stderr_limit", req.StderrBytes != nil).
		Msg("tool call started")

	processID := strings.TrimSpace(req.ProcessID)
	if processID == "" {
		return common.ToolError("invalid_input", "process_id is required for read")
	}

	stdoutLimit, err := resolveBufferLimit(req.StdoutBytes, "stdout_bytes")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}

	stderrLimit, err := resolveBufferLimit(req.StderrBytes, "stderr_bytes")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}

	info, err := runtime.GetProcess(processID)
	if err != nil {
		logProcessError(action, "read_failed")
		return common.ToolError("tool_error", err.Error())
	}

	output, err := runtime.ReadProcess(processID)
	if err != nil {
		logProcessError(action, "read_failed")
		return common.ToolError("tool_error", err.Error())
	}

	output.Stdout = trimRecentOutput(output.Stdout, stdoutLimit)
	output.Stderr = trimRecentOutput(output.Stderr, stderrLimit)

	logProcessComplete(action, info.Status, info.ID, len(output.Stdout), len(output.Stderr), true)

	return encodeProcessOutput(map[string]any{
		"process": info,
		"output":  output,
	})
}

func handleStop(
	ctx context.Context,
	runtime envtypes.Runtime,
	action string,
	req input,
	logEvent anyLogEvent,
) tools.Result {
	logEvent.
		Bool("has_process_id", strings.TrimSpace(req.ProcessID) != "").
		Msg("tool call started")

	processID := strings.TrimSpace(req.ProcessID)
	if processID == "" {
		return common.ToolError("invalid_input", "process_id is required for stop")
	}

	info, err := runtime.StopProcess(ctx, processID)
	if err != nil {
		logProcessError(action, "stop_failed")
		return common.ToolError("tool_error", err.Error())
	}

	logProcessComplete(action, info.Status, info.ID, 0, 0, false)

	return encodeProcessOutput(map[string]any{"process": info})
}

func handleList(runtime envtypes.Runtime, action string, logEvent anyLogEvent) tools.Result {
	logEvent.Msg("tool call started")

	processes := runtime.ListProcesses()

	processLog.Info().
		Str("tool", "process").
		Str("phase", "complete").
		Str("action", action).
		Int("processes", len(processes)).
		Msg("tool call completed")

	return encodeProcessOutput(map[string]any{"processes": processes})
}

type anyLogEvent interface {
	Bool(string, bool) *zerolog.Event
	Int(string, int) *zerolog.Event
	Msg(string)
}

func logProcessError(action, errorKind string) {
	processLog.Warn().
		Str("tool", "process").
		Str("phase", "error").
		Str("action", action).
		Str("error_kind", errorKind).
		Msg("process tool failed")
}

func logProcessComplete(action, status, processID string, stdoutBytes, stderrBytes int, includeOutput bool) {
	event := processLog.Info().
		Str("tool", "process").
		Str("phase", "complete").
		Str("action", action).
		Str("status", status).
		Str("process_id", processID)

	if includeOutput {
		event.Int("stdout_bytes", stdoutBytes).Int("stderr_bytes", stderrBytes)
	}

	event.Msg("tool call completed")
}

func encodeProcessOutput(value any) tools.Result {
	result, err := common.EncodeOutput(value)
	if err != nil {
		return common.ToolError("internal_error", "failed to encode tool output")
	}

	return result
}

func resolveBufferLimit(value *int, field string) (int, error) {
	if value == nil {
		return 0, nil
	}
	if *value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", field)
	}
	return *value, nil
}

func trimRecentOutput(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}

	data := []byte(value[len(value)-maxBytes:])
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[1:]
	}

	return string(data)
}

func cloneEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(env))
	for key, value := range env {
		cloned[key] = value
	}
	return cloned
}
