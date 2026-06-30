package daemon

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	urfavecli "github.com/urfave/cli/v3"

	morphcli "github.com/wandxy/morph/internal/cli"
	"github.com/wandxy/morph/pkg/stringx"
)

var daemonOutput io.Writer = os.Stdout
var getDaemonStatus = morphcli.GetDaemonStatus

func SetOutput(w io.Writer) io.Writer {
	previous := daemonOutput
	if w == nil {
		daemonOutput = io.Discard
	} else {
		daemonOutput = w
	}
	morphcli.SetDaemonOutput(daemonOutput)
	return previous
}

func NewCommand() *urfavecli.Command {
	return &urfavecli.Command{
		Name:     "daemon",
		Usage:    "Start the Morph daemon",
		Commands: []*urfavecli.Command{newStatusCommand()},
		Flags:    []urfavecli.Flag{morphcli.PersistentInstructFlag()},
		Action: func(ctx context.Context, cmd *urfavecli.Command) error {
			if cmd.Args().Len() > 0 {
				return fmt.Errorf("unknown daemon command %q", cmd.Args().First())
			}

			return morphcli.RunDaemonWithConfigRestarts(ctx, cmd)
		},
	}
}

func newStatusCommand() *urfavecli.Command {
	return &urfavecli.Command{
		Name:  "status",
		Usage: "Show daemon health and connection status",
		Action: func(ctx context.Context, _ *urfavecli.Command) error {
			status, err := getDaemonStatus(ctx)
			if writeErr := writeDaemonStatus(daemonOutput, status); writeErr != nil {
				return writeErr
			}
			return err
		},
	}
}

func writeDaemonStatus(out io.Writer, status morphcli.DaemonStatus) error {
	if stringx.String(status.State).Trim() == "" {
		status.State = "unknown"
	}

	_, err := fmt.Fprintf(
		out,
		"state=%s health=%s profile=%s pid=%d rpc=%s:%d uptime=%s started_at=%s\n",
		status.State,
		formatStatusValue(status.Health),
		formatStatusValue(status.Profile),
		status.PID,
		formatStatusValue(status.Address),
		status.Port,
		status.Uptime,
		formatStatusTime(status.StartedAt),
	)
	return err
}

func formatStatusValue(value string) string {
	value = stringx.String(value).Trim()
	if value == "" {
		return "-"
	}

	return value
}

func formatStatusTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}

	return value.Format("2006-01-02T15:04:05Z07:00")
}
