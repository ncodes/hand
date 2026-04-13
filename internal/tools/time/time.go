package time

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

var now = time.Now

func Definition() tools.Definition {
	return tools.Definition{
		Name:        "time",
		Description: "Returns the current server time in RFC3339 format.",
		InputSchema: common.ObjectSchema(nil),
		Groups:      []string{"core"},
		Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
			log.Info().
				Str("tool", "time").
				Str("phase", "start").
				Msg("tool call started")

			value := now().UTC().Format(time.RFC3339)

			log.Info().
				Str("tool", "time").
				Str("phase", "complete").
				Msg("tool call completed")

			return tools.Result{Output: value}, nil
		}),
	}
}
