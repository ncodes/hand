package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/gateway/dispatch"
	gatewaysession "github.com/wandxy/hand/internal/gateway/session"
	telegramprovider "github.com/wandxy/hand/internal/gateway/telegram"
	storage "github.com/wandxy/hand/internal/state/core"
	agentcore "github.com/wandxy/hand/pkg/agent"
	gatewayauth "github.com/wandxy/hand/pkg/gateway/auth"
	"github.com/wandxy/hand/pkg/gateway/bindings"
	"github.com/wandxy/hand/pkg/gateway/httpjson"
	gatewaytypes "github.com/wandxy/hand/pkg/gateway/types"
)

const maxGenericRespondBodyBytes = 1 << 20 // 1MB

type AgentService interface {
	Respond(context.Context, string, agentcore.RespondOptions) (string, error)
	CreateSession(context.Context, string) (storage.Session, error)
	SaveGatewayBinding(context.Context, storage.GatewayBinding) error
	GetGatewayBinding(context.Context, string) (storage.GatewayBinding, bool, error)
}

func newHTTPHandler(cfg config.GatewayConfig, service AgentService) http.Handler {
	return newHTTPHandlerWithDispatcher(cfg, service, nil)
}

func newHTTPHandlerWithDispatcher(
	cfg config.GatewayConfig,
	service AgentService,
	dispatcher *dispatch.Dispatcher,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/v1/respond", handleGenericRespond(cfg, service))
	telegramService, _ := service.(telegramprovider.Service)
	mux.HandleFunc(telegramprovider.WebhookPath, telegramprovider.HandleWebhook(cfg, telegramService, dispatcher))

	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func handleGenericRespond(cfg config.GatewayConfig, service AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			httpjson.WriteError(w, http.StatusMethodNotAllowed, gatewaytypes.ErrorCodeBadRequest, "method not allowed")
			return
		}
		if err := gatewayauth.CheckBearer(r.Header.Get("Authorization"), cfg.AuthToken); err != nil {
			httpjson.WriteError(w, http.StatusUnauthorized, gatewaytypes.ErrorCodeUnauthorized, "unauthorized")
			return
		}
		if service == nil {
			httpjson.WriteError(w, http.StatusInternalServerError, gatewaytypes.ErrorCodeInternalError,
				"gateway request failed")
			return
		}

		var req gatewaytypes.RespondRequest
		if err := decodeGenericRespondRequest(w, r, &req); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, gatewaytypes.ErrorCodeBadRequest, "invalid request")
			return
		}

		req = gatewaytypes.NormalizeRespondRequest(req)
		if err := gatewaytypes.ValidateRespondRequest(req); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, gatewaytypes.ErrorCodeBadRequest, err.Error())
			return
		}

		bindingKey, _ := bindings.Generic(req.ConversationID)

		session, err := gatewaysession.NewResolver(service).Resolve(r.Context(), bindingKey)
		if err != nil {
			log.Warn().Err(err).Str("source", req.Source).Msg("Gateway generic HTTP session resolution failed")
			httpjson.WriteError(w, http.StatusInternalServerError, gatewaytypes.ErrorCodeInternalError,
				"gateway request failed")
			return
		}

		text, err := service.Respond(r.Context(), req.Message, agentcore.RespondOptions{
			SessionID: session.ID,
			Instruct:  req.Instruct,
		})
		if err != nil {
			log.Warn().Err(err).Str("source", req.Source).Msg("Gateway generic HTTP request failed")
			httpjson.WriteError(w, http.StatusInternalServerError, gatewaytypes.ErrorCodeInternalError,
				"gateway request failed")
			return
		}

		httpjson.Write(w, http.StatusOK, gatewaytypes.RespondResponse{
			ConversationID: req.ConversationID,
			SessionID:      session.ID,
			Text:           text,
		})
	}
}

func decodeGenericRespondRequest(w http.ResponseWriter, r *http.Request, req *gatewaytypes.RespondRequest) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxGenericRespondBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(req); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain one JSON object")
	}

	return nil
}
