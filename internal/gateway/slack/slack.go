package slack

import (
	"context"
	"errors"

	"github.com/wandxy/morph/pkg/logutils"
	"github.com/wandxy/morph/pkg/str"

	"github.com/wandxy/morph/internal/config"
	gatewaysession "github.com/wandxy/morph/internal/gateway/session"
	agentcore "github.com/wandxy/morph/pkg/agent"
	"github.com/wandxy/morph/pkg/gateway/bindings"
	"github.com/wandxy/morph/pkg/gateway/pairing"
	slack "github.com/wandxy/morph/pkg/gateway/slack"
)

var log = logutils.Module("gateway.slack")

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
	stringValue1 := str.String(inbound.Text)
	if stringValue1.Trim() == "" {
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
	stringValue2 := str.String(inbound.SenderID)
	senderID := stringValue2.Trim()
	if senderID == "" {
		return false, nil
	}
	if hasAllowedSender(a.cfg.AllowedUsers, senderID) || hasAllowedSender(a.cfg.Slack.AllowedUsers, senderID) {
		return true, nil
	}
	stringValue3 := str.String(a.cfg.PairingSecret)
	manager := pairing.NewManager(pairing.Options{
		Store:  a.service,
		Secret: stringValue3.Trim(),
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
	stringValue4 := str.String(responseMode)
	if stringValue4.Normalized() == config.GatewaySlackResponseModeMessage &&
		!isSlackThreadReply(inbound) {
		target.ThreadTS = ""
	}

	return target
}

func isSlackThreadReply(inbound slack.InboundMessage) bool {
	stringValue5 := str.String(inbound.ThreadTS)
	threadTS := stringValue5.Trim()
	stringValue6 := str.String(inbound.MessageTS)
	messageTS := stringValue6.Trim()
	return threadTS != "" && messageTS != "" && threadTS != messageTS
}

func hasAllowedSender(allowed []string, senderID string) bool {
	stringValue7 := str.String(senderID)
	senderID = stringValue7.Trim()
	if senderID == "" {
		return false
	}
	for _, allowedID := range allowed {
		stringValue8 := str.String(allowedID)
		if stringValue8.Trim() == senderID {
			return true
		}
	}

	return false
}

func isSlackPrivateTarget(target slack.Target) bool {
	stringValue9 := str.String(target.ChannelType)
	switch stringValue9.Trim() {
	case "im", "mpim":
		return true
	default:
		return false
	}
}
