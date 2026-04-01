package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
)

func TestRecordPreflightCompactionTrace_EmitsUnifiedPreflightEvent(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}

	recordPreflightCompactionTrace(
		traceSession,
		&config.Config{
			ModelContextLength:       1000,
			CompactionTriggerPercent: 0.5,
			CompactionWarnPercent:    0.8,
		},
		models.Request{Instructions: "hello"},
		200,
	)

	require.Len(t, traceSession.Events, 1)
	require.Equal(t, "context.preflight", traceSession.Events[0].Type)

	payload, ok := traceSession.Events[0].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "actual", payload["source"])
	require.Equal(t, 200, payload["prompt_tokens"])
	require.Equal(t, 1000, payload["context_limit"])
}

func TestRecordPreflightCompactionTrace_SkipsEventsWhenDisabled(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}

	recordPreflightCompactionTrace(
		traceSession,
		&config.Config{
			ModelContextLength:       1000,
			CompactionEnabled:        new(false),
			CompactionTriggerPercent: 0.5,
			CompactionWarnPercent:    0.8,
		},
		models.Request{Instructions: "hello"},
		0,
	)

	require.Empty(t, traceSession.Events)
}

func TestCompactionEvaluator_UsesDefaultsWhenConfigIsNil(t *testing.T) {
	evaluator := compactionEvaluator(nil)
	estimate := evaluator.Evaluate(models.Request{}, 0)

	require.Equal(t, 128000, estimate.ContextLimit)
	require.Equal(t, int(float64(128000)*0.85), estimate.TriggerThreshold)
	require.Equal(t, int(float64(128000)*0.95), estimate.WarnThreshold)
}
