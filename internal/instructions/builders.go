package instructions

import (
	"fmt"
	"strings"
	"time"
)

const (
	PlanningPolicyInstructionName     = "planning.policy"
	EnvironmentContextInstructionName = "environment.context"
)

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

func BuildPlanningPolicy() Instruction {
	return Instruction{
		Name: PlanningPolicyInstructionName,
		Value: `# Planning Policy

Use plan_tool for tasks with 3 or more meaningful steps, multiple user asks in one request, or longer workflows involving several tool calls.
Do not use plan_tool for trivial one-step work or direct factual answers.
When using plan_tool, keep exactly one step in_progress while active work remains.
Mark steps completed immediately when done.
If a step becomes invalid or fails, cancel it and add a revised replacement step.`,
	}
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
	return New(
		"# Session Summary Task\n\nCreate a structured handoff summary of the provided chat history for another assistant that will continue the work.",
		"Goal: Capture the current progress and important decisions made so far.",
		"Context preservation: Preserve important context, hard constraints, user preferences, and any critical examples or references needed to continue without redoing work.",
		"Remaining work: Make the remaining work explicit through unresolved questions and concrete next actions.",
		`Output format:

Return JSON only with this exact shape:
{
  "session_summary": "required concise summary",
  "current_task": "current task or empty string",
  "discoveries": ["important discovery"],
  "open_questions": ["open question"],
  "next_actions": ["next action"]
}`,
		"Output rules: Do not include markdown fences or extra commentary.",
	)
}

func BuildWebExtractSummary(maxSummaryChars int) string {
	return fmt.Sprintf(`# Web Extract Summary

Condense the extracted web page into markdown that is compact enough for agent context.
Retain verifiable facts, dates, names, numbers, source details, decisions, and action items.
Keep short quoted passages, code fragments, or technical specifics only when they materially affect the answer.
When a query is provided, organize the summary around that query before covering secondary context.
Do not add claims that are not supported by the extracted content.
Keep the summary under %d characters.`, maxSummaryChars)
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
