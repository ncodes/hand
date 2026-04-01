package messages

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNew_NormalizesRoleAndContent(t *testing.T) {
	message, err := New(Role(" User "), "  hello  ")
	require.NoError(t, err)
	require.Equal(t, RoleUser, message.Role)
	require.Equal(t, "hello", message.Content)
	require.False(t, message.CreatedAt.IsZero())
}

func TestNew_RejectsInvalidRole(t *testing.T) {
	_, err := New(Role("invalid"), "hello")
	require.EqualError(t, err, "message role must be one of developer, user, assistant, or tool")
}

func TestNew_RejectsEmptyContent(t *testing.T) {
	_, err := New(RoleUser, "   ")
	require.EqualError(t, err, "message content is required")
}

func TestNew_AllowsDeveloperAndToolRoles(t *testing.T) {
	developer, err := New(RoleDeveloper, "system")
	require.NoError(t, err)
	require.Equal(t, RoleDeveloper, developer.Role)
	toolMessage, err := New(RoleTool, "tool output")
	require.NoError(t, err)
	require.Equal(t, RoleTool, toolMessage.Role)
}

func TestNewMessage_DelegatesToNew(t *testing.T) {
	message, err := NewMessage(Role(" User "), "  hello  ")
	require.NoError(t, err)
	require.Equal(t, RoleUser, message.Role)
	require.Equal(t, "hello", message.Content)
	require.False(t, message.CreatedAt.IsZero())
}

func TestNormalize_TrimsToolFieldsAndSetsTimestampWhenMissing(t *testing.T) {
	message, err := Normalize(Message{
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

func TestNormalize_PreservesProvidedTimestamp(t *testing.T) {
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	message, err := Normalize(Message{
		Role:      RoleAssistant,
		Content:   " hello ",
		CreatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, now, message.CreatedAt)
}

func TestNormalize_AllowsAssistantToolCallsWithoutContent(t *testing.T) {
	message, err := Normalize(Message{
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
	require.Equal(t, []ToolCall{{ID: "call-1", Name: "time", Input: "{}"}}, message.ToolCalls)
}

func TestNormalizeMessage_DelegatesToNormalize(t *testing.T) {
	message, err := NormalizeMessage(Message{
		Role:    RoleAssistant,
		Content: " hello ",
	})
	require.NoError(t, err)
	require.Equal(t, RoleAssistant, message.Role)
	require.Equal(t, "hello", message.Content)
}

func TestNormalize_RejectsEmptyAssistantContentWithoutToolCalls(t *testing.T) {
	_, err := Normalize(Message{Role: RoleAssistant, Content: "   "})
	require.EqualError(t, err, "message content is required")
}

func TestNormalize_RejectsToolMessageWithoutToolCallID(t *testing.T) {
	_, err := Normalize(Message{Role: RoleTool, Content: "result"})
	require.EqualError(t, err, "tool call id is required")
}

func TestNormalize_RejectsInvalidRole(t *testing.T) {
	_, err := Normalize(Message{Role: Role("invalid"), Content: "hello"})
	require.EqualError(t, err, "message role must be one of developer, user, assistant, or tool")
}

func TestNormalize_RejectsInvalidToolCall(t *testing.T) {
	_, err := Normalize(Message{Role: RoleAssistant, ToolCalls: []ToolCall{{Name: "time"}}})
	require.EqualError(t, err, "tool call id is required")
}

func TestNormalize_RejectsToolCallWithoutName(t *testing.T) {
	_, err := Normalize(Message{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call-1"}}})
	require.EqualError(t, err, "tool call name is required")
}

func TestCloneMessagesReturnsNilWhenEmpty(t *testing.T) {
	require.Nil(t, CloneMessages(nil))
	require.Nil(t, CloneMessages([]Message{}))
}

func TestCloneMessagesDeepCopiesToolCalls(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	original := []Message{{
		Role:      RoleAssistant,
		Content:   "hello",
		ToolCalls: []ToolCall{{ID: "call-1", Name: "time", Input: "{}"}},
		CreatedAt: now,
	}}

	cloned := CloneMessages(original)
	cloned[0].Content = "changed"
	cloned[0].ToolCalls[0].Name = "changed"

	require.Equal(t, "hello", original[0].Content)
	require.Equal(t, "time", original[0].ToolCalls[0].Name)
	require.Equal(t, now, cloned[0].CreatedAt)
}
