package up

import (
	"context"

	"github.com/openai/openai-go/v3/option"
	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/pkg/logutils"
)

type runner interface {
	Run(context.Context) error
}

var newAgentRunner = func(cfg *config.Config, modelClient models.Client) runner {
	return agent.NewAgent(context.Background(), cfg, modelClient)
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "up",
		Usage: "start the agent runtime",
		Action: func(ctx context.Context, cmd *cli.Command) error {
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
			if err := cfg.Validate(); err != nil {
				return err
			}

			config.Set(cfg)
			_ = logutils.ConfigureLogger("hand", cfg.LogNoColor)
			logutils.SetLogLevel(cfg.LogLevel)

			log.Info().
				Str("name", cfg.Name).
				Str("model", cfg.Model).
				Str("modelRouter", cfg.ModelRouter).
				Str("modelBaseURL", cfg.ModelBaseURL).
				Str("logLevel", cfg.LogLevel).
				Bool("logNoColor", cfg.LogNoColor).
				Msg("configuration loaded")

			clientOptions := make([]option.RequestOption, 0, 1)
			if cfg.ModelBaseURL != "" {
				clientOptions = append(clientOptions, option.WithBaseURL(cfg.ModelBaseURL))
			}

			modelClient, err := models.NewOpenAIClient(cfg.ModelKey, clientOptions...)
			if err != nil {
				return err
			}

			app := newAgentRunner(cfg, modelClient)
			return app.Run(ctx)
		},
	}
}
