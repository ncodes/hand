package daemon

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	urfavecli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
)

var daemonOutput io.Writer = os.Stdout
var getDaemonStatus = handcli.GetDaemonStatus

func SetOutput(w io.Writer) io.Writer {
	previous := daemonOutput
	if w == nil {
		daemonOutput = io.Discard
	} else {
		daemonOutput = w
	}
	handcli.SetDaemonOutput(daemonOutput)
	return previous
}

func NewCommand() *urfavecli.Command {
	return &urfavecli.Command{
		Name:     "daemon",
		Usage:    "Start the Hand daemon",
		Commands: []*urfavecli.Command{newStatusCommand()},
		Flags:    []urfavecli.Flag{handcli.PersistentInstructFlag()},
		Action: func(ctx context.Context, cmd *urfavecli.Command) error {
			return handcli.RunDaemonWithConfigRestarts(ctx, cmd)
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

func writeDaemonStatus(out io.Writer, status handcli.DaemonStatus) error {
	if strings.TrimSpace(status.State) == "" {
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
	value = strings.TrimSpace(value)
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
