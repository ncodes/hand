package native

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

func TestPlanTool_ReadEmptyPlan(t *testing.T) {
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{Name: "plan_tool", Input: `{}`})
	require.NoError(t, err)

	payload := decodePlanOutputForTest(t, result.Output)
	require.Empty(t, payload.Steps)
	require.Equal(t, envtypes.PlanSummary{}, payload.Summary)
	require.Empty(t, payload.ActiveStepID)
}

func TestPlanTool_ReplacePlan(t *testing.T) {
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

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
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
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
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
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
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

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
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

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
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{Name: "plan_tool", Input: `{"steps":`})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"invalid_input"`)
}

func TestPlanTool_ReturnsInvalidInputWhenMergeDependencyFails(t *testing.T) {
	registry := tools.NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	require.NoError(t, registry.Register(PlanDefinition(&failingPlanRuntime{mergeErr: errors.New("merge failed")})))

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"merge":true,"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"}]}`,
	})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"invalid_input"`)
	require.Contains(t, result.Error, `"message":"merge failed"`)
}

func TestPlanTool_ReturnsInvalidInputWhenReplaceDependencyFails(t *testing.T) {
	registry := tools.NewInMemoryRegistry()
	require.NoError(t, registry.RegisterGroup(tools.Group{Name: "core"}))
	require.NoError(t, registry.Register(PlanDefinition(&failingPlanRuntime{replaceErr: errors.New("replace failed")})))

	result, err := registry.Invoke(tools.WithSessionID(context.Background(), "session-1"), tools.Call{
		Name:  "plan_tool",
		Input: `{"steps":[{"id":"step-1","content":"Implement feature","status":"in_progress"}]}`,
	})
	require.NoError(t, err)
	require.Contains(t, result.Error, `"code":"invalid_input"`)
	require.Contains(t, result.Error, `"message":"replace failed"`)
}

func TestPlanTool_DefaultsToDefaultSessionWhenContextHasNoSessionID(t *testing.T) {
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})

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
	registry := registerTestRuntime(t, t.TempDir(), guardrails.CommandPolicy{})
	traceSession := &traceRecorderStub{}
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

func TestPlanTool_DecodePlanStepsRejectsMissingID(t *testing.T) {
	steps, err := decodePlanSteps([]map[string]any{{
		"id":      " ",
		"content": "Implement feature",
		"status":  envtypes.PlanStatusInProgress,
	}})

	require.Nil(t, steps)
	require.EqualError(t, err, "step id is required")
}

func TestPlanTool_DecodePlanStepsRejectsTerminallessPlanWithoutActiveStep(t *testing.T) {
	steps, err := decodePlanSteps([]map[string]any{{
		"id":      "step-1",
		"content": "Implement feature",
		"status":  envtypes.PlanStatusPending,
	}})

	require.Len(t, steps, 1)
	require.EqualError(t, err, "exactly one step must be in_progress while active work remains")
}

func TestPlanTool_DecodePartialPlanStepsRejectsInvalidInputs(t *testing.T) {
	testCases := []struct {
		name  string
		steps []map[string]any
		err   string
	}{
		{
			name: "missing id",
			steps: []map[string]any{{
				"id": " ",
			}},
			err: "step id is required",
		},
		{
			name: "duplicate id",
			steps: []map[string]any{
				{"id": "step-1"},
				{"id": "step-1"},
			},
			err: "step ids must be unique",
		},
		{
			name: "invalid content type",
			steps: []map[string]any{{
				"id":      "step-1",
				"content": 123,
			}},
			err: "step content is required",
		},
		{
			name: "invalid status",
			steps: []map[string]any{{
				"id":     "step-1",
				"status": "bad",
			}},
			err: "step status is invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			steps, err := decodePartialPlanSteps(tc.steps)
			require.Nil(t, steps)
			require.EqualError(t, err, tc.err)
		})
	}
}

func TestPlanTool_SummarizePlanCountsCancelledAndActivePlanStepIDFallsBackEmpty(t *testing.T) {
	plan := envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "Pending", Status: envtypes.PlanStatusPending},
			{ID: "step-2", Content: "Done", Status: envtypes.PlanStatusCompleted},
			{ID: "step-3", Content: "Skip", Status: envtypes.PlanStatusCancelled},
		},
	}

	require.Equal(t, envtypes.PlanSummary{
		Total:     3,
		Pending:   1,
		Completed: 1,
		Cancelled: 1,
	}, summarizePlan(plan))
	require.Empty(t, activePlanStepID(plan))
}

func TestPlanTool_RecordPlanEventAndClearedNoopWithoutRecorder(t *testing.T) {
	recordPlanEvent(context.Background(), "session-1", envtypes.Plan{})
	recordPlanCleared(context.Background(), "session-1", envtypes.Plan{})
}

func TestPlanTool_RecordPlanEventAndClearedIncludeExpectedPayload(t *testing.T) {
	traceSession := &traceRecorderStub{}
	ctx := tools.WithTraceRecorder(context.Background(), traceSession)
	plan := envtypes.Plan{
		Steps: []envtypes.PlanStep{
			{ID: "step-1", Content: "Do first", Status: envtypes.PlanStatusInProgress},
			{ID: "step-2", Content: "Skip", Status: envtypes.PlanStatusCancelled},
		},
		Explanation: "updated",
	}

	recordPlanEvent(ctx, "session-1", plan)
	recordPlanCleared(ctx, "session-1", envtypes.Plan{Explanation: "updated"})

	require.Len(t, traceSession.Events, 2)
	require.Equal(t, trace.EvtPlanUpdated, traceSession.Events[0].Type)
	require.Equal(t, trace.EvtPlanCleared, traceSession.Events[1].Type)
}

type recordedEvent struct {
	Type    string
	Payload any
}

type traceRecorderStub struct {
	Events []recordedEvent
}

func (s *traceRecorderStub) Record(eventType string, payload any) {
	s.Events = append(s.Events, recordedEvent{Type: eventType, Payload: payload})
}

type failingPlanRuntime struct {
	testRuntime
	mergeErr   error
	replaceErr error
}

func (d *failingPlanRuntime) MergePlan(sessionID string, updates []envtypes.PartialPlanStep, explanation string, clearCompleted bool) (envtypes.Plan, error) {
	if d.mergeErr != nil {
		return envtypes.Plan{}, d.mergeErr
	}
	return d.testRuntime.MergePlan(sessionID, updates, explanation, clearCompleted)
}

func (d *failingPlanRuntime) ReplacePlan(sessionID string, plan envtypes.Plan) (envtypes.Plan, error) {
	if d.replaceErr != nil {
		return envtypes.Plan{}, d.replaceErr
	}
	return d.testRuntime.ReplacePlan(sessionID, plan)
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
