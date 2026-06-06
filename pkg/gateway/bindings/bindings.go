package bindings

import (
	"errors"
	"net/url"
	"strings"
)

const (
	SourceGeneric  = "generic"
	SourceSlack    = "slack"
	SourceTelegram = "telegram"
)

type Key string

type Parts struct {
	Source         string
	AccountID      string
	ConversationID string
	ThreadID       string
}

func Generic(conversationID string) (Key, error) {
	return NewKey(Parts{
		Source:         SourceGeneric,
		ConversationID: conversationID,
	})
}

func Slack(teamID string, channelID string, threadID string) (Key, error) {
	return NewKey(Parts{
		Source:         SourceSlack,
		AccountID:      teamID,
		ConversationID: channelID,
		ThreadID:       threadID,
	})
}

func Telegram(chatID string, topicID string) (Key, error) {
	return NewKey(Parts{
		Source:         SourceTelegram,
		ConversationID: chatID,
		ThreadID:       topicID,
	})
}

func NewKey(parts Parts) (Key, error) {
	source := strings.ToLower(strings.TrimSpace(parts.Source))
	accountID := strings.TrimSpace(parts.AccountID)
	conversationID := strings.TrimSpace(parts.ConversationID)
	threadID := strings.TrimSpace(parts.ThreadID)

	if source == "" {
		return "", errors.New("gateway binding source is required")
	}
	if conversationID == "" {
		return "", errors.New("gateway binding conversation id is required")
	}
	if source == SourceSlack && accountID == "" {
		return "", errors.New("slack team id is required")
	}

	return Key(strings.Join([]string{
		escape(source),
		escape(accountID),
		escape(conversationID),
		escape(threadID),
	}, ":")), nil
}

func (k Key) String() string {
	return string(k)
}

func escape(value string) string {
	return url.QueryEscape(value)
}
