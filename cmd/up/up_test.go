package up

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"
	"google.golang.org/grpc"

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

	newAgentRunner = func(_ context.Context, cfg *config.Config, modelClient, summaryClient models.Client) agentRunner {
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
		"--model.provider", "openrouter",
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
	require.Equal(t, "openrouter", cfg.ModelProvider)
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
	require.Contains(t, startupBuffer.String(), "Streaming")
	require.Contains(t, startupBuffer.String(), "true")
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
	require.Contains(t, logOutput, "provider=openrouter")
	require.Contains(t, logOutput, "rpcEndpoint=0.0.0.0:6000")
	require.Contains(t, logOutput, "streaming=true")
	require.Contains(t, logOutput, "debugTraces=true")
	require.Contains(t, logOutput, "debugTraceDir=/tmp/hand-traces")
	require.NotContains(t, logOutput, "service=hand")
	require.NotContains(t, logOutput, "rpcAddress=0.0.0.0 rpcEndpoint=0.0.0.0:6000 rpcPort=6000")
}

func TestRenderStartupPanel_DisablesColorWhenRequested(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name:          "daemon",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openrouter",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
		LogNoColor:    true,
		Stream:        new(false),
		DebugRequests: true,
		DebugTraces:   true,
		DebugTraceDir: "/tmp/hand-traces",
	})

	require.NotContains(t, output, "\x1b[90m")
	require.Contains(t, output, "Instance: daemon")
	require.Contains(t, output, "Streaming: false")
	require.Contains(t, output, "Debug requests: enabled")
	require.Contains(t, output, "Traces: enabled (/tmp/hand-traces)")
	require.NotContains(t, output, "Ready to accept RPC connections.")
}

func TestRenderStartupPanel_IncludesSummaryModelWhenDistinct(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name:          "daemon",
		Model:         "openai/gpt-4o-mini",
		SummaryModel:  "anthropic/claude-3.5-haiku",
		ModelProvider: "openrouter",
		RPCAddress:    "127.0.0.1",
		RPCPort:       50051,
		LogLevel:      "info",
		LogNoColor:    true,
	})
	require.Contains(t, output, "Summary model: anthropic/claude-3.5-haiku")
}

func TestSetOutput_SwitchesWriterAndRestoresPrevious(t *testing.T) {
	stdout := SetOutput(nil)
	require.Equal(t, io.Discard, startupOutput)
	t.Cleanup(func() { SetOutput(stdout) })

	buf := &bytes.Buffer{}
	prev := SetOutput(buf)
	require.Equal(t, io.Discard, prev)
	require.Equal(t, buf, startupOutput)

	restored := SetOutput(stdout)
	require.Equal(t, buf, restored)
	require.Equal(t, stdout, startupOutput)
}

func TestRenderStartupPanel_NilConfigReturnsBadgeOnly(t *testing.T) {
	out := renderStartupPanel(nil)
	require.Equal(t, handBadge, out)
}

func TestRenderStartupPanel_IncludesSummaryProviderAndAPIModeWhenDistinct(t *testing.T) {
	cfg := &config.Config{
		Name:                "daemon",
		Model:               "openai/gpt-4o-mini",
		ModelProvider:       "openrouter",
		ModelAPIMode:        config.DefaultModelAPIMode,
		SummaryProvider:     "openai",
		SummaryModelAPIMode: "responses",
		RPCAddress:          "127.0.0.1",
		RPCPort:             50051,
		LogLevel:            "info",
		LogNoColor:          true,
	}
	cfg.Normalize()

	out := renderStartupPanel(cfg)
	require.Contains(t, out, "Summary provider: openai")
	require.Contains(t, out, "Summary API mode: responses")
}

func TestServeRPC_ReturnsListenError(t *testing.T) {
	orig := listenFunc
	t.Cleanup(func() { listenFunc = orig })

	listenFunc = func(string, string) (net.Listener, error) {
		return nil, errors.New("listen boom")
	}

	err := serveRPC(context.Background(), &config.Config{
		RPCAddress: "127.0.0.1",
		RPCPort:    50051,
	}, &agentstub.AgentRunnerStub{})

	require.EqualError(t, err, "listen boom")
}

func TestServeRPC_ReturnsWhenGRPCServeFails(t *testing.T) {
	origListen := listenFunc
	origServe := grpcServerServe
	t.Cleanup(func() {
		listenFunc = origListen
		grpcServerServe = origServe
	})

	listenFunc = net.Listen
	grpcServerServe = func(*grpc.Server, net.Listener) error {
		return errors.New("serve boom")
	}

	err := serveRPC(context.Background(), &config.Config{
		RPCAddress: "127.0.0.1",
		RPCPort:    0,
	}, &agentstub.AgentRunnerStub{})

	require.EqualError(t, err, "serve boom")
}

func TestServeRPC_ReturnsNilWhenGRPCServeReturnsServerStopped(t *testing.T) {
	origListen := listenFunc
	origServe := grpcServerServe
	t.Cleanup(func() {
		listenFunc = origListen
		grpcServerServe = origServe
	})

	listenFunc = net.Listen
	grpcServerServe = func(*grpc.Server, net.Listener) error {
		return grpc.ErrServerStopped
	}

	err := serveRPC(context.Background(), &config.Config{
		RPCAddress: "127.0.0.1",
		RPCPort:    0,
	}, &agentstub.AgentRunnerStub{})

	require.NoError(t, err)
}

func TestNewAgentRunnerImpl_ReturnsAgent(t *testing.T) {
	cfg := &config.Config{
		Name:          "t",
		Model:         "openai/gpt-4o-mini",
		ModelProvider: "openrouter",
		ModelKey:      "k",
	}
	cfg.Normalize()

	mc, err := models.NewOpenAIClient("k")
	require.NoError(t, err)
	sc, err := models.NewOpenAIClient("k")
	require.NoError(t, err)

	r := newAgentRunnerImpl(context.Background(), cfg, mc, sc)
	require.NotNil(t, r)
}

func TestServeRPC_StopsWhenContextCancelled(t *testing.T) {
	orig := listenFunc
	t.Cleanup(func() { listenFunc = orig })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveRPC(ctx, &config.Config{
			RPCAddress: "127.0.0.1",
			RPCPort:    0,
		}, &agentstub.AgentRunnerStub{})
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("serveRPC did not return after context cancel")
	}
}

func TestServeRPC_ReturnsPostShutdownServeError(t *testing.T) {
	origListen := listenFunc
	origServe := grpcServerServe
	origPost := postShutdownServeErrHook
	t.Cleanup(func() {
		listenFunc = origListen
		grpcServerServe = origServe
		postShutdownServeErrHook = origPost
	})

	listenFunc = net.Listen
	grpcServerServe = func(srv *grpc.Server, lis net.Listener) error {
		return srv.Serve(lis)
	}
	postShutdownServeErrHook = func(error) error {
		return errors.New("post shutdown")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveRPC(ctx, &config.Config{
			RPCAddress: "127.0.0.1",
			RPCPort:    0,
		}, &agentstub.AgentRunnerStub{})
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.EqualError(t, err, "post shutdown")
	case <-time.After(15 * time.Second):
		t.Fatal("serveRPC did not return")
	}
}

func TestServeRPC_ForcesStopWhenGracefulShutdownSlow(t *testing.T) {
	origListen := listenFunc
	origServe := grpcServerServe
	origTimeout := serveRPCShutdownTimeout
	origGraceful := grpcGracefulStop
	t.Cleanup(func() {
		listenFunc = origListen
		grpcServerServe = origServe
		serveRPCShutdownTimeout = origTimeout
		grpcGracefulStop = origGraceful
	})

	listenFunc = net.Listen
	grpcServerServe = func(srv *grpc.Server, lis net.Listener) error {
		return srv.Serve(lis)
	}
	serveRPCShutdownTimeout = 10 * time.Millisecond
	grpcGracefulStop = func(srv *grpc.Server) {
		time.Sleep(200 * time.Millisecond)
		srv.GracefulStop()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveRPC(ctx, &config.Config{
			RPCAddress: "127.0.0.1",
			RPCPort:    0,
		}, &agentstub.AgentRunnerStub{})
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("serveRPC did not return")
	}
}

func TestNewCommand_ReturnsConfigLoadError(t *testing.T) {
	origServe := serveRPC
	t.Cleanup(func() { serveRPC = origServe })
	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		t.Fatal("serveRPC should not run")
		return nil
	}

	badPath := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(badPath, []byte(":\ninvalid"), 0o600))

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	err := cmd.Run(context.Background(), []string{"hand", "--config", badPath, "up"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse config file")
}

type startupWriteFailAlways struct{}

func (startupWriteFailAlways) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type startupWriteFailAfterFirst struct {
	n int
}

func (w *startupWriteFailAfterFirst) Write(p []byte) (int, error) {
	w.n++
	if w.n == 1 {
		return len(p), nil
	}
	return 0, errors.New("write failed")
}

func TestNewCommand_ReturnsStartupOutputError(t *testing.T) {
	origOut := startupOutput
	origServe := serveRPC
	t.Cleanup(func() {
		startupOutput = origOut
		serveRPC = origServe
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	args := []string{
		"hand",
		"--name", "x",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "k",
		"--model.verify-model", "false",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	}

	startupOutput = startupWriteFailAlways{}
	err := cmd.Run(context.Background(), args)
	require.Error(t, err)

	startupOutput = &startupWriteFailAfterFirst{}
	err = cmd.Run(context.Background(), args)
	require.Error(t, err)
}

func TestNewCommand_ReturnsOpenAIClientFactoryError(t *testing.T) {
	origFactory := openAIClientFactory
	origServe := serveRPC
	t.Cleanup(func() {
		openAIClientFactory = origFactory
		serveRPC = origServe
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }
	openAIClientFactory = func(string, ...option.RequestOption) (*models.OpenAIClient, error) {
		return nil, errors.New("openai factory boom")
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", serverURL,
		"--model.verify-model", "false",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "openai factory boom")
}

func TestNewCommand_ReturnsResolveSummaryAuthError(t *testing.T) {
	origResolve := resolveSummaryAuth
	origServe := serveRPC
	t.Cleanup(func() {
		resolveSummaryAuth = origResolve
		serveRPC = origServe
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }
	resolveSummaryAuth = func(*config.Config) (config.ModelAuth, error) {
		return config.ModelAuth{}, errors.New("summary auth boom")
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", serverURL,
		"--model.verify-model", "false",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "summary auth boom")
}

func TestNewCommand_ReturnsSecondOpenAIClientFactoryError(t *testing.T) {
	origFactory := openAIClientFactory
	origServe := serveRPC
	t.Cleanup(func() {
		openAIClientFactory = origFactory
		serveRPC = origServe
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }

	var n int
	openAIClientFactory = func(key string, opts ...option.RequestOption) (*models.OpenAIClient, error) {
		n++
		if n == 1 {
			return models.NewOpenAIClient(key, opts...)
		}
		return nil, errors.New("summary client boom")
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", serverURL,
		"--model.summary-provider", "openai",
		"--model.summary-base-url", "https://api.openai.com/v1",
		"--model.verify-model", "false",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "summary client boom")
}

func TestNewCommand_ReturnsAgentStartError(t *testing.T) {
	origRunner := newAgentRunner
	origServe := serveRPC
	t.Cleanup(func() {
		newAgentRunner = origRunner
		serveRPC = origServe
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		t.Fatal("serveRPC should not run when Start fails")
		return nil
	}

	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client) agentRunner {
		return &agentstub.AgentRunnerStub{
			StartFunc: func(context.Context) error {
				return errors.New("start failed")
			},
		}
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "openai/gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", serverURL,
		"--model.verify-model", "false",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "start failed")
}

func TestNewCommand_UsesSeparateSummaryClientWhenAuthDiffers(t *testing.T) {
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
	startupOutput = io.Discard
	logutils.SetOutput(io.Discard)

	newAgentRunner = func(_ context.Context, cfg *config.Config, modelClient, summaryClient models.Client) agentRunner {
		require.NotSame(t, modelClient, summaryClient)
		return &agentstub.AgentRunnerStub{
			StartFunc: func(context.Context) error {
				runCalled = true
				return nil
			},
		}
	}

	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		serveCalled = true
		return nil
	}

	cmd := newRootCommandForTest(&configFile)
	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--model.base-url", serverURL,
		"--model.summary-provider", "openai",
		"--model.summary-base-url", "https://api.openai.com/v1",
		"--model.verify-model", "false",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	}))

	require.True(t, runCalled)
	require.True(t, serveCalled)
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
		"--model.provider", "openrouter",
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
