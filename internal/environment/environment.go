package environment

import (
	gctx "context"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/identity"
)

// Environment holds the agent's runtime dependencies, including config and initialized context.
type Environment struct {
	gctx gctx.Context
	cfg  *config.Config
	ctx  *context.Context
}

// NewEnvironment creates the agent environment from the application context and config.
func NewEnvironment(ctx gctx.Context, cfg *config.Config) *Environment {
	return &Environment{
		gctx: ctx,
		cfg:  cfg,
		ctx:  context.NewContext(ctx, cfg),
	}
}

// Prepare prepares the environment for the agent to run.
func (e *Environment) Prepare() error {
	return e.prepareIdentity()
}

// prepareIdentity prepares the identity of the agent.
func (e *Environment) prepareIdentity() error {
	e.ctx.AddInstruction(identity.GetBaseIdentity(e.cfg.Name))
	return nil
}

// GetInstructions returns the instructions for the environment.
func (e *Environment) GetInstructions() context.Instructions {
	return e.ctx.GetInstructions()
}
