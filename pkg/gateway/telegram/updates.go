package telegram

import (
	"errors"
	"strconv"
	"strings"
)

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	EditedMessage *Message       `json:"edited_message,omitempty"`
	CallbackQuery map[string]any `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID       int64  `json:"message_id"`
	MessageThreadID int64  `json:"message_thread_id,omitempty"`
	Text            string `json:"text,omitempty"`
	Caption         string `json:"caption,omitempty"`
	Chat            Chat   `json:"chat"`
	From            *User  `json:"from,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type,omitempty"`
}

type User struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot,omitempty"`
}

type Target struct {
	ChatID           string
	ThreadID         string
	ReplyToMessageID int64
	ChatType         string
}

type InboundMessage struct {
	UpdateID  int64
	MessageID int64
	Text      string
	Target    Target
}

var ErrTelegramChatRequired = errors.New("telegram chat id is required")

func NormalizeUpdate(update Update) (InboundMessage, bool, error) {
	if update.Message == nil {
		return InboundMessage{}, false, nil
	}

	msg := update.Message
	if msg.From != nil && msg.From.IsBot {
		return InboundMessage{}, false, nil
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return InboundMessage{}, false, nil
	}

	chatID := strings.TrimSpace(strconv.FormatInt(msg.Chat.ID, 10))
	if chatID == "" || chatID == "0" {
		return InboundMessage{}, false, ErrTelegramChatRequired
	}

	threadID := ""
	if msg.MessageThreadID != 0 {
		threadID = strconv.FormatInt(msg.MessageThreadID, 10)
	}

	return InboundMessage{
		UpdateID:  update.UpdateID,
		MessageID: msg.MessageID,
		Text:      text,
		Target: Target{
			ChatID:           chatID,
			ThreadID:         threadID,
			ReplyToMessageID: msg.MessageID,
			ChatType:         strings.TrimSpace(msg.Chat.Type),
		},
	}, true, nil
}
