package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent/context/compaction"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/mocks"
	models "github.com/wandxy/hand/internal/model"
	"github.com/wandxy/hand/internal/trace"
)

func TestPreflightCompactionTraceRecordsWarningAndTrigger(t *testing.T) {
	cfg := &config.Config{}
	enabled := true
	cfg.Compaction.Enabled = &enabled
	cfg.Models.Main.ContextLength = 100
	cfg.Compaction.TriggerPercent = 0.5
	cfg.Compaction.WarnPercent = 0.25
	traceSession := &mocks.TraceSessionStub{}

	recordPreflightCompactionTrace(traceSession, cfg, models.Request{}, 60, true)

	require.Len(t, traceSession.Events, 3)
	require.Equal(t, trace.EvtContextPreflight, traceSession.Events[0].Type)
	require.Equal(t, trace.EvtContextCompactionTriggered, traceSession.Events[1].Type)
	require.Equal(t, trace.EvtContextCompactionWarning, traceSession.Events[2].Type)

	disabled := false
	cfg.Compaction.Enabled = &disabled
	traceSession.Events = nil
	recordPreflightCompactionTrace(traceSession, cfg, models.Request{}, 60, true)
	require.Empty(t, traceSession.Events)
	require.True(t, isCompactionEnabled(nil))
	estimate := getCompactionEvaluator(nil).Evaluate(models.Request{}, 0)
	require.Equal(t, 128000, estimate.ContextLimit)
	require.Equal(t, compaction.EstimatedSource, estimate.Source)
	require.Equal(t, compaction.EstimateRequestRough(models.Request{}), estimate.PromptTokens)

	cfg.Compaction.Enabled = &enabled
	traceSession.Events = nil
	recordPreflightCompactionTrace(traceSession, cfg, models.Request{}, 30, false)
	require.Equal(t, []string{
		trace.EvtContextPreflight,
		trace.EvtContextCompactionWarning,
	}, []string{traceSession.Events[0].Type, traceSession.Events[1].Type})
}
