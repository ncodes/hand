package slack

import (
	"encoding/json"
	"errors"
	"strings"
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

	req.Type = strings.TrimSpace(req.Type)
	req.Challenge = strings.TrimSpace(req.Challenge)
	req.TeamID = strings.TrimSpace(req.TeamID)
	req.EventID = strings.TrimSpace(req.EventID)
	return req, nil
}

func NormalizeEventsRequest(req EventsRequest) (InboundMessage, bool, error) {
	if strings.TrimSpace(req.Type) != EventTypeCallback {
		return InboundMessage{}, false, nil
	}
	var event Event
	if err := json.Unmarshal(req.Event, &event); err != nil {
		return InboundMessage{}, false, err
	}

	inbound, ok, err := NormalizeEvent(strings.TrimSpace(req.TeamID), strings.TrimSpace(req.EventID), event)
	if err != nil || !ok {
		return inbound, ok, err
	}

	return inbound, true, nil
}

func NormalizeSocketEnvelope(envelope SocketEnvelope) (InboundMessage, bool, error) {
	if strings.TrimSpace(envelope.Type) != SocketTypeEventsAPI || len(envelope.Payload) == 0 {
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

	inbound.SocketID = strings.TrimSpace(envelope.EnvelopeID)
	inbound.SocketType = strings.TrimSpace(envelope.Type)
	return inbound, true, nil
}

func NormalizeEvent(teamID string, eventID string, event Event) (InboundMessage, bool, error) {
	event.Type = strings.TrimSpace(event.Type)
	if event.Type != EventMessage && event.Type != EventAppMention {
		return InboundMessage{}, false, nil
	}
	if strings.TrimSpace(event.BotID) != "" || isIgnoredMessageSubtype(event.Subtype) {
		return InboundMessage{}, false, nil
	}
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return InboundMessage{}, false, nil
	}
	channelID := strings.TrimSpace(event.Channel)
	if channelID == "" {
		return InboundMessage{}, false, ErrSlackChannelRequired
	}
	if event.Type == EventMessage && !isDirectChannel(event.ChannelType) {
		return InboundMessage{}, false, nil
	}

	teamID = firstNonEmpty(teamID, event.Team)
	messageTS := firstNonEmpty(event.TS, event.EventTS)
	threadTS := firstNonEmpty(event.ThreadTS, messageTS)
	userID := strings.TrimSpace(event.User)
	channelType := strings.TrimSpace(event.ChannelType)
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
		EventID:   strings.TrimSpace(eventID),
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
	switch strings.TrimSpace(subtype) {
	case "", "file_share", "thread_broadcast":
		return false
	default:
		return true
	}
}

func isDirectChannel(channelType string) bool {
	switch strings.TrimSpace(channelType) {
	case "im", "mpim":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}

	return ""
}
