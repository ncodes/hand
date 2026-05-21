package trace

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodePayload_CoversAllKnownEventTypes(t *testing.T) {
	payload := map[string]any{
		"id":                         "call_1",
		"name":                       "plan_tool",
		"tool_call_id":               "call_1",
		"input":                      `{"steps":[]}`,
		"content":                    `{"summary":{"total":3}}`,
		"message":                    "hello",
		"text":                       "hello",
		"error":                      "boom",
		"duration_ms":                123,
		"remaining_iterations":       1,
		"action":                     "block",
		"blocked":                    true,
		"redacted":                   true,
		"findings":                   []map[string]string{{"id": "test"}},
		"source":                     "test",
		"prompt_tokens":              10,
		"completion_tokens":          2,
		"total_tokens":               12,
		"context_limit":              100,
		"trigger_threshold":          80,
		"warn_threshold":             70,
		"session_id":                 "default",
		"source_end_offset":          3,
		"source_message_count":       5,
		"status":                     "running",
		"target_message_count":       3,
		"target_offset":              3,
		"original_length":            100,
		"truncated_length":           50,
		"max_content_length":         50,
		"marker":                     "...",
		"steps":                      []map[string]any{{"id": "step-1", "content": "Do it", "status": "pending"}},
		"summary":                    map[string]any{"total": 1, "pending": 1},
		"active_step_id":             "step-1",
		"background_run_id":          "run_1",
		"result_count":               1,
		"candidate_count":            2,
		"eligible":                   true,
		"episodic_checkpoint_offset": 4,
	}

	for _, eventType := range AllTraceEventTypes() {
		t.Run(eventType, func(t *testing.T) {
			decoded, ok := DecodePayload(eventType, payload)
			require.True(t, ok)
			require.NotNil(t, decoded)
		})
	}
}

func TestDecodePayloadJSON_DecodesKnownPayload(t *testing.T) {
	raw := json.RawMessage(`{"message":"hello"}`)

	payload, ok := DecodePayloadJSON(EvtUserMessageAccepted, raw)

	require.True(t, ok)
	require.Equal(t, UserMessageAcceptedPayload{Message: "hello"}, payload)
}

func TestToolInvocationStartedPayloadFrom_DecodesStructAndMapPayloads(t *testing.T) {
	payload, ok := ToolInvocationStartedPayloadFrom(struct {
		ID    string
		Name  string
		Input string
	}{
		ID:    "call_1",
		Name:  "plan_tool",
		Input: `{"steps":[]}`,
	})

	require.True(t, ok)
	require.Equal(t, ToolInvocationStartedPayload{
		ID:    "call_1",
		Name:  "plan_tool",
		Input: `{"steps":[]}`,
	}, payload)

	payload, ok = ToolInvocationStartedPayloadFrom(map[string]any{
		"id":   "call_2",
		"name": "plan_tool",
		"plan_state": map[string]any{
			"operation":     "update",
			"changed_count": float64(3),
		},
	})

	require.True(t, ok)
	require.Equal(t, ToolInvocationStartedPayload{
		ID:   "call_2",
		Name: "plan_tool",
		PlanState: &PlanToolState{
			Operation:    PlanToolOperationUpdate,
			ChangedCount: 3,
		},
	}, payload)
}

func TestToolInvocationCompletedPayloadFrom_DecodesStructAndMapPayloads(t *testing.T) {
	payload, ok := ToolInvocationCompletedPayloadFrom(struct {
		ToolCallID string
		Name       string
		Content    string
	}{
		ToolCallID: "call_1",
		Name:       "plan_tool",
		Content:    `{"summary":{"total":3}}`,
	})

	require.True(t, ok)
	require.Equal(t, ToolInvocationCompletedPayload{
		ToolCallID: "call_1",
		Name:       "plan_tool",
		Content:    `{"summary":{"total":3}}`,
	}, payload)

	payload, ok = ToolInvocationCompletedPayloadFrom(map[string]any{
		"tool_call_id": "call_2",
		"name":         "plan_tool",
		"plan_state": map[string]any{
			"total_count":     float64(3),
			"completed_count": float64(1),
			"changes": []any{
				map[string]any{
					"index":  float64(2),
					"id":     "step-2",
					"action": "completed",
					"fields": []any{"status"},
				},
			},
		},
	})

	require.True(t, ok)
	require.Equal(t, ToolInvocationCompletedPayload{
		ToolCallID: "call_2",
		Name:       "plan_tool",
		PlanState: &PlanToolState{
			TotalCount:     3,
			CompletedCount: 1,
			Changes: []PlanToolChange{
				{Index: 2, ID: "step-2", Action: "completed", Fields: []string{"status"}},
			},
		},
	}, payload)
}

func TestPlanToolOutputState_DecodesSummaryAndChanges(t *testing.T) {
	state := PlanToolOutputState(`{"summary":{"total":3,"completed":1},"changes":[{"index":2,"id":"step-2","action":"completed","fields":["status"]}]}`)

	require.Equal(t, &PlanToolState{
		TotalCount:     3,
		CompletedCount: 1,
		Changes: []PlanToolChange{
			{Index: 2, ID: "step-2", Action: "completed", Fields: []string{"status"}},
		},
	}, state)
}

func TestPlanToolOutputState_DecodesToolMessageEnvelope(t *testing.T) {
	state := PlanToolOutputState(`{
		"name": "plan_tool",
		"output": "{\"summary\":{\"total\":3,\"completed\":1},\"changes\":[{\"index\":2,\"id\":\"step-2\",\"action\":\"completed\",\"fields\":[\"status\"]}]}"
	}`)

	require.Equal(t, &PlanToolState{
		TotalCount:     3,
		CompletedCount: 1,
		Changes: []PlanToolChange{
			{Index: 2, ID: "step-2", Action: "completed", Fields: []string{"status"}},
		},
	}, state)
}
