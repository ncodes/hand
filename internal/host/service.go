package host

import (
	"context"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	"github.com/wandxy/hand/internal/trace"
)

const (
	EventKindTextDelta = "text_delta"
	EventKindTrace     = "trace_event"
)

type ServiceAPI interface {
	Respond(context.Context, string, RespondOptions) (string, error)
	CreateSession(context.Context, string) (storage.Session, error)
	ListSessions(context.Context) ([]storage.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (storage.Session, error)
	RecallSessionSummary(context.Context, string) (storage.SessionSummary, error)
	CompactSession(context.Context, string) (CompactSessionResult, error)
	RepairSession(context.Context, RepairSessionOptions) (RepairSessionResult, error)
	ContextStatus(context.Context, string) (ContextStatus, error)
	GetSessionTimeline(context.Context, SessionTimelineOptions) (SessionTimeline, error)
}

type RespondOptions struct {
	Instruct     string
	SessionID    string
	Stream       *bool
	OnEvent      func(Event)
	OnTraceEvent func(trace.Event)
}

type Event struct {
	Kind       string
	Channel    string
	Text       string
	TraceEvent *trace.Event
}

type CompactSessionResult struct {
	SessionID            string
	SourceEndOffset      int
	SourceMessageCount   int
	UpdatedAt            time.Time
	CurrentContextLength int
	TotalContextLength   int
}

type RepairSessionOptions = search.VectorRepairOptions

type RepairSessionResult = search.VectorRepairResult

type ContextStatus struct {
	SessionID        string
	Offset           int
	Size             int
	Length           int
	Used             int
	Remaining        int
	UsedPct          float64
	RemainingPct     float64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompactionStatus string
}

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
	Message handmsg.Message
	Offset  int
}

type SessionTimelineTraceEvent struct {
	Event storage.TraceEvent
}
