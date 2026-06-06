package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	agentcore "github.com/wandxy/hand/pkg/agent"
	gatewaytypes "github.com/wandxy/hand/pkg/gateway/types"
)

func TestGenericRespondRejectsMissingAndInvalidBearerToken(t *testing.T) {
	for _, tt := range []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "invalid", authorization: "Bearer wrong-token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			responder := &genericResponderStub{}
			handler := newHTTPHandler(config.GatewayConfig{AuthToken: "secret-token"}, responder)
			req := httptest.NewRequest(http.MethodPost, "/v1/respond", bytes.NewBufferString(`{"conversation_id":"default","message":"hello"}`))
			req.Header.Set("Authorization", tt.authorization)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.False(t, responder.called)
			response := decodeGatewayResponse(t, recorder)
			require.Equal(t, &gatewaytypes.ErrorResponse{
				Code:    gatewaytypes.ErrorCodeUnauthorized,
				Message: "unauthorized",
			}, response.Error)
		})
	}
}

func TestGenericRespondCallsResponderAndReturnsAssistantText(t *testing.T) {
	responder := &genericResponderStub{reply: "hello back"}
	handler := newHTTPHandler(config.GatewayConfig{AuthToken: "secret-token"}, responder)
	req := httptest.NewRequest(http.MethodPost, "/v1/respond", bytes.NewBufferString(`{
		"conversation_id":"default",
		"message":" hello ",
		"user_id":"user-1",
		"source":"generic",
		"instruct":"be brief"
	}`))
	req.Header.Set("Authorization", "Bearer secret-token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "hello", responder.message)
	require.Equal(t, agentcore.RespondOptions{SessionID: "default", Instruct: "be brief"}, responder.options)
	require.Equal(t, gatewaytypes.RespondResponse{
		ConversationID: "default",
		SessionID:      "default",
		Text:           "hello back",
	}, decodeGatewayResponse(t, recorder))
}

func TestGenericRespondRejectsInvalidRequestWithoutCallingResponder(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		message string
	}{
		{name: "empty conversation", body: `{"conversation_id":" ","message":"hello"}`, message: "conversation_id is required"},
		{name: "empty message", body: `{"conversation_id":"default","message":" "}`, message: "message is required"},
		{name: "invalid json", body: `{`, message: "invalid request"},
		{name: "unknown field", body: `{"conversation_id":"default","message":"hello","extra":true}`, message: "invalid request"},
		{name: "trailing json", body: `{"conversation_id":"default","message":"hello"} true`, message: "invalid request"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			responder := &genericResponderStub{}
			handler := newHTTPHandler(config.GatewayConfig{}, responder)
			req := httptest.NewRequest(http.MethodPost, "/v1/respond", bytes.NewBufferString(tt.body))
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.False(t, responder.called)
			response := decodeGatewayResponse(t, recorder)
			require.Equal(t, &gatewaytypes.ErrorResponse{
				Code:    gatewaytypes.ErrorCodeBadRequest,
				Message: tt.message,
			}, response.Error)
		})
	}
}

func TestGenericRespondReturnsSafeErrorWhenResponderFails(t *testing.T) {
	responder := &genericResponderStub{err: errors.New("provider stack trace: secret")}
	handler := newHTTPHandler(config.GatewayConfig{}, responder)
	req := httptest.NewRequest(http.MethodPost, "/v1/respond", bytes.NewBufferString(`{"conversation_id":"default","message":"hello"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	response := decodeGatewayResponse(t, recorder)
	require.Equal(t, &gatewaytypes.ErrorResponse{
		Code:    gatewaytypes.ErrorCodeInternalError,
		Message: "gateway request failed",
	}, response.Error)
	require.NotContains(t, recorder.Body.String(), "secret")
}

func TestGenericRespondReturnsSafeErrorWhenResponderMissing(t *testing.T) {
	handler := newHTTPHandler(config.GatewayConfig{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/respond", bytes.NewBufferString(`{"conversation_id":"default","message":"hello"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	response := decodeGatewayResponse(t, recorder)
	require.Equal(t, &gatewaytypes.ErrorResponse{
		Code:    gatewaytypes.ErrorCodeInternalError,
		Message: "gateway request failed",
	}, response.Error)
}

func TestGenericRespondRejectsUnsupportedMethod(t *testing.T) {
	handler := newHTTPHandler(config.GatewayConfig{}, &genericResponderStub{})
	req := httptest.NewRequest(http.MethodGet, "/v1/respond", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	require.Equal(t, http.MethodPost, recorder.Header().Get("Allow"))
	response := decodeGatewayResponse(t, recorder)
	require.Equal(t, &gatewaytypes.ErrorResponse{
		Code:    gatewaytypes.ErrorCodeBadRequest,
		Message: "method not allowed",
	}, response.Error)
}

type genericResponderStub struct {
	message string
	options agentcore.RespondOptions
	reply   string
	err     error
	called  bool
}

func (s *genericResponderStub) Respond(
	_ context.Context,
	message string,
	opts agentcore.RespondOptions,
) (string, error) {
	s.called = true
	s.message = message
	s.options = opts
	return s.reply, s.err
}

func decodeGatewayResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) gatewaytypes.RespondResponse {
	t.Helper()

	var response gatewaytypes.RespondResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	return response
}
