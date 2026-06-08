package telegram

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	storage "github.com/wandxy/hand/internal/state/core"
	tg "github.com/wandxy/hand/pkg/gateway/telegram"
)

func TestTelegramAdapter_DispatchUpdateResolvesSessionAndStreamsReply(t *testing.T) {
	setTelegramDraftID(t, 77)
	api := &fakeTelegramAPI{}
	responder := &genericResponderStub{
		createdSession: storage.Session{ID: genericCreatedSessionID},
		reply:          "final reply",
	}
	adapter := newTelegramAdapter(responder, api)

	handled, err := adapter.DispatchUpdate(context.Background(), tg.Update{
		UpdateID: 7,
		Message: &tg.Message{
			MessageID: 11,
			Text:      " hello telegram ",
			Chat:      tg.Chat{ID: 123, Type: "private"},
			From:      &tg.User{ID: 9},
		},
	})

	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "hello telegram", responder.message)
	require.Equal(t, genericCreatedSessionID, responder.options.SessionID)
	require.NotNil(t, responder.options.OnEvent)
	require.Equal(t, storage.GatewayBinding{
		Key:       "telegram::123:",
		SessionID: genericCreatedSessionID,
		CreatedAt: responder.savedBinding.CreatedAt,
		UpdatedAt: responder.savedBinding.UpdatedAt,
	}, responder.savedBinding)
	require.Equal(t, []telegramAPICall{
		{method: "sendChatAction", target: tg.Target{ChatID: "123", ReplyToMessageID: 11, ChatType: "private"}, action: "typing"},
		{method: "sendMessageDraft", target: tg.Target{ChatID: "123", ReplyToMessageID: 11, ChatType: "private"}, draftID: 77, text: "stream\n..."},
		{method: "sendMessageDraft", target: tg.Target{ChatID: "123", ReplyToMessageID: 11, ChatType: "private"}, draftID: 77, text: "stream delta\n..."},
		{method: "sendMessage", target: tg.Target{ChatID: "123", ReplyToMessageID: 11, ChatType: "private"}, text: "final reply"},
	}, api.allCalls())
}

func TestTelegramAdapter_IgnoresUnsupportedUpdateWithoutCallingAgent(t *testing.T) {
	responder := &genericResponderStub{}
	adapter := newTelegramAdapter(responder, &fakeTelegramAPI{})

	handled, err := adapter.DispatchUpdate(context.Background(), tg.Update{
		UpdateID:      8,
		EditedMessage: &tg.Message{Text: "edited", Chat: tg.Chat{ID: 123}},
	})

	require.NoError(t, err)
	require.False(t, handled)
	require.False(t, responder.called)
}

func TestTelegramAdapter_RejectsMissingDependencies(t *testing.T) {
	_, err := (*TelegramAdapter)(nil).DispatchUpdate(context.Background(), tg.Update{})
	require.EqualError(t, err, "telegram adapter is required")

	_, err = newTelegramAdapter(nil, &fakeTelegramAPI{}).DispatchUpdate(context.Background(), tg.Update{})
	require.EqualError(t, err, "telegram adapter is required")
}

func TestTelegramAdapter_ReturnsSessionResolutionError(t *testing.T) {
	responder := &genericResponderStub{getBindingErr: errTelegramTest}
	adapter := newTelegramAdapter(responder, &fakeTelegramAPI{})

	handled, err := adapter.DispatchUpdate(context.Background(), tg.Update{
		UpdateID: 7,
		Message: &tg.Message{
			MessageID: 11,
			Text:      "hello",
			Chat:      tg.Chat{ID: 123, Type: "private"},
		},
	})

	require.ErrorIs(t, err, errTelegramTest)
	require.False(t, handled)
	require.False(t, responder.called)
}

func TestTelegramAdapter_ReturnsNormalizationError(t *testing.T) {
	handled, err := newTelegramAdapter(&genericResponderStub{}, &fakeTelegramAPI{}).
		DispatchUpdate(context.Background(), tg.Update{
			UpdateID: 7,
			Message:  &tg.Message{Text: "hello"},
		})

	require.ErrorIs(t, err, tg.ErrTelegramChatRequired)
	require.False(t, handled)
}

func TestTelegramAdapter_ReturnsSafeErrorWhenSenderFails(t *testing.T) {
	api := &fakeTelegramAPI{sendErr: errTelegramTest}
	responder := &genericResponderStub{
		createdSession: storage.Session{ID: genericCreatedSessionID},
		reply:          "final reply",
	}
	adapter := newTelegramAdapter(responder, api)

	handled, err := adapter.DispatchUpdate(context.Background(), tg.Update{
		UpdateID: 7,
		Message: &tg.Message{
			MessageID: 11,
			Text:      "hello",
			Chat:      tg.Chat{ID: -100, Type: "supergroup"},
			From:      &tg.User{ID: 9},
		},
	})

	require.ErrorIs(t, err, errTelegramTest)
	require.True(t, handled)
	require.True(t, responder.called)
}
