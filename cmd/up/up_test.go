package up

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	altsrc "github.com/urfave/cli-altsrc/v3"
	"github.com/urfave/cli-altsrc/v3/yaml"
	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/agent/internal/config"
	"github.com/wandxy/agent/internal/models"
	"github.com/wandxy/agent/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestNewCommand_UsesFlagsToBuildConfig(t *testing.T) {
	original := config.Get()
	originalNewAgentRunner := newAgentRunner
	t.Cleanup(func() {
		config.Set(original)
		newAgentRunner = originalNewAgentRunner
	})
	config.Set(nil)
	configFile := ""
	runCalled := false
	newAgentRunner = func(cfg *config.Config, modelClient models.Client) runner {
		return runnerFunc(func(context.Context) error {
			runCalled = true
			return nil
		})
	}

	cmd := newRootCommandForTest(&configFile)
	require.NoError(t, cmd.Run(context.Background(), []string{
		"agent",
		"--model", "flag-model",
		"--model.router", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", "https://flag.example/v1",
		"--log.level", "debug",
		"--log.no-color=true",
		"up",
	}))

	cfg := config.Get()
	require.Equal(t, "flag-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "flag-key", cfg.ModelKey)
	require.Equal(t, "https://flag.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "debug", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
	require.True(t, runCalled)
}

func TestNewCommand_ReturnsValidationError(t *testing.T) {
	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	err := cmd.Run(context.Background(), []string{
		"agent",
		"--model", "",
		"--model.router", "openrouter",
		"--model.key", "",
		"up",
	})
	require.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
}

func newRootCommandForTest(configFile *string) *cli.Command {
	return &cli.Command{
		Name:           "agent",
		DefaultCommand: "up",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "model.router",
				Value: config.Get().ModelRouter,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL_ROUTER"),
					yaml.YAML("model.router", altsrc.NewStringPtrSourcer(configFile)),
				),
			},
			&cli.StringFlag{
				Name: "model.key",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL_KEY"),
					yaml.YAML("model.key", altsrc.NewStringPtrSourcer(configFile)),
				),
			},
			&cli.StringFlag{
				Name:  "model",
				Value: config.Get().Model,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL"),
					yaml.YAML("model.name", altsrc.NewStringPtrSourcer(configFile)),
				),
			},
			&cli.StringFlag{
				Name:  "model.base-url",
				Value: config.Get().ModelBaseURL,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("MODEL_BASE_URL"),
					yaml.YAML("model.baseUrl", altsrc.NewStringPtrSourcer(configFile)),
				),
			},
			&cli.StringFlag{
				Name:  "log.level",
				Value: config.Get().LogLevel,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("LOG_LEVEL"),
					yaml.YAML("log.level", altsrc.NewStringPtrSourcer(configFile)),
				),
			},
			&cli.BoolFlag{
				Name: "log.no-color",
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("LOG_NO_COLOR"),
					yaml.YAML("log.noColor", altsrc.NewStringPtrSourcer(configFile)),
				),
			},
		},
		Commands: []*cli.Command{
			NewCommand(),
		},
	}
}

type runnerFunc func(context.Context) error

func (f runnerFunc) Run(ctx context.Context) error {
	return f(ctx)
}
