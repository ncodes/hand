package telegram

import (
	"crypto/subtle"
	"errors"
	"strings"
)

const WebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

var ErrWebhookSecretMismatch = errors.New("telegram webhook secret mismatch")

func CheckWebhookSecret(header string, secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}

	header = strings.TrimSpace(header)
	if header == "" {
		return ErrWebhookSecretMismatch
	}

	if subtle.ConstantTimeCompare([]byte(header), []byte(secret)) != 1 {
		return ErrWebhookSecretMismatch
	}

	return nil
}
