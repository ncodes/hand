package writefile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

func Definition(runtime envtypes.Runtime) tools.Definition {
	type input struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		CreateDirs *bool  `json:"create_dirs"`
	}

	return tools.Definition{
		Name:        "write_file",
		Description: "Create or overwrite a text file under an allowed workspace root.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"path":        common.StringSchema("Path to the file relative to an allowed workspace root."),
			"content":     common.StringSchema("Text content to write to the target file."),
			"create_dirs": common.BooleanSchema("When true, create missing parent directories before writing. Defaults to true."),
		}, "path", "content"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			if strings.TrimSpace(req.Path) == "" {
				return common.ToolError("invalid_input", "path is required"), nil
			}

			if guardrails.IsBinary([]byte(req.Content)) {
				return common.ToolError("not_text", "content must be text"), nil
			}

			resolved, err := runtime.FilePolicy().Resolve(req.Path)
			if err != nil {
				return common.FileError(err), nil
			}

			createDirs := true
			if req.CreateDirs != nil {
				createDirs = *req.CreateDirs
			}

			if createDirs {
				if err := common.MkdirAll(filepath.Dir(resolved.Absolute), 0o755); err != nil {
					return common.FileError(err), nil
				}
			}

			_, statErr := common.StatFile(resolved.Absolute)
			created := errors.Is(statErr, os.ErrNotExist)

			if err := common.WriteFile(resolved.Absolute, []byte(req.Content), 0o644); err != nil {
				return common.FileError(err), nil
			}

			return common.EncodeOutput(map[string]any{
				"path":          common.NormalizedDisplayPath(resolved.Relative),
				"bytes_written": len(req.Content),
				"created":       created,
			})
		}),
	}
}
