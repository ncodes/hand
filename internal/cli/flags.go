package cli

import (
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
)

const AppDescription = "Hand is a personal assistant that works and exists for you."

func RootFlags(envFile, configFile *string) []cli.Flag {
	flags := []cli.Flag{
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
			Name:  "rpc.address",
			Usage: "Bind address for the RPC service",
			Value: config.Get().RPCAddress,
		},
		&cli.IntFlag{
			Name:  "rpc.port",
			Usage: "Bind port for the RPC service",
			Value: config.Get().RPCPort,
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
	}

	if envFile != nil {
		flags = append([]cli.Flag{
			&cli.StringFlag{
				Name:        "env-file",
				Usage:       "Load environment overrides from this .env file",
				Value:       ".env",
				Destination: envFile,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("AGENT_ENV_FILE"),
				),
			},
		}, flags...)
	}

	if configFile != nil {
		insertAt := 1
		if envFile == nil {
			insertAt = 0
		}
		configFlag := &cli.StringFlag{
			Name:        "config",
			Aliases:     []string{"c"},
			Usage:       "Read base settings from this YAML config file",
			Value:       "config.yaml",
			Destination: configFile,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("AGENT_CONFIG"),
			),
		}
		flags = append(flags[:insertAt], append([]cli.Flag{configFlag}, flags[insertAt:]...)...)
	}

	return flags
}

func ApplyConfigOverrides(cmd *cli.Command, cfg *config.Config) {
	if cfg == nil || cmd == nil {
		return
	}

	if cmd.IsSet("name") {
		cfg.Name = strings.TrimSpace(cmd.String("name"))
	}
	if cmd.IsSet("model") {
		cfg.Model = strings.TrimSpace(cmd.String("model"))
	}
	if cmd.IsSet("model.router") {
		cfg.ModelRouter = strings.TrimSpace(cmd.String("model.router"))
	}
	if cmd.IsSet("model.key") {
		cfg.ModelKey = strings.TrimSpace(cmd.String("model.key"))
	}
	if cmd.IsSet("model.base-url") {
		cfg.ModelBaseURL = strings.TrimSpace(cmd.String("model.base-url"))
	}
	if cmd.IsSet("rpc.address") {
		cfg.RPCAddress = strings.TrimSpace(cmd.String("rpc.address"))
	}
	if cmd.IsSet("rpc.port") {
		cfg.RPCPort = cmd.Int("rpc.port")
	}
	if cmd.IsSet("log.level") {
		cfg.LogLevel = strings.TrimSpace(cmd.String("log.level"))
	}
	if cmd.IsSet("log.no-color") {
		cfg.LogNoColor = cmd.Bool("log.no-color")
	}
	if cmd.IsSet("debug.requests") {
		cfg.DebugRequests = cmd.Bool("debug.requests")
	}
}
