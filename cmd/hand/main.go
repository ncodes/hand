package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"

	doctorcmd "github.com/wandxy/hand/cmd/doctor"
	sessioncmd "github.com/wandxy/hand/cmd/session"
	tracecmd "github.com/wandxy/hand/cmd/trace"
	upcmd "github.com/wandxy/hand/cmd/up"
	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	_ = logutils.InitLogger("hand")
}

var (
	envFile              = ".env"
	configFile           = "config.yaml"
	rootOutput io.Writer = os.Stdout
)

type chatRunner interface {
	Respond(context.Context, string, rpcclient.RespondOptions) (string, error)
	Close() error
}

var newChatClient = func(ctx context.Context, cfg *config.Config) (chatRunner, error) {
	return rpcclient.NewClient(ctx, rpcclient.Options{
		Address: cfg.RPCAddress,
		Port:    cfg.RPCPort,
	})
}

func main() {
	envFile = resolveEnvFile(os.Args)
	if err := config.PreloadEnvFile(envFile); err != nil {
		log.Fatal().Err(err).Msg("Failed to preload environment")
	}

	cmd := newCommand()
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		if exitErr, ok := errors.AsType[cli.ExitCoder](err); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatal().Err(err).Msg("Failed to run")
	}
}

func newCommand() *cli.Command {
	var cmd *cli.Command
	cmd = &cli.Command{
		Name:        "hand",
		Usage:       "Run and manage your Hand daemon",
		Description: handcli.AppDescription,
		Flags:       append(handcli.RootFlags(&envFile, &configFile), handcli.RequestInstructFlag()),
		Commands: []*cli.Command{
			doctorcmd.NewCommand(),
			sessioncmd.NewCommand(),
			tracecmd.NewCommand(),
			upcmd.NewCommand(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			message := strings.TrimSpace(strings.Join(cmd.Args().Slice(), " "))
			if message == "" {
				return cli.ShowAppHelp(cmd)
			}

			cfg, err := config.Load(cmd.String("env-file"), cmd.String("config"))
			if err != nil {
				return err
			}
			handcli.ApplyConfigOverrides(cmd, cfg)

			config.Set(cfg)
			_ = logutils.ConfigureLogger("hand", cfg.LogNoColor)
			logutils.SetLogLevel(cfg.LogLevel)

			client, err := newChatClient(ctx, cfg)
			if err != nil {
				return err
			}
			defer client.Close()

			instruct := ""
			if cmd.IsSet("instruct") {
				instruct = cfg.Instruct
			}

			reply, err := client.Respond(ctx, message, rpcclient.RespondOptions{
				Instruct:  instruct,
				SessionID: strings.TrimSpace(cmd.String("session")),
			})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(rootOutput, reply)
			return err
		},
	}
	return cmd
}

func resolveEnvFile(args []string) string {
	if value := strings.TrimSpace(os.Getenv("AGENT_ENV_FILE")); value != "" {
		return value
	}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--env-file" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if value, ok := strings.CutPrefix(arg, "--env-file="); ok {
			return strings.TrimSpace(value)
		}
	}

	return envFile
}
