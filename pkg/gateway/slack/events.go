package slack

import (
	"encoding/json"
	"errors"

	"github.com/wandxy/morph/pkg/stringx"
)

const (
	EventTypeURLVerification = "url_verification"
	EventTypeCallback        = "event_callback"
	EventMessage             = "message"
	EventAppMention          = "app_mention"
	SocketTypeEventsAPI      = "events_api"
)

var ErrSlackChannelRequired = errors.New("slack channel id is required")

type EventsRequest struct {
	Type      string          `json:"type"`
	Token     string          `json:"token,omitempty"`
	Challenge string          `json:"challenge,omitempty"`
	TeamID    string          `json:"team_id,omitempty"`
	EventID   string          `json:"event_id,omitempty"`
	EventTime int64           `json:"event_time,omitempty"`
	Event     json.RawMessage `json:"event,omitempty"`
}

type Event struct {
	Type        string `json:"type"`
	Subtype     string `json:"subtype,omitempty"`
	Team        string `json:"team,omitempty"`
	Channel     string `json:"channel,omitempty"`
	ChannelType string `json:"channel_type,omitempty"`
	User        string `json:"user,omitempty"`
	Text        string `json:"text,omitempty"`
	TS          string `json:"ts,omitempty"`
	ThreadTS    string `json:"thread_ts,omitempty"`
	EventTS     string `json:"event_ts,omitempty"`
	BotID       string `json:"bot_id,omitempty"`
}

type SocketEnvelope struct {
	EnvelopeID string          `json:"envelope_id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
}

type SocketAck struct {
	EnvelopeID string `json:"envelope_id"`
}

type Target struct {
	TeamID          string
	ChannelID       string
	ThreadTS        string
	UserID          string
	ChannelType     string
	RecipientUserID string
	RecipientTeamID string
}

type InboundMessage struct {
	EventID    string
	TeamID     string
	ChannelID  string
	ThreadTS   string
	MessageTS  string
	Text       string
	SenderID   string
	Target     Target
	SocketID   string
	SocketType string
}

func DecodeEventsRequest(body []byte) (EventsRequest, error) {
	var req EventsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return EventsRequest{}, err
	}

	req.Type = stringx.String(req.Type).Trim()
	req.Challenge = stringx.String(req.Challenge).Trim()
	req.TeamID = stringx.String(req.TeamID).Trim()
	req.EventID = stringx.String(req.EventID).Trim()
	return req, nil
}

func NormalizeEventsRequest(req EventsRequest) (InboundMessage, bool, error) {
	if stringx.String(req.Type).Trim() != EventTypeCallback {
		return InboundMessage{}, false, nil
	}
	var event Event
	if err := json.Unmarshal(req.Event, &event); err != nil {
		return InboundMessage{}, false, err
	}

	inbound, ok, err := NormalizeEvent(stringx.String(req.TeamID).Trim(), stringx.String(req.EventID).Trim(), event)
	if err != nil || !ok {
		return inbound, ok, err
	}

	return inbound, true, nil
}

func NormalizeSocketEnvelope(envelope SocketEnvelope) (InboundMessage, bool, error) {
	if stringx.String(envelope.Type).Trim() != SocketTypeEventsAPI || len(envelope.Payload) == 0 {
		return InboundMessage{}, false, nil
	}

	var req EventsRequest
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return InboundMessage{}, false, err
	}
	inbound, ok, err := NormalizeEventsRequest(req)
	if err != nil || !ok {
		return inbound, ok, err
	}

	inbound.SocketID = stringx.String(envelope.EnvelopeID).Trim()
	inbound.SocketType = stringx.String(envelope.Type).Trim()
	return inbound, true, nil
}

func NormalizeEvent(teamID string, eventID string, event Event) (InboundMessage, bool, error) {
	event.Type = stringx.String(event.Type).Trim()
	if event.Type != EventMessage && event.Type != EventAppMention {
		return InboundMessage{}, false, nil
	}
	if stringx.String(event.BotID).Trim() != "" || isIgnoredMessageSubtype(event.Subtype) {
		return InboundMessage{}, false, nil
	}
	text := stringx.String(event.Text).Trim()
	if text == "" {
		return InboundMessage{}, false, nil
	}
	channelID := stringx.String(event.Channel).Trim()
	if channelID == "" {
		return InboundMessage{}, false, ErrSlackChannelRequired
	}
	if event.Type == EventMessage && !isDirectChannel(event.ChannelType) {
		return InboundMessage{}, false, nil
	}

	teamID = firstNonEmpty(teamID, event.Team)
	messageTS := firstNonEmpty(event.TS, event.EventTS)
	threadTS := firstNonEmpty(event.ThreadTS, messageTS)
	userID := stringx.String(event.User).Trim()
	channelType := stringx.String(event.ChannelType).Trim()
	target := Target{
		TeamID:      teamID,
		ChannelID:   channelID,
		ThreadTS:    threadTS,
		UserID:      userID,
		ChannelType: channelType,
	}
	if channelType == "im" {
		target.RecipientUserID = userID
		target.RecipientTeamID = teamID
	}

	return InboundMessage{
		EventID:   stringx.String(eventID).Trim(),
		TeamID:    teamID,
		ChannelID: channelID,
		ThreadTS:  threadTS,
		MessageTS: messageTS,
		Text:      text,
		SenderID:  userID,
		Target:    target,
	}, true, nil
}

func isIgnoredMessageSubtype(subtype string) bool {
	switch stringx.String(subtype).Trim() {
	case "", "file_share", "thread_broadcast":
		return false
	default:
		return true
	}
}

func isDirectChannel(channelType string) bool {
	switch stringx.String(channelType).Trim() {
	case "im", "mpim":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = stringx.String(value).Trim(); value != "" {
			return value
		}
	}

	return ""
}
