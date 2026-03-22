package main

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	altsrc "github.com/urfave/cli-altsrc/v3"
	"github.com/urfave/cli-altsrc/v3/yaml"
	cli "github.com/urfave/cli/v3"

	upcmd "github.com/wandxy/agent/cmd/up"
	"github.com/wandxy/agent/internal/config"
	"github.com/wandxy/agent/pkg/logutils"
)

func init() {
	_ = logutils.InitLogger("agent")
}

var (
	envFile    = ".env"
	configFile = "config.yaml"
)

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
		Name:        "agent",
		Usage:       "run and manage the agent",
		Description: "Agent is a personal assistant that works and exists for you.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "env-file",
				Usage:       "load environment overrides from this .env file",
				Value:       ".env",
				Destination: &envFile,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("AGENT_ENV_FILE"),
				),
			},
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "read base settings from this YAML config file",
				Value:       "config.yaml",
				Destination: &configFile,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("AGENT_CONFIG"),
				),
			},
			&cli.StringFlag{
				Name:  "model.router",
				Usage: "model router identifier",
				Value: config.Get().ModelRouter,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL_ROUTER"),
					yaml.YAML("model.router", altsrc.NewStringPtrSourcer(&configFile)),
				),
			},
			&cli.StringFlag{
				Name:  "model.key",
				Usage: "authentication key for the selected model router",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL_KEY"),
					yaml.YAML("model.key", altsrc.NewStringPtrSourcer(&configFile)),
				),
			},
			&cli.StringFlag{
				Name:  "model",
				Usage: "model slug to send to the provider, for example openai/gpt-4o-mini",
				Value: config.Get().Model,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL"),
					yaml.YAML("model.name", altsrc.NewStringPtrSourcer(&configFile)),
				),
			},
			&cli.StringFlag{
				Name:  "model.base-url",
				Usage: "base URL for the model provider API",
				Value: config.Get().ModelBaseURL,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL_BASE_URL"),
					yaml.YAML("model.baseUrl", altsrc.NewStringPtrSourcer(&configFile)),
				),
			},
			&cli.StringFlag{
				Name:  "log.level",
				Usage: "set the minimum log level: debug, info, warn, or error",
				Value: config.Get().LogLevel,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("LOG_LEVEL"),
					yaml.YAML("log.level", altsrc.NewStringPtrSourcer(&configFile)),
				),
			},
			&cli.BoolFlag{
				Name:  "log.no-color",
				Usage: "emit plain log output without ANSI color codes",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("LOG_NO_COLOR"),
					yaml.YAML("log.noColor", altsrc.NewStringPtrSourcer(&configFile)),
				),
			},
		},
		Commands: []*cli.Command{
			upcmd.NewCommand(),
		},
		Action: func(context.Context, *cli.Command) error {
			return cli.ShowAppHelp(cmd)
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
