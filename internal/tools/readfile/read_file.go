package readfile

import (
	"context"

	"github.com/rs/zerolog/log"

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

			log.Info().
				Str("tool", "read_file").
				Str("phase", "start").
				Str("path", common.NormalizedDisplayPath(req.Path)).
				Msg("tool call started")

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
				Msg("tool call completed")

			return common.EncodeOutput(map[string]any{
				"path":    common.NormalizedDisplayPath(resolved.Relative),
				"content": string(content),
				"bytes":   len(content),
			})
		}),
	}
}
