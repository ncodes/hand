package core

import (
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
)

// SearchMessageOptions controls session message search filtering and limits.
type SearchMessageOptions struct {
	IgnoreSessionID       string
	MaxMessagesPerSession int
	MaxSessions           int
	Query                 string
	Role                  handmsg.Role
	ToolName              string
}

// SearchMessageHit is one matching message returned from session search.
type SearchMessageHit struct {
	SessionID       string
	Message         handmsg.Message
	MatchedText     string
	MatchedToolName string
}

// SearchMessageResult groups matching messages for one session.
type SearchMessageResult struct {
	SessionID     string
	LastMatchedAt time.Time
	MatchCount    int
	Messages      []SearchMessageHit
}
