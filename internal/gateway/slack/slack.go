package slack

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	gatewaysession "github.com/wandxy/hand/internal/gateway/session"
	agentcore "github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/gateway/bindings"
	"github.com/wandxy/hand/pkg/gateway/pairing"
	slack "github.com/wandxy/hand/pkg/gateway/slack"
)

type Service interface {
	gatewaysession.Service
	pairing.Store
}

type Adapter struct {
	cfg     config.GatewayConfig
	service Service
	sender  *Sender
}

func NewAdapter(cfg config.GatewayConfig, service Service, api API) *Adapter {
	return &Adapter{
		cfg:     cfg,
		service: service,
		sender:  NewSender(api),
	}
}

func (a *Adapter) DispatchInbound(ctx context.Context, inbound slack.InboundMessage) (bool, error) {
	if a == nil || a.service == nil || a.sender == nil {
		return false, errors.New("slack adapter is required")
	}
	if strings.TrimSpace(inbound.Text) == "" {
		return false, nil
	}
	log.Debug().
		Str("slack_event_id", inbound.EventID).
		Str("slack_channel_id", inbound.ChannelID).
		Str("slack_sender_id", inbound.SenderID).
		Str("slack_channel_type", inbound.Target.ChannelType).
		Msg("Slack inbound message dispatch started")

	authorized, err := a.authorize(ctx, inbound)
	if err != nil || !authorized {
		return true, err
	}

	key, err := bindings.Slack(inbound.TeamID, inbound.ChannelID, inbound.ThreadTS)
	if err != nil {
		return false, err
	}
	session, err := gatewaysession.NewResolver(a.service).Resolve(ctx, key)
	if err != nil {
		return false, err
	}

	responseTarget := getSlackResponseTarget(a.cfg.Slack.ResponseMode, inbound)
	err = a.sender.StreamTurn(ctx, responseTarget, func(onDelta func(string)) (string, error) {
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
		log.Warn().Err(err).Msg("Slack gateway dispatch failed")
		return true, err
	}

	return true, nil
}

func (a *Adapter) authorize(ctx context.Context, inbound slack.InboundMessage) (bool, error) {
	senderID := strings.TrimSpace(inbound.SenderID)
	if senderID == "" {
		return false, nil
	}
	if hasAllowedSender(a.cfg.AllowedUsers, senderID) || hasAllowedSender(a.cfg.Slack.AllowedUsers, senderID) {
		return true, nil
	}

	manager := pairing.NewManager(pairing.Options{
		Store:  a.service,
		Secret: strings.TrimSpace(a.cfg.PairingSecret),
	})
	approved, err := manager.IsApproved(ctx, bindings.SourceSlack, senderID)
	if err != nil {
		return false, err
	}
	if approved {
		return true, nil
	}
	if !isSlackPrivateTarget(inbound.Target) {
		log.Debug().
			Str("slack_sender_id", senderID).
			Str("slack_channel_type", inbound.Target.ChannelType).
			Msg("Slack sender ignored because it is not paired or allowlisted")
		return false, nil
	}

	challenge, err := manager.Request(ctx, pairing.Identity{
		Source:   bindings.SourceSlack,
		SenderID: senderID,
		Metadata: map[string]string{
			"team_id":    inbound.TeamID,
			"channel_id": inbound.ChannelID,
		},
	})
	if err != nil {
		return false, err
	}
	responseTarget := getSlackResponseTarget(a.cfg.Slack.ResponseMode, inbound)
	if err := a.sender.SendFinal(ctx, responseTarget, pairing.ChallengeMessage(challenge)); err != nil {
		return false, err
	}
	log.Debug().
		Str("slack_sender_id", senderID).
		Msg("Slack pairing challenge sent")

	return false, nil
}

func getSlackResponseTarget(responseMode string, inbound slack.InboundMessage) slack.Target {
	target := inbound.Target
	if strings.TrimSpace(strings.ToLower(responseMode)) == config.GatewaySlackResponseModeMessage &&
		!isSlackThreadReply(inbound) {
		target.ThreadTS = ""
	}

	return target
}

func isSlackThreadReply(inbound slack.InboundMessage) bool {
	threadTS := strings.TrimSpace(inbound.ThreadTS)
	messageTS := strings.TrimSpace(inbound.MessageTS)
	return threadTS != "" && messageTS != "" && threadTS != messageTS
}

func hasAllowedSender(allowed []string, senderID string) bool {
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return false
	}
	for _, allowedID := range allowed {
		if strings.TrimSpace(allowedID) == senderID {
			return true
		}
	}

	return false
}

func isSlackPrivateTarget(target slack.Target) bool {
	switch strings.TrimSpace(target.ChannelType) {
	case "im", "mpim":
		return true
	default:
		return false
	}
}
