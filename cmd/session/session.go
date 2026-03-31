package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	sessionstore "github.com/wandxy/hand/internal/storage/session"
	"github.com/wandxy/hand/pkg/logutils"
)

type runner interface {
	CreateSession(context.Context, string) (sessionstore.Session, error)
	ListSessions(context.Context) ([]sessionstore.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (string, error)
	Close() error
}

var (
	sessionOutput io.Writer = os.Stdout
	newClient               = func(ctx context.Context, cfg *config.Config) (runner, error) {
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
		},
	}
}

func sessionClient(ctx context.Context, cmd *cli.Command) (runner, error) {
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
