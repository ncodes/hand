package mocks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

type Runtime struct {
	FilePolicyValue    guardrails.FilesystemPolicy
	CommandPolicyValue guardrails.CommandPolicy
}

func (r *Runtime) FilePolicy() guardrails.FilesystemPolicy { return r.FilePolicyValue }
func (r *Runtime) CommandPolicy() guardrails.CommandPolicy { return r.CommandPolicyValue }
func (r *Runtime) GetPlan(string) envtypes.Plan            { return envtypes.Plan{} }
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
