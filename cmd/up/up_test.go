package up

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestNewCommand_BuildsConfigFromFlags(t *testing.T) {
	original := config.Get()
	originalNewAgentRunner := newAgentRunner
	originalServeGRPC := serveRPC
	t.Cleanup(func() {
		config.Set(original)
		newAgentRunner = originalNewAgentRunner
		serveRPC = originalServeGRPC
	})
	config.Set(nil)
	configFile := ""
	runCalled := false
	serveCalled := false
	newAgentRunner = func(_ context.Context, cfg *config.Config, modelClient models.Client) runner {
		return runnerFunc(func(context.Context) error {
			runCalled = true
			return nil
		})
	}
	serveRPC = func(ctx context.Context, cfg *config.Config) error {
		serveCalled = true
		require.Equal(t, "0.0.0.0", cfg.RPCAddress)
		require.Equal(t, 6000, cfg.RPCPort)
		return nil
	}

	cmd := newRootCommandForTest(&configFile)
	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "flag-model",
		"--model.router", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", "https://flag.example/v1",
		"--rpc.address", "0.0.0.0",
		"--rpc.port", "6000",
		"--log.level", "debug",
		"--log.no-color=true",
		"up",
	}))

	cfg := config.Get()
	require.Equal(t, "flag-agent", cfg.Name)
	require.Equal(t, "flag-model", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "flag-key", cfg.ModelKey)
	require.Equal(t, "https://flag.example/v1", cfg.ModelBaseURL)
	require.Equal(t, "0.0.0.0", cfg.RPCAddress)
	require.Equal(t, 6000, cfg.RPCPort)
	require.Equal(t, "debug", cfg.LogLevel)
	require.True(t, cfg.LogNoColor)
	require.True(t, runCalled)
	require.True(t, serveCalled)
}

func TestNewCommand_ReturnsValidationError(t *testing.T) {
	originalServeGRPC := serveRPC
	t.Cleanup(func() {
		serveRPC = originalServeGRPC
	})
	serveRPC = func(context.Context, *config.Config) error {
		t.Fatal("serveGRPC should not be called on validation failure")
		return nil
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "",
		"--model.router", "openrouter",
		"--model.key", "",
		"up",
	})
	require.EqualError(t, err, "model key is required; set MODEL_KEY, provide it in config, or use --model.key")
}

func newRootCommandForTest(configFile *string) *cli.Command {
	return &cli.Command{
		Name:           "hand",
		DefaultCommand: "up",
		Flags:          handcli.RootFlags(nil, configFile),
		Commands: []*cli.Command{
			NewCommand(),
		},
	}
}

type runnerFunc func(context.Context) error

func (f runnerFunc) Run(ctx context.Context) error {
	return f(ctx)
}
