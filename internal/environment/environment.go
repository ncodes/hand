package environment

import (
	"context"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/identity"
)

// Environment holds the agent's runtime dependencies, including config and initialized context.
type Environment struct {
	ctx  context.Context
	cfg  *config.Config
	hctx *handctx.Context
}

type Context interface {
	GetInstructions() handctx.Instructions
	AddUserMessage(string) error
	AddAssistantMessage(string) error
	GetMessages() []handctx.Message
	GetConversation() handctx.Conversation
}

// NewEnvironment creates the agent environment from the application context and config.
func NewEnvironment(ctx context.Context, cfg *config.Config) *Environment {
	return &Environment{
		ctx:  ctx,
		cfg:  cfg,
		hctx: handctx.NewContext(ctx, cfg),
	}
}

// Prepare prepares the environment for the agent to run.
func (e *Environment) Prepare() error {
	return e.prepareIdentity()
}

// prepareIdentity prepares the identity of the agent.
func (e *Environment) prepareIdentity() error {
	e.hctx.AddInstruction(identity.GetBaseIdentity(e.cfg.Name))
	return nil
}

func (e *Environment) Context() Context {
	return e.hctx
}
