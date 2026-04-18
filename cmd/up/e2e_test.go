package up

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/e2e"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	"github.com/wandxy/hand/internal/models"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func Test_E2E_UpCommand_BootsAndServesRPC(t *testing.T) {
	originalRunner := newAgentRunner
	originalFactory := openAIClientFactory
	originalOutput := startupOutput
	t.Cleanup(func() {
		newAgentRunner = originalRunner
		openAIClientFactory = originalFactory
		startupOutput = originalOutput
	})

	startupBuffer := &bytes.Buffer{}
	startupOutput = startupBuffer

	stub := &agentstub.AgentRunnerStub{
		AgentServiceStub: agentstub.AgentServiceStub{
			Reply:            "hello from up",
			CurrentSessionID: "default",
		},
	}
	newAgentRunner = func(_ context.Context, cfg *config.Config, modelClient, summaryClient models.Client) agentRunner {
		require.NotNil(t, cfg)
		require.NotNil(t, modelClient)
		require.NotNil(t, summaryClient)
		return stub
	}
	openAIClientFactory = func(string, ...option.RequestOption) (*models.OpenAIClient, error) {
		return &models.OpenAIClient{}, nil
	}

	port, err := e2e.ReserveRPCPort()
	require.NoError(t, err)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf(`
name: yaml-up
model:
  name: openai/gpt-4o-mini
  provider: openrouter
  key: config-key
  verifyModel: false
rpc:
  address: 127.0.0.1
  port: %d
`, port)), 0o600))

	envPath := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("NAME=env-up\n"), 0o600))

	envFile := ""
	configFile := ""
	rootCmd := &cli.Command{
		Name:           "hand",
		DefaultCommand: "up",
		Flags:          handcli.RootFlags(&envFile, &configFile),
		Commands: []*cli.Command{
			NewCommand(),
		},
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- rootCmd.Run(runCtx, []string{
			"hand",
			"--env-file", envPath,
			"--config", configPath,
			"--name", "cli-up",
			"up",
		})
	}()

	client, err := e2e.WaitForRPC("127.0.0.1", port, 5*time.Second)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, client.Close())
		cancel()
		require.NoError(t, <-errCh)
	})

	reply, err := client.Respond(context.Background(), "hello rpc", rpcclient.RespondOptions{})
	require.NoError(t, err)
	assert.Equal(t, "hello from up", reply)

	current, err := client.CurrentSession(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "default", current)
	assert.Contains(t, startupBuffer.String(), "cli-up")
	assert.Contains(t, startupBuffer.String(), fmt.Sprintf("127.0.0.1:%d", port))
}
