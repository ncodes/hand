package agent

import (
	"context"

	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	agentcore "github.com/wandxy/hand/pkg/agent"
)

// ServiceAPI is the agent service surface consumed by RPC, CLI, and TUI adapters.
type ServiceAPI interface {
	Respond(context.Context, string, agentcore.RespondOptions) (string, error)
	CreateSession(context.Context, string) (storage.Session, error)
	ListSessions(context.Context) ([]storage.Session, error)
	UseSession(context.Context, string) error
	ArchiveSession(context.Context, string) error
	CurrentSession(context.Context) (storage.Session, error)
	RecallSessionSummary(context.Context, string) (storage.SessionSummary, error)
	CompactSession(context.Context, string) (agentcore.CompactSessionResult, error)
	RepairSession(context.Context, search.VectorRepairOptions) (search.VectorRepairResult, error)
	ContextStatus(context.Context, string) (agentcore.ContextStatus, error)
	GetSessionTimeline(context.Context, SessionTimelineOptions) (SessionTimeline, error)
}
