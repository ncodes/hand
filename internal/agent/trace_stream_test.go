package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/trace"
)

func TestIsStreamableTraceEvent_IncludesLiveToolOutputSafety(t *testing.T) {
	require.True(t, isStreamableTraceEvent(trace.EvtToolOutputSafetyApplied))
}
