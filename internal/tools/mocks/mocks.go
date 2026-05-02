package mocks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	processenv "github.com/wandxy/hand/internal/environment/process"
	envsessionmessages "github.com/wandxy/hand/internal/environment/sessionmessages"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/memory/episodic"
	"github.com/wandxy/hand/internal/tools"
)

type Runtime struct {
	FilePolicyValue              guardrails.FilesystemPolicy
	CommandPolicyValue           guardrails.CommandPolicy
	StartProcessFunc             func(context.Context, string, processenv.StartRequest) (processenv.Info, error)
	GetProcessFunc               func(string, string) (processenv.Info, error)
	ReadProcessFunc              func(string, processenv.ReadRequest) (processenv.Output, error)
	StopProcessFunc              func(context.Context, string, string) (processenv.Info, error)
	ListProcessesFunc            func(string) []processenv.Info
	SearchSessionFunc            func(context.Context, envtypes.SessionSearchRequest) ([]envtypes.SessionSearchResult, error)
	GetSessionMessagesFunc       func(context.Context, envsessionmessages.SessionMessagesRequest) (envsessionmessages.SessionMessagesResponse, error)
	SupportsMemorySearchFunc     func(context.Context) (bool, error)
	SearchMemoryFunc             func(context.Context, memory.SearchQuery) (memory.SearchResult, error)
	SupportsMemoryExtractionFunc func(context.Context) (bool, error)
	ExtractEpisodesFunc          func(context.Context, episodic.Request) (episodic.Result, error)
}

func (r *Runtime) FilePolicy() guardrails.FilesystemPolicy { return r.FilePolicyValue }
func (r *Runtime) CommandPolicy() guardrails.CommandPolicy { return r.CommandPolicyValue }
func (r *Runtime) StartProcess(ctx context.Context, sessionID string, req processenv.StartRequest) (processenv.Info, error) {
	if r != nil && r.StartProcessFunc != nil {
		return r.StartProcessFunc(ctx, sessionID, req)
	}
	return processenv.Info{}, nil
}
func (r *Runtime) GetProcess(sessionID string, processID string) (processenv.Info, error) {
	if r != nil && r.GetProcessFunc != nil {
		return r.GetProcessFunc(sessionID, processID)
	}
	return processenv.Info{}, nil
}
func (r *Runtime) ReadProcess(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
	if r != nil && r.ReadProcessFunc != nil {
		return r.ReadProcessFunc(sessionID, req)
	}
	return processenv.Output{}, nil
}
func (r *Runtime) StopProcess(ctx context.Context, sessionID string, processID string) (processenv.Info, error) {
	if r != nil && r.StopProcessFunc != nil {
		return r.StopProcessFunc(ctx, sessionID, processID)
	}
	return processenv.Info{}, nil
}
func (r *Runtime) ListProcesses(sessionID string) []processenv.Info {
	if r != nil && r.ListProcessesFunc != nil {
		return r.ListProcessesFunc(sessionID)
	}
	return nil
}
func (r *Runtime) SearchSession(ctx context.Context, req envtypes.SessionSearchRequest) ([]envtypes.SessionSearchResult, error) {
	if r != nil && r.SearchSessionFunc != nil {
		return r.SearchSessionFunc(ctx, req)
	}
	return nil, nil
}
func (r *Runtime) GetSessionMessages(ctx context.Context, req envsessionmessages.SessionMessagesRequest) (envsessionmessages.SessionMessagesResponse, error) {
	if r != nil && r.GetSessionMessagesFunc != nil {
		return r.GetSessionMessagesFunc(ctx, req)
	}
	return envsessionmessages.SessionMessagesResponse{}, nil
}
func (r *Runtime) SupportsMemorySearch(ctx context.Context) (bool, error) {
	if r != nil && r.SupportsMemorySearchFunc != nil {
		return r.SupportsMemorySearchFunc(ctx)
	}
	return false, nil
}
func (r *Runtime) SearchMemory(ctx context.Context, query memory.SearchQuery) (memory.SearchResult, error) {
	if r != nil && r.SearchMemoryFunc != nil {
		return r.SearchMemoryFunc(ctx, query)
	}
	return memory.SearchResult{}, nil
}
func (r *Runtime) SupportsMemoryExtraction(ctx context.Context) (bool, error) {
	if r != nil && r.SupportsMemoryExtractionFunc != nil {
		return r.SupportsMemoryExtractionFunc(ctx)
	}
	return false, nil
}
func (r *Runtime) ExtractEpisodes(ctx context.Context, req episodic.Request) (episodic.Result, error) {
	if r != nil && r.ExtractEpisodesFunc != nil {
		return r.ExtractEpisodesFunc(ctx, req)
	}
	return episodic.Result{}, nil
}
func (r *Runtime) GetPlan(string) envtypes.Plan { return envtypes.Plan{} }
func (r *Runtime) ReplacePlan(string, envtypes.Plan) (envtypes.Plan, error) {
	return envtypes.Plan{}, nil
}
func (r *Runtime) MergePlan(string, []envtypes.PartialPlanStep, string, bool) (envtypes.Plan, error) {
	return envtypes.Plan{}, nil
}
func (r *Runtime) ClearPlan(string) envtypes.Plan    { return envtypes.Plan{} }
func (r *Runtime) HydratePlan(string, envtypes.Plan) {}

func NewRuntime(root string, policy guardrails.CommandPolicy) *Runtime {
	return &Runtime{
		FilePolicyValue:    guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots([]string{root})},
		CommandPolicyValue: policy.Normalize(),
	}
}

func RegisterRuntime(
	t *testing.T,
	root string,
	policy guardrails.CommandPolicy,
	definitions ...func(envtypes.Runtime) tools.Definition,
) tools.Registry {
	t.Helper()

	registry := tools.NewInMemoryRegistry()
	runtime := NewRuntime(root, policy)

	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	for _, definition := range definitions {
		require.NoError(t, registry.Register(definition(runtime)))
	}

	return registry
}

func QuoteJSON(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

type FailingPlanRuntime struct {
	Runtime    envtypes.Runtime
	MergeErr   error
	ReplaceErr error
}

func (d *FailingPlanRuntime) FilePolicy() guardrails.FilesystemPolicy { return d.Runtime.FilePolicy() }
func (d *FailingPlanRuntime) CommandPolicy() guardrails.CommandPolicy {
	return d.Runtime.CommandPolicy()
}
func (d *FailingPlanRuntime) StartProcess(ctx context.Context, sessionID string, req processenv.StartRequest) (processenv.Info, error) {
	return d.Runtime.StartProcess(ctx, sessionID, req)
}
func (d *FailingPlanRuntime) GetProcess(sessionID string, processID string) (processenv.Info, error) {
	return d.Runtime.GetProcess(sessionID, processID)
}
func (d *FailingPlanRuntime) ReadProcess(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
	return d.Runtime.ReadProcess(sessionID, req)
}
func (d *FailingPlanRuntime) StopProcess(ctx context.Context, sessionID string, processID string) (processenv.Info, error) {
	return d.Runtime.StopProcess(ctx, sessionID, processID)
}
func (d *FailingPlanRuntime) ListProcesses(sessionID string) []processenv.Info {
	return d.Runtime.ListProcesses(sessionID)
}
func (d *FailingPlanRuntime) SearchSession(ctx context.Context, req envtypes.SessionSearchRequest) ([]envtypes.SessionSearchResult, error) {
	return d.Runtime.SearchSession(ctx, req)
}
func (d *FailingPlanRuntime) GetSessionMessages(ctx context.Context, req envsessionmessages.SessionMessagesRequest) (envsessionmessages.SessionMessagesResponse, error) {
	return d.Runtime.GetSessionMessages(ctx, req)
}
func (d *FailingPlanRuntime) SupportsMemorySearch(ctx context.Context) (bool, error) {
	return d.Runtime.SupportsMemorySearch(ctx)
}
func (d *FailingPlanRuntime) SearchMemory(ctx context.Context, query memory.SearchQuery) (memory.SearchResult, error) {
	return d.Runtime.SearchMemory(ctx, query)
}
func (d *FailingPlanRuntime) SupportsMemoryExtraction(ctx context.Context) (bool, error) {
	return d.Runtime.SupportsMemoryExtraction(ctx)
}
func (d *FailingPlanRuntime) ExtractEpisodes(ctx context.Context, req episodic.Request) (episodic.Result, error) {
	return d.Runtime.ExtractEpisodes(ctx, req)
}
func (d *FailingPlanRuntime) GetPlan(sessionID string) envtypes.Plan {
	return d.Runtime.GetPlan(sessionID)
}
func (d *FailingPlanRuntime) ReplacePlan(sessionID string, plan envtypes.Plan) (envtypes.Plan, error) {
	if d.ReplaceErr != nil {
		return envtypes.Plan{}, d.ReplaceErr
	}
	return d.Runtime.ReplacePlan(sessionID, plan)
}
func (d *FailingPlanRuntime) MergePlan(sessionID string, updates []envtypes.PartialPlanStep, explanation string, clearCompleted bool) (envtypes.Plan, error) {
	if d.MergeErr != nil {
		return envtypes.Plan{}, d.MergeErr
	}
	return d.Runtime.MergePlan(sessionID, updates, explanation, clearCompleted)
}
func (d *FailingPlanRuntime) ClearPlan(sessionID string) envtypes.Plan {
	return d.Runtime.ClearPlan(sessionID)
}
func (d *FailingPlanRuntime) HydratePlan(sessionID string, plan envtypes.Plan) {
	d.Runtime.HydratePlan(sessionID, plan)
}

type RecordedEvent struct {
	Type    string
	Payload any
}

type TraceRecorder struct {
	Events []RecordedEvent
}

func (s *TraceRecorder) Record(eventType string, payload any) {
	s.Events = append(s.Events, RecordedEvent{Type: eventType, Payload: payload})
}
