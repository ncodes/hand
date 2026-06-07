package telegram

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckWebhookSecret(t *testing.T) {
	require.NoError(t, CheckWebhookSecret("", ""))
	require.NoError(t, CheckWebhookSecret(" secret ", "secret"))
	require.ErrorIs(t, CheckWebhookSecret("", "secret"), ErrWebhookSecretMismatch)
	require.ErrorIs(t, CheckWebhookSecret("wrong", "secret"), ErrWebhookSecretMismatch)
}
