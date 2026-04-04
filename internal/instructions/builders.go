package instructions

import (
	"fmt"
	"strings"
)

func BuildBase(name string) Instructions {
	agentName := strings.TrimSpace(name)
	if agentName == "" {
		agentName = "Hand"
	}

	return New(
		fmt.Sprintf("%s is the user's personal agent. %s exists to help the user get real work done and should speak directly and clearly.", agentName, agentName),
		"Prioritize correctness, clarity, and usefulness. Do not invent results, do not pretend work was completed, and acknowledge uncertainty or blockers plainly.",
		"Use tools when they materially improve correctness or allow real action. Treat tool results as more authoritative than guessing, and do not claim to have used a tool when no tool was used.",
		"Preserve the user's intent, avoid unnecessary verbosity, and summarize completed work clearly when stopping or blocked.",
	)
}

func BuildSummary(iterationsLeft int) Instructions {
	instructions := New()
	if iterationsLeft <= 5 {
		instructions = instructions.AppendValue(fmt.Sprintf("Remaining iteration budget: %d.", iterationsLeft))
	}
	instructions = instructions.AppendValue(
		"The maximum number of tool-calling iterations has been reached. Summarize completed work so far and do not call any more tools.",
	)
	return instructions
}

func BuildSessionSummary() Instructions {
	return New(
		"Create a structured handoff summary of the provided chat history for another assistant that will continue the work.",
		"Capture the current progress and important decisions made so far.",
		"Preserve important context, hard constraints, user preferences, and any critical examples or references needed to continue without redoing work.",
		"Make the remaining work explicit through unresolved questions and concrete next actions.",
		`Return JSON only with this exact shape:
{
  "session_summary": "required concise summary",
  "current_task": "current task or empty string",
  "discoveries": ["important discovery"],
  "open_questions": ["open question"],
  "next_actions": ["next action"]
}`,
		"Do not include markdown fences or extra commentary.",
	)
}
