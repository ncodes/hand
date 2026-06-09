package pairing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChallengeMessage_RendersApprovalCommandWithoutSecrets(t *testing.T) {
	message := ChallengeMessage(Challenge{
		Code: "12345678",
		Request: PendingRequest{
			Source:   "telegram",
			SenderID: "sender-secret-id",
		},
	})

	require.Contains(t, message, "hand gateway pairing approve telegram 12345678")
	require.NotContains(t, message, "sender-secret-id")
}

func TestChallengeMessage_DefaultsBlankSourceToGateway(t *testing.T) {
	message := ChallengeMessage(Challenge{Code: "12345678"})

	require.Contains(t, message, "Pair this gateway chat with Hand")
	require.Contains(t, message, "hand gateway pairing approve gateway 12345678")
}
