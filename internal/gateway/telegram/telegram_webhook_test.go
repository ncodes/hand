package telegram

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	storage "github.com/wandxy/hand/internal/state/core"
	tg "github.com/wandxy/hand/pkg/gateway/telegram"
	gatewaytypes "github.com/wandxy/hand/pkg/gateway/types"
)

func TestTelegramWebhookRejectsUnauthorizedRequestsBeforeDispatch(t *testing.T) {
	responder := &genericResponderStub{}
	handler := newWebhookHandler(telegramWebhookConfig(), responder)
	req := httptest.NewRequest(http.MethodPost, WebhookPath, bytes.NewBufferString(`{"update_id":1}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.False(t, responder.called)
	require.Equal(t, &gatewaytypes.ErrorResponse{
		Code:    gatewaytypes.ErrorCodeUnauthorized,
		Message: "unauthorized",
	}, decodeGatewayResponse(t, recorder).Error)
}

func TestTelegramWebhookRejectsUnsupportedRequests(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cfg    config.GatewayConfig
		method string
		body   string
		status int
		error  *gatewaytypes.ErrorResponse
	}{
		{
			name:   "method",
			cfg:    telegramWebhookConfig(),
			method: http.MethodGet,
			status: http.StatusMethodNotAllowed,
			error:  &gatewaytypes.ErrorResponse{Code: gatewaytypes.ErrorCodeBadRequest, Message: "method not allowed"},
		},
		{
			name:   "disabled",
			cfg:    config.GatewayConfig{},
			method: http.MethodPost,
			body:   `{}`,
			status: http.StatusNotFound,
			error:  &gatewaytypes.ErrorResponse{Code: gatewaytypes.ErrorCodeBadRequest, Message: "telegram webhook is disabled"},
		},
		{
			name:   "invalid json",
			cfg:    telegramWebhookConfig(),
			method: http.MethodPost,
			body:   `{`,
			status: http.StatusBadRequest,
			error:  &gatewaytypes.ErrorResponse{Code: gatewaytypes.ErrorCodeBadRequest, Message: "invalid request"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			responder := &genericResponderStub{}
			handler := newWebhookHandler(tt.cfg, responder)
			req := httptest.NewRequest(tt.method, WebhookPath, bytes.NewBufferString(tt.body))
			req.Header.Set(tg.WebhookSecretHeader, "secret")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			require.Equal(t, tt.status, recorder.Code)
			require.False(t, responder.called)
			require.Equal(t, tt.error, decodeGatewayResponse(t, recorder).Error)
		})
	}
}

func TestTelegramWebhookReturnsSafeErrorWhenServiceMissing(t *testing.T) {
	handler := newWebhookHandler(telegramWebhookConfig(), nil)
	req := httptest.NewRequest(http.MethodPost, WebhookPath, bytes.NewBufferString(`{"update_id":1}`))
	req.Header.Set(tg.WebhookSecretHeader, "secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, &gatewaytypes.ErrorResponse{
		Code:    gatewaytypes.ErrorCodeInternalError,
		Message: "gateway request failed",
	}, decodeGatewayResponse(t, recorder).Error)
}

func TestTelegramWebhookAcknowledgesAndDispatchesAsynchronously(t *testing.T) {
	setTelegramDraftID(t, 77)
	origNewTelegramAPI := newTelegramAPI
	t.Cleanup(func() { newTelegramAPI = origNewTelegramAPI })
	api := &fakeTelegramAPI{}
	newTelegramAPI = func(config.GatewayTelegramConfig) telegramAPI {
		return api
	}
	responder := &genericResponderStub{
		createdSession: storage.Session{ID: genericCreatedSessionID},
		reply:          "reply",
	}
	handler := newWebhookHandler(telegramWebhookConfig(), responder)
	req := httptest.NewRequest(http.MethodPost, WebhookPath, bytes.NewBufferString(`{
		"update_id": 15,
		"message": {
			"message_id": 5,
			"text": "hello",
			"chat": {"id": 123, "type": "private"}
		}
	}`))
	req.Header.Set(tg.WebhookSecretHeader, "secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "ok\n", recorder.Body.String())
	require.Eventually(t, func() bool {
		return responder.called
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, "hello", responder.message)
	require.Equal(t, []telegramAPICall{
		{method: "sendMessageDraft", target: tg.Target{ChatID: "123", ReplyToMessageID: 5, ChatType: "private"}, draftID: 77, text: "stream\n..."},
		{method: "sendMessageDraft", target: tg.Target{ChatID: "123", ReplyToMessageID: 5, ChatType: "private"}, draftID: 77, text: "stream delta\n..."},
		{method: "sendMessage", target: tg.Target{ChatID: "123", ReplyToMessageID: 5, ChatType: "private"}, text: "reply"},
	}, api.allCalls())
}

func TestTelegramWebhookAcknowledgesWhenBackgroundDispatchFails(t *testing.T) {
	origNewTelegramAPI := newTelegramAPI
	t.Cleanup(func() { newTelegramAPI = origNewTelegramAPI })
	api := &fakeTelegramAPI{sendErr: errTelegramTest}
	newTelegramAPI = func(config.GatewayTelegramConfig) telegramAPI {
		return api
	}
	responder := &genericResponderStub{
		createdSession: storage.Session{ID: genericCreatedSessionID},
		reply:          "reply",
	}
	handler := newWebhookHandler(telegramWebhookConfig(), responder)
	req := httptest.NewRequest(http.MethodPost, WebhookPath, bytes.NewBufferString(`{
		"update_id": 15,
		"message": {
			"message_id": 5,
			"text": "hello",
			"chat": {"id": -100, "type": "group"}
		}
	}`))
	req.Header.Set(tg.WebhookSecretHeader, "secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Eventually(t, func() bool {
		return responder.called
	}, time.Second, 10*time.Millisecond)
}

func TestTelegramWebhookDispatchUsesGatewayContext(t *testing.T) {
	origNewTelegramAPI := newTelegramAPI
	t.Cleanup(func() { newTelegramAPI = origNewTelegramAPI })
	newTelegramAPI = func(config.GatewayTelegramConfig) telegramAPI {
		return &fakeTelegramAPI{}
	}
	dispatchCtx, cancel := context.WithCancel(context.Background())
	cancel()
	responder := &genericResponderStub{
		createdSession: storage.Session{ID: genericCreatedSessionID},
		reply:          "reply",
	}
	handler := newWebhookHandlerWithDispatchContext(dispatchCtx, telegramWebhookConfig(), responder)
	req := httptest.NewRequest(http.MethodPost, WebhookPath, bytes.NewBufferString(`{
		"update_id": 15,
		"message": {
			"message_id": 5,
			"text": "hello",
			"chat": {"id": 123, "type": "private"}
		}
	}`))
	req.Header.Set(tg.WebhookSecretHeader, "secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Eventually(t, func() bool {
		return responder.called
	}, time.Second, 10*time.Millisecond)
	require.ErrorIs(t, responder.contextErr, context.Canceled)
}

func telegramWebhookConfig() config.GatewayConfig {
	cfg := config.GatewayConfig{}
	cfg.Telegram.Enabled = true
	cfg.Telegram.Mode = config.GatewayTelegramModeWebhook
	cfg.Telegram.BotToken = "telegram-token"
	cfg.Telegram.WebhookSecret = "secret"
	return cfg
}
