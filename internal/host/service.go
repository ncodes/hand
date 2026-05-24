package host

import (
	"context"

	storage "github.com/wandxy/hand/internal/state/core"
	agentcore "github.com/wandxy/hand/pkg/agent"
)

type ServiceAPI interface {
	Respond(context.Context, string, agentcore.RespondOptions) (string, error)
	CreateSession(context.Context, string) (storage.Session, error)
	ListSessions(context.Context) ([]storage.Session, error)
	UseSession(context.Context, string) error
	CurrentSession(context.Context) (storage.Session, error)
	RecallSessionSummary(context.Context, string) (storage.SessionSummary, error)
	CompactSession(context.Context, string) (agentcore.CompactSessionResult, error)
	RepairSession(context.Context, RepairSessionOptions) (RepairSessionResult, error)
	ContextStatus(context.Context, string) (agentcore.ContextStatus, error)
	GetSessionTimeline(context.Context, agentcore.SessionTimelineOptions) (agentcore.SessionTimeline, error)
}
