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

func TestInstructions_StringJoinsValuesWithBlankLines(t *testing.T) {
	instructions := Instructions{{Value: "first"}, {Value: "second"}, {Value: "third"}}
	require.Equal(t, "first\n\nsecond\n\nthird", instructions.String())
}

func TestInstructions_MarshalJSONEncodesJoinedString(t *testing.T) {
	instructions := Instructions{{Value: "first"}, {Value: "second"}}
	data, err := instructions.MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, `"first\n\nsecond"`, string(data))
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
	require.JSONEq(t, `"first\n\nsecond"`, string(data))
}

func TestInstructions_AppendAppendsTrimmedInstruction(t *testing.T) {
	original := Instructions{{Value: "first"}}
	appended := original.Append(Instruction{Value: " second "})
	require.Equal(t, Instructions{{Value: "first"}}, original)
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, appended)
}

func TestInstructions_AppendSkipsEmptyInstruction(t *testing.T) {
	original := Instructions{{Value: "first"}}
	require.Equal(t, original, original.Append(Instruction{Value: "   "}))
}

func TestInstructions_AppendValueAppendsInstruction(t *testing.T) {
	require.Equal(t, Instructions{{Value: "first"}}, Instructions{}.AppendValue(" first "))
}

func TestInstructions_AppendValueAppendsMultipleInstructions(t *testing.T) {
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}}, Instructions{}.AppendValue(" first ", "   ", "second"))
}

func TestInstructions_AppendAppendsMultipleInstructions(t *testing.T) {
	original := Instructions{{Value: "first"}}
	appended := original.Append(Instruction{Value: " second "}, Instruction{Value: "   "}, Instruction{Value: "third"})
	require.Equal(t, Instructions{{Value: "first"}}, original)
	require.Equal(t, Instructions{{Value: "first"}, {Value: "second"}, {Value: "third"}}, appended)
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
	require.Len(t, instructions, 1)
	for _, instruction := range instructions {
		require.NotEmpty(t, instruction.Value)
	}
}

func TestBuildBase_IncludesConfiguredNameInIdentityLayer(t *testing.T) {
	instructions := BuildBase("Wandxie")
	require.Contains(t, instructions[0].Value, "# Base Instructions")
	require.True(t, strings.Contains(instructions[0].Value, "Wandxie is the user's personal agent"))
	require.True(t, strings.Contains(instructions[0].Value, "Wandxie exists to help the user get real work done"))
}

func TestBuildBase_FallsBackToDefaultNameWhenEmpty(t *testing.T) {
	instructions := BuildBase("   ")
	require.Contains(t, instructions[0].Value, "# Base Instructions")
	require.True(t, strings.Contains(instructions[0].Value, "Hand is the user's personal agent"))
	require.True(t, strings.Contains(instructions[0].Value, "Hand exists to help the user get real work done"))
}

func TestBuildBase_IncludesCoreBehaviorGuidance(t *testing.T) {
	instructions := BuildBase("Hand")
	require.Contains(t, instructions[0].Value, "Core behavior:")
	require.Contains(t, instructions[0].Value, "Prioritize correctness, clarity, and usefulness")
	require.Contains(t, instructions[0].Value, "Do not invent results")
	require.Contains(t, instructions[0].Value, "acknowledge uncertainty or blockers plainly")
}

func TestBuildBase_IncludesToolUseGuidance(t *testing.T) {
	instructions := BuildBase("Hand")
	require.Contains(t, instructions[0].Value, "Tool use:")
	require.Contains(t, instructions[0].Value, "Use tools when they materially improve correctness or allow real action")
	require.Contains(t, instructions[0].Value, "Treat tool results as more authoritative than guessing")
	require.Contains(t, instructions[0].Value, "do not claim to have used a tool when no tool was used")
}

func TestBuildBase_IncludesResponseStyleGuidance(t *testing.T) {
	instructions := BuildBase("Hand")
	require.Contains(t, instructions[0].Value, "Response style:")
	require.Contains(t, instructions[0].Value, "Preserve the user's intent")
	require.Contains(t, instructions[0].Value, "avoid unnecessary verbosity")
	require.Contains(t, instructions[0].Value, "summarize completed work clearly when stopping or blocked")
}

func TestBuildPlanningPolicy_ReturnsNamedPlanningInstruction(t *testing.T) {
	instruction := BuildPlanningPolicy()
	require.Equal(t, PlanningPolicyInstructionName, instruction.Name)
	require.Contains(t, instruction.Value, "# Planning Policy")
	require.Contains(t, instruction.Value, "Use plan_tool for tasks with 3 or more meaningful steps")
	require.Contains(t, instruction.Value, "keep exactly one step in_progress")
}

func TestBuildSummary_IncludesBudgetWarningWhenLow(t *testing.T) {
	require.Equal(t, Instructions{
		{Value: "# Summary Fallback\n\nRemaining iteration budget: 2."},
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, BuildSummary(2))
}

func TestBuildSummary_OmitsBudgetWarningWhenNotLow(t *testing.T) {
	require.Equal(t, Instructions{
		{Value: "The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools."},
	}, BuildSummary(6))
}

func TestBuildSessionSummary_ReturnsStructuredSummaryInstructions(t *testing.T) {
	instructions := BuildSessionSummary()
	require.Len(t, instructions, 6)
	require.Equal(t, "# Session Summary Task\n\nCreate a structured handoff summary of the provided chat history for another assistant that will continue the work.", instructions[0].Value)
	require.Equal(t, "Goal: Capture the current progress and important decisions made so far.", instructions[1].Value)
	require.Equal(t, "Context preservation: Preserve important context, hard constraints, user preferences, and any critical examples or references needed to continue without redoing work.", instructions[2].Value)
	require.Equal(t, "Remaining work: Make the remaining work explicit through unresolved questions and concrete next actions.", instructions[3].Value)
	require.Contains(t, instructions[4].Value, "Output format:")
	require.Contains(t, instructions[4].Value, `"session_summary": "required concise summary"`)
	require.Contains(t, instructions[4].Value, `"next_actions": ["next action"]`)
	require.Equal(t, "Output rules: Do not include markdown fences or extra commentary.", instructions[5].Value)
}
