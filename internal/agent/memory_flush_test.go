package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/pkg/agent/message"
	agenttool "github.com/wandxy/hand/pkg/agent/tool"
)

func TestTurn_AvailableMemoryFlushToolDefinitionsExcludesMemoryExtract(t *testing.T) {
	turn := NewTurnWithSessionStore(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&memoryFlushToolRegistryStub{definitions: []agenttool.Definition{
			{Name: "memory_extract"},
			{Name: "memory_add"},
			{Name: "memory_update"},
			{Name: "memory_delete"},
			{Name: "time"},
		}},
		agenttool.Policy{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	definitions, err := turn.availableMemoryFlushToolDefinitions()
	require.NoError(t, err)

	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	require.Equal(t, []string{"memory_add", "memory_update", "memory_delete"}, names)
}

func TestGetMemoryFlushToolError_ReturnsNilForSuccessfulToolPayload(t *testing.T) {
	err := getMemoryFlushToolError(handmsg.Message{
		Role:    handmsg.RoleTool,
		Name:    "memory_add",
		Content: `{"name":"memory_add","output":"ok"}`,
	})

	require.NoError(t, err)
}

func TestGetMemoryFlushToolError_ReturnsStructuredToolError(t *testing.T) {
	err := getMemoryFlushToolError(handmsg.Message{
		Role:    handmsg.RoleTool,
		Name:    "memory_extract",
		Content: `{"name":"memory_extract","error":{"code":"tool_error","message":"context deadline exceeded"}}`,
	})

	require.EqualError(t, err, "memory flush tool memory_extract failed: context deadline exceeded")
}

func TestGetMemoryFlushToolError_ReturnsEncodedToolError(t *testing.T) {
	err := getMemoryFlushToolError(handmsg.Message{
		Role:    handmsg.RoleTool,
		Name:    "memory_extract",
		Content: `{"name":"memory_extract","error":"{\"code\":\"tool_error\",\"message\":\"context deadline exceeded\"}"}`,
	})

	require.EqualError(t, err, "memory flush tool memory_extract failed: context deadline exceeded")
}

type memoryFlushToolRegistryStub struct {
	definitions []agenttool.Definition
}

func (s *memoryFlushToolRegistryStub) Resolve(agenttool.Policy) ([]agenttool.Definition, error) {
	return s.definitions, nil
}

func (s *memoryFlushToolRegistryStub) Invoke(context.Context, agenttool.Call) handmsg.Message {
	return handmsg.Message{}
}
