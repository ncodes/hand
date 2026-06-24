package readfile

import (
	"context"

	"github.com/wandxy/morph/pkg/logutils"

	envtypes "github.com/wandxy/morph/internal/environment/types"
	"github.com/wandxy/morph/internal/guardrails"
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
		Name:         "read_file",
		Description:  "Read a text file from an allowed workspace root.",
		ParallelSafe: true,
		Groups:       []string{"core"},
		Requires:     tools.Capabilities{Filesystem: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"path": common.StringSchema("Path to the text file relative to an allowed workspace root."),
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

			resolved, err := runtime.FilePolicy().Resolve(req.Path)
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
