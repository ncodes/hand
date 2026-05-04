package instructions

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestBuildEnvironmentContext_ReturnsNamedInstructionWithRuntimeFacts(t *testing.T) {
	location := time.FixedZone("WAT", 3600)
	instruction := BuildEnvironmentContext(EnvironmentContext{
		Now:              time.Date(2026, 4, 11, 17, 30, 0, 0, location),
		Timezone:         "Africa/Lagos",
		OS:               "darwin",
		Architecture:     "arm64",
		Platform:         "cli",
		WorkingDirectory: "/workspace/hand",
		FilesystemRoots:  []string{"/workspace/hand", "   "},
		Capabilities: EnvironmentCapabilities{
			Filesystem: true,
			Network:    true,
			Exec:       true,
			Memory:     true,
			Browser:    false,
		},
		HasCapabilities:  true,
		ActiveToolGroups: []string{"core"},
		ActiveTools:      []string{"time", "read_file"},
		Model:            "openai/gpt-5.1",
		SummaryModel:     "openai/gpt-4o-mini",
		ModelProvider:    "openrouter",
		SummaryProvider:  "openai",
		APIMode:          "responses",
		WebProvider:      "tavily",
		SessionID:        "ses_123",
	})

	require.Equal(t, EnvironmentContextInstructionName, instruction.Name)
	require.Contains(t, instruction.Value, "# Environment Context")
	require.Contains(t, instruction.Value, "- Current date: 2026-04-11")
	require.Contains(t, instruction.Value, "- Current time: 2026-04-11T17:30:00+01:00")
	require.Contains(t, instruction.Value, "- Timezone: Africa/Lagos")
	require.Contains(t, instruction.Value, "- OS: darwin")
	require.Contains(t, instruction.Value, "- Architecture: arm64")
	require.Contains(t, instruction.Value, "- Platform: cli")
	require.Contains(t, instruction.Value, "- Working directory: /workspace/hand")
	require.Contains(t, instruction.Value, "- Filesystem roots: /workspace/hand")
	require.Contains(t, instruction.Value, "- Capabilities: filesystem=true, network=true, exec=true, memory=true, browser=false")
	require.Contains(t, instruction.Value, "- Active tool groups: core")
	require.Contains(t, instruction.Value, "- Active tools: time, read_file")
	require.Contains(t, instruction.Value, "- Model: openai/gpt-5.1")
	require.Contains(t, instruction.Value, "- Summary model: openai/gpt-4o-mini")
	require.Contains(t, instruction.Value, "- Model provider: openrouter")
	require.Contains(t, instruction.Value, "- Summary model provider: openai")
	require.Contains(t, instruction.Value, "- API mode: responses")
	require.Contains(t, instruction.Value, "- Web provider: tavily")
	require.Contains(t, instruction.Value, "- Session ID: ses_123")
}

func TestBuildEnvironmentContext_ReturnsEmptyNamedInstructionWithoutFacts(t *testing.T) {
	require.Equal(t, Instruction{Name: EnvironmentContextInstructionName}, BuildEnvironmentContext(EnvironmentContext{}))
}

func TestBuildMemoryContext_ReturnsEmptyNamedInstructionWithoutItems(t *testing.T) {
	require.Equal(t, Instruction{Name: MemoryContextInstructionName}, BuildMemoryContext(nil, 10))
}

func TestBuildMemoryContext_ReturnsNamedInstructionWithMemoryItems(t *testing.T) {
	instruction := BuildMemoryContext([]MemoryContextItem{{
		Kind:  "semantic",
		Title: "Package manager",
		Text:  "Use pnpm",
	}}, 0)

	require.Equal(t, MemoryContextInstructionName, instruction.Name)
	require.Contains(t, instruction.Value, "# Memory Context")
	require.Contains(t, instruction.Value, "Retrieved durable memories")
	require.Contains(t, instruction.Value, "1. kind=semantic; title=Package manager; text=Use pnpm")
}

func TestBuildMemoryContext_TrimsEmptyMemoryItemParts(t *testing.T) {
	instruction := BuildMemoryContext([]MemoryContextItem{{
		Kind:  "   ",
		Title: "  Title  ",
		Text:  "  ",
	}}, 0)

	require.Contains(t, instruction.Value, "1. title=Title")
	require.NotContains(t, instruction.Value, "kind=")
	require.NotContains(t, instruction.Value, "text=")
}

func TestBuildMemoryContext_TruncatesLongInstruction(t *testing.T) {
	maxChars := 4000
	instruction := BuildMemoryContext([]MemoryContextItem{{
		Kind:  "semantic",
		Title: "Long memory",
		Text:  strings.Repeat("x", maxChars),
	}}, maxChars)

	require.Len(t, []rune(instruction.Value), maxChars)
}

func TestBuildMemoryContext_DoesNotTruncateWhenLimitIsDisabled(t *testing.T) {
	instruction := BuildMemoryContext([]MemoryContextItem{{
		Text: strings.Repeat("x", 10),
	}}, 0)

	require.Greater(t, len([]rune(instruction.Value)), 10)
}

func TestBuildPlanningPolicy_ReturnsNamedPlanningInstruction(t *testing.T) {
	instruction := BuildPlanningPolicy()
	require.Equal(t, PlanningPolicyInstructionName, instruction.Name)
	require.Contains(t, instruction.Value, "# Planning Policy")
	require.Contains(t, instruction.Value, "Use plan_tool for tasks with 3 or more meaningful steps")
	require.Contains(t, instruction.Value, "keep exactly one step in_progress")
}

func TestBuildSessionSearchGuidance_ReturnsNamedInstruction(t *testing.T) {
	instruction := BuildSessionSearchGuidance()
	require.Equal(t, SessionSearchInstructionName, instruction.Name)
	require.Contains(t, instruction.Value, "# Session Search Guidance")
	require.Contains(t, instruction.Value, "Use session_search when the user references prior work")
	require.Contains(t, instruction.Value, "treat stable preferences or long-lived facts as durable memory rather than transcript history")
	require.Contains(t, instruction.Value, "Reserve session_search for transcript recall")
}

func TestBuildSessionMessagesGuidance_ReturnsNamedInstruction(t *testing.T) {
	instruction := BuildSessionMessagesGuidance()
	require.Equal(t, SessionMessagesInstructionName, instruction.Name)
	require.Contains(t, instruction.Value, "# Session Messages Guidance")
	require.Contains(t, instruction.Value, "Prefer session_search first for discovery")
	require.Contains(t, instruction.Value, "use session_messages to fetch the exact message text and neighboring context")
	require.Contains(t, instruction.Value, "Do not use session_messages as a substitute for transcript search")
}

func TestBuildMemoryExtractGuidance_ReturnsNamedInstruction(t *testing.T) {
	instruction := BuildMemoryExtractGuidance()
	require.Equal(t, MemoryExtractInstructionName, instruction.Name)
	require.Contains(t, instruction.Value, "# Memory Extract Guidance")
	require.Contains(t, instruction.Value, "Use memory_extract only when the user explicitly asks")
	require.Contains(t, instruction.Value, "Prefer bounded ranges with session_id plus offset_start and offset_end")
	require.Contains(t, instruction.Value, "Do not use memory_extract during ordinary task execution")
}

func TestBuildEpisodicExtractionInstructions_ReturnsCuratedExtractionPrompt(t *testing.T) {
	instructions := BuildEpisodicExtractionInstructions()

	require.Equal(t, strings.TrimSpace(instructions), instructions)
	require.Contains(t, instructions, "Extract curated episodic memory candidates")
	require.Contains(t, instructions, "task trace events")
	require.Contains(t, instructions, "resolved issues")
	require.Contains(t, instructions, "project milestones")
	require.Contains(t, instructions, "discarded approaches")
	require.Contains(t, instructions, "Do not store raw transcript windows")
	require.Contains(t, instructions, "Use trace_events to verify tool execution")
	require.Contains(t, instructions, "For tool_event candidates")
	require.Contains(t, instructions, "metadata.purpose")
	require.Contains(t, instructions, "metadata.artifact_or_command_ref")
	require.Contains(t, instructions, "For decision candidates")
	require.Contains(t, instructions, "metadata.chosen_option")
	require.Contains(t, instructions, "metadata.rejected_alternatives")
	require.Contains(t, instructions, "metadata.source_range")
	require.Contains(t, instructions, "For outcome candidates")
	require.Contains(t, instructions, "metadata.requested_goal")
	require.Contains(t, instructions, "metadata.resulting_change")
	require.Contains(t, instructions, "metadata.verification_status")
	require.Contains(t, instructions, "metadata.remaining_risk")
	require.Contains(t, instructions, "explain why something happened")
	require.Contains(t, instructions, "do not infer motives or causes")
	require.Contains(t, instructions, "successful, failed, partial, and follow-up-required outcomes")
	require.Contains(t, instructions, "failed attempts, partial progress, open follow-ups")
	require.Contains(t, instructions, "outcome_status")
	require.Contains(t, instructions, "discarded approaches and unresolved blockers")
	require.Contains(t, instructions, "uncertainty metadata")
	require.Contains(t, instructions, "Reject low-signal, speculative, temporary, unsafe")
	require.Contains(t, instructions, "Preserve uncertainty in metadata")
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
	require.Len(t, instructions, 1)
	require.Contains(t, instructions[0].Value, "# Session Summary Task")
	require.Contains(t, instructions[0].Value, "Goal: Capture the current progress and important decisions made so far.")
	require.Contains(t, instructions[0].Value, "Context preservation: Preserve important context")
	require.Contains(t, instructions[0].Value, "Remaining work: Make the remaining work explicit")
	require.Contains(t, instructions[0].Value, "Output format:")
	require.Contains(t, instructions[0].Value, `"session_summary": "required concise summary"`)
	require.Contains(t, instructions[0].Value, `"next_actions": ["next action"]`)
	require.Contains(t, instructions[0].Value, "Output rules: Do not include markdown fences or extra commentary.")
}

func TestBuildRecallSessionSummaryWindow_ReturnsRecallWindowInstructions(t *testing.T) {
	instructions := BuildRecallSessionSummaryWindow(2, 5)

	require.Contains(t, instructions.String(), "# Recall Session Summary Window")
	require.Contains(t, instructions.String(), "window 2 of 5")
	require.Contains(t, instructions.String(), "Use only the messages in this recall window")
	require.Contains(t, instructions.String(), "Do not add claims that are not supported by this recall window")
}

func TestBuildRecallSessionSummarySynthesis_ReturnsRecallSynthesisInstructions(t *testing.T) {
	instructions := BuildRecallSessionSummarySynthesis(2, 3)

	require.Contains(t, instructions.String(), "# Recall Session Summary Synthesis")
	require.Contains(t, instructions.String(), "batch 2 of 3")
	require.Contains(t, instructions.String(), "Use only the provided recall window summaries")
	require.Contains(t, instructions.String(), "Do not add claims that are not supported by the recall window summaries")
}

func TestBuildRecallSessionSummaryChunk_ReturnsRecallChunkInstructions(t *testing.T) {
	instructions := BuildRecallSessionSummaryChunk(2, 5, 3, 4)

	require.Contains(t, instructions.String(), "# Recall Session Summary Chunk")
	require.Contains(t, instructions.String(), "chunk 3 of 4")
	require.Contains(t, instructions.String(), "window 2 of 5")
	require.Contains(t, instructions.String(), "Use only the provided recall chunk")
	require.Contains(t, instructions.String(), "Do not add claims that are not supported by this recall chunk")
}

func TestBuildWebExtractSummary_ReturnsSummaryInstructions(t *testing.T) {
	instructions := BuildWebExtractSummary(500)

	require.Contains(t, instructions, "# Web Extract Summary")
	require.Contains(t, instructions, "Condense the extracted web page into markdown")
	require.Contains(t, instructions, "Retain verifiable facts, dates, names, numbers, source details, decisions, and action items")
	require.Contains(t, instructions, "Keep short quoted passages, code fragments, or technical specifics")
	require.Contains(t, instructions, "organize the summary around that query")
	require.Contains(t, instructions, "Do not add claims that are not supported by the extracted content")
	require.Contains(t, instructions, "Keep the summary under 500 characters")
}

func TestBuildWebExtractChunkSummary_ReturnsChunkInstructions(t *testing.T) {
	instructions := BuildWebExtractChunkSummary(500, 2, 5)

	require.Contains(t, instructions, "# Web Extract Chunk Summary")
	require.Contains(t, instructions, "chunk 2 of 5")
	require.Contains(t, instructions, "Retain verifiable facts")
	require.Contains(t, instructions, "Do not add context from other chunks")
	require.Contains(t, instructions, "Keep the chunk summary under 500 characters")
}

func TestBuildWebExtractSynthesis_ReturnsSynthesisInstructions(t *testing.T) {
	instructions := BuildWebExtractSynthesis(500)

	require.Contains(t, instructions, "# Web Extract Summary Synthesis")
	require.Contains(t, instructions, "Combine chunk summaries")
	require.Contains(t, instructions, "Remove repeated details")
	require.Contains(t, instructions, "Do not add claims that are not supported by the chunk summaries")
	require.Contains(t, instructions, "Keep the final summary under 500 characters")
}

func TestBuildRetrievalRerank_ReturnsRerankerInstructions(t *testing.T) {
	instructions := BuildRetrievalRerank()

	require.Contains(t, instructions, "Rank the retrieval candidates for the query.")
	require.Contains(t, instructions, "Return only candidate IDs from the input.")
	require.Contains(t, instructions, "Do not rewrite candidate text or metadata.")
	require.Contains(t, instructions, "Return JSON with an items array ordered from best to worst.")
	require.Contains(t, instructions, "Each item must include candidate_id and score.")
}
