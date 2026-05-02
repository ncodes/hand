package types

import (
	"context"

	planstore "github.com/wandxy/hand/internal/environment/planstore"
	"github.com/wandxy/hand/internal/environment/process"
	sesmsg "github.com/wandxy/hand/internal/environment/sessionmessages"
	sessrc "github.com/wandxy/hand/internal/environment/sessionsearch"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/memory/episodic"
)

type Runtime interface {
	// Policy management
	FilePolicy() guardrails.FilesystemPolicy
	CommandPolicy() guardrails.CommandPolicy

	// Process management
	StartProcess(ctx context.Context, sessionID string, req process.StartRequest) (process.Info, error)
	GetProcess(sessionID string, processID string) (process.Info, error)
	ReadProcess(sessionID string, req process.ReadRequest) (process.Output, error)
	StopProcess(ctx context.Context, sessionID string, processID string) (process.Info, error)
	ListProcesses(sessionID string) []process.Info

	// Session management
	SearchSession(ctx context.Context, req sessrc.SessionSearchRequest) ([]sessrc.SessionSearchResult, error)

	// Memory management
	GetSessionMessages(ctx context.Context, req sesmsg.SessionMessagesRequest) (sesmsg.SessionMessagesResponse, error)
	SupportsMemorySearch(ctx context.Context) (bool, error)
	SearchMemory(ctx context.Context, query memory.SearchQuery) (memory.SearchResult, error)
	SupportsMemoryExtraction(ctx context.Context) (bool, error)
	ExtractEpisodes(ctx context.Context, req episodic.Request) (episodic.Result, error)

	// Plan management
	GetPlan(string) planstore.Plan
	ReplacePlan(string, planstore.Plan) (planstore.Plan, error)
	MergePlan(string, []planstore.PartialPlanStep, string, bool) (planstore.Plan, error)
	ClearPlan(string) planstore.Plan
	HydratePlan(string, planstore.Plan)
}
