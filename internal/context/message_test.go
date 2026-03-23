package context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewMessage_NormalizesRoleAndContent(t *testing.T) {
	message, err := NewMessage(Role(" User "), "  hello  ")

	require.NoError(t, err)
	require.Equal(t, RoleUser, message.Role)
	require.Equal(t, "hello", message.Content)
	require.False(t, message.CreatedAt.IsZero())
}

func TestNewMessage_RejectsInvalidRole(t *testing.T) {
	_, err := NewMessage(Role("invalid"), "hello")
	require.EqualError(t, err, "message role must be one of developer, user, assistant, or tool")
}

func TestNewMessage_RejectsEmptyContent(t *testing.T) {
	_, err := NewMessage(RoleUser, "   ")
	require.EqualError(t, err, "message content is required")
}

func TestNewMessage_AllowsDeveloperAndToolRoles(t *testing.T) {
	developer, err := NewMessage(RoleDeveloper, "system")
	require.NoError(t, err)
	require.Equal(t, RoleDeveloper, developer.Role)

	tool, err := NewMessage(RoleTool, "tool output")
	require.NoError(t, err)
	require.Equal(t, RoleTool, tool.Role)
}

func TestNormalizeMessage_TrimsToolFieldsAndSetsTimestampWhenMissing(t *testing.T) {
	message, err := normalizeMessage(Message{
		Role:       RoleTool,
		Content:    "  result  ",
		Name:       "  time  ",
		ToolCallID: "  call-1  ",
		CreatedAt:  time.Time{},
	})

	require.NoError(t, err)
	require.Equal(t, RoleTool, message.Role)
	require.Equal(t, "result", message.Content)
	require.Equal(t, "time", message.Name)
	require.Equal(t, "call-1", message.ToolCallID)
	require.False(t, message.CreatedAt.IsZero())
}

func TestNormalizeMessage_AllowsAssistantToolCallsWithoutContent(t *testing.T) {
	message, err := normalizeMessage(Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{{
			ID:    " call-1 ",
			Name:  " time ",
			Input: " {} ",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, RoleAssistant, message.Role)
	require.Empty(t, message.Content)
	require.Equal(t, []ToolCall{{
		ID:    "call-1",
		Name:  "time",
		Input: "{}",
	}}, message.ToolCalls)
}

func TestNormalizeMessage_RejectsToolMessageWithoutToolCallID(t *testing.T) {
	_, err := normalizeMessage(Message{
		Role:    RoleTool,
		Content: "result",
	})

	require.EqualError(t, err, "tool call id is required")
}

func TestNormalizeMessage_RejectsInvalidRole(t *testing.T) {
	_, err := normalizeMessage(Message{
		Role:    Role("invalid"),
		Content: "hello",
	})

	require.EqualError(t, err, "message role must be one of developer, user, assistant, or tool")
}

func TestNormalizeMessage_RejectsInvalidToolCall(t *testing.T) {
	_, err := normalizeMessage(Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{{
			Name: "time",
		}},
	})

	require.EqualError(t, err, "tool call id is required")
}

func TestNormalizeToolCalls_RejectsMissingID(t *testing.T) {
	_, err := normalizeToolCalls([]ToolCall{{
		Name: "time",
	}})

	require.EqualError(t, err, "tool call id is required")
}

func TestNormalizeToolCalls_RejectsMissingName(t *testing.T) {
	_, err := normalizeToolCalls([]ToolCall{{
		ID: "call-1",
	}})

	require.EqualError(t, err, "tool call name is required")
}
