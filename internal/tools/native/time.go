package native

import (
	"context"
	"time"

	"github.com/wandxy/hand/internal/tools"
)

var now = time.Now

func TimeDefinition() tools.Definition {
	return tools.Definition{
		Name:        "time",
		Description: "Returns the current server time in RFC3339 format.",
		InputSchema: objectSchema(nil),
		Groups:      []string{"core"},
		Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
			return tools.Result{Output: now().UTC().Format(time.RFC3339)}, nil
		}),
	}
}
