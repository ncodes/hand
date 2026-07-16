package writefile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"

	envtypes "github.com/wandxy/morph/internal/environment/types"
	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
	"github.com/wandxy/morph/internal/tools/common"
)

var log = logutils.Module("tool.writefile")

type input struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	CreateDirs *bool  `json:"create_dirs"`
}

// Definition returns the model-visible tool definition.
func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:        "write_file",
		Description: "Create or overwrite a text file at an absolute or workspace-relative path, subject to the current permission mode.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		Permission: permissions.Operation{
			Resource: permissions.ResourceFile,
			Action:   permissions.ActionUpdate,
			Effects:  []permissions.Effect{permissions.EffectWrite},
		},
		ResolvePermission: func(_ context.Context, call tools.Call) ([]permissions.EvaluationInput, error) {
			var req input
			if err := json.Unmarshal([]byte(call.Input), &req); err != nil {
				return nil, tools.NewPermissionResolutionError("invalid_input", "invalid tool input")
			}
			path := str.String(req.Path).Trim()
			if path == "" {
				return nil, tools.NewPermissionResolutionError("invalid_input", "path is required")
			}

			return []permissions.EvaluationInput{
				{
					Operation: permissions.Operation{
						Resource: permissions.ResourceFile,
						Action:   permissions.ActionUpdate,
						Effects:  []permissions.Effect{permissions.EffectWrite},
						Target:   filepath.ToSlash(filepath.Clean(path)),
					},
				}}, nil
		},
		InputSchema: common.ObjectSchema(map[string]any{
			"path":        common.StringSchema("Absolute path to the file or path relative to the configured workspace root."),
			"content":     common.StringSchema("Text content to write to the target file."),
			"create_dirs": common.BooleanSchema("When true, create missing parent directories before writing. Defaults to true."),
		}, "path", "content"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}
			pathValue := str.String(req.Path)
			if pathValue.Trim() == "" {
				return common.ToolError("invalid_input", "path is required"), nil
			}

			if guardrails.IsBinary([]byte(req.Content)) {
				return common.ToolError("not_text", "content must be text"), nil
			}

			resolved, err := common.ResolveFilesystemPath(ctx, runtime.FilePolicy(), req.Path)
			if err != nil {
				return common.FileError(err), nil
			}

			createDirs := true
			if req.CreateDirs != nil {
				createDirs = *req.CreateDirs
			}

			log.Info().
				Str("tool", "write_file").
				Str("phase", "start").
				Str("path", common.NormalizedDisplayPath(req.Path)).
				Int("content_bytes", len(req.Content)).
				Bool("create_dirs", createDirs).
				Msg("write file tool started")

			if createDirs {
				if err := common.MkdirAll(filepath.Dir(resolved.Absolute), 0o755); err != nil {
					log.Warn().
						Err(err).
						Str("tool", "write_file").
						Str("phase", "error").
						Msg("write file failed")
					return common.FileError(err), nil
				}
			}

			_, statErr := common.StatFile(resolved.Absolute)
			created := errors.Is(statErr, os.ErrNotExist)

			log.Debug().
				Str("tool", "write_file").
				Str("phase", "execute").
				Bool("created", created).
				Msg("write file execution started")

			if err := common.WriteFile(resolved.Absolute, []byte(req.Content), 0o644); err != nil {
				log.Warn().
					Err(err).
					Str("tool", "write_file").
					Str("phase", "error").
					Msg("write file failed")
				return common.FileError(err), nil
			}

			log.Info().
				Str("tool", "write_file").
				Str("phase", "complete").
				Int("bytes_written", len(req.Content)).
				Bool("created", created).
				Msg("write file tool completed")

			return common.EncodeOutput(map[string]any{
				"path":          common.NormalizedDisplayPath(resolved.Relative),
				"bytes_written": len(req.Content),
				"created":       created,
			})
		}),
	}
}
