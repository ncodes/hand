package environment

import (
	"context"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/identity"
	"github.com/wandxy/hand/internal/tools"
	nativetools "github.com/wandxy/hand/internal/tools/native"
)

// Environment holds the agent's runtime dependencies, including config and initialized context.
type Environment struct {
	ctx     context.Context
	cfg     *config.Config
	handCtx *handctx.Context
	tools   tools.Registry
}

type Context interface {
	GetInstructions() handctx.Instructions
	AddMessage(handctx.Message) error
	AddUserMessage(string) error
	AddAssistantMessage(string) error
	GetMessages() []handctx.Message
	GetConversation() handctx.Conversation
}

type ToolRegistry interface {
	List() []tools.Definition
	Invoke(context.Context, tools.Call) (tools.Result, error)
}

// NewEnvironment creates the agent environment from the application context and config.
func NewEnvironment(ctx context.Context, cfg *config.Config) *Environment {
	registry := tools.NewInMemoryRegistry()
	return &Environment{
		ctx:     ctx,
		cfg:     cfg,
		handCtx: handctx.NewContext(ctx, cfg),
		tools:   registry,
	}
}

// Prepare prepares the environment for the agent to run.
func (e *Environment) Prepare() error {
	if err := e.prepareTools(); err != nil {
		return err
	}
	return e.prepareIdentity()
}

func (e *Environment) prepareTools() error {
	return nativetools.Register(e.tools)
}

// prepareIdentity prepares the identity of the agent.
func (e *Environment) prepareIdentity() error {
	e.handCtx.AddInstruction(identity.GetBaseIdentity(e.cfg.Name))
	return nil
}

// Context returns the runtime context exposed by the environment.
func (e *Environment) Context() Context {
	return e.handCtx
}

// Tools returns the tool registry exposed by the environment.
func (e *Environment) Tools() ToolRegistry {
	return e.tools
}
