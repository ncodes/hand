package native

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

type testRuntime struct {
	filePolicy    guardrails.FilesystemPolicy
	commandPolicy guardrails.CommandPolicy
	plans         map[string]envtypes.Plan
}

func (d testRuntime) FilePolicy() guardrails.FilesystemPolicy { return d.filePolicy }
func (d testRuntime) CommandPolicy() guardrails.CommandPolicy { return d.commandPolicy }
func (d *testRuntime) GetPlan(sessionID string) envtypes.Plan {
	if d.plans == nil {
		return envtypes.Plan{}
	}
	return d.plans[sessionID]
}
func (d *testRuntime) ReplacePlan(sessionID string, plan envtypes.Plan) (envtypes.Plan, error) {
	if d.plans == nil {
		d.plans = make(map[string]envtypes.Plan)
	}
	if err := envtypes.ValidatePlan(plan); err != nil {
		return envtypes.Plan{}, err
	}
	d.plans[sessionID] = plan
	return d.plans[sessionID], nil
}
func (d *testRuntime) MergePlan(sessionID string, updates []envtypes.PartialPlanStep, explanation string, clearCompleted bool) (envtypes.Plan, error) {
	store := &guardedTestPlanStore{plans: d.plans}
	plan, err := store.Merge(sessionID, updates, explanation, clearCompleted)
	d.plans = store.plans
	return plan, err
}
func (d *testRuntime) ClearPlan(sessionID string) envtypes.Plan {
	if d.plans != nil {
		delete(d.plans, sessionID)
	}
	return envtypes.Plan{}
}
func (d *testRuntime) HydratePlan(sessionID string, plan envtypes.Plan) {
	if d.plans == nil {
		d.plans = make(map[string]envtypes.Plan)
	}
	d.plans[sessionID] = plan
}

type guardedTestPlanStore struct {
	plans map[string]envtypes.Plan
}

func (s *guardedTestPlanStore) Merge(sessionID string, updates []envtypes.PartialPlanStep, explanation string, clearCompleted bool) (envtypes.Plan, error) {
	if s.plans == nil {
		s.plans = make(map[string]envtypes.Plan)
	}
	plan := s.plans[sessionID]
	indexByID := make(map[string]int, len(plan.Steps))
	for idx, step := range plan.Steps {
		indexByID[step.ID] = idx
	}
	for _, update := range updates {
		if idx, ok := indexByID[update.ID]; ok {
			if update.Content != nil {
				plan.Steps[idx].Content = *update.Content
			}
			if update.Status != nil {
				plan.Steps[idx].Status = *update.Status
			}
			continue
		}
		step := envtypes.PlanStep{ID: update.ID}
		if update.Content != nil {
			step.Content = *update.Content
		}
		if update.Status != nil {
			step.Status = *update.Status
		}
		plan.Steps = append(plan.Steps, step)
	}
	plan.Explanation = explanation
	if clearCompleted {
		filtered := make([]envtypes.PlanStep, 0, len(plan.Steps))
		for _, step := range plan.Steps {
			if step.Status == envtypes.PlanStatusCompleted || step.Status == envtypes.PlanStatusCancelled {
				continue
			}
			filtered = append(filtered, step)
		}
		plan.Steps = filtered
	}
	s.plans[sessionID] = plan
	return plan, nil
}

func registerTestRuntime(t *testing.T, root string, policy guardrails.CommandPolicy) tools.Registry {
	t.Helper()
	registry := tools.NewInMemoryRegistry()
	runtime := &testRuntime{
		filePolicy:    guardrails.FilesystemPolicy{Roots: guardrails.NormalizeRoots([]string{root})},
		commandPolicy: policy.Normalize(),
	}
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	for _, definition := range []tools.Definition{
		TimeDefinition(),
		ListFilesDefinition(runtime),
		ReadFileDefinition(runtime),
		SearchFilesDefinition(runtime),
		WriteFileDefinition(runtime),
		PatchDefinition(runtime),
		PlanDefinition(runtime),
		RunCommandDefinition(runtime),
	} {
		require.NoError(t, registry.Register(definition))
	}

	return registry
}

func quoteJSON(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
