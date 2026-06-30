package telegram

import (
	"crypto/subtle"
	"errors"

	"github.com/wandxy/morph/pkg/stringx"
)

const WebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

var ErrWebhookSecretMismatch = errors.New("telegram webhook secret mismatch")

func CheckWebhookSecret(header string, secret string) error {
	secret = stringx.String(secret).Trim()
	if secret == "" {
		return nil
	}

	header = stringx.String(header).Trim()
	if header == "" {
		return ErrWebhookSecretMismatch
	}

	if subtle.ConstantTimeCompare([]byte(header), []byte(secret)) != 1 {
		return ErrWebhookSecretMismatch
	}

	return nil
}
