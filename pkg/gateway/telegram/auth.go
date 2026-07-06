package telegram

import (
	"crypto/subtle"
	"errors"

	"github.com/wandxy/morph/pkg/str"
)

const WebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

var ErrWebhookSecretMismatch = errors.New("telegram webhook secret mismatch")

func CheckWebhookSecret(header string, secret string) error {
	stringValue1 := str.String(secret)
	secret = stringValue1.Trim()
	if secret == "" {
		return nil
	}
	stringValue2 := str.String(header)
	header = stringValue2.Trim()
	if header == "" {
		return ErrWebhookSecretMismatch
	}

	if subtle.ConstantTimeCompare([]byte(header), []byte(secret)) != 1 {
		return ErrWebhookSecretMismatch
	}

	return nil
}
