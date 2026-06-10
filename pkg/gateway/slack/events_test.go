package slack

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeEventsRequest_TrimsEnvelopeFields(t *testing.T) {
	req, err := DecodeEventsRequest([]byte(`{
		"type": " url_verification ",
		"challenge": " challenge ",
		"team_id": " T1 ",
		"event_id": " Ev1 "
	}`))

	require.NoError(t, err)
	require.Equal(t, EventTypeURLVerification, req.Type)
	require.Equal(t, "challenge", req.Challenge)
	require.Equal(t, "T1", req.TeamID)
	require.Equal(t, "Ev1", req.EventID)
}

func TestDecodeEventsRequest_ReturnsInvalidJSONError(t *testing.T) {
	_, err := DecodeEventsRequest([]byte(`not json`))

	require.Error(t, err)
}

func TestNormalizeEventsRequest_DirectMessage(t *testing.T) {
	req := EventsRequest{
		Type:    EventTypeCallback,
		TeamID:  "T1",
		EventID: "Ev1",
		Event: mustSlackJSON(t, Event{
			Type:        EventMessage,
			Channel:     "D1",
			ChannelType: "im",
			User:        "U1",
			Text:        " hello ",
			TS:          "100.1",
		}),
	}

	inbound, ok, err := NormalizeEventsRequest(req)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Ev1", inbound.EventID)
	require.Equal(t, "T1", inbound.TeamID)
	require.Equal(t, "D1", inbound.ChannelID)
	require.Equal(t, "100.1", inbound.ThreadTS)
	require.Equal(t, "hello", inbound.Text)
	require.Equal(t, Target{
		TeamID:          "T1",
		ChannelID:       "D1",
		ThreadTS:        "100.1",
		UserID:          "U1",
		ChannelType:     "im",
		RecipientUserID: "U1",
		RecipientTeamID: "T1",
	}, inbound.Target)
}

func TestNormalizeEventsRequest_MultiPersonDirectMessage(t *testing.T) {
	req := EventsRequest{
		Type:    EventTypeCallback,
		TeamID:  "T1",
		EventID: "Ev1",
		Event: mustSlackJSON(t, Event{
			Type:        EventMessage,
			Channel:     "G1",
			ChannelType: "mpim",
			User:        "U1",
			Text:        " hello ",
			TS:          "100.1",
		}),
	}

	inbound, ok, err := NormalizeEventsRequest(req)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "G1", inbound.ChannelID)
	require.Equal(t, "mpim", inbound.Target.ChannelType)
	require.Equal(t, "U1", inbound.Target.UserID)
	require.Empty(t, inbound.Target.RecipientUserID)
	require.Empty(t, inbound.Target.RecipientTeamID)
}

func TestNormalizeEventsRequest_AppMentionInChannel(t *testing.T) {
	req := EventsRequest{
		Type:    EventTypeCallback,
		TeamID:  "T1",
		EventID: "Ev2",
		Event: mustSlackJSON(t, Event{
			Type:        EventAppMention,
			Channel:     "C1",
			ChannelType: "channel",
			User:        "U2",
			Text:        "<@BOT> hi",
			TS:          "200.1",
			ThreadTS:    "199.1",
		}),
	}

	inbound, ok, err := NormalizeEventsRequest(req)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "199.1", inbound.ThreadTS)
	require.Equal(t, "channel", inbound.Target.ChannelType)
}

func TestNormalizeEvent_IgnoresUnsupportedEvents(t *testing.T) {
	tests := []struct {
		name  string
		event Event
	}{
		{name: "unsupported type", event: Event{Type: "reaction_added", Channel: "C1", Text: "hi"}},
		{name: "bot event", event: Event{Type: EventMessage, BotID: "B1", Channel: "D1", ChannelType: "im", Text: "hi"}},
		{name: "ignored subtype", event: Event{Type: EventMessage, Subtype: "message_changed", Channel: "D1", ChannelType: "im", Text: "hi"}},
		{name: "empty text", event: Event{Type: EventMessage, Channel: "D1", ChannelType: "im"}},
		{name: "non direct message", event: Event{Type: EventMessage, Channel: "C1", ChannelType: "channel", Text: "hi"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok, err := NormalizeEvent("T1", "Ev1", tt.event)

			require.NoError(t, err)
			require.False(t, ok)
		})
	}
}

func TestNormalizeEventsRequest_IgnoresNonCallbacksAndRejectsInvalidEventJSON(t *testing.T) {
	_, ok, err := NormalizeEventsRequest(EventsRequest{Type: EventTypeURLVerification})
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = NormalizeEventsRequest(EventsRequest{
		Type:  EventTypeCallback,
		Event: mustSlackJSON(t, Event{Type: "reaction_added"}),
	})
	require.NoError(t, err)
	require.False(t, ok)

	_, _, err = NormalizeEventsRequest(EventsRequest{Type: EventTypeCallback, Event: json.RawMessage(`not json`)})
	require.Error(t, err)
}

func TestNormalizeEvent_RequiresChannel(t *testing.T) {
	_, _, err := NormalizeEvent("T1", "Ev1", Event{
		Type:        EventMessage,
		ChannelType: "im",
		Text:        "hi",
	})

	require.ErrorIs(t, err, ErrSlackChannelRequired)
}

func TestNormalizeSocketEnvelope_AddsSocketMetadata(t *testing.T) {
	envelope := SocketEnvelope{
		EnvelopeID: "env1",
		Type:       "events_api",
		Payload: mustSlackJSON(t, EventsRequest{
			Type:    EventTypeCallback,
			TeamID:  "T1",
			EventID: "Ev1",
			Event: mustSlackJSON(t, Event{
				Type:        EventMessage,
				Channel:     "D1",
				ChannelType: "im",
				User:        "U1",
				Text:        "hello",
				TS:          "100.1",
			}),
		}),
	}

	inbound, ok, err := NormalizeSocketEnvelope(envelope)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "env1", inbound.SocketID)
	require.Equal(t, "events_api", inbound.SocketType)
}

func TestNormalizeSocketEnvelope_HandlesIgnoredAndInvalidPayloads(t *testing.T) {
	_, ok, err := NormalizeSocketEnvelope(SocketEnvelope{Type: "hello"})
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = NormalizeSocketEnvelope(SocketEnvelope{Type: SocketTypeEventsAPI})
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = NormalizeSocketEnvelope(SocketEnvelope{
		Type:    SocketTypeEventsAPI,
		Payload: mustSlackJSON(t, EventsRequest{Type: EventTypeURLVerification}),
	})
	require.NoError(t, err)
	require.False(t, ok)

	_, _, err = NormalizeSocketEnvelope(SocketEnvelope{Type: SocketTypeEventsAPI, Payload: json.RawMessage(`not json`)})
	require.Error(t, err)
}

func TestFirstNonEmptyReturnsEmptyForBlankValues(t *testing.T) {
	require.Equal(t, "", firstNonEmpty(" ", ""))
}

func mustSlackJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
