package agent

import (
	"context"

	models "github.com/wandxy/hand/internal/model"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	agentcore "github.com/wandxy/hand/pkg/agent"
)

type ModelList struct {
	Provider string
	AuthType string
	Models   []models.Option
}

type ModelListOptions struct {
	Provider string
}

type ModelSelectOptions struct {
	Provider string
}

type ProviderList struct {
	Providers []models.ProviderOption
}

// ServiceAPI is the agent service surface consumed by RPC, CLI, and TUI adapters.
type ServiceAPI interface {
	Respond(context.Context, string, agentcore.RespondOptions) (string, error)
	ListProviders(context.Context) (ProviderList, error)
	ListModels(context.Context, ...ModelListOptions) (ModelList, error)
	SelectModel(context.Context, string, ...ModelSelectOptions) (models.Option, error)
	SetProviderAPIKey(context.Context, string, string) error
	CreateSession(context.Context, string) (storage.Session, error)
	SaveGatewayBinding(context.Context, storage.GatewayBinding) error
	GetGatewayBinding(context.Context, string) (storage.GatewayBinding, bool, error)
	ListSessions(context.Context, ...storage.SessionListOptions) ([]storage.Session, error)
	UseSession(context.Context, string) error
	ArchiveSession(context.Context, string) error
	UnarchiveSession(context.Context, string) (storage.Session, error)
	RenameSession(context.Context, string, string) (storage.Session, error)
	CurrentSession(context.Context) (storage.Session, error)
	RecallSessionSummary(context.Context, string) (storage.SessionSummary, error)
	CompactSession(context.Context, string) (agentcore.CompactSessionResult, error)
	RepairSession(context.Context, search.VectorRepairOptions) (search.VectorRepairResult, error)
	ContextStatus(context.Context, string) (agentcore.ContextStatus, error)
	GetSessionTimeline(context.Context, SessionTimelineOptions) (SessionTimeline, error)
}
