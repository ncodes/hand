package telegram

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	gatewaysession "github.com/wandxy/hand/internal/gateway/session"
	"github.com/wandxy/hand/pkg/gateway/httpjson"
	tg "github.com/wandxy/hand/pkg/gateway/telegram"
	gatewaytypes "github.com/wandxy/hand/pkg/gateway/types"
)

const (
	WebhookPath         = "/gateway/telegram/webhook"
	maxWebhookBodyBytes = 1 << 20
)

func HandleWebhook(
	dispatchCtx context.Context,
	cfg config.GatewayTelegramConfig,
	service gatewaysession.Service,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			httpjson.WriteError(w, http.StatusMethodNotAllowed, gatewaytypes.ErrorCodeBadRequest, "method not allowed")
			return
		}
		if !cfg.Enabled || cfg.Mode != config.GatewayTelegramModeWebhook {
			httpjson.WriteError(w, http.StatusNotFound, gatewaytypes.ErrorCodeBadRequest, "telegram webhook is disabled")
			return
		}
		if err := tg.CheckWebhookSecret(r.Header.Get(tg.WebhookSecretHeader), cfg.WebhookSecret); err != nil {
			httpjson.WriteError(w, http.StatusUnauthorized, gatewaytypes.ErrorCodeUnauthorized, "unauthorized")
			return
		}
		if service == nil {
			httpjson.WriteError(w, http.StatusInternalServerError, gatewaytypes.ErrorCodeInternalError,
				"gateway request failed")
			return
		}

		var update tg.Update
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)).Decode(&update); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, gatewaytypes.ErrorCodeBadRequest, "invalid request")
			return
		}

		go func() {
			if _, err := newTelegramAdapter(service, newTelegramAPI(cfg)).
				DispatchUpdate(dispatchCtx, update); err != nil {
				log.Warn().Err(err).Msg("Telegram webhook dispatch failed")
			}
		}()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}
