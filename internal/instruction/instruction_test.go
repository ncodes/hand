package instruction

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	handctx "github.com/wandxy/hand/internal/context"
)

func TestBuildBase_ReturnsInstructionList(t *testing.T) {
	instructions := BuildBase("Wandxie")

	require.IsType(t, handctx.Instructions{}, instructions)
	require.Len(t, instructions, 4)
	for _, instruction := range instructions {
		require.NotEmpty(t, instruction.Value)
	}
}

func TestBuildBase_IncludesConfiguredNameInIdentityLayer(t *testing.T) {
	instructions := BuildBase("Wandxie")

	require.True(t, strings.Contains(instructions[0].Value, "Wandxie is the user's personal agent"))
	require.True(t, strings.Contains(instructions[0].Value, "Wandxie exists to help the user get real work done"))
}

func TestBuildBase_FallsBackToDefaultNameWhenEmpty(t *testing.T) {
	instructions := BuildBase("   ")

	require.True(t, strings.Contains(instructions[0].Value, "Hand is the user's personal agent"))
	require.True(t, strings.Contains(instructions[0].Value, "Hand exists to help the user get real work done"))
}

func TestBuildBase_IncludesCoreBehaviorGuidance(t *testing.T) {
	instructions := BuildBase("Hand")

	require.Contains(t, instructions[1].Value, "Prioritize correctness, clarity, and usefulness")
	require.Contains(t, instructions[1].Value, "Do not invent results")
	require.Contains(t, instructions[1].Value, "acknowledge uncertainty or blockers plainly")
}

func TestBuildBase_IncludesToolUseGuidance(t *testing.T) {
	instructions := BuildBase("Hand")

	require.Contains(t, instructions[2].Value, "Use tools when they materially improve correctness or allow real action")
	require.Contains(t, instructions[2].Value, "Treat tool results as more authoritative than guessing")
	require.Contains(t, instructions[2].Value, "do not claim to have used a tool when no tool was used")
}

func TestBuildBase_IncludesResponseStyleGuidance(t *testing.T) {
	instructions := BuildBase("Hand")

	require.Contains(t, instructions[3].Value, "Preserve the user's intent")
	require.Contains(t, instructions[3].Value, "avoid unnecessary verbosity")
	require.Contains(t, instructions[3].Value, "summarize completed work clearly when stopping or blocked")
}

func TestBuildSummary_IncludesBudgetWarningWhenLow(t *testing.T) {
	instructions := BuildSummary(2)

	require.Equal(t, handctx.Instructions{
		{Value: "Remaining iteration budget: 2."},
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, instructions)
}

func TestBuildSummary_OmitsBudgetWarningWhenNotLow(t *testing.T) {
	instructions := BuildSummary(6)

	require.Equal(t, handctx.Instructions{
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, instructions)
}
