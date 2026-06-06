package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	agentcore "github.com/wandxy/hand/pkg/agent"
	gatewayauth "github.com/wandxy/hand/pkg/gateway/auth"
	"github.com/wandxy/hand/pkg/gateway/httpjson"
	gatewaytypes "github.com/wandxy/hand/pkg/gateway/types"
)

const maxGenericRespondBodyBytes = 1 << 20 // 1MB

type Responder interface {
	Respond(context.Context, string, agentcore.RespondOptions) (string, error)
}

func newHTTPHandler(cfg config.GatewayConfig, responder Responder) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/v1/respond", handleGenericRespond(cfg, responder))

	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func handleGenericRespond(cfg config.GatewayConfig, responder Responder) http.HandlerFunc {
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
		if responder == nil {
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

		sessionID := req.ConversationID
		text, err := responder.Respond(r.Context(), req.Message, agentcore.RespondOptions{
			SessionID: sessionID,
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
			SessionID:      sessionID,
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
