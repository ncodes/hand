package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/e2e"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

func Test_E2E_HandRootChat_SimpleAnswer(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCRootChatHarness(t, e2e.NewTextClient("hello back"))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "hello", "world")
	require.NoError(t, err)
	assert.Equal(t, "hello back\n", output)

	messages, err := h.Messages(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, handmsg.RoleUser, messages[0].Role)
	assert.Equal(t, "hello world", messages[0].Content)
	assert.Equal(t, handmsg.RoleAssistant, messages[1].Role)
	assert.Equal(t, "hello back", messages[1].Content)
}

func Test_E2E_HandRootChat_StreamingAnswer(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCRootChatHarness(t, e2e.NewClient(e2e.Step{
		Response: &models.Response{OutputText: "answer"},
		Stream: []models.StreamDelta{
			{Channel: models.StreamChannelReasoning, Text: "thinking"},
			{Channel: models.StreamChannelAssistant, Text: "answer"},
		},
	}))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{
		Name:   "yaml-agent",
		Stream: true,
	})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "hello")
	require.NoError(t, err)
	assert.Equal(t, "\x1b[90mthinking\x1b[0manswer\n", output)
	require.True(t, config.Get().StreamEnabled())
}

func Test_E2E_HandRootChat_ExplicitSession(t *testing.T) {
	resetRootChatE2E(t)

	sessionID := "ses_123456789012345678901"
	h := newRPCRootChatHarness(t, e2e.NewTextClient("session reply"))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{Name: "yaml-agent"})
	client, err := h.Client(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	_, err = client.CreateSession(context.Background(), sessionID)
	require.NoError(t, err)

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "--session", sessionID, "hello")
	require.NoError(t, err)
	assert.Equal(t, "session reply\n", output)

	messages, err := h.Messages(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, "hello", messages[0].Content)
	assert.Equal(t, "session reply", messages[1].Content)
}

func Test_E2E_HandRootChat_RequestInstruct(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCRootChatHarness(t, e2e.NewClient(e2e.Step{
		Check: func(req models.Request) error {
			if !strings.Contains(req.Instructions, "be brief") {
				return errors.New("request instruct missing from model instructions")
			}
			return nil
		},
		Response: &models.Response{OutputText: "brief"},
	}))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "--instruct", "be brief", "hello")
	require.NoError(t, err)
	assert.Equal(t, "brief\n", output)
	assert.Equal(t, "be brief", config.Get().Instruct)
}

func Test_E2E_HandRootChat_MultiTurnContinuity(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCRootChatHarness(t, e2e.NewClient(
		e2e.OutputTextStep("first reply"),
		e2e.Step{
			Check: func(req models.Request) error {
				if len(req.Messages) != 3 {
					return fmt.Errorf("expected 3 messages in follow-up request, got %d", len(req.Messages))
				}
				if req.Messages[0].Role != handmsg.RoleUser || req.Messages[0].Content != "first turn" {
					return errors.New("missing first user message in follow-up request")
				}
				if req.Messages[1].Role != handmsg.RoleAssistant || req.Messages[1].Content != "first reply" {
					return errors.New("missing first assistant reply in follow-up request")
				}
				if req.Messages[2].Role != handmsg.RoleUser || req.Messages[2].Content != "second turn" {
					return errors.New("missing second user message in follow-up request")
				}
				return nil
			},
			Response: &models.Response{OutputText: "second reply"},
		},
	))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{Name: "yaml-agent"})

	firstOutput, err := runRootChatCommand(t, "hand", "--config", configPath, "first", "turn")
	require.NoError(t, err)
	assert.Equal(t, "first reply\n", firstOutput)

	secondOutput, err := runRootChatCommand(t, "hand", "--config", configPath, "second", "turn")
	require.NoError(t, err)
	assert.Equal(t, "second reply\n", secondOutput)

	messages, err := h.Messages(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, messages, 4)
	assert.Equal(t, []handmsg.Role{
		handmsg.RoleUser,
		handmsg.RoleAssistant,
		handmsg.RoleUser,
		handmsg.RoleAssistant,
	}, []handmsg.Role{messages[0].Role, messages[1].Role, messages[2].Role, messages[3].Role})
	assert.Equal(t, "first turn", messages[0].Content)
	assert.Equal(t, "first reply", messages[1].Content)
	assert.Equal(t, "second turn", messages[2].Content)
	assert.Equal(t, "second reply", messages[3].Content)
}

func Test_E2E_HandRootChat_ConfigPrecedenceYAML(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCRootChatHarness(t, e2e.NewTextClient("yaml reply"))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "hello")
	require.NoError(t, err)
	assert.Equal(t, "yaml reply\n", output)
	assert.Equal(t, "yaml-agent", config.Get().Name)
}

func Test_E2E_HandRootChat_ConfigPrecedenceEnvOverridesYAML(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCRootChatHarness(t, e2e.NewTextClient("env reply"))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{Name: "yaml-agent"})
	envPath := writeRootChatEnv(t, "NAME=env-agent\n")

	output, err := runRootChatCommand(t, "hand", "--env-file", envPath, "--config", configPath, "hello")
	require.NoError(t, err)
	assert.Equal(t, "env reply\n", output)
	assert.Equal(t, "env-agent", config.Get().Name)
}

func Test_E2E_HandRootChat_ConfigPrecedenceCLIOverridesEnvAndYAML(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCRootChatHarness(t, e2e.NewTextClient("cli reply"))
	configPath := writeRootChatConfig(t, h, rootChatConfigOptions{Name: "yaml-agent"})
	envPath := writeRootChatEnv(t, "NAME=env-agent\n")

	output, err := runRootChatCommand(
		t,
		"hand",
		"--env-file", envPath,
		"--config", configPath,
		"--name", "cli-agent",
		"hello",
	)
	require.NoError(t, err)
	assert.Equal(t, "cli reply\n", output)
	assert.Equal(t, "cli-agent", config.Get().Name)
}

type rootChatConfigOptions struct {
	Name     string
	Stream   bool
	Instruct string
	NoColor  bool
}

func resetRootChatE2E(t *testing.T) {
	t.Helper()
	clearEnvKeys(
		t,
		"NAME",
		"MODEL_STREAM",
		"INSTRUCT",
		"LOG_NO_COLOR",
		"RPC_ADDRESS",
		"RPC_PORT",
		"AGENT_CONFIG",
		"AGENT_ENV_FILE",
	)
	resetGlobals(t)
}

func newRPCRootChatHarness(t *testing.T, client models.Client) *e2e.RPCHarness {
	t.Helper()

	h, err := e2e.NewRPCHarness(context.Background(), e2e.HarnessOptions{
		Spec:        e2e.DefaultSpec(filepath.Join(t.TempDir(), "hand-home")),
		Config:      e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"}),
		ModelClient: client,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})

	return h
}

func writeRootChatConfig(t *testing.T, h *e2e.RPCHarness, opts rootChatConfigOptions) string {
	t.Helper()

	if opts.Name == "" {
		opts.Name = "yaml-agent"
	}

	content := fmt.Sprintf(
		`name: %s
model:
  verifyModel: false
  stream: %t
rpc:
  address: %s
  port: %d
log:
  noColor: %t
`,
		opts.Name,
		opts.Stream,
		h.Address(),
		h.Port(),
		opts.NoColor,
	)
	if strings.TrimSpace(opts.Instruct) != "" {
		content += "instruct: " + strings.TrimSpace(opts.Instruct) + "\n"
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func writeRootChatEnv(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func runRootChatCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var output bytes.Buffer
	rootOutput = &output

	err := newCommand().Run(context.Background(), args)
	return output.String(), err
}
