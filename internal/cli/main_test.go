package cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	urfavecli "github.com/urfave/cli/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wandxy/hand/internal/config"
	agentstub "github.com/wandxy/hand/internal/mocks/agentstub"
	"github.com/wandxy/hand/internal/profile"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/runtime"
	"github.com/wandxy/hand/internal/trace"
	agent "github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/logutils"
)

func TestNewMainAction_TreatsUnknownArgsAsChat(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(&output, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"hand",
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
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_LOG_LEVEL",
		"HAND_CONFIG", "HAND_ENV_FILE", "HAND_PROFILE")
	resetMainActionState(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: profile-agent
models:
  verify: false
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, ".env"), []byte("HAND_LOG_LEVEL=debug\n"), 0o600))

	var got *config.Config
	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(_ context.Context, cfg *config.Config) (rpcclient.ChatClient, error) {
		got = cfg
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{"hand", "--profile", "Work", "hello"})

	require.NoError(t, err)
	require.Equal(t, "hello", stub.ChatInput)
	require.NotNil(t, got)
	require.Equal(t, "profile-agent", got.Name)
	require.Equal(t, "debug", got.Log.Level)
}

func TestNewMainAction_UsesProfileRuntimeEndpoint(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_LOG_LEVEL",
		"HAND_CONFIG", "HAND_ENV_FILE", "HAND_PROFILE", "HAND_RPC_ADDRESS", "HAND_RPC_PORT")
	resetMainActionState(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, listener.Close())
	})

	profileHome := filepath.Join(home, ".hand", "profiles", "work")
	require.NoError(t, os.MkdirAll(profileHome, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(profileHome, "config.yaml"), []byte(`
name: profile-agent
models:
  verify: false
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

	err = cmd.Run(context.Background(), []string{"hand", "--profile", "Work", "hello"})

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "127.0.0.1", got.RPC.Address)
	require.Equal(t, port, got.RPC.Port)
}

func TestNewMainAction_ForwardsInstruct(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetMainActionState(t)

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--instruct", "be terse",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "be terse", stub.RespondOptions.Instruct)
}

func TestNewMainAction_ForwardsSessionID(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetMainActionState(t)

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--session", "project-a",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "project-a", stub.RespondOptions.SessionID)
}

func TestNewMainAction_StreamsOutput(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
	resetMainActionState(t)

	var output bytes.Buffer
	stub := &agentstub.AgentServiceStub{Reply: "hello back", Deltas: []string{"hello ", "back"}}
	cmd := newMainActionTestCommand(&output, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"hand",
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
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
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
		"hand",
		"--name", "flag-agent",
		"--model.stream=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "\x1b[90mthinking\x1b[0m done\n", output.String())
}

func TestNewMainAction_DoesNotStyleReasoningWhenNoColor(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
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
		"hand",
		"--name", "flag-agent",
		"--model.stream=true",
		"--log.no-color=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "thinking done\n", output.String())
}

func TestNewMainAction_IgnoresTraceEvents(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
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
		"hand",
		"--name", "flag-agent",
		"--model.stream=true",
		"hello",
	})
	require.NoError(t, err)
	require.Equal(t, "hello back\n", output.String())
}

func TestNewMainAction_DoesNotForwardConfiguredInstruct(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_MODEL", "HAND_MODEL_PROVIDER", "OPENROUTER_API_KEY", "HAND_MODEL_BASE_URL",
		"HAND_LOG_LEVEL", "HAND_LOG_NO_COLOR", "HAND_CONFIG", "HAND_ENV_FILE")
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
    name: openai/gpt-4o-mini
    provider: openrouter
session:
  instruct: be terse
`), 0o600))

	stub := &agentstub.AgentServiceStub{Reply: "hello back"}
	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return stub, nil
	})

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--config", configPath,
		"hello",
	})
	require.NoError(t, err)
	require.Empty(t, stub.RespondOptions.Instruct)
}

func TestNewMainAction_ReturnsRPCError(t *testing.T) {
	clearEnv(t, "HAND_NAME", "HAND_CONFIG", "HAND_ENV_FILE")
	resetMainActionState(t)

	cmd := newMainActionTestCommand(io.Discard, func(context.Context, *config.Config) (rpcclient.ChatClient, error) {
		return nil, status.Error(codes.Unavailable, "connection refused")
	})

	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"hello",
	})
	require.EqualError(t, err, "rpc error: code = Unavailable desc = connection refused")
}

func newMainActionTestCommand(output io.Writer, newChatClient NewChatClientFunc) *urfavecli.Command {
	envFile := ".env"
	configFile := "config.yaml"

	return &urfavecli.Command{
		Name:  "hand",
		Flags: append(RootFlags(&envFile, &configFile), RequestInstructFlag()),
		Action: NewMainAction(MainActionOptions{
			Output:        output,
			NewChatClient: newChatClient,
		}),
	}
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
