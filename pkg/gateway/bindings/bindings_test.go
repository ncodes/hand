package bindings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneric_BuildsStableKey(t *testing.T) {
	first, err := Generic("chat-1")
	require.NoError(t, err)

	second, err := Generic(" chat-1 ")
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, "generic::chat-1:", first.String())
}

func TestNewKey_DistinguishesSourcesForSameConversation(t *testing.T) {
	generic, err := NewKey(Parts{Source: SourceGeneric, ConversationID: "same"})
	require.NoError(t, err)

	telegram, err := NewKey(Parts{Source: SourceTelegram, ConversationID: "same"})
	require.NoError(t, err)

	require.NotEqual(t, generic, telegram)
}

func TestSlack_BuildsTeamChannelThreadKey(t *testing.T) {
	key, err := Slack("T123", "C456", "1717618842.000100")
	require.NoError(t, err)

	require.Equal(t, "slack:T123:C456:1717618842.000100", key.String())
}

func TestTelegram_BuildsChatTopicKey(t *testing.T) {
	key, err := Telegram("-100123456", "42")
	require.NoError(t, err)

	require.Equal(t, "telegram::-100123456:42", key.String())
}

func TestNewKey_EscapesSeparators(t *testing.T) {
	key, err := NewKey(Parts{
		Source:         SourceGeneric,
		AccountID:      "acct:1",
		ConversationID: "space room",
		ThreadID:       "thread/1",
	})
	require.NoError(t, err)

	require.Equal(t, "generic:acct%3A1:space+room:thread%2F1", key.String())
}

func TestNewKey_RejectsMissingRequiredParts(t *testing.T) {
	for _, tt := range []struct {
		name string
		part Parts
		err  string
	}{
		{name: "source", part: Parts{ConversationID: "chat"}, err: "gateway binding source is required"},
		{name: "conversation", part: Parts{Source: SourceGeneric}, err: "gateway binding conversation id is required"},
		{name: "slack team", part: Parts{Source: SourceSlack, ConversationID: "C123"}, err: "slack team id is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewKey(tt.part)
			require.EqualError(t, err, tt.err)
		})
	}
}
