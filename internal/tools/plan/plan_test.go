package plan_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/environment"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
	nativemocks "github.com/wandxy/hand/internal/tools/mocks"
	plantool "github.com/wandxy/hand/internal/tools/plan"
	"github.com/wandxy/hand/internal/trace"
)

func TestPlanTool_ReadEmptyPlan(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{Name: "plan_tool", Input: `{}`})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Empty(t, payload.Steps)
	require.Equal(t, envtypes.PlanSummary{}, payload.Summary)
	require.Empty(t, payload.ActiveStepID)
}

func TestPlanTool_ReplacePlan(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name: "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"},` +
			`{"id":"step-2","content":"Write tests","status":"pending"}],"explanation":"starting work"}`,
	})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Len(t, payload.Steps, 2)
	require.Equal(t, "step-1", payload.ActiveStepID)
	require.Equal(t, "starting work", payload.Explanation)
	require.Equal(t, 1, payload.Summary.InProgress)
}

func TestPlanTool_MergeStatusOnlyUpdate(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
	_, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"},{"id":"step-2","content":"Write tests","status":"pending"}]}`,
	})
	require.NoError(t, err)

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"merge":true,"steps":[{"id":"step-1","status":"completed"},{"id":"step-2","status":"in_progress"}],"explanation":"shift active work"}`,
	})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Equal(t, "step-2", payload.ActiveStepID)
	require.Equal(t, "completed", payload.Steps[0].Status)
	require.Equal(t, "in_progress", payload.Steps[1].Status)
}

func TestPlanTool_MergeContentOnlyUpdateAndAppend(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
	_, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"}]}`,
	})
	require.NoError(t, err)

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"merge":true,"steps":[{"id":"step-1","content":"Implement feature thoroughly"},{"id":"step-2","content":"Write tests","status":"pending"}]}`,
	})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Equal(t, "Implement feature thoroughly", payload.Steps[0].Content)
	require.Equal(t, "in_progress", payload.Steps[0].Status)
	require.Equal(t, "step-2", payload.Steps[1].ID)
}

func TestPlanTool_ClearCompletedRemovesTerminalSteps(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Done","status":"completed"},{"id":"step-2","content":"Active","status":"in_progress"}],"clear_completed":true}`,
	})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Len(t, payload.Steps, 1)
	require.Equal(t, "step-2", payload.Steps[0].ID)
}

func TestPlanTool_RejectsInvalidWrites(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

	testCases := []string{
		`{"steps":[{"id":"dup","content":"A","status":"in_progress"},{"id":"dup","content":"B","status":"pending"}]}`,
		`{"steps":[{"id":"step-1","content":"","status":"in_progress"}]}`,
		`{"steps":[{"id":"step-1","content":"A","status":"bad"}]}`,
		`{"steps":[{"id":"step-1","content":"A","status":"pending"},{"id":"step-2","content":"B","status":"pending"}]}`,
		`{"merge":true,"steps":[{"id":"step-1","status":"bad"}]}`,
	}

	for _, input := range testCases {
		result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{Name: "plan_tool", Input: input})
		require.NoError(t, err)
		require.Contains(t, result.Error, `"code":"invalid_input"`)
	}
}

func TestPlanTool_RejectsMalformedJSON(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{Name: "plan_tool", Input: `{"steps":`})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"invalid_input"`)
}

func TestPlanTool_ReturnsInvalidInputWhenMergeDependencyFails(t *testing.T) {
	registry := newPlanFailureRegistry(t, t.TempDir(), guardrails.CommandPolicy{}, errors.New("merge failed"), nil)

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"merge":true,"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"}]}`,
	})
	require.NoError(t, err)
	requireInvalidInputError(t, result, "merge failed")
}

func TestPlanTool_ReturnsInvalidInputWhenReplaceDependencyFails(t *testing.T) {
	registry := newPlanFailureRegistry(t, t.TempDir(), guardrails.CommandPolicy{}, nil, errors.New("replace failed"))

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"}]}`,
	})
	require.NoError(t, err)
	requireInvalidInputError(t, result, "replace failed")
}

func TestPlanTool_DefaultsToDefaultSessionWhenContextHasNoSessionID(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{
		Name:  "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"}]}`,
	})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Equal(t, "step-1", payload.ActiveStepID)

	result, err = registry.Invoke(context.Background(), tools.Call{Name: "plan_tool", Input: `{}`})
	require.NoError(t, err)
	payload = decodePlanOutputForTest(t, result.Output)
	require.Len(t, payload.Steps, 1)
	require.Equal(t, "step-1", payload.Steps[0].ID)

	result, err = registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{Name: "plan_tool", Input: `{}`})
	require.NoError(t, err)
	payload = decodePlanOutputForTest(t, result.Output)
	require.Empty(t, payload.Steps)
}

func TestPlanTool_RecordsPlanUpdatedAndClearedEvents(t *testing.T) {
	registry := registerPlanRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
	traceSession := &nativemocks.TraceRecorder{}
	ctx := tools.WithTraceRecorder(tools.WithSessionID(context.Background(), "session-1"), traceSession)

	result, err := registry.Invoke(ctx, tools.Call{
		Name:  "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Done","status":"completed"}],"clear_completed":true,"explanation":"cleanup"}`,
	})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Empty(t, payload.Steps)
	require.Len(t, traceSession.Events, 2)
	require.Equal(t, trace.EvtPlanUpdated, traceSession.Events[0].Type)
	require.Equal(t, trace.EvtPlanCleared, traceSession.Events[1].Type)
}

func registerPlanRuntime(t *testing.T, root string, policy guardrails.CommandPolicy) tools.Registry {
	t.Helper()
	registry := tools.NewInMemoryRegistry()
	runtime := environment.NewRuntime([]string{root}, policy)
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	require.NoError(t, registry.Register(plantool.Definition(runtime)))
	return registry
}

func newPlanFailureRegistry(t *testing.T, root string, policy guardrails.CommandPolicy, mergeErr, replaceErr error) tools.Registry {
	t.Helper()
	registry := tools.NewInMemoryRegistry()
	runtime := &nativemocks.FailingPlanRuntime{
		Runtime:    environment.NewRuntime([]string{root}, policy),
		MergeErr:   mergeErr,
		ReplaceErr: replaceErr,
	}
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	require.NoError(t, registry.Register(plantool.Definition(runtime)))
	return registry
}

func requireInvalidInputError(t *testing.T, result tools.Result, message string) {
	t.Helper()
	require.Contains(t, result.Error, `"code":"invalid_input"`)
	require.Contains(t, result.Error, `"message":"`+message+`"`)
}

type planToolOutput struct {
	Steps        []envtypes.PlanStep  `json:"steps"`
	Summary      envtypes.PlanSummary `json:"summary"`
	ActiveStepID string               `json:"active_step_id"`
	Explanation  string               `json:"explanation"`
}

func decodePlanOutputForTest(t *testing.T, raw string) planToolOutput {
	t.Helper()
	var payload planToolOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	return payload
}
