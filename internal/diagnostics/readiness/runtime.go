package readiness

import (
	"context"
	"fmt"

	"github.com/wandxy/hand/internal/profile"
	handruntime "github.com/wandxy/hand/internal/runtime"
)

func buildRuntimeGroup(ctx context.Context, active profile.Profile) Group {
	result := handruntime.Probe(ctx, active)
	if result.Err == nil {
		return Group{
			Name: "daemon",
			Checks: []Check{
				check(
					"runtime",
					StatusPass,
					fmt.Sprintf(
						"profile %q is listening on %s:%d",
						result.Metadata.Profile,
						result.Metadata.RPC.Address,
						result.Metadata.RPC.Port,
					),
				),
			},
		}
	}

	status := StatusWarn
	message := result.Err.Error()
	if result.State == handruntime.ProbeStateInvalid {
		status = StatusFail
	}

	return Group{
		Name: "daemon",
		Checks: []Check{
			check(
				"runtime",
				status,
				message,
				commandAction("hand up", "start the daemon for this profile"),
			),
		},
	}
}
