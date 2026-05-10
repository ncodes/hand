package instructions

import (
	"fmt"
	"strings"
	"time"
)

const (
	PlanningPolicyInstructionName     = "planning.policy"
	EnvironmentContextInstructionName = "environment.context"
	MemoryContextInstructionName      = "memory.context"
	SessionSearchInstructionName      = "tool.session_search"
	SessionMessagesInstructionName    = "tool.session_messages"
	MemoryExtractInstructionName      = "tool.memory_extract"
)

type MemoryContextItem struct {
	Kind  string
	Title string
	Text  string
}

type EnvironmentContext struct {
	Now              time.Time
	Timezone         string
	OS               string
	Architecture     string
	Platform         string
	WorkingDirectory string
	FilesystemRoots  []string
	Capabilities     EnvironmentCapabilities
	HasCapabilities  bool
	ActiveToolGroups []string
	ActiveTools      []string
	Model            string
	SummaryModel     string
	ModelProvider    string
	SummaryProvider  string
	APIMode          string
	WebProvider      string
	SessionID        string
}

type EnvironmentCapabilities struct {
	Filesystem bool
	Network    bool
	Exec       bool
	Memory     bool
	Browser    bool
}

func BuildBase(name string) Instructions {
	agentName := strings.TrimSpace(name)
	if agentName == "" {
		agentName = "Hand"
	}

	return New(fmt.Sprintf(
		`# Base Instructions

%s is the user's personal agent. %s exists to help the user get real work done and should speak directly and clearly.
Core behavior: Prioritize correctness, clarity, and usefulness. Do not invent results, do not pretend work was completed, and acknowledge uncertainty or blockers plainly.
Tool use: Use tools when they materially improve correctness or allow real action. Treat tool results as more authoritative than guessing, and do not claim to have used a tool when no tool was used.
Response style: Preserve the user's intent, avoid unnecessary verbosity, and summarize completed work clearly when stopping or blocked.`,
		agentName,
		agentName,
	))
}

func BuildEnvironmentContext(ctx EnvironmentContext) Instruction {
	lines := []string{"# Environment Context", ""}

	if !ctx.Now.IsZero() {
		lines = append(lines,
			fmt.Sprintf("- Current date: %s", ctx.Now.Format("2006-01-02")),
			fmt.Sprintf("- Current time: %s", ctx.Now.Format(time.RFC3339)),
		)
	}

	if timezone := strings.TrimSpace(ctx.Timezone); timezone != "" {
		lines = append(lines, fmt.Sprintf("- Timezone: %s", timezone))
	}

	if osName := strings.TrimSpace(ctx.OS); osName != "" {
		lines = append(lines, fmt.Sprintf("- OS: %s", osName))
	}

	if arch := strings.TrimSpace(ctx.Architecture); arch != "" {
		lines = append(lines, fmt.Sprintf("- Architecture: %s", arch))
	}

	if platform := strings.TrimSpace(ctx.Platform); platform != "" {
		lines = append(lines, fmt.Sprintf("- Platform: %s", platform))
	}

	if workingDirectory := strings.TrimSpace(ctx.WorkingDirectory); workingDirectory != "" {
		lines = append(lines, fmt.Sprintf("- Working directory: %s", workingDirectory))
	}

	if roots := cleanList(ctx.FilesystemRoots); len(roots) > 0 {
		lines = append(lines, fmt.Sprintf("- Filesystem roots: %s", strings.Join(roots, ", ")))
	}

	if ctx.HasCapabilities {
		lines = append(lines, fmt.Sprintf(
			"- Capabilities: filesystem=%t, network=%t, exec=%t, memory=%t, browser=%t",
			ctx.Capabilities.Filesystem,
			ctx.Capabilities.Network,
			ctx.Capabilities.Exec,
			ctx.Capabilities.Memory,
			ctx.Capabilities.Browser,
		))
	}

	if groups := cleanList(ctx.ActiveToolGroups); len(groups) > 0 {
		lines = append(lines, fmt.Sprintf("- Active tool groups: %s", strings.Join(groups, ", ")))
	}

	if activeTools := cleanList(ctx.ActiveTools); len(activeTools) > 0 {
		lines = append(lines, fmt.Sprintf("- Active tools: %s", strings.Join(activeTools, ", ")))
	}

	if model := strings.TrimSpace(ctx.Model); model != "" {
		lines = append(lines, fmt.Sprintf("- Model: %s", model))
	}

	if summaryModel := strings.TrimSpace(ctx.SummaryModel); summaryModel != "" {
		lines = append(lines, fmt.Sprintf("- Summary model: %s", summaryModel))
	}

	if provider := strings.TrimSpace(ctx.ModelProvider); provider != "" {
		lines = append(lines, fmt.Sprintf("- Model provider: %s", provider))
	}

	if summaryProvider := strings.TrimSpace(ctx.SummaryProvider); summaryProvider != "" {
		lines = append(lines, fmt.Sprintf("- Summary model provider: %s", summaryProvider))
	}

	if apiMode := strings.TrimSpace(ctx.APIMode); apiMode != "" {
		lines = append(lines, fmt.Sprintf("- API mode: %s", apiMode))
	}

	if webProvider := strings.TrimSpace(ctx.WebProvider); webProvider != "" {
		lines = append(lines, fmt.Sprintf("- Web provider: %s", webProvider))
	}

	if sessionID := strings.TrimSpace(ctx.SessionID); sessionID != "" {
		lines = append(lines, fmt.Sprintf("- Session ID: %s", sessionID))
	}

	if len(lines) == 2 {
		return Instruction{Name: EnvironmentContextInstructionName}
	}

	return Instruction{
		Name:  EnvironmentContextInstructionName,
		Value: strings.Join(lines, "\n"),
	}
}

func BuildMemoryContext(items []MemoryContextItem, maxChars int) Instruction {
	if len(items) == 0 {
		return Instruction{Name: MemoryContextInstructionName}
	}

	lines := []string{
		"# Memory Context",
		"",
		"Retrieved durable memories that may be relevant to this turn:",
	}
	for idx, item := range items {
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, renderMemoryContextItem(item)))
	}

	value := strings.TrimSpace(strings.Join(lines, "\n"))
	if maxChars > 0 && len([]rune(value)) > maxChars {
		value = string([]rune(value)[:maxChars])
	}

	return Instruction{Name: MemoryContextInstructionName, Value: value}
}

func renderMemoryContextItem(item MemoryContextItem) string {
	parts := make([]string, 0, 3)
	if kind := strings.TrimSpace(item.Kind); kind != "" {
		parts = append(parts, "kind="+kind)
	}
	if title := strings.TrimSpace(item.Title); title != "" {
		parts = append(parts, "title="+title)
	}
	if text := strings.TrimSpace(item.Text); text != "" {
		parts = append(parts, "text="+text)
	}
	return strings.Join(parts, "; ")
}

func BuildPlanningPolicy() Instruction {
	return Instruction{
		Name: PlanningPolicyInstructionName,
		Value: `
# Planning Policy

Use plan_tool for tasks with 3 or more meaningful steps, multiple user asks in one request, or longer workflows involving several tool calls.
Do not use plan_tool for trivial one-step work or direct factual answers.
When using plan_tool, keep exactly one step in_progress while active work remains.
Mark steps completed immediately when done.
If a step becomes invalid or fails, cancel it and add a revised replacement step.`,
	}
}

func BuildSessionSearchGuidance() Instruction {
	return Instruction{
		Name: SessionSearchInstructionName,
		Value: `
# Session Search Guidance

Use session_search when the user references prior work, earlier attempts, previous sessions, or context that likely exists in older transcript history.
Use session_search to recover task-specific transcript context such as prior decisions, explored approaches, unfinished work, or exact earlier statements.
Do not treat session_search as long-term memory for stable user preferences or durable facts. Reserve session_search for transcript recall, and treat stable preferences or long-lived facts as durable memory rather than transcript history.`,
	}
}

func BuildSessionMessagesGuidance() Instruction {
	return Instruction{
		Name: SessionMessagesInstructionName,
		Value: `
# Session Messages Guidance

Use session_messages when you need exact stored transcript content or a small amount of nearby transcript context from a known session.
Prefer session_search first for discovery across prior transcript history, then use session_messages to fetch the exact message text and neighboring context for the best hits.
Use session_messages for bounded retrieval by message id, anchor window, or offset range when the relevant session or message is already known.
Do not use session_messages as a substitute for transcript search, ranking, or unbounded transcript dumps.`,
	}
}

func BuildMemoryExtractGuidance() Instruction {
	return Instruction{
		Name: MemoryExtractInstructionName,
		Value: `
# Memory Extract Guidance

When the user explicitly asks you to remember, capture, save, or retain durable information, call memory_extract before giving the final response.
Use memory_extract proactively after a meaningful interaction has clearly completed and produced durable continuity value, such as an important decision, outcome, correction, preference, unresolved blocker, reflection, or handoff-relevant context.
Prefer bounded ranges with session_id plus offset_start and offset_end when the relevant messages are known.
Do not use memory_extract during active task execution, for every routine turn, for speculative capture, or for low-signal conversational details.
Treat memory_extract as a deliberate capture action: it creates source-linked durable memory and should be used sparingly.`,
	}
}

func BuildEpisodicExtractionInstructions() string {
	return strings.TrimSpace(`Extract curated episodic memory candidates from bounded session messages and task trace events.
Return only JSON matching the schema. Do not store raw transcript windows.
Extract only evidence-backed decisions, outcomes, reflections, task traces, tool events, blockers, resolved issues, milestone episodes, discarded approaches, and explicit durable user corrections/preferences.
Episodic memories may come from ordinary conversation, planning, research, writing, operations, personal preferences, coordination, troubleshooting, or coding; do not assume the session is a software project.
Return the minimum number of candidates needed to preserve the durable story of the interaction.
Prefer one broader outcome or milestone episode over several small step-level memories.
Use metadata.memory_importance as high, medium, or low; emit candidates only when importance is high or medium.
Use metadata.memory_granularity as summary, episode, or execution_detail; reject execution_detail candidates and preserve that detail inside a broader summary or episode when useful.
Use metadata.canonical_group to give overlapping candidates the same durable group label so redundant small candidates can be collapsed.
When the user gives an explicit future-work workflow, checklist, preference, or operating rule, preserve the ordered steps, triggering condition, constraints, and important examples so reflection can turn it into an actionable procedural memory.
Use empty strings for metadata fields that are unknown, absent, or not applicable; do not use placeholder words.
Do not emit separate candidates for routine mechanical steps, ordinary data gathering, record updates, confirmations, or successful actions unless they are consequential for a decision, failure, blocker, verification, future preference, or handoff.
Only use resolved_issue when the evidence shows an actual problem, failure, blocker, conflict, or misunderstanding that was resolved; routine successful completion is not a resolved issue.
Use trace_events to verify tool execution, failures, retries, policy blocks, truncation, plan changes, memory events, and other system-side events that may not be fully narrated in messages.
When a candidate depends on trace evidence, preserve only the trace refs or event details that directly support that candidate in metadata.
For tool_event candidates, include safe tool name, purpose, status, and artifact or command reference as metadata.tool_name, metadata.purpose, metadata.status, and metadata.artifact_or_command_ref when present in the evidence; emit tool_event only for consequential tool use such as failures, verification, risky operations, important produced artifacts or records, external actions, or handoff-relevant references.
For decision candidates, include metadata.chosen_option, metadata.rejected_alternatives, metadata.reason, and metadata.source_range when present in the evidence.
For outcome candidates, include metadata.requested_goal, metadata.resulting_change, metadata.verification_status, and metadata.remaining_risk when present in the evidence; requested_goal may be conversational, analytical, creative, operational, personal, or technical.
For reflection candidates, capture durable meaning-making or emotional interpretation of an episode, not passing mood; include metadata.emotion, metadata.emotional_valence, metadata.emotional_intensity, metadata.emotion_target, metadata.life_domain, and metadata.sensitivity when present in the evidence.
When messages or trace events explain why something happened, include the evidence-backed reason in the candidate text and metadata; do not infer motives or causes that are not supported.
Distinguish successful, failed, partial, and follow-up-required outcomes with metadata.outcome_status values such as success, failed, partial, or follow_up_required.
Capture failed attempts, partial progress, open follow-ups, and unresolved blockers with explicit status metadata such as attempt_status, progress_status, follow_up_status, or blocker_status.
For discarded approaches and unresolved blockers, preserve uncertainty metadata instead of overstating the evidence.
Reject low-signal, speculative, temporary, unsafe, socially trivial, or purely conversational content with a concise reason.
Keep candidate text concise, source-grounded, and useful for future continuity. Preserve uncertainty in metadata when evidence is incomplete.`)
}

func BuildSummary(iterationsLeft int) Instructions {
	instructions := New()
	if iterationsLeft <= 5 {
		instructions = instructions.AppendValue(fmt.Sprintf("# Summary Fallback\n\nRemaining iteration budget: %d.", iterationsLeft))
	}
	instructions = instructions.AppendValue(
		"The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools.",
	)
	return instructions
}

func BuildSessionSummary() Instructions {
	return New(`
# Session Summary Task

Create a structured handoff summary of the provided chat history for another assistant that will continue the work.

Goal: Capture the current progress and important decisions made so far.
Context preservation: Preserve important context, hard constraints, user preferences, and any critical examples or references needed to continue without redoing work.
Remaining work: Make the remaining work explicit through unresolved questions and concrete next actions.

Output format:

Return JSON only with this exact shape:
{
  "session_summary": "required concise summary",
  "current_task": "current task or empty string",
  "discoveries": ["important discovery"],
  "open_questions": ["open question"],
  "next_actions": ["next action"]
}

Output rules: Do not include markdown fences or extra commentary.`)
}

func BuildRecallSessionSummaryWindow(windowIndex, windowCount int) Instructions {
	return New(
		fmt.Sprintf(`
# Recall Session Summary Window

Summarize bounded recall window %d of %d from one session into the structured handoff format.`,
			windowIndex,
			windowCount,
		),
	).Append(
		BuildSessionSummary()...,
	).Append(
		New(`
Scope: Use only the messages in this recall window and any provided authoritative prior summary.
Priority: Prefer the most recent concrete facts, decisions, constraints, unresolved questions, and next actions in this window.
Do not add claims that are not supported by this recall window.`,
		)...,
	)
}

func BuildRecallSessionSummarySynthesis(batchIndex, batchCount int) Instructions {
	return New(fmt.Sprintf(`
# Recall Session Summary Synthesis

Combine bounded recall summaries batch %d of %d into one structured handoff summary.`, batchIndex, batchCount),
	).Append(
		BuildSessionSummary()...,
	).Append(
		New(`
Scope: Use only the provided recall window summaries and any provided authoritative prior summary.
Priority: Prefer the most recent concrete facts, decisions, constraints, unresolved questions, and next actions when consolidating duplicates or conflicts.
Do not add claims that are not supported by the recall window summaries.`,
		)...,
	)
}

func BuildRecallSessionSummaryChunk(windowIndex, windowCount, chunkIndex, chunkCount int) Instructions {
	return New(fmt.Sprintf(`
# Recall Session Summary Chunk

Summarize chunk %d of %d from oversized recall window %d of %d into the structured handoff format.`,
		chunkIndex,
		chunkCount,
		windowIndex,
		windowCount,
	),
	).Append(
		BuildSessionSummary()...,
	).Append(
		New(`
Scope: Use only the provided recall chunk and any provided authoritative prior summary.
Priority: Preserve concrete facts, decisions, constraints, unresolved questions, and next actions from this chunk.
Do not add claims that are not supported by this recall chunk.`,
		)...,
	)
}

func BuildWebExtractSummary(maxSummaryChars int) string {
	return fmt.Sprintf(`
# Web Extract Summary

Condense the extracted web page into markdown that is compact enough for agent context.
Retain verifiable facts, dates, names, numbers, source details, decisions, and action items.
Keep short quoted passages, code fragments, or technical specifics only when they materially affect the answer.
When a query is provided, organize the summary around that query before covering secondary context.
Do not add claims that are not supported by the extracted content.
Keep the summary under %d characters.`, maxSummaryChars)
}

func BuildWebExtractChunkSummary(maxSummaryChars, chunkIndex, chunkCount int) string {
	return fmt.Sprintf(`
# Web Extract Chunk Summary

Condense chunk %d of %d from an extracted web page.
Retain verifiable facts, dates, names, numbers, source details, decisions, and action items from this chunk.
Keep short quoted passages, code fragments, or technical specifics only when they materially affect the answer.
Do not add context from other chunks or claims that are not supported by this chunk.
Keep the chunk summary under %d characters.`, chunkIndex, chunkCount, maxSummaryChars)
}

func BuildWebExtractSynthesis(maxSummaryChars int) string {
	return fmt.Sprintf(`
# Web Extract Summary Synthesis

Combine chunk summaries from one extracted web page into a single markdown summary for agent context.
Remove repeated details while preserving verifiable facts, dates, names, numbers, source details, decisions, and action items.
When a query is provided, organize the final summary around that query before covering secondary context.
Do not add claims that are not supported by the chunk summaries.
Keep the final summary under %d characters.`, maxSummaryChars)
}

func BuildRetrievalRerank() string {
	return strings.Join([]string{
		"Rank the retrieval candidates for the query.",
		"Return only candidate IDs from the input.",
		"Do not rewrite candidate text or metadata.",
		"Return JSON with an items array ordered from best to worst.",
		"Each item must include candidate_id and score.",
	}, "\n")
}

func cleanList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned
}
