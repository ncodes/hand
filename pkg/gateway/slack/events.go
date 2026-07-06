package slack

import (
	"encoding/json"
	"errors"

	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(req.Type)
	req.Type = stringValue1.Trim()
	stringValue2 := str.String(req.Challenge)
	req.Challenge = stringValue2.Trim()
	stringValue3 := str.String(req.TeamID)
	req.TeamID = stringValue3.Trim()
	stringValue4 := str.String(req.EventID)
	req.EventID = stringValue4.Trim()
	return req, nil
}

func NormalizeEventsRequest(req EventsRequest) (InboundMessage, bool, error) {
	stringValue5 := str.String(req.Type)
	if stringValue5.Trim() != EventTypeCallback {
		return InboundMessage{}, false, nil
	}
	var event Event
	if err := json.Unmarshal(req.Event, &event); err != nil {
		return InboundMessage{}, false, err
	}
	stringValue6 := str.String(req.TeamID)
	stringValue7 := str.String(req.EventID)
	inbound, ok, err := NormalizeEvent(stringValue6.Trim(), stringValue7.Trim(), event)
	if err != nil || !ok {
		return inbound, ok, err
	}

	return inbound, true, nil
}

func NormalizeSocketEnvelope(envelope SocketEnvelope) (InboundMessage, bool, error) {
	stringValue8 := str.String(envelope.Type)
	if stringValue8.Trim() != SocketTypeEventsAPI || len(envelope.Payload) == 0 {
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
	stringValue9 := str.String(envelope.EnvelopeID)
	inbound.SocketID = stringValue9.Trim()
	stringValue10 := str.String(envelope.Type)
	inbound.SocketType = stringValue10.Trim()
	return inbound, true, nil
}

func NormalizeEvent(teamID string, eventID string, event Event) (InboundMessage, bool, error) {
	stringValue11 := str.String(event.Type)
	event.Type = stringValue11.Trim()
	if event.Type != EventMessage && event.Type != EventAppMention {
		return InboundMessage{}, false, nil
	}
	stringValue12 := str.String(event.BotID)
	if stringValue12.Trim() != "" || isIgnoredMessageSubtype(event.Subtype) {
		return InboundMessage{}, false, nil
	}
	stringValue13 := str.String(event.Text)
	text := stringValue13.Trim()
	if text == "" {
		return InboundMessage{}, false, nil
	}
	stringValue14 := str.String(event.Channel)
	channelID := stringValue14.Trim()
	if channelID == "" {
		return InboundMessage{}, false, ErrSlackChannelRequired
	}
	if event.Type == EventMessage && !isDirectChannel(event.ChannelType) {
		return InboundMessage{}, false, nil
	}

	teamID = firstNonEmpty(teamID, event.Team)
	messageTS := firstNonEmpty(event.TS, event.EventTS)
	threadTS := firstNonEmpty(event.ThreadTS, messageTS)
	stringValue15 := str.String(event.User)
	userID := stringValue15.Trim()
	stringValue16 := str.String(event.ChannelType)
	channelType := stringValue16.Trim()
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
	stringValue17 := str.String(eventID)
	return InboundMessage{
		EventID:   stringValue17.Trim(),
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
	stringValue18 := str.String(subtype)
	switch stringValue18.Trim() {
	case "", "file_share", "thread_broadcast":
		return false
	default:
		return true
	}
}

func isDirectChannel(channelType string) bool {
	stringValue19 := str.String(channelType)
	switch stringValue19.Trim() {
	case "im", "mpim":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		stringValue20 := str.String(value)
		if value = stringValue20.Trim(); value != "" {
			return value
		}
	}

	return ""
}
