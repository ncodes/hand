package core

import (
	"errors"

	morphmsg "github.com/wandxy/morph/pkg/agent/message"
	"github.com/wandxy/morph/pkg/stringx"
)

const (
	MessageOrderAsc  = "asc"
	MessageOrderDesc = "desc"
)

// MessageQueryOptions controls message listing and counting filters.
type MessageQueryOptions struct {
	Limit  int
	Name   string
	Order  string
	Offset int
	Role   morphmsg.Role
}

// MessageRecord pairs a message with its sequence offset.
type MessageRecord struct {
	Offset  int
	Message morphmsg.Message
}

// NormalizeMessageQueryOrder validates and canonicalizes message query order.
func NormalizeMessageQueryOrder(order string) (string, error) {
	switch stringx.String(order).Normalized() {
	case "", MessageOrderAsc:
		return MessageOrderAsc, nil
	case MessageOrderDesc:
		return MessageOrderDesc, nil
	default:
		return "", errors.New("message order must be asc or desc")
	}
}
