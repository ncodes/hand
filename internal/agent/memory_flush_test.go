package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

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
