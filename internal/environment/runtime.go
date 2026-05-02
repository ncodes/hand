package environment

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/wandxy/hand/internal/environment/planstore"
	"github.com/wandxy/hand/internal/environment/process"
	"github.com/wandxy/hand/internal/environment/sessionmessages"
	"github.com/wandxy/hand/internal/environment/sessionsearch"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/memory/episodic"
	statemanager "github.com/wandxy/hand/internal/state/manager"
)

var getwd = os.Getwd

type Runtime struct {
	filePolicy    guardrails.FilesystemPolicy
	commandPolicy guardrails.CommandPolicy
	processMgr    process.Manager
	plans         planstore.Store
	stateMgr      *statemanager.Manager
	memory        memory.Provider
}

func NewRuntime(roots []string, policy guardrails.CommandPolicy, stateMgr *statemanager.Manager) *Runtime {
	if len(roots) == 0 {
		cwd, err := getwd()
		if err != nil {
			cwd = "."
		}
		roots = []string{filepath.Clean(cwd)}
	}

	return &Runtime{
		filePolicy:    guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots(roots)},
		commandPolicy: policy.Normalize(),
		processMgr:    &process.DefaultManager{},
		plans:         &planstore.MemoryPlanStore{},
		stateMgr:      stateMgr,
	}
}

func (r *Runtime) FilePolicy() guardrails.FilesystemPolicy {
	if r == nil {
		return guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots(nil)}
	}
	return r.filePolicy
}

func (r *Runtime) CommandPolicy() guardrails.CommandPolicy {
	if r == nil {
		return guardrails.CommandPolicy{}.Normalize()
	}
	return r.commandPolicy
}

func (r *Runtime) StartProcess(
	ctx context.Context,
	sessionID string,
	req process.StartRequest,
) (process.Info, error) {
	if r == nil || r.processMgr == nil {
		return process.Info{}, errors.New("process manager is required")
	}
	return r.processMgr.Start(ctx, sessionID, req)
}

func (r *Runtime) GetProcess(sessionID string, processID string) (process.Info, error) {
	if r == nil || r.processMgr == nil {
		return process.Info{}, errors.New("process manager is required")
	}
	return r.processMgr.Get(sessionID, processID)
}

func (r *Runtime) ReadProcess(sessionID string, req process.ReadRequest) (process.Output, error) {
	if r == nil || r.processMgr == nil {
		return process.Output{}, errors.New("process manager is required")
	}
	return r.processMgr.Read(sessionID, req)
}

func (r *Runtime) StopProcess(ctx context.Context, sessionID string, processID string) (process.Info, error) {
	if r == nil || r.processMgr == nil {
		return process.Info{}, errors.New("process manager is required")
	}
	return r.processMgr.Stop(ctx, sessionID, processID)
}

func (r *Runtime) ListProcesses(sessionID string) []process.Info {
	if r == nil || r.processMgr == nil {
		return nil
	}
	return r.processMgr.List(sessionID)
}

func (r *Runtime) SearchSession(
	ctx context.Context,
	req sessionsearch.SessionSearchRequest,
) ([]sessionsearch.SessionSearchResult, error) {
	if r == nil || r.stateMgr == nil {
		return nil, errors.New("state manager is required")
	}

	return sessionsearch.Search(ctx, r.stateMgr, req)
}

func (r *Runtime) GetSessionMessages(
	ctx context.Context,
	req sessionmessages.SessionMessagesRequest,
) (sessionmessages.SessionMessagesResponse, error) {
	if r == nil || r.stateMgr == nil {
		return sessionmessages.SessionMessagesResponse{}, errors.New("state manager is required")
	}

	return sessionmessages.Get(ctx, r.stateMgr, req)
}

func (r *Runtime) SupportsMemorySearch(ctx context.Context) (bool, error) {
	_, supported, err := r.memorySearchProvider(ctx)
	return supported, err
}

func (r *Runtime) memorySearchProvider(ctx context.Context) (memory.SearchProvider, bool, error) {
	if r == nil || r.memory == nil {
		return nil, false, nil
	}

	searchProvider, ok := r.memory.(memory.SearchProvider)
	if !ok {
		return nil, false, nil
	}

	caps, err := r.memory.Capabilities(ctx)
	if err != nil {
		return nil, false, err
	}

	return searchProvider, caps.SupportsSearch, nil
}

func (r *Runtime) SearchMemory(ctx context.Context, query memory.SearchQuery) (memory.SearchResult, error) {
	if r == nil || r.memory == nil {
		return memory.SearchResult{}, errors.New("memory search is not configured")
	}

	searchProvider, supported, err := r.memorySearchProvider(ctx)
	if err != nil {
		return memory.SearchResult{}, err
	}
	if !supported {
		return memory.SearchResult{}, errors.New("memory search is not supported by provider")
	}

	return searchProvider.Search(ctx, query)
}

func (r *Runtime) SupportsMemoryExtraction(ctx context.Context) (bool, error) {
	if r == nil || r.memory == nil {
		return false, nil
	}

	caps, err := r.memory.Capabilities(ctx)
	if err != nil {
		return false, err
	}

	_, supportsExtraction := r.memory.(memory.ExtractionProvider)
	return caps.SupportsEpisodeRecording && supportsExtraction, nil
}

func (r *Runtime) ExtractEpisodes(ctx context.Context, req episodic.Request) (episodic.Result, error) {
	if r == nil || r.memory == nil {
		return episodic.Result{}, errors.New("memory provider is required")
	}
	supported, err := r.SupportsMemoryExtraction(ctx)
	if err != nil {
		return episodic.Result{}, err
	}
	if !supported {
		return episodic.Result{}, errors.New("memory extraction is not supported by provider")
	}

	extractor := r.memory.(memory.ExtractionProvider)
	return extractor.ExtractEpisodes(ctx, req)
}

func (r *Runtime) GetPlan(sessionID string) planstore.Plan {
	if r == nil || r.plans == nil {
		return planstore.Plan{}
	}
	return r.plans.Get(sessionID)
}

func (r *Runtime) ReplacePlan(sessionID string, plan planstore.Plan) (planstore.Plan, error) {
	if r == nil || r.plans == nil {
		return planstore.ClonePlan(plan), errors.New("plan store is required")
	}
	return r.plans.Replace(sessionID, plan)
}

func (r *Runtime) MergePlan(sessionID string, updates []planstore.PartialPlanStep, explanation string, clearCompleted bool) (planstore.Plan, error) {
	if r == nil || r.plans == nil {
		return planstore.Plan{}, errors.New("plan store is required")
	}
	return r.plans.Merge(sessionID, updates, explanation, clearCompleted)
}

func (r *Runtime) ClearPlan(sessionID string) planstore.Plan {
	if r == nil || r.plans == nil {
		return planstore.Plan{}
	}
	return r.plans.Clear(sessionID)
}

func (r *Runtime) HydratePlan(sessionID string, plan planstore.Plan) {
	if r == nil || r.plans == nil {
		return
	}
	r.plans.Hydrate(sessionID, plan)
}
