package instructions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_BuildsTrimmedList(t *testing.T) {
	instructions := New(" first ", "   ", "second")
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, instructions)
}

func TestNew_ReturnsEmptyWhenNoValuesProvided(t *testing.T) {
	require.Empty(t, New())
}

func TestInstructions_StringJoinsValuesWithNewLines(t *testing.T) {
	instructions := Instructions{{Value: "first"}, {Value: "second"}, {Value: "third"}}
	require.Equal(t, "first\nsecond\nthird", instructions.String())
}

func TestInstructions_MarshalJSONEncodesJoinedString(t *testing.T) {
	instructions := Instructions{{Value: "first"}, {Value: "second"}}
	data, err := instructions.MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, `"first\nsecond"`, string(data))
}

func TestInstructions_UnmarshalJSONDecodesStringArray(t *testing.T) {
	var instructions Instructions
	err := instructions.UnmarshalJSON([]byte(`["first","second"]`))
	require.NoError(t, err)
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, instructions)
}

func TestInstructions_UnmarshalJSONRejectsInvalidShape(t *testing.T) {
	var instructions Instructions
	require.Error(t, instructions.UnmarshalJSON([]byte(`"first"`)))
}

func TestInstructions_JSONRoundTripUsesMarshalAndUnmarshalImplementations(t *testing.T) {
	original := Instructions{{Value: "first"}, {Value: "second"}}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	var decoded Instructions
	err = json.Unmarshal([]byte(`["first","second"]`), &decoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
	require.JSONEq(t, `"first\nsecond"`, string(data))
}

func TestInstructions_ChainAppendsTrimmedInstruction(t *testing.T) {
	original := Instructions{{Value: "first"}}
	chained := original.Chain(Instruction{Value: " second "})
	require.Equal(t, Instructions{{Value: "first"}}, original)
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, chained)
}

func TestInstructions_ChainSkipsEmptyInstruction(t *testing.T) {
	original := Instructions{{Value: "first"}}
	require.Equal(t, original, original.Chain(Instruction{Value: "   "}))
}

func TestInstructions_ChainValueAppendsInstruction(t *testing.T) {
	require.Equal(t, Instructions{{Value: "first"}}, Instructions{}.ChainValue(" first "))
}

func TestInstructions_ChainValueAppendsMultipleInstructions(t *testing.T) {
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, Instructions{}.ChainValue(" first ", "   ", "second"))
}

func TestInstructions_ChainAppendsMultipleInstructions(t *testing.T) {
	original := Instructions{{Value: "first"}}
	chained := original.Chain(Instruction{Value: " second "}, Instruction{Value: "   "}, Instruction{Value: "third"})
	require.Equal(t, Instructions{{Value: "first"}}, original)
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}, {Value: "third"}}, chained)
}

func TestInstructions_FirstReturnsZeroValueWhenEmpty(t *testing.T) {
	require.Equal(t, Instruction{}, Instructions{}.First())
}

func TestInstructions_FirstReturnsFirstInstruction(t *testing.T) {
	require.Equal(t, Instruction{Value: "first"}, Instructions{{Value: "first"}, {Value: "second"}}.First())
}

func TestInstructions_GetByNameReturnsNamedInstruction(t *testing.T) {
	instruction, ok := Instructions{{Value: "first"}, {Name: "request.instruct", Value: "be terse"}}.GetByName("request.instruct")
	require.True(t, ok)
	require.Equal(t, Instruction{Name: "request.instruct", Value: "be terse"}, instruction)
}

func TestInstructions_GetByNameTrimsLookupName(t *testing.T) {
	instruction, ok := Instructions{{Name: "request.instruct", Value: "be terse"}}.GetByName(" request.instruct ")
	require.True(t, ok)
	require.Equal(t, Instruction{Name: "request.instruct", Value: "be terse"}, instruction)
}

func TestInstructions_GetByNameRejectsBlankName(t *testing.T) {
	instruction, ok := Instructions{{Name: "request.instruct", Value: "be terse"}}.GetByName("   ")
	require.False(t, ok)
	require.Equal(t, Instruction{}, instruction)
}

func TestInstructions_GetByNameReturnsFalseWhenMissing(t *testing.T) {
	instruction, ok := Instructions{{Name: "request.instruct", Value: "be terse"}}.GetByName("config.instruct")
	require.False(t, ok)
	require.Equal(t, Instruction{}, instruction)
}

func TestInstructions_WithoutNameRemovesMatchingInstruction(t *testing.T) {
	filtered := Instructions{{Value: "first"}, {Name: "request.instruct", Value: "be terse"}}.WithoutName("request.instruct")
	require.Equal(t, Instructions{{Value: "first"}}, filtered)
}

func TestInstructions_WithoutNameTrimsLookupName(t *testing.T) {
	filtered := Instructions{{Value: "first"}, {Name: "request.instruct", Value: "be terse"}}.WithoutName(" request.instruct ")
	require.Equal(t, Instructions{{Value: "first"}}, filtered)
}

func TestInstructions_WithoutNameReturnsOriginalWhenNameBlank(t *testing.T) {
	original := Instructions{{Value: "first"}, {Name: "request.instruct", Value: "be terse"}}
	require.Equal(t, original, original.WithoutName("   "))
}

func TestInstructions_WithoutNameReturnsAllInstructionsWhenMissing(t *testing.T) {
	original := Instructions{{Value: "first"}, {Name: "request.instruct", Value: "be terse"}}
	require.Equal(t, original, original.WithoutName("config.instruct"))
}

func TestBuildBase_ReturnsInstructionList(t *testing.T) {
	instructions := BuildBase("Wandxie")
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
	require.Equal(t, Instructions{
		{Value: "Remaining iteration budget: 2."},
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, BuildSummary(2))
}

func TestBuildSummary_OmitsBudgetWarningWhenNotLow(t *testing.T) {
	require.Equal(t, Instructions{
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, BuildSummary(6))
}
