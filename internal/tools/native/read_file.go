package native

import (
	"context"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

func ReadFileDefinition(dependencies envtypes.Runtime) tools.Definition {
	type input struct {
		Path string `json:"path"`
	}

	return tools.Definition{
		Name:        "read_file",
		Description: "Read a text file from an allowed workspace root.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		InputSchema: objectSchema(map[string]any{
			"path": stringSchema("Path to the text file relative to an allowed workspace root."),
		}, "path"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := decodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			resolved, err := dependencies.FilePolicy().Resolve(req.Path)
			if err != nil {
				return fileError(err), nil
			}

			content, err := guardrails.ReadTextFile(resolved.Absolute, maxReadBytes)
			if err != nil {
				return fileError(err), nil
			}

			return encodeOutput(map[string]any{
				"path":    normalizedDisplayPath(resolved.Relative),
				"content": string(content),
				"bytes":   len(content),
			})
		}),
	}
}
