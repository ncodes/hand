package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/environment"
	envbudget "github.com/wandxy/hand/internal/environment/budget"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/tools"
)

func TestAgent_RespondGeneratesSessionTitleWithSummaryClient(t *testing.T) {
	modelClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "Use a deterministic fallback"}}}
	summaryClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "  Project Launch Plan.  "}}}
	agent := newTitleTestAgent(t, modelClient, summaryClient)

	reply, err := agent.Respond(context.Background(), "help plan the launch", RespondOptions{})

	require.NoError(t, err)
	require.Equal(t, "Use a deterministic fallback", reply)
	require.Len(t, summaryClient.Requests, 1)
	require.Equal(t, "summary-model", summaryClient.Requests[0].Model)
	require.Equal(t, models.APIModeResponses, summaryClient.Requests[0].APIMode)
	require.Contains(t, summaryClient.Requests[0].Instructions, "# Session Title Task")
	require.Contains(t, summaryClient.Requests[0].Messages[0].Content, "help plan the launch")

	session, ok, err := agent.stateMgr.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Project Launch Plan", session.Title)
	require.Equal(t, storage.SessionTitleSourceGenerated, session.TitleSource)
}

func TestAgent_RespondDoesNotGenerateSessionTitleWhenPresent(t *testing.T) {
	modelClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}}
	summaryClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "New Title"}}}
	agent := newTitleTestAgent(t, modelClient, summaryClient)
	require.NoError(t, agent.stateMgr.Save(context.Background(), storage.Session{
		ID:          storage.DefaultSessionID,
		Title:       "Existing Title",
		TitleSource: storage.SessionTitleSourceGenerated,
	}))

	_, err := agent.Respond(context.Background(), "hello", RespondOptions{})

	require.NoError(t, err)
	require.Empty(t, summaryClient.Requests)

	session, ok, err := agent.stateMgr.Get(context.Background(), storage.DefaultSessionID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Existing Title", session.Title)
}

func TestAgent_MaybeGenerateSessionTitleSkipsEmptySessions(t *testing.T) {
	summaryClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "New Title"}}}
	agent := newTitleTestAgent(t, &mocks.ModelClientStub{}, summaryClient)

	agent.maybeGenerateSessionTitle(context.Background(), storage.DefaultSessionID)

	require.Empty(t, summaryClient.Requests)
}

func TestAgent_MaybeGenerateSessionTitleSkipsIncompleteOrDisabledAgents(t *testing.T) {
	summaryClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "New Title"}}}
	(*Agent)(nil).maybeGenerateSessionTitle(context.Background(), storage.DefaultSessionID)
	(&Agent{}).maybeGenerateSessionTitle(context.Background(), storage.DefaultSessionID)
	(&Agent{cfg: testSessionConfig(&config.Config{})}).maybeGenerateSessionTitle(
		context.Background(),
		storage.DefaultSessionID,
	)
	(&Agent{
		cfg:           testSessionConfig(&config.Config{}),
		stateMgr:      mustNewStateManager(t),
		summaryClient: summaryClient,
	}).maybeGenerateSessionTitle(context.Background(), storage.DefaultSessionID)

	require.Empty(t, summaryClient.Requests)
}

func TestAgent_MaybeGenerateSessionTitleSkipsMainClientFallbackWithoutExplicitSummaryModel(t *testing.T) {
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "New Title"}}}
	agent := newTitleTestAgent(t, client, client)
	agent.cfg.Models.Summary.Name = ""
	require.NoError(t, agent.stateMgr.AppendMessages(context.Background(), storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "name this session"},
	}))

	agent.maybeGenerateSessionTitle(context.Background(), storage.DefaultSessionID)

	require.Empty(t, client.Requests)
}

func TestAgent_MaybeGenerateSessionTitleSkipsWhenFallbackTitleIsEmpty(t *testing.T) {
	summaryClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "New Title"}}}
	agent := newTitleTestAgent(t, &mocks.ModelClientStub{}, summaryClient)
	require.NoError(t, agent.stateMgr.AppendMessages(context.Background(), storage.DefaultSessionID, []handmsg.Message{
		{Role: handmsg.RoleUser, Content: "!!!"},
	}))

	agent.maybeGenerateSessionTitle(context.Background(), storage.DefaultSessionID)

	require.Empty(t, summaryClient.Requests)
}

func TestAgent_RespondFallsBackToUserMessageWhenTitleGenerationFails(t *testing.T) {
	cases := []struct {
		name          string
		summaryClient *mocks.ModelClientStub
	}{
		{
			name:          "model error",
			summaryClient: &mocks.ModelClientStub{Errors: []error{errors.New("title failed")}},
		},
		{
			name:          "empty output",
			summaryClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "  "}}},
		},
		{
			name:          "invalid output",
			summaryClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "Deployment Conversation"}}},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			modelClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}}
			agent := newTitleTestAgent(t, modelClient, tt.summaryClient)

			_, err := agent.Respond(
				context.Background(),
				"Please compare deployment options for the project",
				RespondOptions{},
			)

			require.NoError(t, err)

			session, ok, err := agent.stateMgr.Get(context.Background(), storage.DefaultSessionID)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, "Please compare deployment options for the project", session.Title)
			require.Equal(t, storage.SessionTitleSourceGenerated, session.TitleSource)
		})
	}
}

func TestNormalizeGeneratedSessionTitle(t *testing.T) {
	require.Equal(t, "Project Launch Plan", normalizeGeneratedSessionTitle(`"Project Launch Plan."`))
	require.Empty(t, normalizeGeneratedSessionTitle("Launch Conversation"))
	require.Empty(t, normalizeGeneratedSessionTitle("   "))
	require.Len(
		t,
		[]rune(normalizeGeneratedSessionTitle("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabc")),
		maxSessionTitleRunes,
	)
}

func TestIsSameModelClient(t *testing.T) {
	client := &mocks.ModelClientStub{}
	other := &mocks.ModelClientStub{}

	require.True(t, isSameModelClient(nil, nil))
	require.False(t, isSameModelClient(client, nil))
	require.True(t, isSameModelClient(client, client))
	require.False(t, isSameModelClient(client, other))
	require.False(t, isSameModelClient(client, titleValueClient{}))
	require.False(t, isSameModelClient(titleValueClient{}, titleValueClient{}))
}

func newTitleTestAgent(
	t *testing.T,
	modelClient *mocks.ModelClientStub,
	summaryClient *mocks.ModelClientStub,
) *Agent {
	t.Helper()

	originalFactory := newEnvironment
	t.Cleanup(func() {
		newEnvironment = originalFactory
	})
	newEnvironment = func(context.Context, *config.Config) environment.Environment {
		return &mocks.EnvironmentStub{
			InstructionsList: instruct.Instructions{{Value: "system prompt"}},
			ToolRegistry:     tools.NewInMemoryRegistry(),
			IterationBudget:  envbudget.New(constants.DefaultMaxIterations),
			TraceSession:     &mocks.TraceSessionStub{},
		}
	}

	cfg := testSessionConfig(&config.Config{
		Name: "Title Test",
		Models: config.ModelsConfig{
			Main: config.MainModelConfig{
				Name:    "main-model",
				APIMode: models.APIModeResponses,
			},
			Summary: config.SummaryModelConfig{
				Name:    "summary-model",
				APIMode: models.APIModeResponses,
			},
		},
	})
	agent := NewAgent(context.Background(), cfg, modelClient, summaryClient)
	require.NoError(t, agent.Start(context.Background()))
	require.NoError(t, agent.stateMgr.Save(context.Background(), storage.Session{
		ID:        storage.DefaultSessionID,
		UpdatedAt: time.Now().UTC(),
	}))

	return agent
}

func TestGetSessionTitleContext(t *testing.T) {
	contextText, fallback := getSessionTitleContext([]handmsg.Message{
		{Role: handmsg.RoleTool, Content: "ignored"},
		{Role: handmsg.RoleUser, Content: "  explain deployment windows  "},
		{Role: handmsg.RoleAssistant, Content: "Use staged rollout."},
	})

	require.Contains(t, contextText, "User: explain deployment windows")
	require.Contains(t, contextText, "Assistant: Use staged rollout.")
	require.Equal(t, "explain deployment windows", fallback)
}

func TestGetSessionTitleContextHandlesMissingAssistantAndUser(t *testing.T) {
	contextText, fallback := getSessionTitleContext([]handmsg.Message{
		{Role: handmsg.RoleUser, Content: "   "},
		{Role: handmsg.RoleUser, Content: "  compare launch options  "},
	})
	require.Equal(t, "User: compare launch options", contextText)
	require.Equal(t, "compare launch options", fallback)

	contextText, fallback = getSessionTitleContext([]handmsg.Message{
		{Role: handmsg.RoleAssistant, Content: "No user yet."},
	})
	require.Empty(t, contextText)
	require.Empty(t, fallback)
}

func TestFallbackSessionTitleFromUserMessage(t *testing.T) {
	require.Empty(t, fallbackSessionTitleFromUserMessage("   "))
	require.Equal(t, "Compare options", fallbackSessionTitleFromUserMessage("Compare options!!!"))
	require.Equal(
		t,
		"one two three four five six seven eight",
		fallbackSessionTitleFromUserMessage("one two three four five six seven eight nine"),
	)
	require.Len(
		t,
		[]rune(fallbackSessionTitleFromUserMessage("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabc")),
		maxSessionTitleRunes,
	)
}

func TestTrimTitleRunes(t *testing.T) {
	require.Empty(t, trimTitleRunes("title", 0))
	require.Equal(t, "title", trimTitleRunes(" title ", 10))
	require.Equal(t, "tit", trimTitleRunes(" title ", 3))
}

type titleValueClient struct {
	values []string
}

func (c titleValueClient) Complete(context.Context, models.Request) (*models.Response, error) {
	return nil, nil
}

func (c titleValueClient) CompleteStream(context.Context, models.Request, func(models.StreamDelta)) (*models.Response, error) {
	return nil, nil
}
