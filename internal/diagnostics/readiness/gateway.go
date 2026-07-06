package readiness

import (
	"fmt"
	"net"
	"strings"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/pkg/str"
)

func buildGatewayGroup(cfg *config.Config) Group {
	if cfg == nil {
		return Group{Name: "gateway", Checks: []Check{check("config", StatusFail, "config is required")}}
	}

	return Group{Name: "gateway", Checks: []Check{
		buildGatewayListenerCheck(cfg.Gateway),
		buildGatewayTelegramCheck(cfg.Gateway),
		buildGatewaySlackCheck(cfg.Gateway),
	}}
}

func buildGatewayListenerCheck(cfg config.GatewayConfig) Check {
	if !cfg.Enabled {
		return check("listener", StatusPass, "disabled")
	}
	stringValue1 := str.String(cfg.Address)
	address := stringValue1.Trim()
	stringValue2 := str.String(cfg.AuthToken)
	if !isReadinessLoopbackGatewayAddress(address) && stringValue2.Trim() == "" {
		return check(
			"listener",
			StatusWarn,
			fmt.Sprintf("enabled on %s:%d without gateway auth token", address, cfg.Port),
			commandAction("morph config set gateway.authToken <token>", "set gateway auth token for non-loopback binds"),
		)
	}

	auth := "loopback"
	stringValue3 := str.String(cfg.AuthToken)
	if stringValue3.Trim() != "" {
		auth = "configured"
	}
	return check("listener", StatusPass, fmt.Sprintf("enabled on %s:%d, auth=%s", address, cfg.Port, auth))
}

func buildGatewayTelegramCheck(cfg config.GatewayConfig) Check {
	tg := cfg.Telegram
	if !tg.Enabled {
		return check("telegram", StatusPass, "disabled")
	}
	stringValue4 := str.String(tg.Mode)
	mode := stringValue4.Trim()
	stringValue5 := str.String(tg.BotToken)
	if stringValue5.Trim() == "" {
		return check(
			"telegram",
			StatusWarn,
			fmt.Sprintf("enabled in %s mode without bot token", mode),
			commandAction(
				"morph config set gateway.telegram.botToken <bot-token>",
				"configure Telegram bot token",
			),
		)
	}
	stringValue6 := str.String(tg.WebhookSecret)
	if mode == config.GatewayTelegramModeWebhook && stringValue6.Trim() == "" {
		return check(
			"telegram",
			StatusWarn,
			"enabled in webhook mode without webhook secret",
			commandAction(
				"morph config set gateway.telegram.webhookSecret <secret-token>",
				"configure Telegram webhook secret token",
			),
		)
	}

	return check("telegram", StatusPass, fmt.Sprintf("enabled in %s mode, bot token configured", mode))
}

func buildGatewaySlackCheck(cfg config.GatewayConfig) Check {
	slack := cfg.Slack
	if !slack.Enabled {
		return check("slack", StatusPass, "disabled")
	}
	stringValue7 := str.String(slack.Mode)
	mode := stringValue7.Trim()
	stringValue8 := str.String(slack.BotToken)
	if stringValue8.Trim() == "" {
		return check(
			"slack",
			StatusWarn,
			fmt.Sprintf("enabled in %s mode without bot token", mode),
			commandAction("morph config set gateway.slack.botToken <bot-token>", "configure Slack bot token"),
		)
	}
	switch mode {
	case config.GatewaySlackModeSocket:
		stringValue9 := str.String(slack.AppToken)
		if stringValue9.Trim() == "" {
			return check(
				"slack",
				StatusWarn,
				"enabled in socket mode without app token",
				commandAction("morph config set gateway.slack.appToken <app-token>", "configure Slack app token"),
			)
		}
	case config.GatewaySlackModeHTTP:
		stringValue10 := str.String(slack.SigningSecret)
		if stringValue10.Trim() == "" {
			return check(
				"slack",
				StatusWarn,
				"enabled in http mode without signing secret",
				commandAction(
					"morph config set gateway.slack.signingSecret <signing-secret>",
					"configure Slack signing secret",
				),
			)
		}
	}

	return check("slack", StatusPass, fmt.Sprintf("enabled in %s mode, bot token configured", mode))
}

func isReadinessLoopbackGatewayAddress(address string) bool {
	stringValue11 := str.String(strings.Trim(address, "[]"))
	address = stringValue11.Trim()
	if address == "" || strings.EqualFold(address, "localhost") {
		return true
	}

	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}
