package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/model"
	"github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/agent/message"
	"github.com/wandxy/hand/pkg/agent/session"
	"github.com/wandxy/hand/pkg/agent/tool"
)

func TestAgent_RespondCompletesWithPublicDependencies(t *testing.T) {
	store := newMemorySessionStore()
	client := &fakeModelClient{
		complete: func(_ context.Context, request model.Request) (*model.Response, error) {
			require.Len(t, request.Messages, 1)
			require.Equal(t, message.RoleUser, request.Messages[0].Role)

			return &model.Response{OutputText: "hello from public core", PromptTokens: 12}, nil
		},
	}

	core, err := agent.NewAgent(agent.Options{
		Model:        "test-model",
		API:          model.APIOpenAICompletions,
		ModelClient:  client,
		SessionStore: store,
	})
	require.NoError(t, err)

	reply, err := core.Respond(context.Background(), "hello", agent.RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "hello from public core", reply)
	require.Equal(t, 12, store.lastPromptTokens)
	require.Len(t, store.messages[session.DefaultID], 2)
}

func TestAgent_RespondRunsToolLoopWithPublicDependencies(t *testing.T) {
	store := newMemorySessionStore()
	registry := &fakeToolRegistry{
		invoke: func(_ context.Context, call tool.Call) message.Message {
			require.Equal(t, "lookup", call.Name)

			return message.Message{
				Role:       message.RoleTool,
				Name:       call.Name,
				ToolCallID: call.ID,
				Content:    `{"ok":true}`,
			}
		},
	}
	client := &fakeModelClient{}
	client.complete = func(_ context.Context, request model.Request) (*model.Response, error) {
		if len(request.Messages) == 1 {
			return &model.Response{
				RequiresToolCalls: true,
				ToolCalls: []model.ToolCall{{
					ID:    "call_1",
					Name:  "lookup",
					Input: `{"query":"hello"}`,
				}},
			}, nil
		}

		require.Equal(t, message.RoleTool, request.Messages[len(request.Messages)-1].Role)
		return &model.Response{OutputText: "tool result handled"}, nil
	}

	core, err := agent.New(agent.Options{
		Model:         "test-model",
		API:           model.APIOpenAICompletions,
		ModelClient:   client,
		SessionStore:  store,
		ToolRegistry:  registry,
		MaxIterations: 2,
	})
	require.NoError(t, err)

	reply, err := core.Respond(context.Background(), "hello", agent.RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "tool result handled", reply)
	require.Len(t, registry.calls, 1)
	require.Len(t, store.messages[session.DefaultID], 4)
}

type memorySessionStore struct {
	messages         map[string][]message.Message
	lastPromptTokens int
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{messages: map[string][]message.Message{}}
}

func (s *memorySessionStore) Resolve(_ context.Context, id string) (session.Session, error) {
	if id == "" {
		id = session.DefaultID
	}

	return session.Session{ID: id}, nil
}

func (s *memorySessionStore) GetMessages(
	_ context.Context,
	id string,
	_ session.MessageQuery,
) ([]message.Message, error) {
	return message.CloneMessages(s.messages[id]), nil
}

func (s *memorySessionStore) AppendMessages(_ context.Context, id string, messages []message.Message) error {
	if id == "" {
		id = session.DefaultID
	}

	s.messages[id] = append(s.messages[id], message.CloneMessages(messages)...)
	return nil
}

func (s *memorySessionStore) UpdateLastPromptTokens(_ context.Context, _ string, tokens int) error {
	s.lastPromptTokens = tokens
	return nil
}

type fakeModelClient struct {
	complete func(context.Context, model.Request) (*model.Response, error)
}

func (c *fakeModelClient) Complete(ctx context.Context, request model.Request) (*model.Response, error) {
	return c.complete(ctx, request)
}

func (c *fakeModelClient) CompleteStream(
	ctx context.Context,
	request model.Request,
	_ func(model.StreamDelta),
) (*model.Response, error) {
	return c.complete(ctx, request)
}

type fakeToolRegistry struct {
	calls  []tool.Call
	invoke func(context.Context, tool.Call) message.Message
}

func (r *fakeToolRegistry) Resolve(tool.Policy) ([]tool.Definition, error) {
	return []tool.Definition{{Name: "lookup"}}, nil
}

func (r *fakeToolRegistry) Invoke(ctx context.Context, call tool.Call) message.Message {
	r.calls = append(r.calls, call)
	return r.invoke(ctx, call)
}
