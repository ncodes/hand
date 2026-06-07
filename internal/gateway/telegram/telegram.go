package telegram

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"

	gatewaysession "github.com/wandxy/hand/internal/gateway/session"
	agentcore "github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/gateway/bindings"
	tg "github.com/wandxy/hand/pkg/gateway/telegram"
)

type TelegramAdapter struct {
	service gatewaysession.Service
	sender  *telegramSender
}

func newTelegramAdapter(service gatewaysession.Service, api telegramAPI) *TelegramAdapter {
	return &TelegramAdapter{
		service: service,
		sender:  newTelegramSender(api),
	}
}

func (a *TelegramAdapter) DispatchUpdate(ctx context.Context, update tg.Update) (bool, error) {
	if a == nil || a.service == nil || a.sender == nil {
		return false, errors.New("telegram adapter is required")
	}

	inbound, ok, err := tg.NormalizeUpdate(update)
	if err != nil || !ok {
		return ok, err
	}

	key, _ := bindings.Telegram(inbound.Target.ChatID, inbound.Target.ThreadID)
	session, err := gatewaysession.NewResolver(a.service).Resolve(ctx, key)
	if err != nil {
		return false, err
	}

	err = a.sender.StreamTurn(ctx, inbound.Target, func(onDelta func(string)) (string, error) {
		return a.service.Respond(ctx, inbound.Text, agentcore.RespondOptions{
			SessionID: session.ID,
			OnEvent: func(event agentcore.Event) {
				if event.Kind == agentcore.EventKindTextDelta && event.Channel == "assistant" {
					onDelta(event.Text)
				}
			},
		})
	})
	if err != nil {
		log.Warn().Err(err).Msg("Telegram gateway dispatch failed")
		return true, err
	}

	return true, nil
}
