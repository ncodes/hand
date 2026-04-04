package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/pkg/logutils"
)

var (
	sessionOutput io.Writer = os.Stdout
	newClient               = func(ctx context.Context, cfg *config.Config) (rpcclient.SessionClient, error) {
		return rpcclient.NewClient(ctx, rpcclient.Options{
			Address: cfg.RPCAddress,
			Port:    cfg.RPCPort,
		})
	}
)

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
					client, err := sessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()

					session, err := client.CreateSession(ctx, strings.TrimSpace(cmd.Args().First()))
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
					client, err := sessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()

					sessions, err := client.ListSessions(ctx)
					if err != nil {
						return err
					}
					for _, session := range sessions {
						if _, err := fmt.Fprintln(sessionOutput, session.ID); err != nil {
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
					client, err := sessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()

					id := strings.TrimSpace(cmd.Args().First())
					if err := client.UseSession(ctx, id); err != nil {
						return err
					}
					_, err = fmt.Fprintln(sessionOutput, id)
					return err
				},
			},
			{
				Name:  "current",
				Usage: "Show the current session selection",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := sessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()

					id, err := client.CurrentSession(ctx)
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(sessionOutput, id)
					return err
				},
			},
			{
				Name:      "compact",
				Usage:     "Force summary compaction for a session",
				ArgsUsage: "[session-id]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := sessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()

					result, err := client.CompactSession(ctx, strings.TrimSpace(cmd.Args().First()))
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
				Name:      "status",
				Usage:     "Show session context usage",
				ArgsUsage: "[session-id]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					client, err := sessionClient(ctx, cmd)
					if err != nil {
						return err
					}
					defer client.Close()

					result, err := client.GetSession(ctx, strings.TrimSpace(cmd.Args().First()))
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

func formatSessionTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func sessionClient(ctx context.Context, cmd *cli.Command) (rpcclient.SessionClient, error) {
	cfg, err := config.Load(cmd.String("env-file"), cmd.String("config"))
	if err != nil {
		return nil, err
	}

	handcli.ApplyConfigOverrides(cmd, cfg)

	config.Set(cfg)
	_ = logutils.ConfigureLogger("hand", cfg.LogNoColor)
	logutils.SetLogLevel(cfg.LogLevel)

	return newClient(ctx, cfg)
}
