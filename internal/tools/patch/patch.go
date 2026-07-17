package patch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"

	envtypes "github.com/wandxy/morph/internal/environment/types"
	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/tools"
	"github.com/wandxy/morph/internal/tools/common"
)

var (
	log       = logutils.Module("tool.patch")
	mkdirAll  = common.MkdirAll
	writeFile = common.WriteFile
)

type input struct {
	Patch string `json:"patch"`
	Strip int    `json:"strip"`
}

type patchTarget struct {
	newPath string
	isNew   bool
}

// Definition returns the model-visible tool definition.
func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:        "patch",
		Description: "Apply a unified diff patch to absolute or workspace-relative paths, subject to the current permission policy.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		Permission: permissions.Operation{
			Resource: permissions.ResourceFile,
			Action:   permissions.ActionUpdate,
			Effects:  []permissions.Effect{permissions.EffectWrite},
		},
		ResolvePermission: resolvePermissionForRuntime(runtime),
		InputSchema: common.ObjectSchema(map[string]any{
			"patch": common.StringSchema("Unified diff patch content to apply."),
			"strip": common.IntegerSchema("Number of leading path components to strip from file paths, similar to git apply -p."),
		}, "patch"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}
			patchValue := str.String(req.Patch)
			if patchValue.Trim() == "" {
				return common.ToolError("invalid_input", "patch is required"), nil
			}
			patchValue2 := str.String(req.Patch)
			log.Info().
				Str("tool", "patch").
				Str("phase", "start").
				Int("patch_chars", len([]rune(patchValue2.Trim()))).
				Int("strip", req.Strip).
				Msg("patch tool started")

			log.Debug().
				Str("tool", "patch").
				Str("phase", "execute").
				Msg("patch application started")

			applied, created, err := applyUnifiedDiff(ctx, runtime.FilePolicy(), req.Patch, req.Strip)
			if err != nil {
				log.Warn().
					Err(err).
					Str("tool", "patch").
					Str("phase", "error").
					Msg("patch application failed")
				if strings.Contains(err.Error(), "conflict") {
					return common.ToolError("conflict", err.Error()), nil
				}
				if strings.Contains(err.Error(), "outside allowed roots") {
					return common.ToolError("path_outside_roots", err.Error()), nil
				}
				if strings.Contains(err.Error(), "delete") || strings.Contains(err.Error(), "rename") || strings.Contains(err.Error(), "binary") {
					return common.ToolError("invalid_input", err.Error()), nil
				}

				return common.ToolError("internal_error", err.Error()), nil
			}

			log.Info().
				Str("tool", "patch").
				Str("phase", "complete").
				Int("applied_files", len(applied)).
				Int("created_files", len(created)).
				Msg("patch tool completed")

			return common.EncodeOutput(map[string]any{
				"applied_files": applied,
				"created_files": created,
			})
		}),
	}
}

func resolvePermission(ctx context.Context, call tools.Call) ([]permissions.EvaluationInput, error) {
	return resolvePermissionForRuntime(nil)(ctx, call)
}

func resolvePermissionForRuntime(runtime envtypes.Runtime) tools.PermissionResolver {
	return func(_ context.Context, call tools.Call) ([]permissions.EvaluationInput, error) {
		var req input
		if err := json.Unmarshal([]byte(call.Input), &req); err != nil {
			return nil, tools.NewPermissionResolutionError("invalid_input", "invalid tool input")
		}
		if str.String(req.Patch).Trim() == "" {
			return nil, tools.NewPermissionResolutionError("invalid_input", "patch is required")
		}

		files, _, err := gitdiff.Parse(strings.NewReader(req.Patch))
		if err != nil {
			return nil, tools.NewPermissionResolutionError("internal_error", err.Error())
		}
		if len(files) == 0 {
			return nil, tools.NewPermissionResolutionError("invalid_input", "invalid patch")
		}

		inputs := make([]permissions.EvaluationInput, 0, len(files))
		for _, file := range files {
			target, err := getPatchTarget(file, req.Strip)
			if err != nil {
				return nil, tools.NewPermissionResolutionError("invalid_input", err.Error())
			}
			action := permissions.ActionUpdate
			if target.isNew {
				action = permissions.ActionCreate
			}
			permissionTarget, targetScope := common.ResolveFilesystemPermissionTarget(
				common.FilesystemPolicyFromRuntime(runtime),
				target.newPath,
			)
			inputs = append(inputs, permissions.EvaluationInput{Operation: permissions.Operation{
				Resource:    permissions.ResourceFile,
				Action:      action,
				Effects:     []permissions.Effect{permissions.EffectWrite},
				Target:      permissionTarget,
				TargetScope: targetScope,
			}})
		}
		sort.Slice(inputs, func(i, j int) bool {
			return inputs[i].Operation.Target < inputs[j].Operation.Target
		})

		return inputs, nil
	}
}

func applyUnifiedDiff(
	ctx context.Context,
	policy guardrails.FilesystemPolicy,
	raw string,
	strip int,
) ([]string, []string, error) {
	files, _, err := gitdiff.Parse(strings.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}

	if len(files) == 0 {
		return nil, nil, errors.New("invalid patch")
	}

	applied := make([]string, 0, len(files))
	created := make([]string, 0)

	for _, file := range files {
		target, err := getPatchTarget(file, strip)
		if err != nil {
			return nil, nil, err
		}

		action := permissions.ActionUpdate
		if target.isNew {
			action = permissions.ActionCreate
		}
		resolved, err := common.ResolveFilesystemPathForOperation(ctx, policy, target.newPath, action)
		if err != nil {
			return nil, nil, err
		}

		src, err := patchSourceBytes(file, resolved.Absolute)
		if err != nil {
			return nil, nil, err
		}

		var dst bytes.Buffer
		if err := gitdiff.Apply(&dst, bytes.NewReader(src), file); err != nil {
			if errors.Is(err, &gitdiff.Conflict{}) {
				return nil, nil, errors.New("patch conflict: hunk does not apply cleanly")
			}

			return nil, nil, err
		}

		if target.isNew {
			if err := mkdirAll(filepath.Dir(resolved.Absolute), 0o755); err != nil {
				return nil, nil, err
			}
			created = append(created, filepath.ToSlash(resolved.Relative))
		}

		if err := writeFile(resolved.Absolute, dst.Bytes(), 0o644); err != nil {
			return nil, nil, err
		}
		applied = append(applied, filepath.ToSlash(resolved.Relative))
	}

	sort.Strings(applied)
	sort.Strings(created)
	return applied, created, nil
}

func getPatchTarget(file *gitdiff.File, strip int) (patchTarget, error) {
	oldPath := stripPath(file.OldName, strip)
	newPath := stripPath(file.NewName, strip)
	if file.IsBinary {
		return patchTarget{}, errors.New("binary patches are not supported")
	}
	if file.IsDelete || newPath == "/dev/null" {
		return patchTarget{}, errors.New("delete patches are not supported")
	}
	isNew := file.IsNew || oldPath == "/dev/null"
	if file.IsRename || !isNew && oldPath != newPath {
		return patchTarget{}, errors.New("rename patches are not supported")
	}

	return patchTarget{newPath: newPath, isNew: isNew}, nil
}

func patchSourceBytes(file *gitdiff.File, absolutePath string) ([]byte, error) {
	if file.IsNew || file.OldName == "/dev/null" {
		return nil, nil
	}

	return guardrails.ReadTextFile(absolutePath, common.MaxReadBytes)
}

func stripPath(path string, strip int) string {
	pathValue := str.String(path)
	path = pathValue.Trim()
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	if path == "/dev/null" {
		return path
	}

	parts := strings.Split(filepath.ToSlash(path), "/")
	if strip >= len(parts) {
		return parts[len(parts)-1]
	}

	return filepath.FromSlash(strings.Join(parts[strip:], "/"))
}
