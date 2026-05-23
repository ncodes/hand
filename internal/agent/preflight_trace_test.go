package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/trace"
)

func TestRecordPreflightCompactionTrace_EmitsUnifiedPreflightEvent(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}

	recordPreflightCompactionTrace(
		traceSession,
		&config.Config{
			Models:     config.ModelsConfig{Main: config.MainModelConfig{ContextLength: 1000}},
			Compaction: config.CompactionConfig{TriggerPercent: 0.5, WarnPercent: 0.8},
		},
		models.Request{Instructions: "hello"},
		200,
		true,
	)

	require.Len(t, traceSession.Events, 1)
	require.Equal(t, trace.EvtContextPreflight, traceSession.Events[0].Type)

	payload, ok := traceSession.Events[0].Payload.(trace.ContextEventPayload)
	require.True(t, ok)
	require.Equal(t, "actual", payload.Source)
	require.Equal(t, 200, payload.PromptTokens)
	require.Equal(t, 1000, payload.ContextLimit)
}

func TestRecordPreflightCompactionTrace_SkipsEventsWhenDisabled(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}

	recordPreflightCompactionTrace(
		traceSession,
		&config.Config{
			Models:     config.ModelsConfig{Main: config.MainModelConfig{ContextLength: 1000}},
			Compaction: config.CompactionConfig{Enabled: new(false), TriggerPercent: 0.5, WarnPercent: 0.8},
		},
		models.Request{Instructions: "hello"},
		0,
		true,
	)

	require.Empty(t, traceSession.Events)
}

func TestRecordPreflightCompactionTrace_SkipsTriggeredEventWhenContextCannotCompact(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}

	recordPreflightCompactionTrace(
		traceSession,
		&config.Config{
			Models:     config.ModelsConfig{Main: config.MainModelConfig{ContextLength: 100}},
			Compaction: config.CompactionConfig{TriggerPercent: 0.5, WarnPercent: 0.8},
		},
		models.Request{Instructions: strings.Repeat("a", 400)},
		0,
		false,
	)

	eventTypes := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		eventTypes = append(eventTypes, event.Type)
	}
	require.Contains(t, eventTypes, trace.EvtContextPreflight)
	require.Contains(t, eventTypes, trace.EvtContextCompactionWarning)
	require.NotContains(t, eventTypes, trace.EvtContextCompactionTriggered)
}

func TestCompactionEvaluator_UsesDefaultsWhenConfigIsNil(t *testing.T) {
	evaluator := getCompactionEvaluator(nil)
	estimate := evaluator.Evaluate(models.Request{}, 0)

	require.Equal(t, 128000, estimate.ContextLimit)
	require.Equal(t, int(float64(128000)*0.85), estimate.TriggerThreshold)
	require.Equal(t, int(float64(128000)*0.95), estimate.WarnThreshold)
}
