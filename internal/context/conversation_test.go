package context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConversation_AppendPreservesOrder(t *testing.T) {
	conversation := NewConversation()
	firstTime := time.Now().UTC().Add(-time.Minute)

	require.NoError(t, conversation.Append(Message{Role: RoleUser, Content: "first", CreatedAt: firstTime}))
	require.NoError(t, conversation.Append(Message{Role: RoleAssistant, Content: "second"}))

	messages := conversation.Messages()
	require.Len(t, messages, 2)
	require.Equal(t, "first", messages[0].Content)
	require.Equal(t, firstTime, messages[0].CreatedAt)
	require.Equal(t, "second", messages[1].Content)
	require.False(t, messages[1].CreatedAt.IsZero())
}

func TestConversation_AppendUserAndAssistantSetTimestamps(t *testing.T) {
	conversation := NewConversation()

	require.NoError(t, conversation.AppendUser("hello"))
	require.NoError(t, conversation.AppendAssistant("hi"))

	messages := conversation.Messages()
	require.Len(t, messages, 2)
	require.Equal(t, RoleUser, messages[0].Role)
	require.Equal(t, RoleAssistant, messages[1].Role)
	require.False(t, messages[0].CreatedAt.IsZero())
	require.False(t, messages[1].CreatedAt.IsZero())
}

func TestConversation_AppendUserAndAssistantRejectNilReceiver(t *testing.T) {
	var conversation *Conversation

	err := conversation.AppendUser("hello")
	require.EqualError(t, err, "conversation is required")

	err = conversation.AppendAssistant("hi")
	require.EqualError(t, err, "conversation is required")
}

func TestConversation_AppendUserAndAssistantRejectEmptyContent(t *testing.T) {
	conversation := NewConversation()

	err := conversation.AppendUser("   ")
	require.EqualError(t, err, "message content is required")

	err = conversation.AppendAssistant("   ")
	require.EqualError(t, err, "message content is required")
}

func TestConversation_MessagesReturnsSafeCopy(t *testing.T) {
	conversation := NewConversation()
	require.NoError(t, conversation.AppendUser("hello"))

	messages := conversation.Messages()
	messages[0].Content = "changed"
	messages = append(messages, Message{Role: RoleAssistant, Content: "new"})

	original := conversation.Messages()
	require.Len(t, original, 1)
	require.Equal(t, "hello", original[0].Content)
}

func TestConversation_MessagesReturnsEmptySliceWhenConversationIsEmpty(t *testing.T) {
	conversation := NewConversation()

	messages := conversation.Messages()
	require.NotNil(t, messages)
	require.Empty(t, messages)
}

func TestConversation_LenAndEmptyReflectState(t *testing.T) {
	conversation := NewConversation()
	require.True(t, conversation.Empty())
	require.Equal(t, 0, conversation.Len())

	require.NoError(t, conversation.AppendUser("hello"))
	require.False(t, conversation.Empty())
	require.Equal(t, 1, conversation.Len())
}

func TestConversation_AppendRejectsInvalidMessage(t *testing.T) {
	conversation := NewConversation()

	err := conversation.Append(Message{Role: Role("invalid"), Content: "hello"})
	require.EqualError(t, err, "message role must be one of developer, user, assistant, or tool")

	err = conversation.Append(Message{Role: RoleUser, Content: "   "})
	require.EqualError(t, err, "message content is required")
}

func TestConversation_AppendRejectsNilReceiver(t *testing.T) {
	var conversation *Conversation

	err := conversation.Append(Message{Role: RoleUser, Content: "hello"})
	require.EqualError(t, err, "conversation is required")
}
