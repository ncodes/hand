package agent

import (
	"github.com/wandxy/hand/pkg/agent/message"
	"github.com/wandxy/hand/pkg/agent/session"
)

type SessionTimelineOptions struct {
	SessionID     string
	MessageOffset int
	MessageLimit  int
	TraceOffset   int
	TraceLimit    int
}

type SessionTimeline struct {
	SessionID             string
	Title                 string
	TitleSource           string
	Messages              []SessionTimelineMessage
	TraceEvents           []SessionTimelineTraceEvent
	MessagesHasMore       bool
	TracesHasMore         bool
	TracesTruncatedBefore bool
	FirstTraceSequence    int
	LastTraceSequence     int
}

type SessionTimelineMessage struct {
	Message message.Message
	Offset  int
}

type SessionTimelineTraceEvent struct {
	Event session.TraceEvent
}
