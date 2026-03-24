package instruction

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	handctx "github.com/wandxy/hand/internal/context"
)

func TestBuildBaseReturnsInstructionList(t *testing.T) {
	instructions := BuildBase("Wandxie")

	require.IsType(t, handctx.Instructions{}, instructions)
	require.Len(t, instructions, 1)
	require.NotEmpty(t, instructions[0].Value)
}

func TestBuildBaseIncludesAgentNameAndCoreIdentity(t *testing.T) {
	instructions := BuildBase("Wandxie")

	require.True(t, strings.Contains(instructions[0].Value, "You are Wandxie,"))
	require.True(t, strings.Contains(instructions[0].Value, "developed by Wandxy"))
	require.True(t, strings.Contains(instructions[0].Value, "helpful, knowledgeable, and straightforward"))
}

func TestBuildSummaryIncludesBudgetWarningWhenLow(t *testing.T) {
	instructions := BuildSummary(2)

	require.Equal(t, handctx.Instructions{
		{Value: "Remaining iteration budget: 2."},
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, instructions)
}

func TestBuildSummaryOmitsBudgetWarningWhenNotLow(t *testing.T) {
	instructions := BuildSummary(6)

	require.Equal(t, handctx.Instructions{
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, instructions)
}
