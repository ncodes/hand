package host

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/environment"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	handtools "github.com/wandxy/hand/internal/tools"
	agenttool "github.com/wandxy/hand/pkg/agent/tool"
)

func TestToolRegistry_ResolveUsesEnvironmentPolicyAndConvertsDefinitions(t *testing.T) {
	stub := &mocks.ToolRegistryStub{
		Definitions: handtools.Definitions{{
			Name:        "memory_extract",
			Description: "Extract memory",
			InputSchema: map[string]any{"type": "object"},
			Groups:      []string{"core"},
			Requires:    handtools.Capabilities{Memory: true},
			Platforms:   []string{"darwin"},
		}},
	}
	env := &mocks.EnvironmentStub{
		ToolRegistry: stub,
		Policy: handtools.Policy{
			GroupNames:   []string{"core"},
			Capabilities: handtools.Capabilities{Memory: true},
			Platform:     "darwin",
		},
	}
	registry := NewToolRegistry(env, nil)

	definitions, err := registry.Resolve(agenttool.Policy{})
	require.NoError(t, err)
	require.Equal(t, []agenttool.Definition{{
		Name:        "memory_extract",
		Description: "Extract memory",
		InputSchema: map[string]any{"type": "object"},
		Groups:      []string{"core"},
		Requires:    agenttool.Capabilities{Memory: true},
		Platforms:   []string{"darwin"},
	}}, definitions)
	require.Equal(t, env.Policy, stub.LastToolPolicy)
}

func TestToolRegistry_InvokeDelegatesToHostInvoker(t *testing.T) {
	env := &mocks.EnvironmentStub{}
	var capturedEnv any
	var capturedCall models.ToolCall
	registry := NewToolRegistry(
		env,
		func(_ context.Context, runtimeEnv environment.Environment, toolCall models.ToolCall) handmsg.Message {
			capturedEnv = runtimeEnv
			capturedCall = toolCall
			return handmsg.Message{
				Role:       handmsg.RoleTool,
				Name:       toolCall.Name,
				ToolCallID: toolCall.ID,
				Content:    `{"ok":true}`,
			}
		},
	)

	message := registry.Invoke(context.Background(), agenttool.Call{ID: "call-1", Name: "time", Input: "{}"})

	require.Same(t, env, capturedEnv)
	require.Equal(t, models.ToolCall{ID: "call-1", Name: "time", Input: "{}"}, capturedCall)
	require.Equal(t, handmsg.RoleTool, message.Role)
	require.Equal(t, "time", message.Name)
	require.Equal(t, "call-1", message.ToolCallID)
	require.Equal(t, `{"ok":true}`, message.Content)
}
