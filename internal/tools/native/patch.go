package native

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

func PatchDefinition(dependencies envtypes.Runtime) tools.Definition {
	type input struct {
		// Patch is the unified diff payload to apply.
		Patch string `json:"patch"`
		// Strip removes leading path components like git's -p flag.
		Strip int `json:"strip"`
	}

	return tools.Definition{
		Name:        "patch",
		Description: "Apply a unified diff patch under allowed workspace roots.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		InputSchema: map[string]any{"type": "object"},
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := decodeInput(call, &req); result.Error != "" {
				return result, nil
			}
			if strings.TrimSpace(req.Patch) == "" {
				return toolError("invalid_input", "patch is required"), nil
			}

			applied, created, err := applyUnifiedDiff(dependencies.FilePolicy(), req.Patch, req.Strip)
			if err != nil {
				if strings.Contains(err.Error(), "conflict") {
					return toolError("conflict", err.Error()), nil
				}
				if strings.Contains(err.Error(), "outside allowed roots") {
					return toolError("path_outside_roots", err.Error()), nil
				}
				if strings.Contains(err.Error(), "delete") || strings.Contains(err.Error(), "rename") || strings.Contains(err.Error(), "binary") {
					return toolError("invalid_input", err.Error()), nil
				}
				return toolError("internal_error", err.Error()), nil
			}

			return encodeOutput(map[string]any{
				"applied_files": applied,
				"created_files": created,
			})
		}),
	}
}

func applyUnifiedDiff(policy guardrails.FilesystemPolicy, raw string, strip int) ([]string, []string, error) {
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
		oldPath := stripPath(file.OldName, strip)
		newPath := stripPath(file.NewName, strip)

		if file.IsBinary {
			return nil, nil, errors.New("binary patches are not supported")
		}
		if file.IsDelete || newPath == "/dev/null" {
			return nil, nil, errors.New("delete patches are not supported")
		}
		isNewFile := file.IsNew || oldPath == "/dev/null"
		if file.IsRename || (!isNewFile && oldPath != newPath) {
			return nil, nil, errors.New("rename patches are not supported")
		}

		resolved, err := policy.Resolve(newPath)
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

		if isNewFile {
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

func patchSourceBytes(file *gitdiff.File, absolutePath string) ([]byte, error) {
	if file.IsNew || file.OldName == "/dev/null" {
		return nil, nil
	}
	return guardrails.ReadTextFile(absolutePath, maxReadBytes)
}

func stripPath(path string, strip int) string {
	path = strings.TrimSpace(path)
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
