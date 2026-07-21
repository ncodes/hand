package readfile

import (
	"context"
	"encoding/json"

	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"

	envtypes "github.com/wandxy/morph/internal/environment/types"
	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
	"github.com/wandxy/morph/internal/tools/common"
)

var log = logutils.Module("tool.readfile")

// Definition returns the model-visible tool definition.
func Definition(runtime envtypes.Runtime) tools.Definition {
	type input struct {
		Path string `json:"path"`
	}

	return tools.Definition{
		Name:          "read_file",
		Description:   "Read a text file at an absolute or workspace-relative path, subject to the current permission policy.",
		ParallelSafe:  true,
		Groups:        []string{"core"},
		Requires:      tools.Capabilities{Filesystem: true},
		SemanticIndex: tools.ProjectSemanticIndex(tools.ProjectJSONFieldsForSemanticIndex("path", "content")),
		Permission: permissions.Operation{
			Resource: permissions.ResourceFile,
			Action:   permissions.ActionRead,
			Effects:  []permissions.Effect{permissions.EffectRead},
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
			target, targetScope := common.ResolveFilesystemPermissionTarget(
				common.FilesystemPolicyFromRuntime(runtime),
				path,
			)
			return []permissions.EvaluationInput{{Operation: permissions.Operation{
				Resource:    permissions.ResourceFile,
				Action:      permissions.ActionRead,
				Effects:     []permissions.Effect{permissions.EffectRead},
				Target:      target,
				TargetScope: targetScope,
			}}}, nil
		},
		InputSchema: common.ObjectSchema(map[string]any{
			"path": common.StringSchema("Absolute path to the text file or path relative to the configured workspace root."),
		}, "path"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			log.Info().
				Str("tool", "read_file").
				Str("phase", "start").
				Str("path", common.NormalizedDisplayPath(req.Path)).
				Msg("read file tool started")

			resolved, err := common.ResolveFilesystemPathForOperation(
				ctx,
				common.FilesystemPolicyFromRuntime(runtime),
				req.Path,
				permissions.ActionRead,
			)
			if err != nil {
				log.Warn().
					Err(err).
					Str("tool", "read_file").
					Str("phase", "error").
					Msg("read file failed")
				return common.FileError(err), nil
			}

			log.Debug().
				Str("tool", "read_file").
				Str("phase", "execute").
				Str("path", common.NormalizedDisplayPath(resolved.Relative)).
				Msg("read file execution started")

			content, err := guardrails.ReadTextFile(resolved.Absolute, common.MaxReadBytes)
			if err != nil {
				log.Warn().
					Err(err).
					Str("tool", "read_file").
					Str("phase", "error").
					Msg("read file failed")
				return common.FileError(err), nil
			}

			log.Info().
				Str("tool", "read_file").
				Str("phase", "complete").
				Int("bytes", len(content)).
				Msg("read file tool completed")

			return common.EncodeOutput(map[string]any{
				"path":    common.NormalizedDisplayPath(resolved.Relative),
				"content": string(content),
				"bytes":   len(content),
			})
		}),
	}
}
