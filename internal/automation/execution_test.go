package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/morph/internal/config"
	models "github.com/wandxy/morph/internal/model"
	modelclient "github.com/wandxy/morph/internal/model/client"
	"github.com/wandxy/morph/internal/profile"
	state "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/pkg/str"
)

func TestAgentRunner_RunPromptThroughRuntime(t *testing.T) {
	factory := &automationModelClientFactoryStub{}
	agent := &automationRuntimeAgentStub{output: "done"}
	runner := newExecutionTestRunner(t, AgentRunnerOptions{
		ModelClientFactory: factory,
		AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return agent
		},
	})

	result, err := runner.RunAutomation(context.Background(), Job{
		Profile: "work",
		Payload: Payload{
			Kind:          PayloadPrompt,
			Prompt:        "summarize this",
			Model:         "override-model",
			Provider:      "openai",
			BaseURL:       "https://override.example/v1",
			MaxIterations: 3,
			ToolGroups:    []string{" shell ", "shell", "memory"},
		},
	})
	require.NoError(t, err)

	require.True(t, agent.started)
	require.True(t, agent.closed)
	require.True(t, agent.created)
	require.Equal(t, "summarize this", agent.respondPrompt)
	require.Equal(t, testAutomationExecutionSessionID, agent.respondOptions.SessionID)
	require.Equal(t, []string{"shell", "memory"}, agent.respondOptions.ToolGroups)
	require.NotNil(t, agent.respondOptions.Stream)
	require.False(t, *agent.respondOptions.Stream)
	require.Equal(t, RunStatusOK, result.Status)
	require.Equal(t, "done", result.Output)
	require.Equal(t, testAutomationExecutionSessionID, result.SessionID)
	require.Equal(t, "override-model", result.Model)
	require.Equal(t, "openai", result.Provider)
	require.Len(t, factory.requests, 1)
	require.Equal(t, modelclient.ModelRoleMain, factory.requests[0].Role)
	require.Equal(t, "override-model", factory.requests[0].Model)
	require.Equal(t, "https://override.example/v1", factory.requests[0].BaseURL)
}

func TestAgentRunner_RunPromptSessionTargets(t *testing.T) {
	for _, test := range []struct {
		name        string
		target      string
		wantSession string
		wantCreated bool
	}{
		{name: "main", target: SessionTargetMain, wantSession: state.DefaultSessionID},
		{name: "session", target: SessionTargetPrefix + testAutomationExecutionSessionID, wantSession: testAutomationExecutionSessionID},
		{name: "current", target: SessionTargetCurrent, wantSession: state.DefaultSessionID},
		{name: "origin", target: SessionTargetOrigin, wantSession: state.DefaultSessionID},
		{name: "default isolated", target: "", wantSession: testAutomationExecutionSessionID, wantCreated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			agent := &automationRuntimeAgentStub{output: "done"}
			runner := newExecutionTestRunner(t, AgentRunnerOptions{
				AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
					return agent
				},
			})

			result, err := runner.RunAutomation(context.Background(), Job{
				SessionTarget: test.target,
				Payload:       Payload{Prompt: "hello"},
			})
			require.NoError(t, err)
			require.Equal(t, test.wantSession, result.SessionID)
			require.Equal(t, test.wantSession, agent.respondOptions.SessionID)
			require.Equal(t, test.wantCreated, agent.created)
		})
	}
}

func TestAgentRunner_SystemEventIsNoop(t *testing.T) {
	for _, test := range []struct {
		name        string
		target      string
		wantSession string
	}{
		{name: "named session", target: SessionTargetPrefix + testAutomationExecutionSessionID, wantSession: testAutomationExecutionSessionID},
		{name: "main", target: SessionTargetMain, wantSession: state.DefaultSessionID},
	} {
		t.Run(test.name, func(t *testing.T) {
			agent := &automationRuntimeAgentStub{}
			runner := newExecutionTestRunner(t, AgentRunnerOptions{
				AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
					return agent
				},
			})

			result, err := runner.RunAutomation(context.Background(), Job{
				SessionTarget: test.target,
				Payload:       Payload{Kind: PayloadSystemEvent, SystemEvent: "wake"},
			})
			require.NoError(t, err)
			require.Equal(t, RunStatusSkipped, result.Status)
			require.Equal(t, automationNoopOutput, result.Output)
			require.Equal(t, test.wantSession, result.SessionID)
			require.False(t, agent.started)
		})
	}
}

func TestAgentRunner_Validation(t *testing.T) {
	runner := newExecutionTestRunner(t, AgentRunnerOptions{})

	for _, test := range []struct {
		name string
		job  Job
		err  string
	}{
		{
			name: "prompt required",
			job:  Job{Payload: Payload{Kind: PayloadPrompt}},
			err:  "automation prompt payload is required",
		},
		{
			name: "system event required",
			job:  Job{Payload: Payload{Kind: PayloadSystemEvent}},
			err:  "automation system event payload is required",
		},
		{
			name: "bad kind",
			job:  Job{Payload: Payload{Kind: "unknown", Prompt: "hello"}},
			err:  `unsupported automation payload kind "unknown"`,
		},
		{
			name: "bad target",
			job:  Job{SessionTarget: "workspace", Payload: Payload{Prompt: "hello"}},
			err:  `unsupported automation session target "workspace"`,
		},
		{
			name: "bad session target id",
			job:  Job{SessionTarget: "session:bad", Payload: Payload{Prompt: "hello"}},
			err:  "session id must be a valid ses_ nanoid",
		},
		{
			name: "negative max iterations",
			job:  Job{Payload: Payload{Prompt: "hello", MaxIterations: -1}},
			err:  "automation max iterations must be non-negative",
		},
		{
			name: "negative max runtime",
			job:  Job{Payload: Payload{Prompt: "hello", MaxRuntime: -time.Second}},
			err:  "automation max runtime must be non-negative",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := runner.RunAutomation(context.Background(), test.job)
			require.EqualError(t, err, test.err)
		})
	}
}

func TestAgentRunner_PropagatesRuntimeErrors(t *testing.T) {
	expected := errors.New("boom")

	_, err := newExecutionTestRunner(t, AgentRunnerOptions{}).RunAutomation(nil, Job{Payload: Payload{Prompt: "hello"}})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = newExecutionTestRunner(t, AgentRunnerOptions{}).RunAutomation(ctx, Job{Payload: Payload{Prompt: "hello"}})
	require.ErrorIs(t, err, context.Canceled)

	_, err = (*AgentRunner)(nil).RunAutomation(context.Background(), Job{})
	require.EqualError(t, err, "automation runner is required")

	_, err = RunnerFunc(nil).RunAutomation(context.Background(), Job{})
	require.EqualError(t, err, "automation runner is required")

	runner := newExecutionTestRunner(t, AgentRunnerOptions{
		ProfileResolver: func(str.String) (profile.Profile, error) {
			return profile.Profile{}, expected
		},
	})
	_, err = runner.RunAutomation(context.Background(), Job{Payload: Payload{Prompt: "hello"}})
	require.ErrorIs(t, err, expected)

	runner = newExecutionTestRunner(t, AgentRunnerOptions{
		ConfigLoader: func(string, string) (*config.Config, error) {
			return nil, expected
		},
	})
	_, err = runner.RunAutomation(context.Background(), Job{Payload: Payload{Prompt: "hello"}})
	require.ErrorIs(t, err, expected)

	runner = newExecutionTestRunner(t, AgentRunnerOptions{
		ModelClientFactory: &automationModelClientFactoryStub{err: expected},
	})
	_, err = runner.RunAutomation(context.Background(), Job{Payload: Payload{Prompt: "hello"}})
	require.ErrorIs(t, err, expected)

	runner = newExecutionTestRunner(t, AgentRunnerOptions{
		AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return &automationRuntimeAgentStub{startErr: expected}
		},
	})
	_, err = runner.RunAutomation(context.Background(), Job{Payload: Payload{Prompt: "hello"}})
	require.ErrorIs(t, err, expected)

	runner = newExecutionTestRunner(t, AgentRunnerOptions{
		AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return nil
		},
	})
	_, err = runner.RunAutomation(context.Background(), Job{Payload: Payload{Prompt: "hello"}})
	require.EqualError(t, err, "automation runtime agent is required")

	runner = newExecutionTestRunner(t, AgentRunnerOptions{
		AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return &automationRuntimeAgentStub{createErr: expected}
		},
	})
	_, err = runner.RunAutomation(context.Background(), Job{Payload: Payload{Prompt: "hello"}})
	require.ErrorIs(t, err, expected)

	runner = newExecutionTestRunner(t, AgentRunnerOptions{
		AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return &automationRuntimeAgentStub{currentErr: expected}
		},
	})
	_, err = runner.RunAutomation(context.Background(), Job{
		SessionTarget: SessionTargetCurrent,
		Payload:       Payload{Prompt: "hello"},
	})
	require.ErrorIs(t, err, expected)

	runner = newExecutionTestRunner(t, AgentRunnerOptions{
		AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return &automationRuntimeAgentStub{respondErr: expected}
		},
	})
	_, err = runner.RunAutomation(context.Background(), Job{Payload: Payload{Prompt: "hello"}})
	require.ErrorIs(t, err, expected)
}

func TestAgentRunner_ModelClientBranches(t *testing.T) {
	runner := newExecutionTestRunner(t, AgentRunnerOptions{})

	_, _, err := runner.newModelClients(nil)
	require.EqualError(t, err, "config is required")

	cfg := automationRunnerTestConfig()
	cfg.Models.Main.APIKey = ""
	cfg.Normalize()
	_, _, err = runner.newModelClients(cfg)
	require.ErrorContains(t, err, "model")

	factory := &automationModelClientFactoryStub{}
	cfg = automationRunnerTestConfig()
	cfg.Models.Summary.Name = "summary-model"
	cfg.Models.Summary.Provider = "missing-provider"
	cfg.Normalize()
	_, _, err = runner.newModelClients(cfg)
	require.ErrorContains(t, err, "missing-provider")

	cfg = automationRunnerTestConfig()
	cfg.Models.Summary.Name = "summary-model"
	cfg.Models.Summary.Provider = "openai"
	cfg.Models.Summary.APIKey = "summary-key"
	cfg.Models.Summary.BaseURL = "https://summary.example/v1"
	cfg.Normalize()
	runner = newExecutionTestRunner(t, AgentRunnerOptions{ModelClientFactory: factory})

	mainClient, summaryClient, err := runner.newModelClients(cfg)
	require.NoError(t, err)
	require.NotNil(t, mainClient)
	require.NotNil(t, summaryClient)
	require.Len(t, factory.requests, 2)
	require.Equal(t, modelclient.ModelRoleMain, factory.requests[0].Role)
	require.Equal(t, modelclient.ModelRoleSummary, factory.requests[1].Role)
	require.Equal(t, "summary-model", factory.requests[1].Model)

	expected := errors.New("summary client failed")
	factory = &automationModelClientFactoryStub{err: expected, errAt: 2}
	cfg = automationRunnerTestConfig()
	cfg.Models.Summary.Name = "summary-model"
	cfg.Models.Summary.Provider = "openai"
	cfg.Models.Summary.APIKey = "summary-key"
	cfg.Models.Summary.BaseURL = "https://summary.example/v1"
	cfg.Normalize()
	runner = newExecutionTestRunner(t, AgentRunnerOptions{ModelClientFactory: factory})
	_, _, err = runner.newModelClients(cfg)
	require.ErrorIs(t, err, expected)
}

func TestAgentRunner_ProfileAndConfigHelpers(t *testing.T) {
	defaultRunner := NewAgentRunner(AgentRunnerOptions{})
	require.NotNil(t, defaultRunner.resolveProfile)
	require.NotNil(t, defaultRunner.loadConfig)
	require.NotNil(t, defaultRunner.clientFactory)
	require.NotNil(t, defaultRunner.agentFactory)

	cfg := automationRunnerTestConfig()
	applyPayloadOverrides(nil, Payload{Model: "ignored"})
	applyPayloadOverrides(cfg, Payload{Model: "next-model"})
	require.Equal(t, "next-model", cfg.Models.Main.Name)
	applyPayloadOverrides(cfg, Payload{Provider: "openai", BaseURL: "https://next.example/v1", MaxIterations: 4})
	require.Equal(t, "https://next.example/v1", cfg.Models.Main.BaseURL)
	require.Equal(t, 4, cfg.Session.MaxIterations)

	require.NotNil(t, newRuntimeAgent(context.Background(), cfg, automationModelClientStub{}, automationModelClientStub{}))

	original := profile.Active()
	originalResolveProfile := resolveProfileFromOptions
	t.Cleanup(func() {
		profile.SetActive(original)
		resolveProfileFromOptions = originalResolveProfile
	})
	active := profile.WithMetadataPaths(profile.Profile{Name: "work", HomeDir: t.TempDir()})
	profile.SetActive(active)

	resolved, err := resolveAutomationProfile("")
	require.NoError(t, err)
	require.Equal(t, "work", resolved.Name)

	resolved, err = resolveAutomationProfile("work")
	require.NoError(t, err)
	require.Equal(t, active.ConfigPath, resolved.ConfigPath)

	resolved, err = resolveAutomationProfile("automation-test")
	require.NoError(t, err)
	require.Equal(t, "automation-test", resolved.Name)

	profile.SetActive(profile.Profile{})
	var resolvedOptions profile.ResolveOptions
	resolveProfileFromOptions = func(opts profile.ResolveOptions) (profile.Profile, error) {
		resolvedOptions = opts
		return profile.Profile{Name: "default", HomeDir: t.TempDir()}, nil
	}
	resolved, err = resolveAutomationProfile("")
	require.NoError(t, err)
	require.Equal(t, profile.ResolveOptions{}, resolvedOptions)
	require.Equal(t, "default", resolved.Name)
}

func TestAgentRunner_TimeoutCancelsRespond(t *testing.T) {
	agent := &automationRuntimeAgentStub{output: "late"}
	runner := newExecutionTestRunner(t, AgentRunnerOptions{
		AgentFactory: func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return agent
		},
	})

	_, err := runner.RunAutomation(context.Background(), Job{
		Payload: Payload{
			Prompt:     "hello",
			MaxRuntime: time.Nanosecond,
		},
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, agent.respondContext.Err(), context.DeadlineExceeded)
	require.True(t, agent.closed)
}

func TestPreparePayloadNormalizesToolGroups(t *testing.T) {
	payload, err := preparePayload(Payload{
		Prompt:     " hello ",
		ToolGroups: []string{" Shell ", "shell", "", "memory"},
	})
	require.NoError(t, err)
	require.Equal(t, PayloadPrompt, payload.Kind)
	require.Equal(t, "hello", payload.Prompt)
	require.Equal(t, []string{"shell", "memory"}, payload.ToolGroups)
}

func TestResolveRunSessionIDRejectsUnknownTarget(t *testing.T) {
	_, err := resolveRunSessionID(context.Background(), &automationRuntimeAgentStub{}, sessionTarget{Kind: "unknown"})
	require.EqualError(t, err, `unsupported automation session target "unknown"`)

	require.Empty(t, staticTargetSessionID(sessionTarget{Kind: SessionTargetOrigin}))
}

func newExecutionTestRunner(t *testing.T, opts AgentRunnerOptions) *AgentRunner {
	t.Helper()

	if opts.ProfileResolver == nil {
		opts.ProfileResolver = func(name str.String) (profile.Profile, error) {
			trimmed := name.Trim()
			if trimmed == "" {
				trimmed = "default"
			}
			return profile.Profile{
				Name:       trimmed,
				HomeDir:    t.TempDir(),
				ConfigPath: "config.yaml",
				EnvPath:    ".env",
			}, nil
		}
	}
	if opts.ConfigLoader == nil {
		opts.ConfigLoader = func(string, string) (*config.Config, error) {
			return automationRunnerTestConfig(), nil
		}
	}
	if opts.ModelClientFactory == nil {
		opts.ModelClientFactory = &automationModelClientFactoryStub{}
	}
	if opts.AgentFactory == nil {
		opts.AgentFactory = func(context.Context, *config.Config, models.Client, models.Client) RuntimeAgent {
			return &automationRuntimeAgentStub{output: "ok"}
		}
	}

	return NewAgentRunner(opts)
}

func automationRunnerTestConfig() *config.Config {
	cfg := config.NewDefaultConfig()
	cfg.Models.Main.Name = "gpt-test"
	cfg.Models.Main.Provider = "openai"
	cfg.Models.Main.APIKey = "test-key"
	cfg.Models.Main.BaseURL = "https://api.openai.test/v1"
	cfg.Models.Summary.Name = ""
	cfg.Models.Summary.Provider = ""
	cfg.Models.Summary.APIKey = ""
	cfg.Models.Summary.BaseURL = ""
	cfg.Normalize()

	return cfg
}
