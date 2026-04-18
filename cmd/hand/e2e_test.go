package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/e2e"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

func Test_E2E_HandRootChat_SimpleAnswer(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewTextClient("hello back"), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

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

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(e2e.Step{
		Response: &models.Response{OutputText: "answer"},
		Stream: []models.StreamDelta{
			{Channel: models.StreamChannelReasoning, Text: "thinking"},
			{Channel: models.StreamChannelAssistant, Text: "answer"},
		},
	}), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{
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
	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewTextClient("session reply"), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})
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

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(e2e.Step{
		Check: func(req models.Request) error {
			if !strings.Contains(req.Instructions, "be brief") {
				return errors.New("request instruct missing from model instructions")
			}
			return nil
		},
		Response: &models.Response{OutputText: "brief"},
	}), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "--instruct", "be brief", "hello")
	require.NoError(t, err)
	assert.Equal(t, "brief\n", output)
	assert.Equal(t, "be brief", config.Get().Instruct)
}

func Test_E2E_HandRootChat_MultiTurnContinuity(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(
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
	), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

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

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewTextClient("yaml reply"), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "hello")
	require.NoError(t, err)
	assert.Equal(t, "yaml reply\n", output)
	assert.Equal(t, "yaml-agent", config.Get().Name)
}

func Test_E2E_HandRootChat_ConfigPrecedenceEnvOverridesYAML(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewTextClient("env reply"), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})
	envPath := writeRootChatEnv(t, "NAME=env-agent\n")

	output, err := runRootChatCommand(t, "hand", "--env-file", envPath, "--config", configPath, "hello")
	require.NoError(t, err)
	assert.Equal(t, "env reply\n", output)
	assert.Equal(t, "env-agent", config.Get().Name)
}

func Test_E2E_HandRootChat_ConfigPrecedenceCLIOverridesEnvAndYAML(t *testing.T) {
	resetRootChatE2E(t)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewTextClient("cli reply"), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})
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

func Test_E2E_HandRootChat_UnavailableRPCReturnsError(t *testing.T) {
	resetRootChatE2E(t)

	port, err := strconv.Atoi(nextTestPort(t))
	require.NoError(t, err)

	configPath := writeRPCConfig(t, "127.0.0.1", port, e2e.RPCConfigOptions{
		Name: "yaml-agent",
	})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "hello")
	require.Error(t, err)
	assert.Empty(t, output)
	assert.Contains(t, err.Error(), "connection refused")
}

func Test_E2E_HandStartup_InvalidConfigBlocksStartup(t *testing.T) {
	resetRootChatE2E(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
model:
  name: openai/gpt-4o-mini
  provider: anthropic
  key: config-key
`), 0o600))

	err := newCommand().Run(canceledContext(), []string{
		"hand",
		"--config", configPath,
		"--rpc.port", nextTestPort(t),
		"up",
	})
	require.EqualError(t, err, "model provider must be one of: openai, openrouter")
}

func Test_E2E_HandRootChat_FileGuardrailFailureReturnsCoherentAnswer(t *testing.T) {
	resetRootChatE2E(t)

	outsidePath := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outsidePath, []byte("secret"), 0o600))

	client := e2e.NewClient(
		e2e.ToolStep(models.ToolCall{
			ID:    "call-1",
			Name:  "read_file",
			Input: fmt.Sprintf(`{"path":%q}`, outsidePath),
		}),
		e2e.Step{
			Check: e2e.ToolError("call-1", "read_file", "path_outside_roots", "path is outside allowed roots"),
			Response: &models.Response{
				OutputText: "I can't read that path because it is outside the allowed workspace.",
			},
		},
	)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), client, e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"}))
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "read the file outside the workspace")
	require.NoError(t, err)
	assert.Equal(t, "I can't read that path because it is outside the allowed workspace.\n", output)
}

func Test_E2E_HandRootChat_CommandDeniedReturnsCoherentAnswer(t *testing.T) {
	resetRootChatE2E(t)

	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.ExecDeny = []string{"git push"}

	client := e2e.NewClient(
		e2e.ToolStep(models.ToolCall{
			ID:    "call-1",
			Name:  "run_command",
			Input: `{"command":"git","args":["push","origin","main"]}`,
		}),
		e2e.Step{
			Check: e2e.ToolError("call-1", "run_command", "command_denied", "matched deny rule"),
			Response: &models.Response{
				OutputText: "I can't run that command because command execution policy denies it.",
			},
		},
	)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), client, cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "push the current branch")
	require.NoError(t, err)
	assert.Equal(t, "I can't run that command because command execution policy denies it.\n", output)
}

func Test_E2E_HandRootChat_CommandApprovalRequiredReturnsCoherentAnswer(t *testing.T) {
	resetRootChatE2E(t)

	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.ExecAsk = []string{"git push"}

	client := e2e.NewClient(
		e2e.ToolStep(models.ToolCall{
			ID:    "call-1",
			Name:  "run_command",
			Input: `{"command":"git","args":["push","origin","main"]}`,
		}),
		e2e.Step{
			Check: e2e.ToolError("call-1", "run_command", "approval_required", "command requires approval: git push"),
			Response: &models.Response{
				OutputText: "That command requires approval before I can run it.",
			},
		},
	)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), client, cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "push the current branch")
	require.NoError(t, err)
	assert.Equal(t, "That command requires approval before I can run it.\n", output)
}

func Test_E2E_HandRootChat_TimeToolSynthesizesFinalAnswer(t *testing.T) {
	resetRootChatE2E(t)

	toolCall := models.ToolCall{ID: "call-1", Name: "time", Input: "{}"}
	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(
		e2e.ToolStep(toolCall),
		e2e.Step{
			Check: e2e.CombineChecks(
				e2e.AssertToolRoundTrip(toolCall),
				e2e.ToolOutputString("call-1", "time", func(value string) error {
					_, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
					if err != nil {
						return fmt.Errorf("expected RFC3339 time output: %w", err)
					}
					return nil
				}),
			),
			Response: &models.Response{OutputText: "The current time has been retrieved."},
		},
	), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "what time is it?")
	require.NoError(t, err)
	assert.Equal(t, "The current time has been retrieved.\n", output)
}

func Test_E2E_HandRootChat_ReadFileToolSynthesizesFinalAnswer(t *testing.T) {
	resetRootChatE2E(t)

	home := filepath.Join(t.TempDir(), "hand-home")
	workspace := filepath.Join(home, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("hello from file"), 0o600))
	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.FSRoots = []string{workspace}

	toolCall := models.ToolCall{ID: "call-1", Name: "read_file", Input: `{"path":"notes.txt"}`}
	h := newRPCHarness(t, home, e2e.NewClient(
		e2e.ToolStep(toolCall),
		e2e.Step{
			Check: e2e.CombineChecks(
				e2e.AssertToolRoundTrip(toolCall),
				e2e.ToolOutputJSON("call-1", "read_file", func(payload map[string]any) error {
					if strings.TrimSpace(fmt.Sprint(payload["path"])) != "notes.txt" {
						return fmt.Errorf("expected read path notes.txt, got %v", payload["path"])
					}
					if !strings.Contains(fmt.Sprint(payload["content"]), "hello from file") {
						return fmt.Errorf("expected file content in tool output, got %v", payload["content"])
					}
					return nil
				}),
			),
			Response: &models.Response{OutputText: "The file says hello from file."},
		},
	), cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "read notes.txt and summarize it")
	require.NoError(t, err)
	assert.Equal(t, "The file says hello from file.\n", output)
}

func Test_E2E_HandRootChat_WriteFileToolSynthesizesFinalAnswer(t *testing.T) {
	resetRootChatE2E(t)

	home := filepath.Join(t.TempDir(), "hand-home")
	workspace := filepath.Join(home, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.FSRoots = []string{workspace}

	toolCall := models.ToolCall{
		ID:    "call-1",
		Name:  "write_file",
		Input: `{"path":"drafts/out.txt","content":"written by tool","create_dirs":true}`,
	}
	h := newRPCHarness(t, home, e2e.NewClient(
		e2e.ToolStep(toolCall),
		e2e.Step{
			Check: e2e.CombineChecks(
				e2e.AssertToolRoundTrip(toolCall),
				e2e.ToolOutputJSON("call-1", "write_file", func(payload map[string]any) error {
					if strings.TrimSpace(fmt.Sprint(payload["path"])) != "drafts/out.txt" {
						return fmt.Errorf("expected write path drafts/out.txt, got %v", payload["path"])
					}
					if fmt.Sprint(payload["created"]) != "true" {
						return fmt.Errorf("expected created=true, got %v", payload["created"])
					}
					raw, err := os.ReadFile(filepath.Join(workspace, "drafts", "out.txt"))
					if err != nil {
						return err
					}
					if string(raw) != "written by tool" {
						return fmt.Errorf("expected written file content, got %q", string(raw))
					}
					return nil
				}),
			),
			Response: &models.Response{OutputText: "I wrote the requested file."},
		},
	), cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "write drafts/out.txt with the requested content")
	require.NoError(t, err)
	assert.Equal(t, "I wrote the requested file.\n", output)
}

func Test_E2E_HandRootChat_RunCommandToolSynthesizesFinalAnswer(t *testing.T) {
	resetRootChatE2E(t)

	home := filepath.Join(t.TempDir(), "hand-home")
	workspace := filepath.Join(home, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.FSRoots = []string{workspace}

	toolCall := models.ToolCall{
		ID:    "call-1",
		Name:  "run_command",
		Input: `{"command":"pwd"}`,
	}
	h := newRPCHarness(t, home, e2e.NewClient(
		e2e.ToolStep(toolCall),
		e2e.Step{
			Check: e2e.CombineChecks(
				e2e.AssertToolRoundTrip(toolCall),
				e2e.ToolOutputJSON("call-1", "run_command", func(payload map[string]any) error {
					if fmt.Sprint(payload["exit_code"]) != "0" {
						return fmt.Errorf("expected exit_code=0, got %v", payload["exit_code"])
					}
					expected := filepath.Join(home, "workspace")
					if !strings.Contains(fmt.Sprint(payload["stdout"]), expected) {
						return fmt.Errorf("expected stdout to contain %q, got %v", expected, payload["stdout"])
					}
					return nil
				}),
			),
			Response: &models.Response{OutputText: "The command ran successfully in the workspace."},
		},
	), cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "run pwd and tell me where you are")
	require.NoError(t, err)
	assert.Equal(t, "The command ran successfully in the workspace.\n", output)
}

func Test_E2E_HandRootChat_PlanToolPersistsAcrossLaterTurn(t *testing.T) {
	resetRootChatE2E(t)

	toolCall := models.ToolCall{
		ID:   "call-1",
		Name: "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Inspect the bug","status":"in_progress"},` +
			`{"id":"step-2","content":"Write the fix","status":"pending"}],` +
			`"explanation":"track the current work"}`,
	}

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(
		e2e.ToolStep(toolCall),
		e2e.Step{
			Check: e2e.CombineChecks(
				e2e.AssertToolRoundTrip(toolCall),
				e2e.ToolOutputJSON("call-1", "plan_tool", func(payload map[string]any) error {
					steps, ok := payload["steps"].([]any)
					if !ok || len(steps) != 2 {
						return fmt.Errorf("expected two plan steps, got %v", payload["steps"])
					}
					if !strings.Contains(fmt.Sprint(steps[0]), "Inspect the bug") {
						return fmt.Errorf("expected persisted plan content, got %v", steps[0])
					}
					return nil
				}),
			),
			Response: &models.Response{OutputText: "Plan saved."},
		},
		e2e.Step{
			Check: func(req models.Request) error {
				if !strings.Contains(req.Instructions, "# Plan Context") {
					return errors.New("expected hydrated plan context in instructions")
				}
				if !strings.Contains(req.Instructions, "- [in_progress] Inspect the bug") {
					return errors.New("expected active plan step in later turn instructions")
				}
				if !strings.Contains(req.Instructions, "- [pending] Write the fix") {
					return errors.New("expected pending plan step in later turn instructions")
				}
				if !strings.Contains(req.Instructions, "track the current work") {
					return errors.New("expected plan explanation in later turn instructions")
				}
				return nil
			},
			Response: &models.Response{OutputText: "I still have the saved plan in context."},
		},
	), nil)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	firstOutput, err := runRootChatCommand(t, "hand", "--config", configPath, "create a plan for this task")
	require.NoError(t, err)
	assert.Equal(t, "Plan saved.\n", firstOutput)

	secondOutput, err := runRootChatCommand(t, "hand", "--config", configPath, "what is the current plan?")
	require.NoError(t, err)
	assert.Equal(t, "I still have the saved plan in context.\n", secondOutput)
}

func Test_E2E_HandRootChat_DisabledFilesystemCapabilityOmitsFileTools(t *testing.T) {
	resetRootChatE2E(t)

	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.CapFilesystem = new(false)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(e2e.Step{
		Check: e2e.MissingTools("list_files", "read_file", "search_files", "write_file", "patch"),
		Response: &models.Response{
			OutputText: "Filesystem access is unavailable in this run.",
		},
	}), cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "list the workspace files")
	require.NoError(t, err)
	assert.Equal(t, "Filesystem access is unavailable in this run.\n", output)
}

func Test_E2E_HandRootChat_DisabledExecCapabilityOmitsExecTools(t *testing.T) {
	resetRootChatE2E(t)

	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.CapExec = new(false)

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(e2e.Step{
		Check: e2e.MissingTools("run_command", "process"),
		Response: &models.Response{
			OutputText: "Command execution is unavailable in this run.",
		},
	}), cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "run git status")
	require.NoError(t, err)
	assert.Equal(t, "Command execution is unavailable in this run.\n", output)
}

func Test_E2E_HandRootChat_DisabledNetworkCapabilityOmitsWebTools(t *testing.T) {
	resetRootChatE2E(t)

	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.CapNetwork = new(false)
	cfg.WebProvider = "firecrawl"
	cfg.WebAPIKey = "test-key"

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(e2e.Step{
		Check: e2e.MissingTools("web_search", "web_extract"),
		Response: &models.Response{
			OutputText: "Network-backed web tools are unavailable in this run.",
		},
	}), cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "search the web for recent news")
	require.NoError(t, err)
	assert.Equal(t, "Network-backed web tools are unavailable in this run.\n", output)
}

func Test_E2E_HandRootChat_IterationBudgetExhaustionFallsBackCoherently(t *testing.T) {
	resetRootChatE2E(t)

	cfg := e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	cfg.MaxIterations = 1

	h := newRPCHarness(t, filepath.Join(t.TempDir(), "hand-home"), e2e.NewClient(
		e2e.ToolStep(models.ToolCall{ID: "call-1", Name: "time", Input: "{}"}),
		e2e.Step{
			Check: func(req models.Request) error {
				if len(req.Tools) != 0 {
					return fmt.Errorf("expected summary fallback request without tools, got %d", len(req.Tools))
				}
				if !strings.Contains(req.Instructions, "Remaining iteration budget: 0.") {
					return errors.New("expected summary fallback instructions")
				}
				return e2e.ToolMessagePresent("call-1", "time")(req)
			},
			Response: &models.Response{
				OutputText: "I hit the iteration limit before finishing, so I am returning a summary instead.",
			},
		},
	), cfg)
	configPath := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "yaml-agent"})

	output, err := runRootChatCommand(t, "hand", "--config", configPath, "what time is it?")
	require.NoError(t, err)
	assert.Equal(t, "I hit the iteration limit before finishing, so I am returning a summary instead.\n", output)
}

func Test_E2E_HandLiveHarness_RootChat(t *testing.T) {
	resetRootChatE2E(t)

	os.Setenv("HAND_E2E_LIVE_CONFIG", "/Users/nedy/projects/wandxy/hand/config.yaml")

	configPath := strings.TrimSpace(os.Getenv("HAND_E2E_LIVE_CONFIG"))
	if configPath == "" {
		t.Skip("set HAND_E2E_LIVE_CONFIG to run live harness e2e")
	}

	envPath := strings.TrimSpace(os.Getenv("HAND_E2E_LIVE_ENV_FILE"))
	h, err := e2e.NewLiveRPCHarness(
		context.Background(),
		filepath.Join(t.TempDir(), "hand-home"),
		envPath,
		configPath,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})

	configFile := writeRPCConfig(t, h.Address(), h.Port(), e2e.RPCConfigOptions{Name: "live-agent"})

	t.Run("simple answer", func(t *testing.T) {
		output, runErr := runRootChatCommand(t, "hand", "--config", configFile, "Reply with the token ALPHA-42 and nothing else.")
		require.NoError(t, runErr)
		assert.Contains(t, strings.ToUpper(output), "ALPHA-42")
	})

	t.Run("multi turn continuity", func(t *testing.T) {
		firstOutput, runErr := runRootChatCommand(t, "hand", "--config", configFile, "Remember the token BRAVO-77 for this session. Reply with STORED only.")
		require.NoError(t, runErr)
		assert.Contains(t, strings.ToUpper(firstOutput), "STORED")

		secondOutput, runErr := runRootChatCommand(t, "hand", "--config", configFile, "What token did I ask you to remember for this session? Reply with the token only.")
		require.NoError(t, runErr)
		assert.Contains(t, strings.ToUpper(secondOutput), "BRAVO-77")
	})
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

func newRPCHarness(t *testing.T, home string, client models.Client, cfg *config.Config) *e2e.RPCHarness {
	t.Helper()

	if cfg == nil {
		cfg = e2e.DefaultConfig(e2e.ConfigOptions{StorageBackend: "sqlite"})
	}

	h, err := e2e.NewDefaultRPCHarness(context.Background(), home, client, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})

	return h
}

func writeRPCConfig(t *testing.T, address string, port int, opts e2e.RPCConfigOptions) string {
	t.Helper()

	path, err := e2e.WriteRPCConfigFile(t.TempDir(), address, port, opts)
	require.NoError(t, err)
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
