package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openai/openai-go/v3/option"
	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"

	doctorcmd "github.com/wandxy/hand/cmd/doctor"
	upcmd "github.com/wandxy/hand/cmd/up"
	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/diagnostics"
	"github.com/wandxy/hand/internal/models"
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
	Run(context.Context) error
	Chat(context.Context, string) (string, error)
}

var newChatAgent = func(ctx context.Context, cfg *config.Config, modelClient models.Client) chatRunner {
	return agent.NewAgent(ctx, cfg, modelClient)
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
		Name:        "agent",
		Usage:       "run and manage the agent",
		Description: "Agent is a personal assistant that works and exists for you.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "env-file",
				Usage:       "Load environment overrides from this .env file",
				Value:       ".env",
				Destination: &envFile,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("AGENT_ENV_FILE"),
				),
			},
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Read base settings from this YAML config file",
				Value:       "config.yaml",
				Destination: &configFile,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("AGENT_CONFIG"),
				),
			},
			&cli.StringFlag{
				Name:  "name",
				Usage: "The name of your hand",
				Value: config.Get().Name,
			},
			&cli.StringFlag{
				Name:  "model.router",
				Usage: "Model router identifier",
				Value: config.Get().ModelRouter,
			},
			&cli.StringFlag{
				Name:  "model.key",
				Usage: "Authentication key for the selected model router",
			},
			&cli.StringFlag{
				Name:  "model",
				Usage: "Model slug to send to the provider, for example openai/gpt-4o-mini",
				Value: config.Get().Model,
			},
			&cli.StringFlag{
				Name:  "model.base-url",
				Usage: "Base URL for the model provider API",
				Value: config.Get().ModelBaseURL,
			},
			&cli.StringFlag{
				Name:  "log.level",
				Usage: "Set the minimum log level: debug, info, warn, or error",
				Value: config.Get().LogLevel,
			},
			&cli.BoolFlag{
				Name:  "log.no-color",
				Usage: "Emit plain log output without ANSI color codes",
			},
			&cli.BoolFlag{
				Name:  "debug.requests",
				Usage: "Log sanitized model request payloads at debug level",
			},
		},
		Commands: []*cli.Command{
			doctorcmd.NewCommand(),
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

			if cmd.IsSet("name") {
				cfg.Name = cmd.String("name")
			}
			if cmd.IsSet("model") {
				cfg.Model = cmd.String("model")
			}
			if cmd.IsSet("model.router") {
				cfg.ModelRouter = cmd.String("model.router")
			}
			if cmd.IsSet("model.key") {
				cfg.ModelKey = cmd.String("model.key")
			}
			if cmd.IsSet("model.base-url") {
				cfg.ModelBaseURL = cmd.String("model.base-url")
			}
			if cmd.IsSet("log.level") {
				cfg.LogLevel = cmd.String("log.level")
			}
			if cmd.IsSet("log.no-color") {
				cfg.LogNoColor = cmd.Bool("log.no-color")
			}
			if cmd.IsSet("debug.requests") {
				cfg.DebugRequests = cmd.Bool("debug.requests")
			}

			report := diagnostics.Build(cmd.String("env-file"), cmd.String("config"), cfg, nil)
			if report.HasFailures() {
				return errors.New(report.FirstFailure())
			}

			config.Set(cfg)
			_ = logutils.ConfigureLogger("hand", cfg.LogNoColor)
			logutils.SetLogLevel(cfg.LogLevel)

			clientOptions := make([]option.RequestOption, 0, 1)
			if cfg.ModelBaseURL != "" {
				clientOptions = append(clientOptions, option.WithBaseURL(cfg.ModelBaseURL))
			}

			auth, _ := cfg.ResolveModelAuth()
			modelClient, err := models.NewOpenAIClient(auth.APIKey, clientOptions...)
			if err != nil {
				return err
			}

			app := newChatAgent(ctx, cfg, modelClient)
			if err := app.Run(ctx); err != nil {
				return err
			}

			reply, err := app.Chat(ctx, message)
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
