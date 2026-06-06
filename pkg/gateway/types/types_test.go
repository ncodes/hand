package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRespondRequestTrimsFieldsAndDefaultsSource(t *testing.T) {
	req := NormalizeRespondRequest(RespondRequest{
		ConversationID: " chat-1 ",
		Message:        " hello ",
		UserID:         " user-1 ",
		Instruct:       " terse ",
	})

	require.Equal(t, RespondRequest{
		ConversationID: "chat-1",
		Message:        "hello",
		UserID:         "user-1",
		Source:         SourceGenericHTTP,
		Instruct:       "terse",
	}, req)
}

func TestValidateRespondRequestRequiresConversationIDAndMessage(t *testing.T) {
	require.ErrorIs(t, ValidateRespondRequest(RespondRequest{Message: "hello"}), ErrConversationIDRequired)
	require.ErrorIs(t, ValidateRespondRequest(RespondRequest{ConversationID: "chat"}), ErrMessageRequired)
	require.NoError(t, ValidateRespondRequest(RespondRequest{ConversationID: "chat", Message: "hello"}))
}

func TestNewErrorResponseTrimsFields(t *testing.T) {
	require.Equal(t, ErrorResponse{Code: "bad_request", Message: "invalid"}, NewErrorResponse(" bad_request ", " invalid "))
}
