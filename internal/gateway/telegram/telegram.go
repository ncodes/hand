package telegram

import (
	"context"
	"errors"

	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/stringx"

	"github.com/wandxy/morph/internal/config"
	gatewaysession "github.com/wandxy/morph/internal/gateway/session"
	agentcore "github.com/wandxy/morph/pkg/agent"
	"github.com/wandxy/morph/pkg/gateway/bindings"
	"github.com/wandxy/morph/pkg/gateway/pairing"
	tg "github.com/wandxy/morph/pkg/gateway/telegram"
)

var log = logutils.Module("gateway.telegram")

type TelegramAdapter struct {
	cfg     config.GatewayConfig
	service Service
	sender  *telegramSender
}

type Service interface {
	gatewaysession.Service
	pairing.Store
}

func newTelegramAdapter(cfg config.GatewayConfig, service Service, api telegramAPI) *TelegramAdapter {
	return &TelegramAdapter{
		cfg:     cfg,
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

	authorized, err := a.authorize(ctx, inbound)
	if err != nil || !authorized {
		return true, err
	}

	key, _ := bindings.Telegram(inbound.Target.ChatID, inbound.Target.ThreadID)
	session, err := gatewaysession.NewResolver(a.service).Resolve(ctx, key)
	if err != nil {
		return false, err
	}

	stopTyping := a.sender.StartTyping(ctx, inbound.Target)
	defer stopTyping()

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

func (a *TelegramAdapter) authorize(ctx context.Context, inbound tg.InboundMessage) (bool, error) {
	senderID := stringx.String(inbound.SenderID).Trim()
	if senderID == "" && inbound.Target.ChatType == "private" {
		senderID = stringx.String(inbound.Target.ChatID).Trim()
	}
	if senderID == "" {
		return false, nil
	}

	if hasAllowedSender(a.cfg.AllowedUsers, senderID) || hasAllowedSender(a.cfg.Telegram.AllowedUsers, senderID) {
		return true, nil
	}

	manager := pairing.NewManager(pairing.Options{
		Store:  a.service,
		Secret: gatewayPairingSecret(a.cfg),
	})
	approved, err := manager.IsApproved(ctx, bindings.SourceTelegram, senderID)
	if err != nil {
		return false, err
	}
	if approved {
		return true, nil
	}

	if inbound.Target.ChatType != "private" {
		return false, nil
	}

	challenge, err := manager.Request(ctx, pairing.Identity{
		Source:      bindings.SourceTelegram,
		SenderID:    senderID,
		DisplayName: inbound.SenderName,
		Metadata: map[string]string{
			"chat_id": inbound.Target.ChatID,
		},
	})
	if err != nil {
		return false, err
	}
	if err := a.sender.SendFinal(ctx, inbound.Target, pairing.ChallengeMessage(challenge)); err != nil {
		return false, err
	}

	return false, nil
}

func hasAllowedSender(allowed []string, senderID string) bool {
	senderID = stringx.String(senderID).Trim()
	if senderID == "" {
		return false
	}

	for _, allowedID := range allowed {
		if stringx.String(allowedID).Trim() == senderID {
			return true
		}
	}

	return false
}

func gatewayPairingSecret(cfg config.GatewayConfig) string {
	return stringx.String(cfg.PairingSecret).Trim()
}
