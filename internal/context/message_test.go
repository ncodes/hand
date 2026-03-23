package context

import (
	"testing"

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
