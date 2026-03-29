package native

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

var currentGOOS = goruntime.GOOS

func RunCommandDefinition(dependencies envtypes.Runtime) tools.Definition {
	type input struct {
		Command        string            `json:"command"`
		Args           []string          `json:"args"`
		Cwd            string            `json:"cwd"`
		Env            map[string]string `json:"env"`
		TimeoutSeconds int               `json:"timeout_seconds"`
	}

	return tools.Definition{
		Name:        "run_command",
		Description: "Run a short-lived, non-interactive command once and return stdout, stderr, exit code, and timeout status.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Exec: true},
		InputSchema: objectSchema(map[string]any{
			"command": stringSchema("Command to run. Uses the shell when args are omitted."),
			"args": map[string]any{
				"type":        "array",
				"description": "Arguments passed directly to the command.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"cwd": stringSchema("Working directory relative to an allowed workspace root."),
			"env": map[string]any{
				"type":        "object",
				"description": "Environment variable overrides.",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
			"timeout_seconds": integerSchema("Timeout in seconds."),
		}, "command"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := decodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			if strings.TrimSpace(req.Command) == "" {
				return toolError("invalid_input", "command is required"), nil
			}

			cwd := req.Cwd
			if strings.TrimSpace(cwd) == "" {
				cwd = dependencies.FilePolicy().Roots[0]
			}

			resolved, err := dependencies.FilePolicy().Resolve(cwd)
			if err != nil {
				return fileError(err), nil
			}

			eval := guardrails.EvaluateCommand(dependencies.CommandPolicy(), req.Command, req.Args)
			switch eval.Decision {
			case guardrails.CommandDenied:
				return toolError("command_denied", eval.Reason), nil
			case guardrails.CommandApprovalRequired:
				message := "command requires approval"
				if eval.Rule != "" {
					message = "command requires approval: " + eval.Rule
				}
				return toolError("approval_required", message), nil
			}

			timeout := withTimeoutSeconds(req.TimeoutSeconds)
			runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			if err := runCtx.Err(); err != nil {
				return toolError("command_failed", err.Error()), nil
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

			if err := cmd.Start(); err != nil {
				return toolError("command_failed", err.Error()), nil
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

			if runCtx.Err() == context.DeadlineExceeded {
				return encodeOutput(map[string]any{
					"exit_code": -1,
					"stdout":    trimOutput(stdout.String(), maxOutputBytes),
					"stderr":    trimOutput(stderr.String(), maxOutputBytes),
					"timed_out": true,
				})
			}

			if runCtx.Err() == context.Canceled {
				return toolError("command_failed", runCtx.Err().Error()), nil
			}

			exitCode := 0
			if err != nil {
				if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
					exitCode = exitErr.ExitCode()
				} else {
					return toolError("command_failed", err.Error()), nil
				}
			}

			return encodeOutput(map[string]any{
				"exit_code": exitCode,
				"stdout":    trimOutput(stdout.String(), maxOutputBytes),
				"stderr":    trimOutput(stderr.String(), maxOutputBytes),
				"timed_out": false,
			})
		}),
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
