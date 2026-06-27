package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	urfavecli "github.com/urfave/cli/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	agentstub "github.com/wandxy/morph/internal/mocks/agentstub"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	provider_ollama "github.com/wandxy/morph/internal/model/provider_ollama"
	"github.com/wandxy/morph/internal/profile"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/runtime"
	"github.com/wandxy/morph/internal/trace"
	agent "github.com/wandxy/morph/pkg/agent"
	"github.com/wandxy/morph/pkg/logutils"
)

func TestNewMainAction_TreatsUnknownArgsAsChat(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(&output, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"--rpc.address", "127.0.0.1",
		"--rpc.port", "50051",
		"hello",
		"world",
	})
	require.NoError(t, err)
	require.Equal(t, "hello world", stub.ChatInput)
	require.Empty(t, stub.RespondOptions.Instruct)
	require.Empty(t, stub.RespondOptions.SessionID)
	require.True(t, stub.Closed)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewMainAction_UsesProfileConfigAndEnv(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_LOG_LEVEL",
		"MORPH_CONFIG", "MORPH_ENV_FILE", "MORPH_PROFILE")
	resetMainActionState(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	profileHome := filepath.Join(home, ".morph", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: profile-agent
models:
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), []byte("MORPH_LOG_LEVEL=debug\n"), 0o600))

	var got *config.Config
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(_ context.Context, cfg *config.Config) (rpcclient.ChatClient, error) {
		got = cfg
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{"morph", "--profile", "Work", "hello"})

	require.NoError(t, err)
	require.Equal(t, "hello", stub.ChatInput)
	require.NotNil(t, got)
	require.Equal(t, "profile-agent", got.Name)
	require.Equal(t, "debug", got.Log.Level)
}

func TestNewMainAction_UsesProfileRuntimeEndpoint(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_LOG_LEVEL",
		"MORPH_CONFIG", "MORPH_ENV_FILE", "MORPH_PROFILE", "MORPH_RPC_ADDRESS", "MORPH_RPC_PORT")
	resetMainActionState(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, listener.Close())
	})

	profileHome := filepath.Join(home, ".morph", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: profile-agent
models:
`), 0o600))
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: profileHome}))
	port := listener.Addr().(*net.TCPAddr).Port
	_, err = runtime.WriteActive("127.0.0.1", port)
	require.NoError(t, err)

	var got *config.Config
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(_ context.Context, cfg *config.Config) (rpcclient.ChatClient, error) {
		got = cfg
		return stub, nil
	})

	err = cmd.Run(context.Background(), []string{"morph", "--profile", "Work", "hello"})

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "127.0.0.1", got.RPC.Address)
	require.Equal(t, port, got.RPC.Port)
}

func TestNewMainAction_ForwardsInstruct(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"--instruct", "be terse",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "be terse", stub.RespondOptions.Instruct)
}

func TestNewMainAction_ForwardsSessionID(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"--session", "project-a",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "project-a", stub.RespondOptions.SessionID)
}

func TestNewMainAction_StreamsOutput(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{Reply: "hello back", Deltas: []string{"hello ", "back"}}
	cmd := newMainActionTestCommand(&output, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"--model.stream=true",
		"hello",
	})
	require.NoError(t, err)
	require.NotNil(t, stub.RespondOptions.Stream)
	require.True(t, *stub.RespondOptions.Stream)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewMainAction_StylesReasoningOutput(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{
		Reply: "thinking done",
		Events: []rpcclient.Event{
			{Kind: agent.EventKindTextDelta, Channel: "reasoning", Text: "thinking"},
			{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: " done"},
		},
	}
	cmd := newMainActionTestCommand(&output, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"--model.stream=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "\x1b[90mthinking\x1b[0m done\n", output.String())
}

func TestNewMainAction_DoesNotStyleReasoningWhenNoColor(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{
		Reply: "thinking done",
		Events: []rpcclient.Event{
			{Kind: agent.EventKindTextDelta, Channel: "reasoning", Text: "thinking"},
			{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: " done"},
		},
	}
	cmd := newMainActionTestCommand(&output, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"--model.stream=true",
		"--log.no-color=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "thinking done\n", output.String())
}

func TestNewMainAction_IgnoresTraceEvents(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	traceEvent := trace.Event{Type: trace.EvtInputSafetyBlocked, Payload: map[string]any{"blocked": true}}
	stub := &agentstub.AgentServiceStub{
		Reply: "hello back",
		Events: []rpcclient.Event{
			{Kind: agent.EventKindTrace, TraceEvent: &traceEvent},
			{Kind: agent.EventKindTextDelta, Channel: "assistant", Text: "hello back"},
		},
	}
	cmd := newMainActionTestCommand(&output, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"--model.stream=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewMainAction_DoesNotForwardConfiguredInstruct(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "OPENROUTER_API_KEY", "MORPH_MODEL_BASE_URL",
		"MORPH_LOG_LEVEL", "MORPH_LOG_NO_COLOR", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  providers:
    openrouter:
      apiKey: config-key
  main:
    name: gpt-4o-mini
    provider: openrouter
session:
  instruct: be terse
`), 0o600))

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--config", configPath,
		"hello",
	})
	require.NoError(t, err)
	require.Empty(t, stub.RespondOptions.Instruct)
}

func TestNewMainAction_EnsuresDaemonAndCleansStartedRuntime(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	started := false
	cleaned := false
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommandWithOptions(io.Discard, MainActionOptions{
		NewChatClient: func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
			require.True(t, started)
			require.False(t, cleaned)
			return stub, nil
		},
		EnsureDaemonRunning: func(context.Context, *config.Config) (func() error, error) {
			started = true
			return func() error {
				cleaned = true
				return nil
			}, nil
		},
	})

	err := cmd.Run(context.Background(), []string{"morph", "--name", "flag-agent", "hello"})

	require.NoError(t, err)
	require.True(t, cleaned)
	require.True(t, stub.Closed)
}

func TestNewMainAction_PullsOllamaModelBeforeStartingDaemon(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "MORPH_MODEL_BASE_URL",
		"MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	pulled := false
	started := false
	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommandWithOptions(&output, MainActionOptions{
		PullOllamaModel: func(
			_ context.Context,
			baseURL string,
			model string,
			headers map[string]string,
			onProgress func(provider_ollama.PullProgress),
		) error {
			require.False(t, started)
			require.Equal(t, constants.DefaultOllamaBaseURL, baseURL)
			require.Equal(t, "qwen3:8b", model)
			require.Nil(t, headers)
			require.NotNil(t, onProgress)
			onProgress(provider_ollama.PullProgress{Status: "pulling manifest"})
			onProgress(provider_ollama.PullProgress{Status: "downloading", Completed: 25, Total: 100})
			pulled = true
			return nil
		},
		EnsureDaemonRunning: func(context.Context, *config.Config) (func() error, error) {
			require.True(t, pulled)
			started = true
			return nil, nil
		},
		NewChatClient: func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
			require.True(t, started)
			return stub, nil
		},
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--provider", "ollama",
		"--model", "qwen3:8b",
		"--pull",
		"hello",
	})

	require.NoError(t, err)
	require.True(t, pulled)
	require.Equal(t, "hello", stub.ChatInput)
	require.Equal(t, "Ollama pull: pulling manifest\nOllama pull: downloading 25%\nhello back\n", output.String())
}

func TestNewMainAction_PullQuietSuppressesProgressOutput(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "MORPH_MODEL_BASE_URL",
		"MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommandWithOptions(&output, MainActionOptions{
		PullOllamaModel: func(
			_ context.Context,
			_ string,
			_ string,
			_ map[string]string,
			onProgress func(provider_ollama.PullProgress),
		) error {
			require.Nil(t, onProgress)
			return nil
		},
		NewChatClient: func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
			return stub, nil
		},
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--provider", "ollama",
		"--model", "qwen3:8b",
		"--pull",
		"--pull-quiet",
		"hello",
	})

	require.NoError(t, err)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewMainAction_PullRejectsNonOllamaProvider(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_MODEL", "MORPH_MODEL_PROVIDER", "MORPH_MODEL_BASE_URL",
		"MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	cmd := newMainActionTestCommandWithOptions(io.Discard, MainActionOptions{
		PullOllamaModel: func(
			context.Context,
			string,
			string,
			map[string]string,
			func(provider_ollama.PullProgress),
		) error {
			t.Fatal("pull should not run for non-Ollama providers")
			return nil
		},
		EnsureDaemonRunning: func(context.Context, *config.Config) (func() error, error) {
			t.Fatal("daemon should not start when --pull is invalid")
			return nil, nil
		},
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--provider", "openrouter",
		"--pull",
		"hello",
	})

	require.EqualError(t, err, `--pull is only supported with provider "ollama"`)
}

func TestPullSelectedOllamaModel_RejectsInvalidInputs(t *testing.T) {
	err := pullSelectedOllamaModel(
		t.Context(),
		nil,
		func(context.Context, string, string, map[string]string, func(provider_ollama.PullProgress)) error {
			t.Fatal("pull should not run without config")
			return nil
		},
		nil,
	)
	require.EqualError(t, err, "config is required")

	err = pullSelectedOllamaModel(t.Context(), &config.Config{}, nil, nil)
	require.EqualError(t, err, "ollama puller is required")

	cfg := config.NewDefaultConfig()
	cfg.Models.Main.Provider = constants.ModelProviderOllama
	cfg.Models.Main.API = modelprovider.APIOpenAIResponses

	err = pullSelectedOllamaModel(
		t.Context(),
		cfg,
		func(context.Context, string, string, map[string]string, func(provider_ollama.PullProgress)) error {
			t.Fatal("pull should not run for unsupported Ollama API")
			return nil
		},
		nil,
	)
	require.EqualError(t, err, "--pull is only supported with Ollama chat APIs")

	profileHome := t.TempDir()
	profile.SetActive(profile.Profile{HomeDir: profileHome})
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte("{bad json"), 0o600))

	cfg.Models.Main.API = modelprovider.APIOllamaNative
	err = pullSelectedOllamaModel(
		t.Context(),
		cfg,
		func(context.Context, string, string, map[string]string, func(provider_ollama.PullProgress)) error {
			t.Fatal("pull should not run when model auth cannot be resolved")
			return nil
		},
		nil,
	)
	require.ErrorContains(t, err, "parse credential store")
}

func TestPullProgressWriter_IgnoresDisabledAndEmptyProgress(t *testing.T) {
	require.Nil(t, pullProgressWriter(io.Discard, false))
	require.Nil(t, pullProgressWriter(nil, true))

	var output bytes.Buffer
	onProgress := pullProgressWriter(&output, true)
	require.NotNil(t, onProgress)

	onProgress(provider_ollama.PullProgress{Status: " "})

	require.Empty(t, output.String())
}

func TestNewMainAction_ReturnsDaemonStartErrorBeforeCreatingClient(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	expectedErr := errors.New("daemon unavailable")
	cmd := newMainActionTestCommandWithOptions(io.Discard, MainActionOptions{
		NewChatClient: func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
			t.Fatal("chat client should not be created when daemon startup fails")
			return nil, nil
		},
		EnsureDaemonRunning: func(context.Context, *config.Config) (func() error, error) {
			return nil, expectedErr
		},
	})

	err := cmd.Run(context.Background(), []string{"morph", "--name", "flag-agent", "hello"})

	require.ErrorIs(t, err, expectedErr)
}

func TestNewMainAction_ReturnsRPCError(t *testing.T) {
	clearEnv(t, "MORPH_NAME", "MORPH_CONFIG", "MORPH_ENV_FILE")
	resetMainActionState(t)

	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return nil, status.Error(codes.Unavailable, "connection refused")
	})

	err := cmd.Run(context.Background(), []string{
		"morph",
		"--name", "flag-agent",
		"hello",
	})
	require.EqualError(t, err, "rpc error: code = Unavailable desc = connection refused")
}

func newMainActionTestCommand(output io.Writer, newChatClient NewChatClientFunc) *urfavecli.Command {
	return newMainActionTestCommandWithOptions(output, MainActionOptions{
		NewChatClient:       newChatClient,
		EnsureDaemonRunning: noopEnsureDaemonRunning,
	})
}

func newMainActionTestCommandWithOptions(output io.Writer, opts MainActionOptions) *urfavecli.Command {
	envFile := ".env"
	configFile := "config.yaml"
	opts.Output = output
	if opts.EnsureDaemonRunning == nil {
		opts.EnsureDaemonRunning = noopEnsureDaemonRunning
	}

	return &urfavecli.Command{
		Name:   "morph",
		Flags:  append(RootFlags(&envFile, &configFile), RequestInstructFlag()),
		Action: NewMainAction(opts),
	}
}

func noopEnsureDaemonRunning(context.Context, *config.Config) (func() error, error) {
	return nil, nil
}

func resetMainActionState(t *testing.T) {
	t.Helper()

	originalConfig := config.Get()
	originalProfile := profile.Active()
	t.Cleanup(func() {
		config.Set(originalConfig)
		profile.SetActive(originalProfile)
	})

	logutils.SetOutput(io.Discard)
	config.Set(nil)
	t.Setenv("HOME", t.TempDir())
	profile.SetActive(profile.Profile{})
}
