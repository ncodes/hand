package up

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
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
	originalStartupOutput := startupOutput

	t.Cleanup(func() {
		config.Set(original)
		newAgentRunner = originalNewAgentRunner
		serveRPC = originalServeGRPC
		startupOutput = originalStartupOutput
		logutils.SetOutput(io.Discard)
	})

	config.Set(nil)
	configFile := ""
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	runCalled := false
	serveCalled := false
	startupBuffer := &bytes.Buffer{}
	logBuffer := &bytes.Buffer{}
	startupOutput = startupBuffer
	logutils.SetOutput(logBuffer)

	newAgentRunner = func(_ context.Context, cfg *config.Config, modelClient models.Client) agentRunner {
		return &agentstub.AgentRunnerStub{
			StartFunc: func(context.Context) error {
				runCalled = true
				return nil
			},
		}
	}

	serveRPC = func(ctx context.Context, cfg *config.Config, app agentRunner) error {
		serveCalled = true
		require.Equal(t, "0.0.0.0", cfg.RPCAddress)
		require.Equal(t, 6000, cfg.RPCPort)
		require.NotNil(t, app)
		return nil
	}

	cmd := newRootCommandForTest(&configFile)
	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.router", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", serverURL,
		"--rpc.address", "0.0.0.0",
		"--rpc.port", "6000",
		"--debug.traces",
		"--debug.trace-dir", "/tmp/hand-traces",
		"--log.level", "debug",
		"up",
	}))

	cfg := config.Get()
	require.Equal(t, "flag-agent", cfg.Name)
	require.Equal(t, "openai/gpt-4o-mini", cfg.Model)
	require.Equal(t, "openrouter", cfg.ModelRouter)
	require.Equal(t, "flag-key", cfg.ModelKey)
	require.Equal(t, serverURL, cfg.ModelBaseURL)
	require.Equal(t, "0.0.0.0", cfg.RPCAddress)
	require.Equal(t, 6000, cfg.RPCPort)
	require.True(t, cfg.DebugTraces)
	require.Equal(t, "/tmp/hand-traces", cfg.DebugTraceDir)
	require.Equal(t, "debug", cfg.LogLevel)
	require.False(t, cfg.LogNoColor)
	require.True(t, runCalled)
	require.True(t, serveCalled)

	require.Contains(t, startupBuffer.String(), "\x1b[90m██   ██  █████  ███    ██ ██████")
	require.Contains(t, startupBuffer.String(), handcli.AppDescription)
	require.Contains(t, startupBuffer.String(), "Instance")
	require.Contains(t, startupBuffer.String(), "flag-agent")
	require.Contains(t, startupBuffer.String(), "RPC")
	require.Contains(t, startupBuffer.String(), "0.0.0.0:6000")
	require.Contains(t, startupBuffer.String(), "Debug requests")
	require.Contains(t, startupBuffer.String(), "disabled")
	require.Contains(t, startupBuffer.String(), "Traces")
	require.Contains(t, startupBuffer.String(), "enabled (/tmp/hand-traces)")
	require.Contains(t, startupBuffer.String(), "enabled (/tmp/hand-traces)\n\n")

	logOutput := stripANSI(logBuffer.String())
	require.Contains(t, logOutput, "Configuration loaded")
	require.Contains(t, logOutput, "Starting Hand services")
	require.Contains(t, logOutput, "name=flag-agent")
	require.Contains(t, logOutput, "model=openai/gpt-4o-mini")
	require.Contains(t, logOutput, "router=openrouter")
	require.Contains(t, logOutput, "rpcEndpoint=0.0.0.0:6000")
	require.Contains(t, logOutput, "debugTraces=true")
	require.Contains(t, logOutput, "debugTraceDir=/tmp/hand-traces")
	require.NotContains(t, logOutput, "service=hand")
	require.NotContains(t, logOutput, "rpcAddress=0.0.0.0 rpcEndpoint=0.0.0.0:6000 rpcPort=6000")
}

func TestRenderStartupPanel_DisablesColorWhenRequested(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name:          "daemon",
		Model:         "openai/gpt-4o-mini",
		ModelRouter:   "openrouter",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
		LogNoColor:    true,
		DebugRequests: true,
		DebugTraces:   true,
		DebugTraceDir: "/tmp/hand-traces",
	})

	require.NotContains(t, output, "\x1b[90m")
	require.Contains(t, output, "Instance: daemon")
	require.Contains(t, output, "Debug requests: enabled")
	require.Contains(t, output, "Traces: enabled (/tmp/hand-traces)")
	require.NotContains(t, output, "Ready to accept RPC connections.")
}

func TestRenderStartupPanel_IncludesSummaryModelWhenDistinct(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name:         "daemon",
		Model:        "openai/gpt-4o-mini",
		SummaryModel: "anthropic/claude-3.5-haiku",
		ModelRouter:  "openrouter",
		RPCAddress:   "127.0.0.1",
		RPCPort:      50051,
		LogLevel:     "info",
		LogNoColor:   true,
	})
	require.Contains(t, output, "Summary model: anthropic/claude-3.5-haiku")
}

func TestNewCommand_ReturnsValidationError(t *testing.T) {
	originalServeGRPC := serveRPC

	t.Cleanup(func() {
		serveRPC = originalServeGRPC
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error {
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

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func newOpenRouterModelsServer(t *testing.T, model string) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"` + model + `","context_length":128000}]}`))
	}))
	t.Cleanup(server.Close)

	return server.URL
}
