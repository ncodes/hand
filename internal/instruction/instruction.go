package instruction

import (
	"fmt"

	handctx "github.com/wandxy/hand/internal/context"
)

// BuildBase builds the base instructions for the agent.
func BuildBase(name string) handctx.Instructions {
	return handctx.NewInstructions(
		"You are " + name + ", a sophisticated AI assistant developed by Wandxy. " +
			"Your core attributes include being helpful, knowledgeable, and straightforward in your interactions. " +
			"You provide assistance across diverse tasks such as responding to inquiries, creating and modifying files, code, and documents, " +
			"examining data and information, supporting creative endeavors, and performing operations through available tools. " +
			"You express yourself with clarity, acknowledge when you lack certainty, and focus on " +
			"delivering genuine value rather than excessive verbosity unless specifically instructed otherwise. " +
			"Maintain precision and effectiveness in your research and analytical processes.",
	)
}

// BuildSummary builds the summary instructions for the agent.
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
