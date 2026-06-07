package telegram

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUpdate_AcceptsTextMessageWithTopic(t *testing.T) {
	msg, ok, err := NormalizeUpdate(Update{
		UpdateID: 7,
		Message: &Message{
			MessageID:       9,
			MessageThreadID: 42,
			Text:            " hello ",
			Chat:            Chat{ID: -100123, Type: "supergroup"},
			From:            &User{ID: 1},
		},
	})

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, InboundMessage{
		UpdateID:  7,
		MessageID: 9,
		Text:      "hello",
		Target: Target{
			ChatID:           "-100123",
			ThreadID:         "42",
			ReplyToMessageID: 9,
			ChatType:         "supergroup",
		},
	}, msg)
}

func TestNormalizeUpdate_IgnoresUnsupportedUpdates(t *testing.T) {
	for _, update := range []Update{
		{UpdateID: 1, EditedMessage: &Message{Text: "edited"}},
		{UpdateID: 2, CallbackQuery: map[string]any{"id": "callback"}},
		{UpdateID: 3, Message: &Message{Caption: "media", Chat: Chat{ID: 1}}},
		{UpdateID: 4, Message: &Message{Text: "bot", Chat: Chat{ID: 1}, From: &User{IsBot: true}}},
	} {
		msg, ok, err := NormalizeUpdate(update)
		require.NoError(t, err)
		require.False(t, ok)
		require.Zero(t, msg)
	}
}

func TestNormalizeUpdate_RejectsMissingChat(t *testing.T) {
	_, _, err := NormalizeUpdate(Update{Message: &Message{Text: "hello"}})

	require.ErrorIs(t, err, ErrTelegramChatRequired)
}
