package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	cli "github.com/urfave/cli/v3"

	morphcli "github.com/wandxy/morph/internal/cli"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/profile"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/rpc/rpcauth"
	"github.com/wandxy/morph/pkg/str"
)

var (
	browserOutput         io.Writer = os.Stdout
	rotateOwnerCredential           = rpcauth.Rotate
	newClient                       = func(ctx context.Context, cfg *config.Config) (browserClient, error) {
		return rpcclient.NewClient(ctx, rpcclient.Options{
			Address: cfg.RPC.Address, Port: cfg.RPC.Port,
			PermissionSurface: permissions.SurfaceCLI, PermissionPreset: cfg.Permissions.EffectivePreset(),
		})
	}
)

type browserClient interface {
	Close() error
	BrowserAPI() rpcclient.BrowserAPI
}

func SetOutput(output io.Writer) io.Writer {
	previous := browserOutput
	if output == nil {
		browserOutput = io.Discard
	} else {
		browserOutput = output
	}

	return previous
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "browser",
		Usage: "Inspect and manage the daemon browser runtime",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "Print machine-readable JSON"},
		},
		Commands: []*cli.Command{
			newStatusCommand(),
			newProfilesCommand(),
			newSessionsCommand(),
			newStartCommand(),
			newStopCommand(),
			newConfigCommand(),
			newAuthCommand(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newAuthCommand() *cli.Command {
	return &cli.Command{
		Name: "auth", Usage: "Manage browser RPC owner authentication",
		Commands: []*cli.Command{{
			Name: "rotate", Usage: "Rotate the profile RPC owner credential",
			Action: func(_ context.Context, cmd *cli.Command) error {
				active := profile.Active()
				if _, err := rotateOwnerCredential(active.HomeDir); err != nil {
					return err
				}
				if cmd.Bool("json") {
					return writeJSON(map[string]any{
						"rotated": true, "restart_required": true,
						"browser_attachment_approvals_invalidated": true,
					})
				}

				_, err := fmt.Fprintln(
					browserOutput,
					"rotated RPC owner credential; restart the daemon, reconnect local clients, and reapprove browser attachments",
				)
				return err
			},
		}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newStatusCommand() *cli.Command {
	return &cli.Command{
		Name: "status", Usage: "Show browser runtime status",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api, closeClient, err := getBrowserAPI(ctx, cmd)
			if err != nil {
				return err
			}
			defer closeClient()
			statusValue, err := api.Status(ctx)
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				return writeJSON(statusValue)
			}

			_, err = fmt.Fprintf(
				browserOutput, "enabled: %t\nprofiles: %d\nsessions: %d\n",
				statusValue.Enabled, len(statusValue.Profiles), len(statusValue.Sessions),
			)
			return err
		},
	}
}

func newProfilesCommand() *cli.Command {
	return &cli.Command{
		Name: "profiles", Usage: "List configured browser profiles",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api, closeClient, err := getBrowserAPI(ctx, cmd)
			if err != nil {
				return err
			}
			defer closeClient()
			profiles, err := api.Profiles(ctx)
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				return writeJSON(profiles)
			}

			writer := tabwriter.NewWriter(browserOutput, 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(writer, "NAME\tMODE\tDEFAULT\tAVAILABLE\tWARNING"); err != nil {
				return err
			}
			for _, profile := range profiles {
				if _, err := fmt.Fprintf(
					writer, "%s\t%s\t%t\t%t\t%s\n",
					profile.Name, profile.Mode, profile.Default, profile.Available, profile.Warning,
				); err != nil {
					return err
				}
			}

			return writer.Flush()
		},
	}
}

func newSessionsCommand() *cli.Command {
	return &cli.Command{
		Name: "sessions", Usage: "List browser sessions",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api, closeClient, err := getBrowserAPI(ctx, cmd)
			if err != nil {
				return err
			}
			defer closeClient()
			sessions, err := api.Sessions(ctx)
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				return writeJSON(sessions)
			}

			writer := tabwriter.NewWriter(browserOutput, 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(writer, "ID\tPROFILE\tSTATE\tLAST ACTIVE\tERROR\tWARNING"); err != nil {
				return err
			}
			for _, session := range sessions {
				if _, err := fmt.Fprintf(
					writer, "%s\t%s\t%s\t%s\t%s\t%s\n", session.ID, session.Profile, session.State,
					formatTime(session.LastActive), session.Error, session.Warning,
				); err != nil {
					return err
				}
			}

			return writer.Flush()
		},
	}
}

func newStartCommand() *cli.Command {
	return &cli.Command{
		Name: "start", Usage: "Start a managed browser session", ArgsUsage: "[profile]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "owner-session", Value: defaultOwnerSessionID, Usage: "Owner session identity"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api, closeClient, err := getBrowserAPI(ctx, cmd)
			if err != nil {
				return err
			}
			defer closeClient()
			profileName := str.String(cmd.Args().First()).Trim()
			session, err := api.Start(ctx, profileName, cmd.String("owner-session"))
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				return writeJSON(session)
			}

			_, err = fmt.Fprintln(browserOutput, session.ID)
			if err == nil && session.Warning != "" {
				_, err = fmt.Fprintln(browserOutput, "WARNING:", session.Warning)
			}
			return err
		},
	}
}

func newStopCommand() *cli.Command {
	return &cli.Command{
		Name: "stop", Usage: "Stop a managed browser session", ArgsUsage: "<session-id>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "owner-session", Value: defaultOwnerSessionID, Usage: "Owner session identity"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			id := str.String(cmd.Args().First()).Trim()
			if id == "" {
				return errors.New("browser session id is required")
			}
			api, closeClient, err := getBrowserAPI(ctx, cmd)
			if err != nil {
				return err
			}
			defer closeClient()
			session, err := api.Stop(ctx, id, cmd.String("owner-session"))
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				return writeJSON(session)
			}

			_, err = fmt.Fprintf(browserOutput, "%s %s\n", session.ID, session.State)
			return err
		},
	}
}

func newConfigCommand() *cli.Command {
	return &cli.Command{
		Name: "config", Usage: "Show effective browser configuration",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api, closeClient, err := getBrowserAPI(ctx, cmd)
			if err != nil {
				return err
			}
			defer closeClient()
			configValue, err := api.EffectiveConfig(ctx)
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				return writeJSON(configValue)
			}

			_, err = fmt.Fprintf(
				browserOutput,
				"enabled: %t\ncapability: %t\ndefault profile: %s\nnetwork strict: %t\npermission preset: %s\nexecutable configured: %t\n",
				configValue.Enabled, configValue.CapabilityEnabled, configValue.DefaultProfile,
				configValue.NetworkStrict, configValue.PermissionPreset, configValue.ExecutableConfigured,
			)
			return err
		},
	}
}

const defaultOwnerSessionID = "browser-cli"

func getBrowserAPI(ctx context.Context, cmd *cli.Command) (rpcclient.BrowserAPI, func(), error) {
	cfg, _, err := morphcli.LoadConfig(cmd)
	if err != nil {
		return nil, func() {}, err
	}
	cfg.Normalize()
	client, err := newClient(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}
	api := client.BrowserAPI()
	if api == nil {
		_ = client.Close()
		return nil, func() {}, errors.New("browser RPC client is unavailable")
	}

	return api, func() { _ = client.Close() }, nil
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(browserOutput)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}

	return value.Local().Format(time.RFC3339)
}
