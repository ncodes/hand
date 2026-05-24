package host

import "github.com/wandxy/hand/internal/agent"

const (
	EventKindTextDelta = agent.EventKindTextDelta
	EventKindTrace     = agent.EventKindTrace
)

type ServiceAPI = agent.ServiceAPI

type RespondOptions = agent.RespondOptions

type Event = agent.Event

type CompactSessionResult = agent.CompactSessionResult

type RepairSessionOptions = agent.RepairSessionOptions

type RepairSessionResult = agent.RepairSessionResult

type ContextStatus = agent.ContextStatus

type SessionTimelineOptions = agent.SessionTimelineOptions

type SessionTimeline = agent.SessionTimeline

type SessionTimelineMessage = agent.SessionTimelineMessage

type SessionTimelineTraceEvent = agent.SessionTimelineTraceEvent
