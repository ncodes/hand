package process

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog"
	processenv "github.com/wandxy/morph/internal/environment/process"
	envtypes "github.com/wandxy/morph/internal/environment/types"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
	"github.com/wandxy/morph/internal/tools/common"
	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"
)

var processLog = logutils.Module("tool.process")

type input struct {
	Action       string            `json:"action"`
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	Cwd          string            `json:"cwd"`
	Env          map[string]string `json:"env"`
	Label        string            `json:"label"`
	OutputBytes  *int              `json:"output_buffer_bytes"`
	ProcessID    string            `json:"process_id"`
	StdoutCursor *int              `json:"stdout_cursor"`
	StderrCursor *int              `json:"stderr_cursor"`
	StdoutBytes  *int              `json:"stdout_bytes"`
	StderrBytes  *int              `json:"stderr_bytes"`
}

// Definition returns the model-visible tool definition.
func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:        "process",
		Description: "Start, inspect, read, stop, or list tracked background processes.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Exec: true},
		Permission: permissions.Operation{
			Resource: permissions.ResourceProcess,
			Action:   permissions.ActionManage,
			Effects:  []permissions.Effect{permissions.EffectRead, permissions.EffectWrite, permissions.EffectExecution},
		},
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
			"label": common.StringSchema(
				"Optional human-friendly label for action=start. The tool still returns a canonical process id; status, read, and stop can use either the returned id or this label.",
			),
			"output_buffer_bytes": common.IntegerSchema(
				"Optional maximum bytes to retain per stdout/stderr buffer for action=start.",
			),
			"process_id": common.StringSchema("Tracked process identifier or label. Required for status, read, and stop."),
			"stdout_cursor": common.IntegerSchema(
				"Optional stdout cursor from a previous read for incremental reads on action=read.",
			),
			"stderr_cursor": common.IntegerSchema(
				"Optional stderr cursor from a previous read for incremental reads on action=read.",
			),
			"stdout_bytes": common.IntegerSchema(
				"Optional maximum stdout bytes to return from stdout_cursor, or from the beginning when no cursor is provided, for action=read.",
			),
			"stderr_bytes": common.IntegerSchema(
				"Optional maximum stderr bytes to return from stderr_cursor, or from the beginning when no cursor is provided, for action=read.",
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
			actionValue := str.String(req.Action)
			action := actionValue.Normalized()
			if action == "" {
				return common.ToolError("invalid_input", "action is required"), nil
			}
			if err := validateActionFields(action, req); err != nil {
				return common.ToolError("invalid_input", err.Error()), nil
			}

			logEvent := processLog.Info().
				Str("tool", "process").
				Str("phase", "start").
				Str("action", action)

			switch action {
			case "start":
				return handleStart(ctx, runtime, action, req, logEvent), nil

			case "status":
				return handleStatus(ctx, runtime, action, req, logEvent), nil

			case "read":
				return Handead(ctx, runtime, action, req, logEvent), nil

			case "stop":
				return handleStop(ctx, runtime, action, req, logEvent), nil

			case "list":
				return handleList(ctx, runtime, action, logEvent), nil

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
	sessionID := normalizeSessionID(ctx)
	cwdValue := str.String(req.Cwd)
	labelValue := str.String(req.Label)
	logEvent.
		Str("session_id", sessionID).
		Bool("cwd_provided", cwdValue.Trim() != "").
		Int("args_count", len(req.Args)).
		Int("env_overrides", len(req.Env)).
		Bool("label_provided", labelValue.Trim() != "").
		Bool("output_buffer_limit", req.OutputBytes != nil).
		Msg("process tool start requested")
	commandValue := str.String(req.Command)
	command := commandValue.Trim()
	if command == "" {
		return common.ToolError("invalid_input", "command is required for start")
	}

	outputBufferBytes, err := resolveOutputBufferLimit(req.OutputBytes, "output_buffer_bytes")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}
	cwdValue2 := str.String(req.Cwd)
	labelValue2 := str.String(req.Label)
	info, err := runtime.StartProcess(ctx, sessionID, processenv.StartRequest{
		Command:           command,
		Args:              append([]string(nil), req.Args...),
		CWD:               cwdValue2.Trim(),
		Env:               cloneEnv(req.Env),
		Label:             labelValue2.Trim(),
		OutputBufferBytes: outputBufferBytes,
	})
	if err != nil {
		logProcessError(action, "start_failed")
		return common.ToolError("tool_error", err.Error())
	}

	logProcessComplete(action, info.Status, info.ID, 0, 0, false)

	return encodeProcessOutput(map[string]any{"process": info})
}

func handleStatus(ctx context.Context, runtime envtypes.Runtime, action string, req input, logEvent anyLogEvent) tools.Result {
	sessionID := normalizeSessionID(ctx)
	processIDValue := str.String(req.ProcessID)
	logEvent.
		Str("session_id", sessionID).
		Bool("has_process_id", processIDValue.Trim() != "").
		Msg("process tool status requested")
	processIDValue2 := str.String(req.ProcessID)
	processID := processIDValue2.Trim()
	if processID == "" {
		return common.ToolError("invalid_input", "process_id is required for status")
	}

	info, err := runtime.GetProcess(sessionID, processID)
	if err != nil {
		logProcessError(action, "status_failed")
		return common.ToolError("tool_error", err.Error())
	}

	logProcessComplete(action, info.Status, info.ID, 0, 0, false)

	return encodeProcessOutput(map[string]any{"process": info})
}

func Handead(ctx context.Context, runtime envtypes.Runtime, action string, req input, logEvent anyLogEvent) tools.Result {
	sessionID := normalizeSessionID(ctx)
	processIDValue3 := str.String(req.ProcessID)
	logEvent.
		Str("session_id", sessionID).
		Bool("has_process_id", processIDValue3.Trim() != "").
		Bool("stdout_cursor", req.StdoutCursor != nil).
		Bool("stderr_cursor", req.StderrCursor != nil).
		Bool("stdout_limit", req.StdoutBytes != nil).
		Bool("stderr_limit", req.StderrBytes != nil).
		Msg("process tool output requested")
	processIDValue4 := str.String(req.ProcessID)
	processID := processIDValue4.Trim()
	if processID == "" {
		return common.ToolError("invalid_input", "process_id is required for read")
	}

	stdoutCursor, err := resolveCursor(req.StdoutCursor, "stdout_cursor")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}
	stderrCursor, err := resolveCursor(req.StderrCursor, "stderr_cursor")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}

	stdoutLimit, err := resolveBufferLimit(req.StdoutBytes, "stdout_bytes")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}

	stderrLimit, err := resolveBufferLimit(req.StderrBytes, "stderr_bytes")
	if err != nil {
		return common.ToolError("invalid_input", err.Error())
	}
	stdoutCursor = defaultCursor(stdoutCursor)
	stderrCursor = defaultCursor(stderrCursor)

	info, err := runtime.GetProcess(sessionID, processID)
	if err != nil {
		logProcessError(action, "read_failed")
		return common.ToolError("tool_error", err.Error())
	}

	output, err := runtime.ReadProcess(sessionID, processenv.ReadRequest{
		ProcessID:    processID,
		StdoutCursor: stdoutCursor,
		StderrCursor: stderrCursor,
	})
	if err != nil {
		logProcessError(action, "read_failed")
		return common.ToolError("tool_error", err.Error())
	}

	if stdout, bytesRead, limited := limitOutput(output.Stdout, stdoutLimit); limited && !output.StdoutCursorExpired {
		output.Stdout = stdout
		output.NextStdoutCursor = *stdoutCursor + bytesRead
	} else {
		output.Stdout = stdout
	}
	if stderr, bytesRead, limited := limitOutput(output.Stderr, stderrLimit); limited && !output.StderrCursorExpired {
		output.Stderr = stderr
		output.NextStderrCursor = *stderrCursor + bytesRead
	} else {
		output.Stderr = stderr
	}

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
	sessionID := normalizeSessionID(ctx)
	processIDValue5 := str.String(req.ProcessID)
	logEvent.
		Str("session_id", sessionID).
		Bool("has_process_id", processIDValue5.Trim() != "").
		Msg("process tool stop requested")
	processIDValue6 := str.String(req.ProcessID)
	processID := processIDValue6.Trim()
	if processID == "" {
		return common.ToolError("invalid_input", "process_id is required for stop")
	}

	info, err := runtime.StopProcess(ctx, sessionID, processID)
	if err != nil {
		logProcessError(action, "stop_failed")
		return common.ToolError("tool_error", err.Error())
	}

	logProcessComplete(action, info.Status, info.ID, 0, 0, false)

	return encodeProcessOutput(map[string]any{"process": info})
}

func handleList(ctx context.Context, runtime envtypes.Runtime, action string, logEvent anyLogEvent) tools.Result {
	sessionID := normalizeSessionID(ctx)
	logEvent.Str("session_id", sessionID).Msg("process tool list requested")

	processes := runtime.ListProcesses(sessionID)

	processLog.Info().
		Str("tool", "process").
		Str("phase", "complete").
		Str("action", action).
		Int("processes", len(processes)).
		Msg("process tool list returned")

	return encodeProcessOutput(map[string]any{"processes": processes})
}

type anyLogEvent interface {
	Str(string, string) *zerolog.Event
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

	event.Msg("process tool action completed")
}

func encodeProcessOutput(value any) tools.Result {
	result, err := common.EncodeOutput(value)
	if err != nil {
		return common.ToolError("internal_error", "failed to encode tool output")
	}

	return result
}

func validateActionFields(action string, req input) error {
	if action == "read" {
		return nil
	}

	fields := make([]string, 0, 4)
	if hasPositiveInt(req.StdoutCursor) {
		fields = append(fields, "stdout_cursor")
	}
	if hasPositiveInt(req.StderrCursor) {
		fields = append(fields, "stderr_cursor")
	}
	if hasPositiveInt(req.StdoutBytes) {
		fields = append(fields, "stdout_bytes")
	}
	if hasPositiveInt(req.StderrBytes) {
		fields = append(fields, "stderr_bytes")
	}
	if len(fields) == 0 {
		return nil
	}

	message := fmt.Sprintf(
		"invalid process %s request: %s %s only valid for action=read",
		action,
		strings.Join(fields, ", "),
		pluralVerb(len(fields), "is", "are"),
	)
	if action == "start" {
		message += "; use output_buffer_bytes to configure retained output"
	}

	return fmt.Errorf("%s", message)
}

func hasPositiveInt(value *int) bool {
	return value != nil && *value > 0
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

func resolveOutputBufferLimit(value *int, field string) (int, error) {
	if value == nil || *value == 0 {
		return 0, nil
	}
	if *value < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to zero", field)
	}
	return *value, nil
}

func resolveCursor(value *int, field string) (*int, error) {
	if value == nil {
		return nil, nil
	}
	if *value < 0 {
		return nil, fmt.Errorf("%s must be greater than or equal to zero", field)
	}
	cursor := *value
	return &cursor, nil
}

func pluralVerb(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}

	return plural
}

func defaultCursor(value *int) *int {
	if value != nil {
		return value
	}

	cursor := 0
	return &cursor
}

func limitOutput(value string, maxBytes int) (string, int, bool) {
	if maxBytes <= 0 {
		return value, len([]byte(value)), false
	}

	data := []byte(value)
	if len(data) <= maxBytes {
		return value, len(data), false
	}

	data = data[:maxBytes]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}

	return string(data), len(data), true
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

func normalizeSessionID(ctx context.Context) string {
	sessionIDFromContextValue := str.String(tools.SessionIDFromContext(ctx))
	sessionID := sessionIDFromContextValue.Trim()
	if sessionID == "" {
		return "default"
	}
	return sessionID
}
