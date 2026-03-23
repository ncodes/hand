package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	handcontext "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/models"
)

type modelClientStub struct {
	requests  []models.GenerateRequest
	responses []*models.GenerateResponse
	err       error
	callCount int
}

type environmentStub struct {
	prepareErr error
	context    environment.Context
}

func (s *environmentStub) Prepare() error {
	return s.prepareErr
}

func (s *environmentStub) Context() environment.Context {
	return s.context
}

func (s *modelClientStub) Chat(_ context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	if s.callCount >= len(s.responses) {
		return nil, errors.New("missing stubbed response")
	}
	response := s.responses[s.callCount]
	s.callCount++
	return response, nil
}

func TestAgent_RunInitializesConversationState(t *testing.T) {
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &environmentStub{context: handcontext.NewContext(context.Background(), &config.Config{})}
	}

	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &modelClientStub{})

	require.NoError(t, agent.Run(context.Background()))
	require.True(t, agent.Conversation().Empty())
}

func TestAgent_RunRejectsNilAgent(t *testing.T) {
	var agent *Agent

	err := agent.Run(context.Background())
	require.EqualError(t, err, "agent is required")
}

func TestAgent_RunRejectsNilConfig(t *testing.T) {
	agent := NewAgent(context.Background(), nil, &modelClientStub{})

	err := agent.Run(context.Background())
	require.EqualError(t, err, "config is required")
}

func TestAgent_RunReturnsEnvironmentPrepareError(t *testing.T) {
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &environmentStub{
			prepareErr: errors.New("prepare failed"),
			context:    handcontext.NewContext(context.Background(), &config.Config{}),
		}
	}

	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent"}, &modelClientStub{})

	err := agent.Run(context.Background())
	require.EqualError(t, err, "prepare failed")
}

func TestAgent_ChatAppendsConversationAcrossTurns(t *testing.T) {
	client := &modelClientStub{
		responses: []*models.GenerateResponse{
			{OutputText: "hello back"},
			{OutputText: "still here"},
		},
	}
	agent := NewAgent(context.Background(), &config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		runtimeContext := handcontext.NewContext(context.Background(), &config.Config{})
		runtimeContext.AddInstruction(handcontext.Instruction{Value: "system prompt"})
		return &environmentStub{
			context: runtimeContext,
		}
	}

	require.NoError(t, agent.Run(context.Background()))

	reply, err := agent.Chat(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "hello back", reply)

	reply, err = agent.Chat(context.Background(), "again")
	require.NoError(t, err)
	require.Equal(t, "still here", reply)

	require.Len(t, client.requests, 2)
	require.Equal(t, "system prompt", client.requests[0].Instructions)
	require.Equal(t, []handcontext.Message{
		{Role: handcontext.RoleUser, Content: "hello", CreatedAt: client.requests[0].Messages[0].CreatedAt},
	}, client.requests[0].Messages)

	require.Len(t, client.requests[1].Messages, 3)
	require.Equal(t, handcontext.RoleUser, client.requests[1].Messages[0].Role)
	require.Equal(t, "hello", client.requests[1].Messages[0].Content)
	require.Equal(t, handcontext.RoleAssistant, client.requests[1].Messages[1].Role)
	require.Equal(t, "hello back", client.requests[1].Messages[1].Content)
	require.Equal(t, handcontext.RoleUser, client.requests[1].Messages[2].Role)
	require.Equal(t, "again", client.requests[1].Messages[2].Content)

	conversation := agent.Conversation()
	require.Len(t, conversation.Messages(), 4)
	require.Equal(t, "still here", conversation.Messages()[3].Content)
}

func TestAgent_ChatDoesNotAppendAssistantWhenModelFails(t *testing.T) {
	client := &modelClientStub{
		err: errors.New("upstream failed"),
	}
	agent := NewAgent(context.Background(), &config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &environmentStub{context: handcontext.NewContext(context.Background(), &config.Config{})}
	}

	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "upstream failed")

	conversation := agent.Conversation()
	require.Len(t, conversation.Messages(), 1)
	require.Equal(t, handcontext.RoleUser, conversation.Messages()[0].Role)
	require.Equal(t, "hello", conversation.Messages()[0].Content)
}

func TestAgent_ChatRejectsNilAgent(t *testing.T) {
	var agent *Agent

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "agent is required")
}

func TestAgent_ChatRejectsUninitializedEnvironment(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, &modelClientStub{})

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "environment has not been initialized")
}

func TestAgent_ChatRejectsMissingModelClient(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, nil)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &environmentStub{context: handcontext.NewContext(context.Background(), &config.Config{})}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "model client is required")
}

func TestAgent_ChatRejectsEmptyMessage(t *testing.T) {
	agent := NewAgent(context.Background(), &config.Config{Name: "Test Agent", Model: "test-model"}, &modelClientStub{})
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &environmentStub{context: handcontext.NewContext(context.Background(), &config.Config{})}
	}
	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "   ")
	require.EqualError(t, err, "message is required")
}

func TestAgent_ConversationReturnsCopy(t *testing.T) {
	client := &modelClientStub{
		responses: []*models.GenerateResponse{
			{OutputText: "hello back"},
		},
	}
	agent := NewAgent(context.Background(), &config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &environmentStub{context: handcontext.NewContext(context.Background(), &config.Config{})}
	}

	require.NoError(t, agent.Run(context.Background()))
	_, err := agent.Chat(context.Background(), "hello")
	require.NoError(t, err)

	conversation := agent.Conversation()
	messages := conversation.Messages()
	messages[0].Content = "changed"

	require.Equal(t, "hello", agent.Conversation().Messages()[0].Content)
}

func TestAgent_ConversationReturnsEmptyForNilAgent(t *testing.T) {
	var agent *Agent

	conversation := agent.Conversation()
	require.True(t, conversation.Empty())
}

func TestAgent_ChatReturnsAssistantAppendErrorForEmptyOutput(t *testing.T) {
	client := &modelClientStub{
		responses: []*models.GenerateResponse{
			{OutputText: "   "},
		},
	}
	agent := NewAgent(context.Background(), &config.Config{
		Name:  "Test Agent",
		Model: "test-model",
	}, client)
	originalFactory := newRuntimeEnvironment
	t.Cleanup(func() {
		newRuntimeEnvironment = originalFactory
	})
	newRuntimeEnvironment = func(context.Context, *config.Config) runtimeEnvironment {
		return &environmentStub{context: handcontext.NewContext(context.Background(), &config.Config{})}
	}

	require.NoError(t, agent.Run(context.Background()))

	_, err := agent.Chat(context.Background(), "hello")
	require.EqualError(t, err, "message content is required")
}
