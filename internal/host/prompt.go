package host

import (
	"context"

	"github.com/wandxy/hand/internal/environment"
	instruct "github.com/wandxy/hand/internal/instructions"
	agentprompt "github.com/wandxy/hand/pkg/agent/prompt"
)

type PromptProvider struct {
	env environment.Environment
}

func NewPromptProvider(env environment.Environment) *PromptProvider {
	return &PromptProvider{env: env}
}

func (p *PromptProvider) LoadBaseInstructions(
	context.Context,
	agentprompt.RunContext,
) (agentprompt.Instructions, error) {
	if p == nil || p.env == nil {
		return nil, nil
	}

	return promptInstructionsFromInstructions(p.env.Instructions()), nil
}

func (p *PromptProvider) BuildEnvironmentInstruction(
	context.Context,
	agentprompt.EnvironmentInput,
) (agentprompt.Instruction, error) {
	return agentprompt.Instruction{}, nil
}

func promptInstructionsFromInstructions(instructions instruct.Instructions) agentprompt.Instructions {
	if len(instructions) == 0 {
		return nil
	}

	result := make(agentprompt.Instructions, 0, len(instructions))
	for _, instruction := range instructions {
		result = append(result, agentprompt.Instruction{
			Name:  instruction.Name,
			Value: instruction.Value,
		})
	}

	return result
}
