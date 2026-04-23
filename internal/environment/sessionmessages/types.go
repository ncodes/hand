package sessionmessages

import (
	"errors"
	"slices"
)

type SessionMessagesRequest struct {
	SessionID       string `json:"session_id,omitempty"`
	AnchorMessageID uint   `json:"anchor_message_id,omitempty"`
	MessageIDs      []uint `json:"message_ids,omitempty"`
	OffsetStart     *int   `json:"offset_start,omitempty"`
	OffsetEnd       *int   `json:"offset_end,omitempty"`
	Before          int    `json:"before,omitempty"`
	After           int    `json:"after,omitempty"`
	MaxChars        int    `json:"max_chars,omitempty"`
}

type SessionMessagesResponse struct {
	SessionID string                 `json:"session_id"`
	Messages  []SessionMessageRecord `json:"messages"`
	Truncated bool                   `json:"truncated,omitempty"`
}

type SessionMessageRecord struct {
	MessageID  uint                    `json:"message_id"`
	Offset     int                     `json:"offset"`
	Role       string                  `json:"role"`
	Name       string                  `json:"name,omitempty"`
	ToolName   string                  `json:"tool_name,omitempty"`
	ToolCallID string                  `json:"tool_call_id,omitempty"`
	ToolCalls  []SessionToolCallRecord `json:"tool_calls,omitempty"`
	CreatedAt  string                  `json:"created_at"`
	Content    string                  `json:"content"`
	Truncated  bool                    `json:"truncated,omitempty"`
}

type SessionToolCallRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Input     string `json:"input"`
	Truncated bool   `json:"truncated,omitempty"`
}

type SessionMessagesSelector string

const (
	SessionMessagesSelectorMessageIDs  SessionMessagesSelector = "message_ids"
	SessionMessagesSelectorAnchor      SessionMessagesSelector = "anchor_message_id"
	SessionMessagesSelectorOffsetRange SessionMessagesSelector = "offset_range"
)

func (r SessionMessagesRequest) Selector() (SessionMessagesSelector, error) {
	hasMessageIDs := len(r.MessageIDs) > 0
	hasAnchor := r.AnchorMessageID > 0
	hasOffsetRange := r.OffsetStart != nil || r.OffsetEnd != nil

	selectorCount := 0
	if hasMessageIDs {
		selectorCount++
	}
	if hasAnchor {
		selectorCount++
	}
	if hasOffsetRange {
		selectorCount++
	}

	if selectorCount != 1 {
		return "", errors.New("exactly one session message selector must be provided")
	}

	switch {
	case hasMessageIDs:
		return SessionMessagesSelectorMessageIDs, nil
	case hasAnchor:
		return SessionMessagesSelectorAnchor, nil
	default:
		return SessionMessagesSelectorOffsetRange, nil
	}
}

func (r SessionMessagesRequest) Validate() error {
	selector, err := r.Selector()
	if err != nil {
		return err
	}

	if r.MaxChars < 0 {
		return errors.New("max_chars must be greater than or equal to zero")
	}

	switch selector {
	case SessionMessagesSelectorMessageIDs:
		if r.Before != 0 || r.After != 0 {
			return errors.New("before and after are only supported with anchor_message_id")
		}
		if slices.Contains(r.MessageIDs, 0) {
			return errors.New("message_ids must contain only positive ids")
		}
	case SessionMessagesSelectorAnchor:
		if r.Before < 0 || r.After < 0 {
			return errors.New("before and after must be greater than or equal to zero")
		}
	case SessionMessagesSelectorOffsetRange:
		if r.Before != 0 || r.After != 0 {
			return errors.New("before and after are only supported with anchor_message_id")
		}
		if r.OffsetStart == nil || r.OffsetEnd == nil {
			return errors.New("offset_start and offset_end are required together")
		}
		if *r.OffsetStart < 0 {
			return errors.New("offset_start must be greater than or equal to zero")
		}
		if *r.OffsetEnd <= *r.OffsetStart {
			return errors.New("offset_end must be greater than offset_start")
		}
	}

	return nil
}
