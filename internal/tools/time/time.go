package time

import (
	"context"
	"time"

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
			return tools.Result{Output: now().UTC().Format(time.RFC3339)}, nil
		}),
	}
}
