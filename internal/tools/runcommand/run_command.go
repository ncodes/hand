package runcommand

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

var currentGOOS = goruntime.GOOS
var commandContext = common.CommandContext

func Definition(runtime envtypes.Runtime) tools.Definition {
	type input struct {
		Command        string            `json:"command"`
		Args           []string          `json:"args"`
		Cwd            string            `json:"cwd"`
		Env            map[string]string `json:"env"`
		TimeoutSeconds int               `json:"timeout_seconds"`
	}

	return tools.Definition{
		Name: "run_command",
		Description: common.JoinStrings(
			"Run a short-lived, non-interactive command.",
			"Default timeout 30s, max 120s.",
			"Kills the process (main/child/background) on timeout.",
		),
		Groups:   []string{"core"},
		Requires: tools.Capabilities{Exec: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"command": common.StringSchema("Command to run. Uses the shell when args are omitted."),
			"args": map[string]any{
				"type":        "array",
				"description": "Arguments passed directly to the command.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"cwd": common.StringSchema("Working directory relative to an allowed workspace root."),
			"env": map[string]any{
				"type":        "object",
				"description": "Environment variable overrides.",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
			"timeout_seconds": common.IntegerSchema(common.JoinStrings(
				"Timeout in seconds. Default 30. Max 120.",
				"Terminates the command/processes when reached.",
			)),
		}, "command"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			if strings.TrimSpace(req.Command) == "" {
				return common.ToolError("invalid_input", "command is required"), nil
			}

			cwd := req.Cwd
			if strings.TrimSpace(cwd) == "" {
				cwd = runtime.FilePolicy().Roots[0]
			}

			resolved, err := runtime.FilePolicy().Resolve(cwd)
			if err != nil {
				return common.FileError(err), nil
			}

			eval := guardrails.EvaluateCommand(runtime.CommandPolicy(), req.Command, req.Args)
			switch eval.Decision {
			case guardrails.CommandDenied:
				return common.ToolError("command_denied", eval.Reason), nil
			case guardrails.CommandApprovalRequired:
				message := "command requires approval"
				if eval.Rule != "" {
					message = "command requires approval: " + eval.Rule
				}

				return common.ToolError("approval_required", message), nil
			}

			timeout := common.WithTimeoutSeconds(req.TimeoutSeconds)

			log.Info().
				Str("tool", "run_command").
				Str("phase", "start").
				Str("cwd", common.NormalizedDisplayPath(req.Cwd)).
				Int("args_count", len(req.Args)).
				Int("env_overrides", len(req.Env)).
				Bool("shell_mode", len(req.Args) == 0).
				Int("timeout_seconds", timeout).
				Msg("tool call started")

			runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			if err := runCtx.Err(); err != nil {
				log.Warn().
					Err(err).
					Str("tool", "run_command").
					Str("phase", "error").
					Msg("run command failed before execution")
				return common.ToolError("command_failed", err.Error()), nil
			}

			cmd := buildCommand(context.Background(), req.Command, req.Args)
			configureCommandProcess(cmd)

			cmd.Dir = resolved.Absolute
			cmd.Env = os.Environ()
			if len(req.Env) > 0 {
				for key, value := range req.Env {
					cmd.Env = append(cmd.Env, key+"="+value)
				}
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			log.Debug().
				Str("tool", "run_command").
				Str("phase", "execute").
				Msg("command execution started")

			startedAt := time.Now()
			if err := cmd.Start(); err != nil {
				log.Warn().
					Err(err).
					Str("tool", "run_command").
					Str("phase", "error").
					Msg("command execution failed to start")
				return common.ToolError("command_failed", err.Error()), nil
			}

			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case err = <-done:
			case <-runCtx.Done():
				terminateCommandProcess(cmd)
				err = <-done
			}

			elapsedSeconds := time.Since(startedAt).Seconds()

			if runCtx.Err() == context.DeadlineExceeded {
				log.Info().
					Str("tool", "run_command").
					Str("phase", "complete").
					Int("exit_code", -1).
					Bool("timed_out", true).
					Int("stdout_bytes", len(stdout.String())).
					Int("stderr_bytes", len(stderr.String())).
					Float64("elapsed_seconds", elapsedSeconds).
					Msg("tool call completed")

				return common.EncodeOutput(runCommandOutput(
					-1,
					common.TrimOutput(stdout.String(), common.MaxOutputBytes),
					common.TrimOutput(stderr.String(), common.MaxOutputBytes),
					true,
					timeout,
					elapsedSeconds,
				))
			}

			if runCtx.Err() == context.Canceled {
				log.Warn().
					Err(runCtx.Err()).
					Str("tool", "run_command").
					Str("phase", "error").
					Msg("command execution canceled")
				return common.ToolError("command_failed", runCtx.Err().Error()), nil
			}

			exitCode := 0
			if err != nil {
				if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
					exitCode = exitErr.ExitCode()
				} else {
					log.Warn().
						Err(err).
						Str("tool", "run_command").
						Str("phase", "error").
						Msg("command execution failed")
					return common.ToolError("command_failed", err.Error()), nil
				}
			}

			log.Info().
				Str("tool", "run_command").
				Str("phase", "complete").
				Int("exit_code", exitCode).
				Bool("timed_out", false).
				Int("stdout_bytes", len(stdout.String())).
				Int("stderr_bytes", len(stderr.String())).
				Float64("elapsed_seconds", elapsedSeconds).
				Msg("tool call completed")

			return common.EncodeOutput(runCommandOutput(
				exitCode,
				common.TrimOutput(stdout.String(), common.MaxOutputBytes),
				common.TrimOutput(stderr.String(), common.MaxOutputBytes),
				false,
				timeout,
				elapsedSeconds,
			))
		}),
	}
}

func runCommandOutput(exitCode int, stdout, stderr string, timedOut bool, timeoutSeconds int, elapsedSeconds float64) map[string]any {
	remainingSeconds := 0.0
	if !timedOut {
		remainingSeconds = float64(timeoutSeconds) - elapsedSeconds
		if remainingSeconds < 0 {
			remainingSeconds = 0
		}
	}

	return map[string]any{
		"exit_code":         exitCode,
		"stdout":            stdout,
		"stderr":            stderr,
		"timed_out":         timedOut,
		"timeout_seconds":   timeoutSeconds,
		"elapsed_seconds":   elapsedSeconds,
		"remaining_seconds": remainingSeconds,
	}
}

func buildCommand(ctx context.Context, command string, args []string) *exec.Cmd {
	command = strings.TrimSpace(command)
	if len(args) > 0 {
		return commandContext(ctx, command, args...)
	}

	if currentGOOS == "windows" {
		return commandContext(ctx, "cmd", "/C", command)
	}

	return commandContext(ctx, "sh", "-lc", command)
}
