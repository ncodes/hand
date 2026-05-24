package core

import (
	"errors"
	"strings"

	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

const (
	MessageOrderAsc  = "asc"
	MessageOrderDesc = "desc"
)

// MessageQueryOptions controls message listing and counting filters.
type MessageQueryOptions struct {
	Archived bool
	Limit    int
	Name     string
	Order    string
	Offset   int
	Role     handmsg.Role
}

// MessageRecord pairs a message with its sequence offset.
type MessageRecord struct {
	Offset  int
	Message handmsg.Message
}

// NormalizeMessageQueryOrder validates and canonicalizes message query order.
func NormalizeMessageQueryOrder(order string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(order)) {
	case "", MessageOrderAsc:
		return MessageOrderAsc, nil
	case MessageOrderDesc:
		return MessageOrderDesc, nil
	default:
		return "", errors.New("message order must be asc or desc")
	}
}
