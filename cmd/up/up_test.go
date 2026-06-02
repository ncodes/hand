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
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"
	"google.golang.org/grpc"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	models "github.com/wandxy/hand/internal/model"
	modelclient "github.com/wandxy/hand/internal/model/client"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
	provider_openai "github.com/wandxy/hand/internal/model/provider_openai"
	"github.com/wandxy/hand/internal/profile"
	"github.com/wandxy/hand/pkg/logutils"
)

type modelClientFactoryStub struct {
	newClient func(modelclient.ClientRequest) (models.Client, error)
}

func (s modelClientFactoryStub) NewClient(req modelclient.ClientRequest) (models.Client, error) {
	return s.newClient(req)
}

func init() {
	logutils.SetOutput(io.Discard)
}

func TestNewCommand_BuildsConfigFromFlags(t *testing.T) {
	isolateCommandProfile(t)
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
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	runCalled := false
	serveCalled := false
	startupBuffer := &bytes.Buffer{}
	logBuffer := &bytes.Buffer{}
	startupOutput = startupBuffer
	logutils.SetOutput(logBuffer)

	newAgentRunner = func(_ context.Context, cfg *config.Config, modelClient, summaryClient, rerankerClient models.Client) agentRunner {
		return &agentstub.AgentRunnerStub{
			StartFunc: func(context.Context) error {
				runCalled = true
				return nil
			},
		}
	}

	serveRPC = func(ctx context.Context, cfg *config.Config, app agentRunner) error {
		serveCalled = true
		require.Equal(t, "0.0.0.0", cfg.RPC.Address)
		require.Equal(t, 6000, cfg.RPC.Port)
		require.NotNil(t, app)
		return nil
	}

	cmd := newRootCommandForTest(&configFile)
	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "flag-key",
		"--model.base-url", serverURL,
		"--rpc.address", "0.0.0.0",
		"--rpc.port", "6000",
		"--trace.enabled",
		"--trace.disk.dir", "/tmp/hand-traces",
		"--log.level", "debug",
		"up",
	}))

	cfg := config.Get()
	require.Equal(t, "flag-agent", cfg.Name)
	require.Equal(t, "gpt-4o-mini", cfg.Models.Main.Name)
	require.Equal(t, "openrouter", cfg.Models.Main.Provider)
	require.Equal(t, "flag-key", cfg.Models.Providers["openrouter"].APIKey)
	require.Equal(t, serverURL, cfg.Models.Main.BaseURL)
	require.Equal(t, "0.0.0.0", cfg.RPC.Address)
	require.Equal(t, 6000, cfg.RPC.Port)
	require.True(t, cfg.Trace.Enabled)
	require.Equal(t, "/tmp/hand-traces", cfg.Trace.Disk.Dir)
	require.Equal(t, "debug", cfg.Log.Level)
	require.False(t, cfg.Log.NoColor)
	require.True(t, runCalled)
	require.True(t, serveCalled)

	startupOutput := stripANSI(startupBuffer.String())
	require.Regexp(t, regexp.MustCompile(`(?m)^ +│   Version: dev \(commit unknown\)$`), startupOutput)
	require.Regexp(t, regexp.MustCompile(`(?m)^ +│   Instance: flag-agent$`), startupOutput)
	require.Regexp(t, regexp.MustCompile(`(?m)^ +█ █ █    ░██     ░██.+│   Summary provider: openrouter$`), startupOutput)
	require.Regexp(t, regexp.MustCompile(`(?m)^ +▀▀▀▀▀    ░██     ░██░░████████ ███  ░██░░██████ +│   Logs: debug \(color\)$`), startupOutput)
	require.Contains(t, startupBuffer.String(), "\x1b[38;5;38m")
	require.Contains(t, startupBuffer.String(), "\x1b[38;5;44m")
	require.Contains(t, startupBuffer.String(), "\x1b[38;5;49m")
	require.Contains(t, startupBuffer.String(), "\x1b[38;5;48m")
	require.Contains(t, startupBuffer.String(), "\x1b[38;5;83m")
	require.NotContains(t, startupBuffer.String(), "██   ██  █████  ███    ██ ██████")
	require.NotContains(t, startupBuffer.String(), handcli.AppDescription)
	require.Contains(t, startupBuffer.String(), "Version")
	require.Contains(t, startupBuffer.String(), "Instance")
	require.Contains(t, startupBuffer.String(), "flag-agent")
	require.Contains(t, startupBuffer.String(), "Summary model")
	require.Contains(t, startupBuffer.String(), "gpt-4o-mini")
	require.Contains(t, startupBuffer.String(), "Summary provider")
	require.Contains(t, startupBuffer.String(), "Storage")
	require.Contains(t, startupBuffer.String(), "sqlite")
	require.Contains(t, startupBuffer.String(), "RPC")
	require.Contains(t, startupBuffer.String(), "0.0.0.0:6000")
	require.Contains(t, startupBuffer.String(), "Streaming")
	require.Contains(t, startupBuffer.String(), "true")
	require.Contains(t, startupBuffer.String(), "Debug requests")
	require.Contains(t, startupBuffer.String(), "enabled")
	require.Contains(t, startupBuffer.String(), "Traces")
	require.Contains(t, startupBuffer.String(), "enabled (/tmp/hand-traces)")
	require.Contains(t, startupBuffer.String(), "Safety")
	require.Contains(t, startupBuffer.String(), "input=enabled, output=enabled, pii=disabled")
	require.Contains(t, startupBuffer.String(), "Reranker")

	logOutput := stripANSI(logBuffer.String())
	require.Contains(t, logOutput, "Configuration loaded")
	require.Contains(t, logOutput, "Vector retrieval configured")
	require.Contains(t, logOutput, "Starting Hand services")
	require.NotContains(t, logOutput, "name=flag-agent")
	require.NotContains(t, logOutput, "model=gpt-4o-mini")
	require.NotContains(t, logOutput, "provider=openrouter")
	require.NotContains(t, logOutput, "summaryModel=gpt-4o-mini")
	require.NotContains(t, logOutput, "summaryProvider=openrouter")
	require.NotContains(t, logOutput, "storage=sqlite")
	require.NotContains(t, logOutput, "inputSafety=true")
	require.NotContains(t, logOutput, "outputSafety=true")
	require.NotContains(t, logOutput, "piiSafety=false")
	require.NotContains(t, logOutput, "rpcEndpoint=0.0.0.0:6000")
	require.NotContains(t, logOutput, "streaming=true")
	require.NotContains(t, logOutput, "traceEnabled=true")
	require.NotContains(t, logOutput, "traceDir=/tmp/hand-traces")
	require.NotContains(t, logOutput, "embeddingModel=")
	require.NotContains(t, logOutput, "embeddingProvider=")
	require.NotContains(t, logOutput, "reranker=")
	require.NotContains(t, logOutput, "service=hand")
	require.NotContains(t, logOutput, "rpcAddress=0.0.0.0 rpcEndpoint=0.0.0.0:6000 rpcPort=6000")
}

func TestNewCommand_RestartsDaemonWhenConfigFileChanges(t *testing.T) {
	isolateCommandProfile(t)
	originalFactory := modelClientFactory
	originalRunner := newAgentRunner
	originalServe := serveRPC
	originalOutput := startupOutput
	originalDebounce := daemonConfigWatchDebounce
	events, _, restoreWatcher := stubConfigWatcher()
	t.Cleanup(func() {
		modelClientFactory = originalFactory
		newAgentRunner = originalRunner
		serveRPC = originalServe
		startupOutput = originalOutput
		daemonConfigWatchDebounce = originalDebounce
		restoreWatcher()
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeUpTestConfig(t, configPath, "first")

	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	startupOutput = io.Discard
	daemonConfigWatchDebounce = 10 * time.Millisecond

	var runnersMu sync.Mutex
	runners := make([]*agentstub.AgentRunnerStub, 0, 2)
	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client, models.Client) agentRunner {
		runner := &agentstub.AgentRunnerStub{}
		runnersMu.Lock()
		runners = append(runners, runner)
		runnersMu.Unlock()
		return runner
	}

	servedNames := make(chan string, 2)
	firstServeStarted := make(chan struct{})
	serveRPC = func(ctx context.Context, cfg *config.Config, _ agentRunner) error {
		servedNames <- cfg.Name
		if cfg.Name == "first" {
			close(firstServeStarted)
			<-ctx.Done()
		}

		return nil
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run(context.Background(), []string{"hand", "--config", configPath, "up"})
	}()

	select {
	case <-firstServeStarted:
	case <-time.After(time.Second):
		t.Fatal("first daemon run did not start")
	}

	writeUpTestConfig(t, configPath, "second")
	events <- fsnotify.Event{Name: configPath, Op: fsnotify.Write}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not restart after config change")
	}

	require.Equal(t, "first", <-servedNames)
	require.Equal(t, "second", <-servedNames)
	runnersMu.Lock()
	defer runnersMu.Unlock()
	require.Len(t, runners, 2)
	require.True(t, runners[0].Closed)
	require.True(t, runners[1].Closed)
}

func TestNewCommand_KeepsRunningWhenChangedConfigIsInvalid(t *testing.T) {
	isolateCommandProfile(t)
	originalFactory := modelClientFactory
	originalRunner := newAgentRunner
	originalServe := serveRPC
	originalOutput := startupOutput
	originalDebounce := daemonConfigWatchDebounce
	events, _, restoreWatcher := stubConfigWatcher()
	t.Cleanup(func() {
		modelClientFactory = originalFactory
		newAgentRunner = originalRunner
		serveRPC = originalServe
		startupOutput = originalOutput
		daemonConfigWatchDebounce = originalDebounce
		restoreWatcher()
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeUpTestConfig(t, configPath, "stable")

	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client, models.Client) agentRunner {
		return &agentstub.AgentRunnerStub{}
	}
	startupOutput = io.Discard
	daemonConfigWatchDebounce = 10 * time.Millisecond

	serveStarted := make(chan struct{})
	serveDone := make(chan struct{})
	serveCalls := make(chan string, 2)
	serveRPC = func(ctx context.Context, cfg *config.Config, _ agentRunner) error {
		serveCalls <- cfg.Name
		close(serveStarted)
		<-ctx.Done()
		close(serveDone)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run(ctx, []string{"hand", "--config", configPath, "up"})
	}()

	select {
	case <-serveStarted:
	case <-time.After(time.Second):
		t.Fatal("daemon did not start")
	}
	select {
	case name := <-serveCalls:
		require.Equal(t, "stable", name)
	default:
		t.Fatal("initial daemon run was not recorded")
	}

	require.NoError(t, os.WriteFile(configPath, []byte("name: ["), 0o600))
	events <- fsnotify.Event{Name: configPath, Op: fsnotify.Write}
	require.Never(t, func() bool {
		select {
		case <-serveCalls:
			return true
		default:
			return false
		}
	}, 100*time.Millisecond, 10*time.Millisecond, "invalid config restarted daemon")

	cancel()
	select {
	case <-serveDone:
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop after context cancellation")
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("command did not return after context cancellation")
	}
}

func TestRenderStartupPanel_DisablesColorWhenRequested(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name:   "daemon",
		Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter", Stream: new(false)}},
		RPC:    config.RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:    config.LogConfig{Level: "info", NoColor: true},
		Debug:  config.DebugConfig{Requests: true},
		Trace:  config.TraceConfig{Enabled: true, Disk: config.TraceDiskConfig{Dir: "/tmp/hand-traces"}},
	})

	require.NotContains(t, output, "\x1b[90m")
	require.NotContains(t, output, "\x1b[38;5;")
	require.Regexp(t, regexp.MustCompile(`(?m)^ +│   Version: dev \(commit unknown\)$`), output)
	require.Regexp(t, regexp.MustCompile(`(?m)^ +│   Instance: daemon$`), output)
	require.Regexp(t, regexp.MustCompile(`(?m)^ +█ █ █    ░██     ░██.+│   Summary model: gpt-4o-mini$`), output)
	require.Regexp(t, regexp.MustCompile(`(?m)^ +▀▀▀▀▀    ░██     ░██░░████████ ███  ░██░░██████ +│   RPC: 127.0.0.1:50051$`), output)
	require.NotContains(t, output, handcli.AppDescription)
	require.Contains(t, output, "Instance: daemon")
	require.Contains(t, output, "Summary model: gpt-4o-mini")
	require.Contains(t, output, "Summary provider: openrouter")
	require.Contains(t, output, "Storage: sqlite")
	require.Contains(t, output, "Streaming: false")
	require.Contains(t, output, "Debug requests: enabled")
	require.Contains(t, output, "Traces: enabled (/tmp/hand-traces)")
	require.Contains(t, output, "Safety: input=enabled, output=enabled, pii=disabled")
	require.NotContains(t, output, "Ready to accept RPC connections.")
}

func TestRenderStartupPanel_IncludesSafetyMode(t *testing.T) {
	inputSafety := false
	outputSafety := true
	piiSafety := true
	output := renderStartupPanel(&config.Config{
		Name:   "daemon",
		Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter"}},
		RPC:    config.RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:    config.LogConfig{Level: "info", NoColor: true},
		Safety: config.SafetyConfig{
			Input:  &inputSafety,
			Output: &outputSafety,
			PII:    &piiSafety,
		},
	})

	require.Contains(t, output, "Safety: input=disabled, output=enabled, pii=enabled")
}

func TestRenderStartupPanel_IncludesEmbeddingModelWhenVectorEnabled(t *testing.T) {
	rerankDisabled := false
	output := renderStartupPanel(&config.Config{
		Name: "daemon",
		Models: config.ModelsConfig{
			Main:      config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter"},
			Embedding: config.EmbeddingModelConfig{Name: "text-embedding-3-small", Provider: "openai"},
		},
		Search:   config.SearchConfig{Vector: config.SearchVectorConfig{Enabled: true}},
		RPC:      config.RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:      config.LogConfig{Level: "info", NoColor: true},
		Reranker: config.RerankerConfig{Enabled: &rerankDisabled},
	})

	require.Contains(t, output, "Embedding model: text-embedding-3-small")
	require.Contains(t, output, "Embedding provider: openai")
	require.Contains(t, output, "Reranker: deterministic")
}

func TestRenderStartupPanel_HidesEmbeddingModelWhenVectorDisabled(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name: "daemon",
		Models: config.ModelsConfig{
			Main:      config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter"},
			Embedding: config.EmbeddingModelConfig{Name: "text-embedding-3-small"},
		},
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: config.LogConfig{Level: "info", NoColor: true},
	})

	require.NotContains(t, output, "Embedding model")
	require.NotContains(t, output, "Embedding provider")
}

func TestRenderStartupPanel_IncludesStorageBackend(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name:    "daemon",
		Models:  config.ModelsConfig{Main: config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter"}},
		Storage: config.StorageConfig{Backend: "memory"},
		RPC:     config.RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log:     config.LogConfig{Level: "info", NoColor: true},
	})

	require.Contains(t, output, "Storage: memory")
}

func TestRenderStartupPanel_IncludesEffectiveSummaryModelAndProvider(t *testing.T) {
	output := renderStartupPanel(&config.Config{
		Name: "daemon",
		Models: config.ModelsConfig{
			Main:    config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter"},
			Summary: config.SummaryModelConfig{Name: "gpt-4o-mini", Provider: "openai"},
		},
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: config.LogConfig{Level: "info", NoColor: true},
	})

	require.Contains(t, output, "Summary model: gpt-4o-mini")
	require.Contains(t, output, "Summary provider: openai")
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

func TestRenderStartupPanel_IncludesSummaryProviderAndAPIWhenDistinct(t *testing.T) {
	cfg := &config.Config{
		Name: "daemon",
		Models: config.ModelsConfig{
			Main:    config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter", API: modelprovider.APIOpenAICompletions},
			Summary: config.SummaryModelConfig{Provider: "openai", API: modelprovider.APIOpenAIResponses},
		},
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051},
		Log: config.LogConfig{Level: "info", NoColor: true},
	}
	cfg.Normalize()

	out := renderStartupPanel(cfg)
	require.Contains(t, out, "Summary provider: openai")
	require.Contains(t, out, "Summary API: openai-responses")
}

func TestServeRPC_ReturnsListenError(t *testing.T) {
	orig := listenFunc
	t.Cleanup(func() { listenFunc = orig })

	listenFunc = func(string, string) (net.Listener, error) {
		return nil, errors.New("listen boom")
	}

	err := serveRPC(context.Background(), &config.Config{
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 50051},
	}, &agentstub.AgentRunnerStub{})

	require.EqualError(t, err, "listen boom")
}

func TestConfigFileFingerprintHandlesMissingAndChangedFiles(t *testing.T) {
	originalStat := osStat
	t.Cleanup(func() { osStat = originalStat })

	_, err := getConfigFileFingerprint(" ")
	require.EqualError(t, err, "config path is required")

	missingPath := filepath.Join(t.TempDir(), "missing.yaml")
	fingerprint, err := getConfigFileFingerprint(missingPath)
	require.NoError(t, err)
	require.Equal(t, configFileFingerprint{}, fingerprint)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeUpTestConfig(t, configPath, "first")

	fingerprint, err = getConfigFileFingerprint(configPath)
	require.NoError(t, err)
	require.NotEqual(t, configFileFingerprint{}, fingerprint)

	current, changed, err := hasConfigFileChanged(configPath, fingerprint)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, fingerprint, current)

	writeUpTestConfig(t, configPath, "second")

	current, changed, err = hasConfigFileChanged(configPath, fingerprint)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEqual(t, fingerprint, current)

	osStat = func(string) (os.FileInfo, error) {
		return nil, errors.New("stat failed")
	}
	_, err = getConfigFileFingerprint(configPath)
	require.EqualError(t, err, "stat failed")
	_, _, err = hasConfigFileChanged(configPath, fingerprint)
	require.EqualError(t, err, "stat failed")
}

func TestNewFSNotifyConfigWatcherWatchesConfigDirectory(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	watcher, err := newFSNotifyConfigWatcher(configPath)
	require.NoError(t, err)
	require.NotNil(t, watcher.events)
	require.NotNil(t, watcher.errors)
	require.NoError(t, watcher.close())

	_, err = newFSNotifyConfigWatcher(" ")
	require.EqualError(t, err, "config path is required")
}

func TestNewFSNotifyConfigWatcherReturnsSetupErrors(t *testing.T) {
	originalCreate := createFSNotifyWatcher
	originalMkdir := mkdirAllConfigWatchDir
	originalAdd := addConfigWatchDir
	t.Cleanup(func() {
		createFSNotifyWatcher = originalCreate
		mkdirAllConfigWatchDir = originalMkdir
		addConfigWatchDir = originalAdd
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	createFSNotifyWatcher = func() (*fsnotify.Watcher, error) {
		return nil, errors.New("create watcher failed")
	}
	_, err := newFSNotifyConfigWatcher(configPath)
	require.EqualError(t, err, "create watcher failed")

	createFSNotifyWatcher = originalCreate
	mkdirAllConfigWatchDir = func(string, os.FileMode) error {
		return errors.New("mkdir failed")
	}
	_, err = newFSNotifyConfigWatcher(configPath)
	require.EqualError(t, err, "mkdir failed")

	mkdirAllConfigWatchDir = originalMkdir
	addConfigWatchDir = func(*fsnotify.Watcher, string) error {
		return errors.New("add watch failed")
	}
	_, err = newFSNotifyConfigWatcher(configPath)
	require.EqualError(t, err, "add watch failed")
}

func TestIsConfigFileWatchEventFiltersPathAndOperation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	require.True(t, isConfigFileWatchEvent(fsnotify.Event{Name: configPath, Op: fsnotify.Write}, configPath))
	require.True(t, isConfigFileWatchEvent(fsnotify.Event{Name: configPath, Op: fsnotify.Create}, configPath))
	require.True(t, isConfigFileWatchEvent(fsnotify.Event{Name: configPath, Op: fsnotify.Rename}, configPath))
	require.True(t, isConfigFileWatchEvent(fsnotify.Event{Name: configPath, Op: fsnotify.Remove}, configPath))
	require.False(t, isConfigFileWatchEvent(fsnotify.Event{Name: "", Op: fsnotify.Write}, configPath))
	require.False(t, isConfigFileWatchEvent(fsnotify.Event{Name: filepath.Join(filepath.Dir(configPath), "other.yaml"), Op: fsnotify.Write}, configPath))
	require.False(t, isConfigFileWatchEvent(fsnotify.Event{Name: configPath, Op: fsnotify.Chmod}, configPath))
}

func TestRunDaemonWithConfigRestartsReturnsConfigFingerprintError(t *testing.T) {
	isolateCommandProfile(t)
	originalStat := osStat
	t.Cleanup(func() { osStat = originalStat })

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeUpTestConfig(t, configPath, "daemon")
	osStat = func(string) (os.FileInfo, error) {
		return nil, errors.New("stat failed")
	}

	err := runParsedUpCommand(t, []string{"--config", configPath, "up"}, func(ctx context.Context, cmd *cli.Command) error {
		return runDaemonWithConfigRestarts(ctx, cmd, 0)
	})

	require.EqualError(t, err, "stat failed")
}

func TestRunDaemonUntilConfigChangeReturnsWatcherSetupError(t *testing.T) {
	restore := stubDaemonStartup(t)
	defer restore()
	originalWatcher := newConfigWatcher
	t.Cleanup(func() { newConfigWatcher = originalWatcher })

	newConfigWatcher = func(string) (configWatcher, error) {
		return configWatcher{}, errors.New("watcher failed")
	}
	serveRPC = func(ctx context.Context, _ *config.Config, _ agentRunner) error {
		<-ctx.Done()
		return nil
	}

	snapshot := daemonConfigSnapshot{
		cfg:    newUpTestConfig("daemon"),
		inputs: handcli.ConfigInputs{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")},
	}
	_, restart, err := runDaemonUntilConfigChange(context.Background(), nil, snapshot, 10*time.Millisecond)

	require.False(t, restart)
	require.EqualError(t, err, "watcher failed")
}

func TestRunDaemonUntilConfigChangeKeepsDaemonErrorWhenWatcherSetupFails(t *testing.T) {
	restore := stubDaemonStartup(t)
	defer restore()
	originalWatcher := newConfigWatcher
	t.Cleanup(func() { newConfigWatcher = originalWatcher })

	newConfigWatcher = func(string) (configWatcher, error) {
		return configWatcher{}, errors.New("watcher failed")
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		return errors.New("daemon failed")
	}

	snapshot := daemonConfigSnapshot{
		cfg:    newUpTestConfig("daemon"),
		inputs: handcli.ConfigInputs{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")},
	}
	_, restart, err := runDaemonUntilConfigChange(context.Background(), nil, snapshot, 10*time.Millisecond)

	require.False(t, restart)
	require.EqualError(t, err, "daemon failed")
}

func TestRunDaemonUntilConfigChangeReturnsDaemonError(t *testing.T) {
	restore := stubDaemonStartup(t)
	defer restore()
	_, _, restoreWatcher := stubConfigWatcher()
	defer restoreWatcher()

	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		return errors.New("serve failed")
	}

	snapshot := daemonConfigSnapshot{
		cfg:    newUpTestConfig("daemon"),
		inputs: handcli.ConfigInputs{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")},
	}
	_, restart, err := runDaemonUntilConfigChange(context.Background(), nil, snapshot, 10*time.Millisecond)

	require.False(t, restart)
	require.EqualError(t, err, "serve failed")
}

func TestRunDaemonUntilConfigChangeReturnsStopErrorAfterContextCancel(t *testing.T) {
	restore := stubDaemonStartup(t)
	defer restore()
	_, _, restoreWatcher := stubConfigWatcher()
	defer restoreWatcher()

	serveStarted := make(chan struct{})
	serveRPC = func(ctx context.Context, _ *config.Config, _ agentRunner) error {
		close(serveStarted)
		<-ctx.Done()
		return errors.New("stop failed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		snapshot := daemonConfigSnapshot{
			cfg:    newUpTestConfig("daemon"),
			inputs: handcli.ConfigInputs{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")},
		}
		_, _, err := runDaemonUntilConfigChange(ctx, nil, snapshot, 10*time.Millisecond)
		done <- err
	}()

	select {
	case <-serveStarted:
	case <-time.After(time.Second):
		t.Fatal("daemon run did not start")
	}
	cancel()

	select {
	case err := <-done:
		require.EqualError(t, err, "stop failed")
	case <-time.After(time.Second):
		t.Fatal("daemon run did not stop")
	}
}

func TestRunDaemonUntilConfigChangeIgnoresConfigStatError(t *testing.T) {
	restore := stubDaemonStartup(t)
	defer restore()
	events, _, restoreWatcher := stubConfigWatcher()
	defer restoreWatcher()

	originalStat := osStat
	t.Cleanup(func() { osStat = originalStat })
	osStat = func(string) (os.FileInfo, error) {
		return nil, errors.New("stat failed")
	}

	serveStarted := make(chan struct{})
	serveRPC = func(ctx context.Context, _ *config.Config, _ agentRunner) error {
		close(serveStarted)
		<-ctx.Done()
		return nil
	}

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		snapshot := daemonConfigSnapshot{
			cfg:    newUpTestConfig("daemon"),
			inputs: handcli.ConfigInputs{ConfigPath: configPath},
		}
		_, _, err := runDaemonUntilConfigChange(ctx, nil, snapshot, 10*time.Millisecond)
		done <- err
	}()

	select {
	case <-serveStarted:
	case <-time.After(time.Second):
		t.Fatal("daemon run did not start")
	}
	events <- fsnotify.Event{Name: configPath, Op: fsnotify.Write}
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("daemon run did not stop")
	}
}

func TestRunDaemonUntilConfigChangeHandlesWatcherNoise(t *testing.T) {
	restore := stubDaemonStartup(t)
	defer restore()
	events, errorsCh, restoreWatcher := stubConfigWatcher()
	defer restoreWatcher()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeUpTestConfig(t, configPath, "daemon")
	fingerprint, err := getConfigFileFingerprint(configPath)
	require.NoError(t, err)

	serveStarted := make(chan struct{})
	serveRPC = func(ctx context.Context, _ *config.Config, _ agentRunner) error {
		close(serveStarted)
		<-ctx.Done()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		snapshot := daemonConfigSnapshot{
			cfg:         newUpTestConfig("daemon"),
			inputs:      handcli.ConfigInputs{ConfigPath: configPath},
			fingerprint: fingerprint,
		}
		_, _, err := runDaemonUntilConfigChange(ctx, nil, snapshot, 10*time.Millisecond)
		done <- err
	}()

	select {
	case <-serveStarted:
	case <-time.After(time.Second):
		t.Fatal("daemon run did not start")
	}

	errorsCh <- errors.New("watch failed")
	events <- fsnotify.Event{Name: filepath.Join(filepath.Dir(configPath), "other.yaml"), Op: fsnotify.Write}
	events <- fsnotify.Event{Name: configPath, Op: fsnotify.Write}
	close(errorsCh)
	close(events)
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("daemon run did not stop")
	}
}

func TestConfigReloadTimerResetAndStop(t *testing.T) {
	timer, reload := resetConfigReloadTimer(nil, 0)
	require.NotNil(t, timer)
	require.NotNil(t, reload)

	timer, reload = resetConfigReloadTimer(timer, time.Hour)
	require.NotNil(t, timer)
	require.NotNil(t, reload)

	timer.Reset(0)
	select {
	case <-timer.C:
	case <-time.After(time.Second):
		t.Fatal("timer did not fire")
	}

	timer, reload = resetConfigReloadTimer(timer, time.Hour)
	require.NotNil(t, timer)
	require.NotNil(t, reload)
	stopConfigReloadTimer(timer)

	expiredForReset := time.NewTimer(0)
	time.Sleep(10 * time.Millisecond)
	timer, reload = resetConfigReloadTimer(expiredForReset, time.Hour)
	require.NotNil(t, timer)
	require.NotNil(t, reload)
	stopConfigReloadTimer(timer)

	expiredForStop := time.NewTimer(0)
	time.Sleep(10 * time.Millisecond)
	stopConfigReloadTimer(expiredForStop)

	stoppedForStop := time.NewTimer(time.Hour)
	require.True(t, stoppedForStop.Stop())
	stopConfigReloadTimer(stoppedForStop)
	stopConfigReloadTimer(nil)
}

func TestRunDaemonUntilConfigChangeReturnsShutdownErrorOnRestart(t *testing.T) {
	restore := stubDaemonStartup(t)
	defer restore()
	events, _, restoreWatcher := stubConfigWatcher()
	defer restoreWatcher()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeUpTestConfig(t, configPath, "first")
	fingerprint, err := getConfigFileFingerprint(configPath)
	require.NoError(t, err)

	serveStarted := make(chan struct{})
	serveRPC = func(ctx context.Context, _ *config.Config, _ agentRunner) error {
		close(serveStarted)
		<-ctx.Done()
		return errors.New("shutdown failed")
	}

	done := make(chan error, 1)
	err = runParsedUpCommand(t, []string{"--config", configPath, "up"}, func(ctx context.Context, cmd *cli.Command) error {
		go func() {
			snapshot := daemonConfigSnapshot{
				cfg:         newUpTestConfig("first"),
				inputs:      handcli.ConfigInputs{ConfigPath: configPath},
				fingerprint: fingerprint,
			}
			_, _, err := runDaemonUntilConfigChange(ctx, cmd, snapshot, 10*time.Millisecond)
			done <- err
		}()

		select {
		case <-serveStarted:
		case <-time.After(time.Second):
			t.Fatal("daemon run did not start")
		}
		writeUpTestConfig(t, configPath, "second")
		events <- fsnotify.Event{Name: configPath, Op: fsnotify.Write}

		select {
		case err := <-done:
			return err
		case <-time.After(time.Second):
			t.Fatal("daemon run did not stop")
			return nil
		}
	})

	require.EqualError(t, err, "shutdown failed")
}

func TestRunDaemonOnceClosesAgentAndReturnsCloseError(t *testing.T) {
	isolateCommandProfile(t)
	originalFactory := modelClientFactory
	originalRunner := newAgentRunner
	originalServe := serveRPC
	originalOutput := startupOutput
	t.Cleanup(func() {
		modelClientFactory = originalFactory
		newAgentRunner = originalRunner
		serveRPC = originalServe
		startupOutput = originalOutput
	})

	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	startupOutput = io.Discard

	runner := &agentstub.AgentRunnerStub{
		AgentServiceStub: agentstub.AgentServiceStub{CloseErr: errors.New("close failed")},
	}
	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client, models.Client) agentRunner {
		return runner
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		return nil
	}

	cfg := newUpTestConfig("daemon")
	err := runDaemonOnce(context.Background(), cfg)

	require.EqualError(t, err, "close failed")
	require.True(t, runner.Closed)
}

func TestRunDaemonOnceKeepsServeErrorWhenCloseAlsoFails(t *testing.T) {
	isolateCommandProfile(t)
	originalFactory := modelClientFactory
	originalRunner := newAgentRunner
	originalServe := serveRPC
	originalOutput := startupOutput
	t.Cleanup(func() {
		modelClientFactory = originalFactory
		newAgentRunner = originalRunner
		serveRPC = originalServe
		startupOutput = originalOutput
	})

	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	startupOutput = io.Discard

	runner := &agentstub.AgentRunnerStub{
		AgentServiceStub: agentstub.AgentServiceStub{CloseErr: errors.New("close failed")},
	}
	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client, models.Client) agentRunner {
		return runner
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		return errors.New("serve failed")
	}

	cfg := newUpTestConfig("daemon")
	err := runDaemonOnce(context.Background(), cfg)

	require.EqualError(t, err, "serve failed")
	require.True(t, runner.Closed)
}

func TestRunDaemonOnceReturnsSummaryClientFactoryError(t *testing.T) {
	isolateCommandProfile(t)
	t.Setenv("OPENAI_API_KEY", "openai-key")
	originalFactory := modelClientFactory
	originalServe := serveRPC
	originalOutput := startupOutput
	t.Cleanup(func() {
		modelClientFactory = originalFactory
		serveRPC = originalServe
		startupOutput = originalOutput
	})

	var calls int
	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("summary client failed")
			}
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		t.Fatal("serveRPC should not run")
		return nil
	}
	startupOutput = io.Discard

	cfg := newUpTestConfig("daemon")
	cfg.Models.Summary.Provider = "openai"
	cfg.Models.Summary.BaseURL = "https://openai.example/v1"
	cfg.Normalize()

	err := runDaemonOnce(context.Background(), cfg)

	require.EqualError(t, err, "summary client failed")
	require.Equal(t, 2, calls)
}

func TestRunDaemonOnceReturnsRerankerClientFactoryError(t *testing.T) {
	isolateCommandProfile(t)
	originalFactory := modelClientFactory
	originalResolve := resolveRerankerAuth
	originalServe := serveRPC
	originalOutput := startupOutput
	t.Cleanup(func() {
		modelClientFactory = originalFactory
		resolveRerankerAuth = originalResolve
		serveRPC = originalServe
		startupOutput = originalOutput
	})

	var calls int
	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("reranker client failed")
			}
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	resolveRerankerAuth = func(*config.Config) (config.ModelAuth, error) {
		return config.ModelAuth{Provider: "anthropic", API: modelprovider.APIAnthropicMessages, APIKey: "reranker-key"}, nil
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		t.Fatal("serveRPC should not run")
		return nil
	}
	startupOutput = io.Discard

	cfg := newUpTestConfig("daemon")
	cfg.Search.Vector.Enabled = true
	cfg.Reranker.Type = constants.RerankerLLM
	cfg.Normalize()

	err := runDaemonOnce(context.Background(), cfg)

	require.EqualError(t, err, "reranker client failed")
	require.Equal(t, 2, calls)
}

func TestRunDaemonOnceReturnsRerankerAuthError(t *testing.T) {
	isolateCommandProfile(t)
	originalFactory := modelClientFactory
	originalResolve := resolveRerankerAuth
	originalServe := serveRPC
	originalOutput := startupOutput
	t.Cleanup(func() {
		modelClientFactory = originalFactory
		resolveRerankerAuth = originalResolve
		serveRPC = originalServe
		startupOutput = originalOutput
	})

	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	resolveRerankerAuth = func(*config.Config) (config.ModelAuth, error) {
		return config.ModelAuth{}, errors.New("reranker auth failed")
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error {
		t.Fatal("serveRPC should not run")
		return nil
	}
	startupOutput = io.Discard

	cfg := newUpTestConfig("daemon")
	cfg.Search.Vector.Enabled = true
	cfg.Reranker.Type = constants.RerankerLLM
	cfg.Normalize()

	err := runDaemonOnce(context.Background(), cfg)

	require.EqualError(t, err, "reranker auth failed")
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
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 0},
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
		RPC: config.RPCConfig{Address: "127.0.0.1", Port: 0},
	}, &agentstub.AgentRunnerStub{})

	require.NoError(t, err)
}

func TestServeRPC_WritesRuntimeMetadataWithActualPort(t *testing.T) {
	origListen := listenFunc
	origServe := grpcServerServe
	originalProfile := profile.Active()
	t.Cleanup(func() {
		listenFunc = origListen
		grpcServerServe = origServe
		profile.SetActive(originalProfile)
	})

	home := t.TempDir()
	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: profileHome}))
	listenFunc = net.Listen
	grpcServerServe = func(*grpc.Server, net.Listener) error {
		return grpc.ErrServerStopped
	}

	cfg := &config.Config{RPC: config.RPCConfig{Address: "127.0.0.1", Port: 0}}
	err := serveRPC(context.Background(), cfg, &agentstub.AgentRunnerStub{})

	require.NoError(t, err)
	require.Greater(t, cfg.RPC.Port, 0)
	data, err := os.ReadFile(filepath.Join(profileHome, "runtime.json"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"profile": "work"`)
	require.Contains(t, string(data), `"address": "127.0.0.1"`)
	require.Contains(t, string(data), `"port": `)
}

func TestNewAgentRunnerImpl_ReturnsAgent(t *testing.T) {
	cfg := &config.Config{
		Name:   "t",
		Models: config.ModelsConfig{Providers: map[string]config.ProviderModelConfig{"openrouter": {APIKey: "k"}}, Main: config.MainModelConfig{Name: "gpt-4o-mini", Provider: "openrouter"}},
	}
	cfg.Normalize()

	mc, err := provider_openai.NewOpenAIClient("k", models.APIOpenAIResponses)
	require.NoError(t, err)
	sc, err := provider_openai.NewOpenAIClient("k", models.APIOpenAIResponses)
	require.NoError(t, err)

	r := newAgentRunnerImpl(context.Background(), cfg, mc, sc, sc)
	require.NotNil(t, r)
}

func TestServeRPC_StopsWhenContextCancelled(t *testing.T) {
	orig := listenFunc
	t.Cleanup(func() { listenFunc = orig })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveRPC(ctx, &config.Config{
			RPC: config.RPCConfig{Address: "127.0.0.1", Port: 0},
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
			RPC: config.RPCConfig{Address: "127.0.0.1", Port: 0},
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
			RPC: config.RPCConfig{Address: "127.0.0.1", Port: 0},
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
	isolateCommandProfile(t)
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
	isolateCommandProfile(t)
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
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "k",
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

func TestNewCommand_ReturnsModelClientFactoryError(t *testing.T) {
	isolateCommandProfile(t)
	origFactory := modelClientFactory
	origServe := serveRPC
	t.Cleanup(func() {
		modelClientFactory = origFactory
		serveRPC = origServe
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }
	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			return nil, errors.New("model factory boom")
		},
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "flag-key",
		"--model.base-url", serverURL,
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "model factory boom")
}

func TestNewCommand_ReturnsResolveSummaryAuthError(t *testing.T) {
	isolateCommandProfile(t)
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
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "flag-key",
		"--model.base-url", serverURL,
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "summary auth boom")
}

func TestNewCommand_ReturnsSecondModelClientFactoryError(t *testing.T) {
	isolateCommandProfile(t)
	t.Setenv("OPENAI_API_KEY", "openai-key")
	origFactory := modelClientFactory
	origServe := serveRPC
	t.Cleanup(func() {
		modelClientFactory = origFactory
		serveRPC = origServe
	})

	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }

	var n int
	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			n++
			if n == 1 {
				return &provider_openai.OpenAIClient{}, nil
			}
			return nil, errors.New("summary client boom")
		},
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "flag-key",
		"--model.base-url", serverURL,
		"--model.summary-provider", "openai",
		"--model.summary-base-url", "https://api.openai.com/v1",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "summary client boom")
}

func TestNewCommand_PassesResolvedAuthToModelClientFactory(t *testing.T) {
	isolateCommandProfile(t)
	t.Setenv("OPENAI_API_KEY", "openai-key")
	origFactory := modelClientFactory
	origRunner := newAgentRunner
	origServe := serveRPC
	origStartupOutput := startupOutput
	t.Cleanup(func() {
		modelClientFactory = origFactory
		newAgentRunner = origRunner
		serveRPC = origServe
		startupOutput = origStartupOutput
	})

	var calls []modelclient.ClientRequest
	modelClientFactory = modelClientFactoryStub{
		newClient: func(req modelclient.ClientRequest) (models.Client, error) {
			calls = append(calls, req)
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client, models.Client) agentRunner {
		return &agentstub.AgentRunnerStub{}
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }
	startupOutput = io.Discard

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "router-key",
		"--model.base-url", serverURL,
		"--model.summary-provider", "openai",
		"--model.summary-base-url", "https://openai.example/v1",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	}))

	require.Equal(t, []modelclient.ClientRequest{
		{
			Role:       modelclient.ModelRoleMain,
			Model:      "gpt-4o-mini",
			Provider:   "openrouter",
			API:        modelprovider.APIOpenAIResponses,
			APIKey:     "router-key",
			BaseURL:    serverURL,
			MaxRetries: constants.DefaultModelMaxRetries,
		},
		{
			Role:       modelclient.ModelRoleSummary,
			Model:      "gpt-4o-mini",
			Provider:   "openai",
			API:        modelprovider.APIOpenAIResponses,
			APIKey:     "openai-key",
			BaseURL:    "https://openai.example/v1",
			MaxRetries: constants.DefaultModelMaxRetries,
		},
	}, calls)
}

func TestNewCommand_PassesSeparateRerankerClientWhenAuthDiffers(t *testing.T) {
	isolateCommandProfile(t)
	t.Setenv("HAND_RERANKER_TYPE", constants.RerankerLLM)
	origFactory := modelClientFactory
	origRunner := newAgentRunner
	origServe := serveRPC
	origStartupOutput := startupOutput
	origResolveRerankerAuth := resolveRerankerAuth
	t.Cleanup(func() {
		modelClientFactory = origFactory
		newAgentRunner = origRunner
		serveRPC = origServe
		startupOutput = origStartupOutput
		resolveRerankerAuth = origResolveRerankerAuth
	})

	rerankerAuth := config.ModelAuth{
		Provider: "anthropic",
		API:      modelprovider.APIAnthropicMessages,
		APIKey:   "reranker-key",
		BaseURL:  "https://anthropic.example",
	}
	resolveRerankerAuth = func(*config.Config) (config.ModelAuth, error) {
		return rerankerAuth, nil
	}

	var calls []modelclient.ClientRequest
	clients := make([]models.Client, 0, 2)
	modelClientFactory = modelClientFactoryStub{
		newClient: func(req modelclient.ClientRequest) (models.Client, error) {
			calls = append(calls, req)
			client := &provider_openai.OpenAIClient{}
			clients = append(clients, client)
			return client, nil
		},
	}
	newAgentRunner = func(_ context.Context, _ *config.Config, modelClient, summaryClient, rerankerClient models.Client) agentRunner {
		require.Same(t, modelClient, summaryClient)
		require.NotSame(t, modelClient, rerankerClient)
		return &agentstub.AgentRunnerStub{}
	}
	serveRPC = func(context.Context, *config.Config, agentRunner) error { return nil }
	startupOutput = io.Discard

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	require.NoError(t, cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "router-key",
		"--model.base-url", serverURL,
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	}))

	require.Len(t, clients, 2)
	require.Equal(t, []modelclient.ClientRequest{
		{
			Role:       modelclient.ModelRoleMain,
			Model:      "gpt-4o-mini",
			Provider:   "openrouter",
			API:        modelprovider.APIOpenAIResponses,
			APIKey:     "router-key",
			BaseURL:    serverURL,
			MaxRetries: constants.DefaultModelMaxRetries,
		},
		{
			Role:       modelclient.ModelRoleReranker,
			Model:      "gpt-4o-mini",
			Provider:   "anthropic",
			API:        modelprovider.APIAnthropicMessages,
			APIKey:     "reranker-key",
			BaseURL:    "https://anthropic.example",
			MaxRetries: constants.DefaultModelMaxRetries,
		},
	}, calls)
}

func TestNewCommand_ReturnsAgentStartError(t *testing.T) {
	isolateCommandProfile(t)
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

	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client, models.Client) agentRunner {
		return &agentstub.AgentRunnerStub{
			StartFunc: func(context.Context) error {
				return errors.New("start failed")
			},
		}
	}

	configFile := ""
	cmd := newRootCommandForTest(&configFile)
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "flag-key",
		"--model.base-url", serverURL,
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	})
	require.EqualError(t, err, "start failed")
}

func TestNewCommand_UsesSeparateSummaryClientWhenAuthDiffers(t *testing.T) {
	isolateCommandProfile(t)
	t.Setenv("OPENAI_API_KEY", "openai-key")
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
	serverURL := newOpenRouterModelsServer(t, "gpt-4o-mini")
	runCalled := false
	serveCalled := false
	startupOutput = io.Discard
	logutils.SetOutput(io.Discard)

	newAgentRunner = func(_ context.Context, cfg *config.Config, modelClient, summaryClient, rerankerClient models.Client) agentRunner {
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
		"--model", "gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.api-key", "flag-key",
		"--model.base-url", serverURL,
		"--model.summary-provider", "openai",
		"--model.summary-base-url", "https://api.openai.com/v1",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"up",
	}))

	require.True(t, runCalled)
	require.True(t, serveCalled)
}

func TestNewCommand_ReturnsValidationError(t *testing.T) {
	isolateCommandProfile(t)
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
		"--model.api-key", "",
		"up",
	})

	require.ErrorContains(t, err, "hand auth login openrouter")
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

func runParsedUpCommand(t *testing.T, args []string, action func(context.Context, *cli.Command) error) error {
	t.Helper()

	configFile := ""
	root := &cli.Command{
		Name:  "hand",
		Flags: handcli.RootFlags(nil, &configFile),
		Commands: []*cli.Command{
			{
				Name:   "up",
				Action: action,
			},
		},
	}

	return root.Run(context.Background(), append([]string{"hand"}, args...))
}

func isolateCommandProfile(t *testing.T) {
	t.Helper()

	originalProfile := profile.Active()
	t.Cleanup(func() {
		profile.SetActive(originalProfile)
	})
	profile.SetActive(profile.Profile{})
	t.Setenv("HOME", t.TempDir())
	clearEnv(
		t,
		profile.EnvName,
		"HAND_ENV_FILE",
		"HAND_CONFIG",
		"HAND_NAME",
		"HAND_MODEL",
		"HAND_MODEL_PROVIDER",
		"HAND_MODEL_API",
		"HAND_MODEL_API_KEY",
		"HAND_MODEL_BASE_URL",
		"HAND_MODEL_MAX_RETRIES",
		"HAND_MODEL_SUMMARY",
		"HAND_MODEL_SUMMARY_PROVIDER",
		"HAND_MODEL_SUMMARY_API",
		"HAND_MODEL_SUMMARY_API_KEY",
		"HAND_MODEL_SUMMARY_BASE_URL",
		"HAND_RPC_ADDRESS",
		"HAND_RPC_PORT",
		"HAND_LOG_LEVEL",
		"HAND_LOG_NO_COLOR",
		"HAND_DEBUG_REQUESTS",
		"HAND_SAFETY_INPUT",
		"HAND_SAFETY_OUTPUT",
		"HAND_SAFETY_PII",
	)
}

func stubDaemonStartup(t *testing.T) func() {
	t.Helper()

	isolateCommandProfile(t)
	originalFactory := modelClientFactory
	originalRunner := newAgentRunner
	originalServe := serveRPC
	originalOutput := startupOutput

	modelClientFactory = modelClientFactoryStub{
		newClient: func(modelclient.ClientRequest) (models.Client, error) {
			return &provider_openai.OpenAIClient{}, nil
		},
	}
	newAgentRunner = func(context.Context, *config.Config, models.Client, models.Client, models.Client) agentRunner {
		return &agentstub.AgentRunnerStub{}
	}
	startupOutput = io.Discard

	return func() {
		modelClientFactory = originalFactory
		newAgentRunner = originalRunner
		serveRPC = originalServe
		startupOutput = originalOutput
	}
}

func stubConfigWatcher() (chan fsnotify.Event, chan error, func()) {
	originalWatcher := newConfigWatcher
	events := make(chan fsnotify.Event, 16)
	errors := make(chan error, 16)
	newConfigWatcher = func(string) (configWatcher, error) {
		return configWatcher{
			events: events,
			errors: errors,
			close:  func() error { return nil },
		}, nil
	}

	return events, errors, func() {
		newConfigWatcher = originalWatcher
	}
}

func newUpTestConfig(name string) *config.Config {
	cfg := config.NewDefaultConfig()
	cfg.Name = name
	cfg.Models.Main.Name = "gpt-4o-mini"
	cfg.Models.Main.Provider = "openrouter"
	cfg.Models.Providers = map[string]config.ProviderModelConfig{
		"openrouter": {APIKey: "test-key"},
	}
	cfg.RPC.Address = "127.0.0.1"
	cfg.RPC.Port = 50051
	cfg.Log.NoColor = true
	cfg.Normalize()

	return cfg
}

func writeUpTestConfig(t *testing.T, path string, name string) {
	t.Helper()

	data, err := newUpTestConfig(name).ToYAML()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	keys = append(keys, "OPENAI_API_KEY", "OPENROUTER_API_KEY", "ANTHROPIC_API_KEY", "COPILOT_GITHUB_TOKEN")

	for _, key := range keys {
		original, ok := os.LookupEnv(key)
		if ok {
			t.Cleanup(func() {
				require.NoError(t, os.Setenv(key, original))
			})
		} else {
			t.Cleanup(func() {
				require.NoError(t, os.Unsetenv(key))
			})
		}
		require.NoError(t, os.Unsetenv(key))
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
