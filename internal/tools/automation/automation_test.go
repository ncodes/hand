package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storage "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/internal/state/storememory"
	"github.com/wandxy/morph/internal/tools"
	toolmocks "github.com/wandxy/morph/internal/tools/mocks"
)

var testAutomationToolJobID = "auto_projectaprojectaproje"

func TestDefinition_AddCapturesContext(t *testing.T) {
	store := storememory.NewStore()
	service := &automationToolServiceStub{store: store}
	definition := Definition(&toolmocks.Runtime{
		AutomationServiceValue: service,
		AutomationServiceOK:    true,
	})
	ctx := tools.WithSessionID(context.Background(), "ses_projectaprojectaproje")

	result, err := definition.Handler.Invoke(ctx, tools.Call{Input: `{
		"action": "add",
		"capture_context": true,
		"job": {
			"id": "` + testAutomationToolJobID + `",
			"enabled": true,
			"schedule": {"kind": "every", "every": 3600000000000},
			"payload": {"kind": "prompt", "prompt": "summarize"}
		}
	}`})

	require.NoError(t, err)
	require.Empty(t, result.Error)
	var out output
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
	require.Equal(t, "origin", out.Job.SessionTarget)
	require.Equal(t, "ses_projectaprojectaproje", out.Job.Payload.Metadata["origin_session_id"])
}

func TestInvoke_StatusAndRun(t *testing.T) {
	store := storememory.NewStore()
	service := &automationToolServiceStub{store: store}
	ctx := context.Background()
	_, err := store.CreateJob(ctx, storage.AutomationJob{
		ID:      testAutomationToolJobID,
		Enabled: true,
		State: storage.AutomationJobState{
			RunningAt: time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	status, err := invoke(ctx, service, input{Action: "status"})
	require.NoError(t, err)
	require.Equal(t, map[string]int{"enabled": 1, "jobs": 1, "running": 1}, status.Counts)

	service.run = storage.AutomationRun{
		ID:     "autorun_projectaprojectapr",
		JobID:  testAutomationToolJobID,
		Status: storage.AutomationRunStatusOK,
	}
	run, err := invoke(ctx, service, input{Action: "run", ID: testAutomationToolJobID})
	require.NoError(t, err)
	require.Equal(t, "ok", run.Status)
	require.Equal(t, testAutomationToolJobID, service.runID)
	require.Equal(t, storage.AutomationRunStatusOK, run.Run.Status)
}

func TestInvoke_ManageJobAndRunHistory(t *testing.T) {
	store := storememory.NewStore()
	service := &automationToolServiceStub{store: store}
	ctx := context.Background()
	_, err := store.CreateJob(ctx, storage.AutomationJob{
		ID:       testAutomationToolJobID,
		Name:     "Daily",
		Enabled:  true,
		Schedule: storage.AutomationSchedule{Kind: storage.AutomationScheduleEvery, Every: time.Hour},
		Payload:  storage.AutomationPayload{Kind: storage.AutomationPayloadPrompt, Prompt: "summarize"},
	})
	require.NoError(t, err)
	run, err := store.CreateRun(ctx, storage.AutomationRun{JobID: testAutomationToolJobID})
	require.NoError(t, err)

	list, err := invoke(ctx, service, input{Action: "list", Query: jobQueryInput{IncludeDisabled: true}})
	require.NoError(t, err)
	require.Len(t, list.Jobs, 1)
	require.Equal(t, testAutomationToolJobID, list.Jobs[0].ID)

	updated, err := invoke(ctx, service, input{
		Action: "update",
		ID:     testAutomationToolJobID,
		Job: storage.AutomationJob{
			Name: "Renamed",
			Schedule: storage.AutomationSchedule{
				Kind:  storage.AutomationScheduleEvery,
				Every: 2 * time.Hour,
			},
			Payload: storage.AutomationPayload{
				Kind:   storage.AutomationPayloadPrompt,
				Prompt: "next",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed", updated.Job.Name)
	require.Equal(t, 2*time.Hour, updated.Job.Schedule.Every)

	paused, err := invoke(ctx, service, input{Action: "pause", ID: testAutomationToolJobID})
	require.NoError(t, err)
	require.False(t, paused.Job.Enabled)

	resumed, err := invoke(ctx, service, input{Action: "resume", ID: testAutomationToolJobID})
	require.NoError(t, err)
	require.True(t, resumed.Job.Enabled)

	runs, err := invoke(ctx, service, input{
		Action:   "runs",
		RunQuery: runQueryInput{JobID: testAutomationToolJobID, Status: []string{string(storage.AutomationRunStatusRunning)}},
	})
	require.NoError(t, err)
	require.Equal(t, []storage.AutomationRun{run}, runs.Runs)

	removed, err := invoke(ctx, service, input{Action: "remove", ID: testAutomationToolJobID})
	require.NoError(t, err)
	require.Equal(t, "ok", removed.Status)
	_, ok, err := store.GetJob(ctx, testAutomationToolJobID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestDefinition_ReturnsToolErrorWhenServiceUnsupported(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})

	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Input: `{"action":"status"}`})

	require.NoError(t, err)
	require.Contains(t, result.Error, "automation service is not supported")
}

func TestDefinition_ReturnsDecodeAndRuntimeErrors(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})

	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Input: `{`})
	require.NoError(t, err)
	require.NotEmpty(t, result.Error)

	expected := errors.New("service failed")
	definition = Definition(&toolmocks.Runtime{AutomationServiceErr: expected})
	result, err = definition.Handler.Invoke(context.Background(), tools.Call{Input: `{"action":"status"}`})
	require.NoError(t, err)
	require.Contains(t, result.Error, expected.Error())

	definition = Definition(nil)
	result, err = definition.Handler.Invoke(context.Background(), tools.Call{Input: `{"action":"status"}`})
	require.NoError(t, err)
	require.Contains(t, result.Error, "automation runtime is required")

	definition = Definition(&toolmocks.Runtime{
		AutomationServiceValue: &automationToolServiceStub{store: storememory.NewStore()},
		AutomationServiceOK:    true,
	})
	result, err = definition.Handler.Invoke(context.Background(), tools.Call{Input: `{"action":"missing"}`})
	require.NoError(t, err)
	require.Contains(t, result.Error, "unsupported automation action")
}

func TestDefinition_RunUsesAutomationService(t *testing.T) {
	service := &automationToolServiceStub{
		store: storememory.NewStore(),
		run: storage.AutomationRun{
			ID:     "autorun_projectaprojectapr",
			JobID:  testAutomationToolJobID,
			Status: storage.AutomationRunStatusOK,
		},
	}
	definition := Definition(&toolmocks.Runtime{
		AutomationServiceValue: service,
		AutomationServiceOK:    true,
	})

	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Input: `{
		"action": "run",
		"id": "` + testAutomationToolJobID + `"
	}`})

	require.NoError(t, err)
	require.Empty(t, result.Error)
	require.Equal(t, testAutomationToolJobID, service.runID)
	var out output
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
	require.Equal(t, "ok", out.Status)
	require.Equal(t, storage.AutomationRunStatusOK, out.Run.Status)
}

func TestDefinition_RunRequiresAutomationService(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})

	result, err := definition.Handler.Invoke(context.Background(), tools.Call{Input: `{
		"action": "run",
		"id": "` + testAutomationToolJobID + `"
	}`})

	require.NoError(t, err)
	require.Contains(t, result.Error, "automation service is not supported")
}

func TestGetService_RejectsMissingRuntimeAndPropagatesErrors(t *testing.T) {
	_, err := getService(context.Background(), nil)
	require.EqualError(t, err, "automation runtime is required")

	expected := errors.New("service failed")
	_, err = getService(context.Background(), &toolmocks.Runtime{
		AutomationServiceErr: expected,
	})
	require.ErrorIs(t, err, expected)
}

func TestDefinition_SchemaDescribesConditionalRequirements(t *testing.T) {
	definition := Definition(&toolmocks.Runtime{})
	properties := definition.InputSchema["properties"].(map[string]any)
	job := properties["job"].(map[string]any)["properties"].(map[string]any)
	schedule := job["schedule"].(map[string]any)["properties"].(map[string]any)
	payload := job["payload"].(map[string]any)["properties"].(map[string]any)
	delivery := job["delivery"].(map[string]any)["properties"].(map[string]any)

	require.Contains(t, properties["id"].(map[string]any)["description"], "Required for update, pause, resume, run, and remove")
	require.Contains(t, properties["query"].(map[string]any)["description"], "Only used when action=list")
	require.Contains(t, properties["run_query"].(map[string]any)["description"], "Only used when action=runs")
	require.Contains(t, schedule["kind"].(map[string]any)["description"], "If kind=at, at is required")
	require.Contains(t, payload["kind"].(map[string]any)["description"], "If kind=system_event, system_event is required")
	require.Contains(t, delivery["webhook_url"].(map[string]any)["description"], "Required when delivery.mode=webhook")
}

func TestInvoke_RejectsUnsupportedAction(t *testing.T) {
	_, err := invoke(context.Background(), &automationToolServiceStub{store: storememory.NewStore()}, input{Action: "missing"})
	require.EqualError(t, err, "unsupported automation action")
}

func TestInvoke_PropagatesServiceErrors(t *testing.T) {
	expected := errors.New("store failed")

	_, err := invoke(context.Background(), &automationToolServiceStub{listErr: expected}, input{Action: "status"})
	require.ErrorIs(t, err, expected)

	_, err = invoke(context.Background(), nil, input{Action: "run", ID: testAutomationToolJobID})
	require.EqualError(t, err, "automation service is required")

	service := &automationToolServiceStub{store: storememory.NewStore(), runErr: expected}
	_, err = invoke(context.Background(), service, input{Action: "run", ID: testAutomationToolJobID})
	require.ErrorIs(t, err, expected)
}

func TestCaptureContext_HandlesMissingSessionAndExistingMetadata(t *testing.T) {
	job := storage.AutomationJob{SessionTarget: "main"}
	require.Equal(t, job, captureContext(context.Background(), job))

	ctx := tools.WithSessionID(context.Background(), "ses_projectaprojectaproje")
	job = storage.AutomationJob{
		SessionTarget: "main",
		Payload: storage.AutomationPayload{
			Metadata: map[string]string{"existing": "true"},
		},
	}
	captured := captureContext(ctx, job)
	require.Equal(t, "main", captured.SessionTarget)
	require.Equal(t, "true", captured.Payload.Metadata["existing"])
	require.Equal(t, "ses_projectaprojectaproje", captured.Payload.Metadata["origin_session_id"])
}

func TestInvoke_RejectsInvalidNestedJob(t *testing.T) {
	tests := []struct {
		name  string
		input input
		err   string
	}{
		{
			name: "add missing schedule kind",
			input: input{
				Action: "add",
				Job: storage.AutomationJob{
					Enabled: true,
					Payload: storage.AutomationPayload{
						Kind:   storage.AutomationPayloadPrompt,
						Prompt: "summarize",
					},
				},
			},
			err: "automation schedule kind is required",
		},
		{
			name: "add missing one-shot time",
			input: input{
				Action: "add",
				Job: storage.AutomationJob{
					Schedule: storage.AutomationSchedule{Kind: storage.AutomationScheduleAt},
					Payload: storage.AutomationPayload{
						Kind:   storage.AutomationPayloadPrompt,
						Prompt: "summarize",
					},
				},
			},
			err: "automation one-shot schedule time is required",
		},
		{
			name: "update missing prompt",
			input: input{
				Action: "update",
				ID:     testAutomationToolJobID,
				Job: storage.AutomationJob{
					Payload: storage.AutomationPayload{Kind: storage.AutomationPayloadPrompt},
				},
			},
			err: "automation prompt payload is required",
		},
		{
			name: "update missing system event",
			input: input{
				Action: "update",
				ID:     testAutomationToolJobID,
				Job: storage.AutomationJob{
					Payload: storage.AutomationPayload{Kind: storage.AutomationPayloadSystemEvent},
				},
			},
			err: "automation system event payload is required",
		},
		{
			name: "update unsupported payload kind",
			input: input{
				Action: "update",
				ID:     testAutomationToolJobID,
				Job: storage.AutomationJob{
					Payload: storage.AutomationPayload{Kind: storage.AutomationPayloadKind("unknown")},
				},
			},
			err: "unsupported automation payload kind",
		},
		{
			name: "update missing every interval",
			input: input{
				Action: "update",
				ID:     testAutomationToolJobID,
				Job: storage.AutomationJob{
					Schedule: storage.AutomationSchedule{Kind: storage.AutomationScheduleEvery},
				},
			},
			err: "automation interval schedule must be greater than zero",
		},
		{
			name: "update missing cron expression",
			input: input{
				Action: "update",
				ID:     testAutomationToolJobID,
				Job: storage.AutomationJob{
					Schedule: storage.AutomationSchedule{Kind: storage.AutomationScheduleCron},
				},
			},
			err: "automation cron schedule expression is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := invoke(context.Background(), &automationToolServiceStub{store: storememory.NewStore()}, test.input)
			require.EqualError(t, err, test.err)
		})
	}
}

func TestPatchFromInput_CoversOptionalBranches(t *testing.T) {
	at := time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC)
	patch, err := patchFromInput(input{
		ID: testAutomationToolJobID,
		Job: storage.AutomationJob{
			Schedule: storage.AutomationSchedule{
				Kind: storage.AutomationScheduleAt,
				At:   at,
			},
			Payload: storage.AutomationPayload{
				Kind:        storage.AutomationPayloadSystemEvent,
				SystemEvent: "wake",
			},
			Delivery: storage.AutomationDelivery{
				Mode: storage.AutomationDeliveryLocal,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, at, patch.Schedule.At)
	require.Equal(t, "wake", patch.Payload.SystemEvent)
	require.Equal(t, storage.AutomationDeliveryLocal, patch.Delivery.Mode)
}
