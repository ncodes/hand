package readfile

import (
	"context"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

func Definition(runtime envtypes.Runtime) tools.Definition {
	type input struct {
		Path string `json:"path"`
	}

	return tools.Definition{
		Name:        "read_file",
		Description: "Read a text file from an allowed workspace root.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"path": common.StringSchema("Path to the text file relative to an allowed workspace root."),
		}, "path"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			resolved, err := runtime.FilePolicy().Resolve(req.Path)
			if err != nil {
				return common.FileError(err), nil
			}

			content, err := guardrails.ReadTextFile(resolved.Absolute, common.MaxReadBytes)
			if err != nil {
				return common.FileError(err), nil
			}

			return common.EncodeOutput(map[string]any{
				"path":    common.NormalizedDisplayPath(resolved.Relative),
				"content": string(content),
				"bytes":   len(content),
			})
		}),
	}
}
