package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	cli "github.com/urfave/cli/v3"

	morphcli "github.com/wandxy/morph/internal/cli"
	"github.com/wandxy/morph/internal/config"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/runtime"
	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"
)

var (
	sessionOutput io.Writer = os.Stdout
	newClient               = func(ctx context.Context, cfg *config.Config) (sessionClient, error) {
		return rpcclient.NewClient(ctx, rpcclient.Options{
			Address: cfg.RPC.Address,
			Port:    cfg.RPC.Port,
		})
	}
)

type sessionClient interface {
	Close() error
	SessionAPI() rpcclient.SessionAPI
}

func SetOutput(w io.Writer) io.Writer {
	previous := sessionOutput
	if w == nil {
		sessionOutput = io.Discard
		return previous
	}
	sessionOutput = w
	return previous
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "session",
		Usage: "Manage persisted chat sessions over RPC",
		Commands: []*cli.Command{
			{
				Name:      "new",
				Usage:     "Create a new session",
				ArgsUsage: "<session-id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessions := client.SessionAPI()

					autoSwitch := false
					stringValue1 := str.String(cmd.Args().First())
					session, err := sessions.CreateWithOptions(ctx, rpcclient.CreateSessionOptions{
						ID:         stringValue1.Trim(),
						AutoSwitch: &autoSwitch,
					})
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(sessionOutput, session.ID)
					return err
				},
			},
			{
				Name:  "list",
				Usage: "List persisted sessions",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessionClient := client.SessionAPI()

					sessions, err := sessionClient.List(ctx)
					if err != nil {
						return err
					}
					for _, session := range sessions {
						if _, err := fmt.Fprintln(sessionOutput, getSessionListLabel(session.ID, session.Title)); err != nil {
							return err
						}
					}

					return nil
				},
			},
			{
				Name:      "use",
				Usage:     "Mark a session as the current session",
				ArgsUsage: "<session-id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessions := client.SessionAPI()
					stringValue2 := str.String(cmd.Args().First())
					id := stringValue2.Trim()
					if err := sessions.Use(ctx, id); err != nil {
						return err
					}
					_, err = fmt.Fprintln(sessionOutput, id)
					return err
				},
			},
			{
				Name:      "unarchive",
				Usage:     "Restore an archived session",
				ArgsUsage: "<session-id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessions := client.SessionAPI()
					stringValue3 := str.String(cmd.Args().First())
					session, err := sessions.Unarchive(ctx, stringValue3.Trim())
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(sessionOutput, session.ID)
					return err
				},
			},
			{
				Name:  "current",
				Usage: "Show the current session selection",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessions := client.SessionAPI()

					session, err := sessions.Current(ctx)
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(sessionOutput, session.ID)
					return err
				},
			},
			{
				Name:      "compact",
				Usage:     "Force summary compaction for a session",
				ArgsUsage: "[session-id]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessions := client.SessionAPI()
					stringValue4 := str.String(cmd.Args().First())
					result, err := sessions.Compact(ctx, stringValue4.Trim())
					if err != nil {
						return err
					}

					_, err = fmt.Fprintf(
						sessionOutput,
						"id=%s source_end_offset=%d source_message_count=%d updated_at=%s current_context_length=%d total_context_length=%d\n",
						result.SessionID,
						result.SourceEndOffset,
						result.SourceMessageCount,
						result.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
						result.CurrentContextLength,
						result.TotalContextLength,
					)
					return err
				},
			},
			{
				Name:      "repair",
				Usage:     "Repair session storage artifacts",
				ArgsUsage: "[session-id]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "full",
						Usage: "Rebuild all repairable artifacts instead of only missing or stale artifacts",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessions := client.SessionAPI()
					stringValue5 := str.String(cmd.Args().First())
					result, err := sessions.Repair(
						ctx,
						rpcclient.RepairSessionOptions{
							SessionID: stringValue5.Trim(),
							Full:      cmd.Bool("full"),
						},
					)
					if err != nil {
						return err
					}

					_, err = fmt.Fprintf(
						sessionOutput,
						"sessions_scanned=%d messages_scanned=%d rows_scanned=%d missing_rows=%d stale_rows=%d unchanged_rows=%d rebuilt_rows=%d deleted_sources=%d batches=%d\n",
						result.SessionsScanned,
						result.MessagesScanned,
						result.RowsScanned,
						result.MissingRows,
						result.StaleRows,
						result.UnchangedRows,
						result.RebuiltRows,
						result.DeletedSources,
						result.Batches,
					)
					return err
				},
			},
			{
				Name:      "status",
				Usage:     "Show session context usage",
				ArgsUsage: "[session-id]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := getSessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()
					sessions := client.SessionAPI()
					stringValue6 := str.String(cmd.Args().First())
					result, err := sessions.Status(ctx, stringValue6.Trim())
					if err != nil {
						return err
					}

					_, err = fmt.Fprintf(
						sessionOutput,
						"id=%s created_at=%s updated_at=%s compaction_status=%s offset=%d size=%d length=%d used=%d remaining=%d pct_used=%.4f pct_remaining=%.4f\n",
						result.SessionID,
						formatSessionTime(result.CreatedAt),
						formatSessionTime(result.UpdatedAt),
						result.CompactionStatus,
						result.Offset,
						result.Size,
						result.Length,
						result.Used,
						result.Remaining,
						result.UsedPct,
						result.RemainingPct,
					)
					return err
				},
			},
		},
	}
}

func getSessionListLabel(id string, title string) string {
	stringValue7 := str.String(id)
	id = stringValue7.Trim()
	stringValue8 := str.String(title)
	title = stringValue8.Trim()
	if title == "" {
		return id
	}
	if id == "" {
		return title
	}

	return fmt.Sprintf("%s (%s)", title, id)
}

func formatSessionTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func getSessionClient(ctx context.Context, cmd *cli.Command) (sessionClient, error) {
	cfg, inputs, err := morphcli.LoadConfig(cmd)
	if err != nil {
		return nil, err
	}

	morphcli.ApplyConfigOverrides(cmd, cfg)
	morphcli.AddStartupFilesystemRoots(cfg, inputs)

	endpoint, err := runtime.ResolveRPC(ctx, cmd, cfg)
	if err != nil {
		return nil, err
	}
	cfg.RPC = endpoint

	config.Set(cfg)
	_ = logutils.ConfigureLogger("morph", cfg.Log.NoColor)
	logutils.SetLogLevel(cfg.Log.Level)

	return newClient(ctx, cfg)
}
