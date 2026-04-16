package environment

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	envplanstore "github.com/wandxy/hand/internal/environment/planstore"
	processenv "github.com/wandxy/hand/internal/environment/process"
	envsessionsearch "github.com/wandxy/hand/internal/environment/sessionsearch"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	sessionstore "github.com/wandxy/hand/internal/session"
)

var getwd = os.Getwd

type Runtime struct {
	filePolicy    guardrails.FilesystemPolicy
	commandPolicy guardrails.CommandPolicy
	processMgr    processenv.Manager
	plans         envplanstore.Store
	sessionMgr    *sessionstore.Manager
}

func NewRuntime(roots []string, policy guardrails.CommandPolicy, sessionMgr *sessionstore.Manager) *Runtime {
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
		processMgr:    &processenv.DefaultManager{},
		plans:         &envplanstore.MemoryPlanStore{},
		sessionMgr:    sessionMgr,
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
	req processenv.StartRequest,
) (processenv.Info, error) {
	if r == nil || r.processMgr == nil {
		return processenv.Info{}, errors.New("process manager is required")
	}
	return r.processMgr.Start(ctx, sessionID, req)
}

func (r *Runtime) GetProcess(sessionID string, processID string) (processenv.Info, error) {
	if r == nil || r.processMgr == nil {
		return processenv.Info{}, errors.New("process manager is required")
	}
	return r.processMgr.Get(sessionID, processID)
}

func (r *Runtime) ReadProcess(sessionID string, req processenv.ReadRequest) (processenv.Output, error) {
	if r == nil || r.processMgr == nil {
		return processenv.Output{}, errors.New("process manager is required")
	}
	return r.processMgr.Read(sessionID, req)
}

func (r *Runtime) StopProcess(ctx context.Context, sessionID string, processID string) (processenv.Info, error) {
	if r == nil || r.processMgr == nil {
		return processenv.Info{}, errors.New("process manager is required")
	}
	return r.processMgr.Stop(ctx, sessionID, processID)
}

func (r *Runtime) ListProcesses(sessionID string) []processenv.Info {
	if r == nil || r.processMgr == nil {
		return nil
	}
	return r.processMgr.List(sessionID)
}

func (r *Runtime) SearchSession(ctx context.Context, req envtypes.SessionSearchRequest) ([]envtypes.SessionSearchResult, error) {
	if r == nil || r.sessionMgr == nil {
		return nil, errors.New("session manager is required")
	}

	return envsessionsearch.Search(ctx, r.sessionMgr, req)
}

func (r *Runtime) GetPlan(sessionID string) envtypes.Plan {
	if r == nil || r.plans == nil {
		return envtypes.Plan{}
	}
	return r.plans.Get(sessionID)
}

func (r *Runtime) ReplacePlan(sessionID string, plan envtypes.Plan) (envtypes.Plan, error) {
	if r == nil || r.plans == nil {
		return envplanstore.ClonePlan(plan), errors.New("plan store is required")
	}
	return r.plans.Replace(sessionID, plan)
}

func (r *Runtime) MergePlan(sessionID string, updates []envtypes.PartialPlanStep, explanation string, clearCompleted bool) (envtypes.Plan, error) {
	if r == nil || r.plans == nil {
		return envtypes.Plan{}, errors.New("plan store is required")
	}
	return r.plans.Merge(sessionID, updates, explanation, clearCompleted)
}

func (r *Runtime) ClearPlan(sessionID string) envtypes.Plan {
	if r == nil || r.plans == nil {
		return envtypes.Plan{}
	}
	return r.plans.Clear(sessionID)
}

func (r *Runtime) HydratePlan(sessionID string, plan envtypes.Plan) {
	if r == nil || r.plans == nil {
		return
	}
	r.plans.Hydrate(sessionID, plan)
}
