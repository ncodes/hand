package context

import (
	gctx "context"

	"github.com/wandxy/hand/internal/config"
)

// Context holds the agent's runtime context and instructions.
type Context struct {
	cfg          *config.Config
	ctx          gctx.Context
	instructions Instructions
}

// NewContext creates a new context with the given configuration and go context.
func NewContext(ctx gctx.Context, cfg *config.Config) *Context {
	return &Context{cfg: cfg, ctx: ctx, instructions: Instructions{}}
}

// AddInstruction adds a new instruction to the context.
func (c *Context) AddInstruction(instruction Instruction) {
	c.instructions = append(c.instructions, instruction)
}

// GetInstructions returns the instructions for the context.
func (c *Context) GetInstructions() Instructions {
	return c.instructions
}
