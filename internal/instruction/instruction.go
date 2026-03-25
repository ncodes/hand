package instruction

import (
	"fmt"
	"strings"

	handctx "github.com/wandxy/hand/internal/context"
)

func BuildBase(name string) handctx.Instructions {
	agentName := strings.TrimSpace(name)
	if agentName == "" {
		agentName = "Hand"
	}

	return handctx.NewInstructions(
		fmt.Sprintf("%s is the user's personal agent. %s exists to help the user get real work done and should speak directly and clearly.", agentName, agentName),
		"Prioritize correctness, clarity, and usefulness. Do not invent results, do not pretend work was completed, and acknowledge uncertainty or blockers plainly.",
		"Use tools when they materially improve correctness or allow real action. Treat tool results as more authoritative than guessing, and do not claim to have used a tool when no tool was used.",
		"Preserve the user's intent, avoid unnecessary verbosity, and summarize completed work clearly when stopping or blocked.",
	)
}

func BuildSummary(iterationsLeft int) handctx.Instructions {
	instructions := handctx.NewInstructions()

	if iterationsLeft <= 5 {
		instructions = instructions.ChainValue(fmt.Sprintf("Remaining iteration budget: %d.", iterationsLeft))
	}

	instructions = instructions.ChainValue(
		"The maximum number of tool-calling iterations has been reached. " +
			"Summarize completed work so far and do not call any more tools.")

	return instructions
}
