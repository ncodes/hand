package environment

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/guardrails"
	instructionpkg "github.com/wandxy/hand/internal/instruction"
	"github.com/wandxy/hand/internal/personality"
	"github.com/wandxy/hand/internal/tools"
	nativetools "github.com/wandxy/hand/internal/tools/native"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/internal/workspace"
)

var loadWorkspaceRules = workspace.Load
var loadPersonality = personality.Load

type Environment struct {
	ctx     context.Context
	cfg     *config.Config
	handCtx *handctx.Context
	tools   tools.Registry
	traces  trace.Factory
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

func NewEnvironment(ctx context.Context, cfg *config.Config) *Environment {
	registry := tools.NewInMemoryRegistry()
	traceFactory := trace.NoopFactory()
	if cfg != nil && cfg.DebugTraces {
		traceDir := cfg.DebugTraceDir
		if traceDir == "" {
			traceDir = datadir.DebugTraceDir()
		}
		traceFactory = trace.NewFactory(traceDir, guardrails.NewRedactor())
	}
	return &Environment{
		ctx:     ctx,
		cfg:     cfg,
		handCtx: handctx.NewContext(ctx, cfg),
		tools:   registry,
		traces:  traceFactory,
	}
}

func (e *Environment) Prepare() error {
	if err := e.prepareTools(); err != nil {
		return err
	}
	return e.prepareInstructions()
}

func (e *Environment) prepareTools() error {
	return nativetools.Register(e.tools)
}

func (e *Environment) prepareInstructions() error {

	// build base instructions
	for _, instruction := range instructionpkg.BuildBase(e.cfg.Name) {
		e.handCtx.AddInstruction(instruction)
	}

	// load personality overlay
	personalityOverlay, err := loadPersonality()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load personality overlays")
	} else if personalityOverlay.Found {
		e.handCtx.AddInstruction(handctx.Instruction{Value: personalityOverlay.Content})
	}

	// load workspace rules
	workspaceRules, err := loadWorkspaceRules(e.cfg.RulesFiles...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load workspace rules")
		return nil
	}
	if workspaceRules.Found {
		e.handCtx.AddInstruction(handctx.Instruction{Value: workspaceRules.Content})
	}

	return nil
}

func (e *Environment) Context() Context {
	return e.handCtx
}

func (e *Environment) Tools() ToolRegistry {
	return e.tools
}

func (e *Environment) NewIterationBudget() IterationBudget {
	if e == nil || e.cfg == nil || e.cfg.MaxIterations <= 0 {
		return NewIterationBudget(config.DefaultMaxIterations)
	}
	return NewIterationBudget(e.cfg.MaxIterations)
}

func (e *Environment) NewTraceSession() trace.Session {
	if e == nil || e.traces == nil {
		return trace.NoopSession()
	}
	metadata := trace.Metadata{Source: "agent"}
	if e.cfg != nil {
		metadata.AgentName = e.cfg.Name
		metadata.Model = e.cfg.Model
		metadata.APIMode = e.cfg.ModelAPIMode
	}
	return e.traces.NewSession(e.ctx, metadata)
}
