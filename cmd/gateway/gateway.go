package gateway

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/runtime"
	"github.com/wandxy/hand/pkg/logutils"
)

var (
	gatewayOutput io.Writer = os.Stdout
	newClient               = func(ctx context.Context, cfg *config.Config) (gatewayClient, error) {
		return rpcclient.NewClient(ctx, rpcclient.Options{
			Address: cfg.RPC.Address,
			Port:    cfg.RPC.Port,
		})
	}
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

			result, err := client.GatewayAPI().ListPairings(ctx, strings.TrimSpace(cmd.Args().First()))
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
			source := strings.TrimSpace(cmd.Args().Get(0))
			code := strings.TrimSpace(cmd.Args().Get(1))
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
			source := strings.TrimSpace(cmd.Args().Get(0))
			senderID := strings.TrimSpace(cmd.Args().Get(1))
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
			source := strings.TrimSpace(cmd.Args().First())
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
	cfg, inputs, err := handcli.LoadConfig(cmd)
	if err != nil {
		return nil, err
	}

	handcli.ApplyConfigOverrides(cmd, cfg)
	handcli.AddStartupFilesystemRoots(cfg, inputs)

	endpoint, err := runtime.ResolveRPC(ctx, cmd, cfg)
	if err != nil {
		return nil, err
	}

	cfg.RPC = endpoint
	config.Set(cfg)

	_ = logutils.ConfigureLogger("hand", cfg.Log.NoColor)
	logutils.SetLogLevel(cfg.Log.Level)

	return newClient(ctx, cfg)
}
