package gateway

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	cli "github.com/urfave/cli/v3"

	morphcli "github.com/wandxy/morph/internal/cli"
	"github.com/wandxy/morph/internal/config"
	telegramgateway "github.com/wandxy/morph/internal/gateway/telegram"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/runtime"
	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"
)

var (
	gatewayOutput io.Writer = os.Stdout
	newClient               = func(ctx context.Context, cfg *config.Config) (gatewayClient, error) {
		return rpcclient.NewClient(ctx, rpcclient.Options{
			Address: cfg.RPC.Address,
			Port:    cfg.RPC.Port,
		})
	}
	setTelegramWebhook = telegramgateway.SetWebhook
)

type gatewayClient interface {
	Close() error
	GatewayAPI() rpcclient.GatewayAPI
}

func SetOutput(w io.Writer) io.Writer {
	previous := gatewayOutput
	if w == nil {
		gatewayOutput = io.Discard
		return previous
	}
	gatewayOutput = w
	return previous
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "gateway",
		Usage: "Manage external gateway integrations",
		Commands: []*cli.Command{
			newStatusCommand(),
			newStartCommand(),
			newStopCommand(),
			newRestartCommand(),
			newSetWebhookCommand(),
			{
				Name:  "pairing",
				Usage: "Manage gateway sender pairings",
				Commands: []*cli.Command{
					newPairingListCommand(),
					newPairingApproveCommand(),
					newPairingRevokeCommand(),
					newPairingClearPendingCommand(),
				},
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newSetWebhookCommand() *cli.Command {
	return &cli.Command{
		Name:  "setwebhook",
		Usage: "Register gateway webhooks with providers",
		Commands: []*cli.Command{
			newSetTelegramWebhookCommand(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newSetTelegramWebhookCommand() *cli.Command {
	return &cli.Command{
		Name:      "telegram",
		Usage:     "Register the Telegram webhook URL with Telegram",
		ArgsUsage: "[url]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			stringValue1 := str.String(cmd.Args().First())
			url := stringValue1.Trim()

			cfg, err := loadGatewayConfig(cmd)
			if err != nil {
				return err
			}

			if err := setTelegramWebhook(ctx, cfg.Gateway.Telegram, url); err != nil {
				return err
			}

			if url == "" {
				_, err = fmt.Fprintln(gatewayOutput, "telegram webhook unset")
				return err
			}
			_, err = fmt.Fprintf(gatewayOutput, "telegram webhook set url=%s\n", url)
			return err
		},
	}
}

func newStatusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show daemon gateway runtime status",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			client, err := getGatewayClient(ctx, cmd)
			if err != nil {
				return err
			}
			defer client.Close()

			status, err := client.GatewayAPI().GatewayStatus(ctx)
			if err != nil {
				return err
			}

			return writeGatewayStatus(gatewayOutput, status)
		},
	}
}

func newStartCommand() *cli.Command {
	return &cli.Command{
		Name:  "start",
		Usage: "Start the daemon gateway runtime",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runGatewayRuntimeCommand(ctx, cmd, func(api rpcclient.GatewayAPI) (rpcclient.GatewayStatus, error) {
				return api.Start(ctx)
			})
		},
	}
}

func newStopCommand() *cli.Command {
	return &cli.Command{
		Name:  "stop",
		Usage: "Stop the daemon gateway runtime",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runGatewayRuntimeCommand(ctx, cmd, func(api rpcclient.GatewayAPI) (rpcclient.GatewayStatus, error) {
				return api.Stop(ctx)
			})
		},
	}
}

func newRestartCommand() *cli.Command {
	return &cli.Command{
		Name:  "restart",
		Usage: "Restart the daemon gateway runtime",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runGatewayRuntimeCommand(ctx, cmd, func(api rpcclient.GatewayAPI) (rpcclient.GatewayStatus, error) {
				return api.Restart(ctx)
			})
		},
	}
}

func runGatewayRuntimeCommand(
	ctx context.Context,
	cmd *cli.Command,
	run func(rpcclient.GatewayAPI) (rpcclient.GatewayStatus, error),
) error {
	client, err := getGatewayClient(ctx, cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	status, err := run(client.GatewayAPI())
	if err != nil {
		return err
	}

	return writeGatewayStatus(gatewayOutput, status)
}

func writeGatewayStatus(out io.Writer, status rpcclient.GatewayStatus) error {
	_, err := fmt.Fprintf(
		out,
		"state=%s address=%s port=%d telegram=%s slack=%s",
		status.State,
		status.Address,
		status.Port,
		status.TelegramMode,
		status.SlackMode,
	)
	if err != nil {
		return err
	}
	stringValue2 := str.String(status.LastError)
	if stringValue2.Trim() != "" {
		if _, err := fmt.Fprintf(out, " last_error=%q", status.LastError); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(out)
	return err
}

func newPairingListCommand() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "List pending and approved gateway pairings",
		ArgsUsage: "[source]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			client, err := getGatewayClient(ctx, cmd)
			if err != nil {
				return err
			}
			defer client.Close()
			stringValue3 := str.String(cmd.Args().First())
			result, err := client.GatewayAPI().ListPairings(ctx, stringValue3.Trim())
			if err != nil {
				return err
			}

			return writePairingList(gatewayOutput, result)
		},
	}
}

func writePairingList(out io.Writer, result rpcclient.GatewayPairingList) error {
	writer := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "pending"); err != nil {
		return err
	}
	if len(result.Pending) == 0 {
		if _, err := fmt.Fprintln(writer, "  none"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(writer, "  source\tsender id\tname\texpires"); err != nil {
			return err
		}
		for _, request := range result.Pending {
			if _, err := fmt.Fprintf(
				writer,
				"  %s\t%s\t%s\t%s\n",
				request.Source,
				request.SenderID,
				request.DisplayName,
				request.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
			); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, "approved"); err != nil {
		return err
	}
	if len(result.Approved) == 0 {
		if _, err := fmt.Fprintln(writer, "  none"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(writer, "  source\tsender id\tname"); err != nil {
			return err
		}
		for _, sender := range result.Approved {
			if _, err := fmt.Fprintf(
				writer,
				"  %s\t%s\t%s\n",
				sender.Source,
				sender.SenderID,
				sender.DisplayName,
			); err != nil {
				return err
			}
		}
	}

	return writer.Flush()
}

func newPairingApproveCommand() *cli.Command {
	return &cli.Command{
		Name:      "approve",
		Usage:     "Approve a pending gateway sender pairing",
		ArgsUsage: "<source> <code>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			stringValue4 := str.String(cmd.Args().Get(0))
			source := stringValue4.Trim()
			stringValue5 := str.String(cmd.Args().Get(1))
			code := stringValue5.Trim()
			if source == "" || code == "" {
				return fmt.Errorf("source and code are required")
			}

			client, err := getGatewayClient(ctx, cmd)
			if err != nil {
				return err
			}
			defer client.Close()

			sender, ok, err := client.GatewayAPI().ApprovePairing(ctx, source, code)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no pending gateway pairing matched code")
			}

			_, err = fmt.Fprintf(gatewayOutput, "approved %s %s\n", sender.Source, sender.SenderID)
			return err
		},
	}
}

func newPairingRevokeCommand() *cli.Command {
	return &cli.Command{
		Name:      "revoke",
		Usage:     "Revoke an approved gateway sender pairing",
		ArgsUsage: "<source> <sender-id>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			stringValue6 := str.String(cmd.Args().Get(0))
			source := stringValue6.Trim()
			stringValue7 := str.String(cmd.Args().Get(1))
			senderID := stringValue7.Trim()
			if source == "" || senderID == "" {
				return fmt.Errorf("source and sender id are required")
			}

			client, err := getGatewayClient(ctx, cmd)
			if err != nil {
				return err
			}
			defer client.Close()
			if err := client.GatewayAPI().RevokePairing(ctx, source, senderID); err != nil {
				return err
			}

			_, err = fmt.Fprintf(gatewayOutput, "revoked %s %s\n", source, senderID)
			return err
		},
	}
}

func newPairingClearPendingCommand() *cli.Command {
	return &cli.Command{
		Name:      "clear-pending",
		Usage:     "Clear pending gateway pairing requests",
		ArgsUsage: "[source]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			stringValue8 := str.String(cmd.Args().First())
			source := stringValue8.Trim()
			client, err := getGatewayClient(ctx, cmd)
			if err != nil {
				return err
			}
			defer client.Close()
			if err := client.GatewayAPI().ClearPendingPairings(ctx, source); err != nil {
				return err
			}

			_, err = fmt.Fprintln(gatewayOutput, "cleared pending pairings")
			return err
		},
	}
}

func getGatewayClient(ctx context.Context, cmd *cli.Command) (gatewayClient, error) {
	cfg, err := loadGatewayConfig(cmd)
	if err != nil {
		return nil, err
	}

	endpoint, err := runtime.ResolveRPC(ctx, cmd, cfg)
	if err != nil {
		return nil, err
	}

	cfg.RPC = endpoint
	config.Set(cfg)

	return newClient(ctx, cfg)
}

func loadGatewayConfig(cmd *cli.Command) (*config.Config, error) {
	cfg, inputs, err := morphcli.LoadConfig(cmd)
	if err != nil {
		return nil, err
	}

	morphcli.ApplyConfigOverrides(cmd, cfg)
	morphcli.AddStartupFilesystemRoots(cfg, inputs)

	config.Set(cfg)

	_ = logutils.ConfigureLogger("morph", cfg.Log.NoColor)
	logutils.SetLogLevel(cfg.Log.Level)

	return cfg, nil
}
