package environment

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/personality"
	"github.com/wandxy/hand/internal/tools"
	nativetools "github.com/wandxy/hand/internal/tools/native"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/internal/workspace"
)

var (
	loadWorkspaceRules = workspace.Load
	loadPersonality    = personality.Load
)

const configInstructInstructionName = "config.instruct"

type Environment interface {
	// Prepare registers native tools and builds system instructions from config,
	// personality overlays, workspace rules, and optional config instruct. Call once
	// before using Instructions or Tools for a run.
	Prepare() error

	// Instructions returns a copy of the base prompts.
	Instructions() instructions.Instructions

	// Tools returns the registry for tools.
	Tools() ToolRegistry

	// ToolPolicy reflects configured capabilities and platform for resolving and gating tools.
	ToolPolicy() tools.Policy

	// NewIterationBudget creates the tool-calling iteration limit from config (max iterations).
	NewIterationBudget() IterationBudget

	// NewTraceSession opens a trace sink for this agent run when debug tracing is enabled.
	NewTraceSession() trace.Session
}

type environment struct {
	ctx          context.Context
	cfg          *config.Config
	instructions instructions.Instructions
	tools        tools.Registry
	traces       trace.Factory
	runtime      *Runtime
}

type ToolRegistry interface {
	GetGroup(string) (tools.Group, bool)
	List() []tools.Definition
	ListGroups() []tools.Group
	Resolve(tools.Policy) ([]tools.Definition, error)
	Invoke(context.Context, tools.Call) (tools.Result, error)
}

func (e *environment) ToolPolicy() tools.Policy {
	if e == nil || e.cfg == nil {
		return tools.Policy{
			Capabilities: tools.Capabilities{
				Filesystem: true,
				Network:    true,
				Exec:       true,
				Memory:     true,
			},
			Platform: "cli",
		}
	}

	return tools.Policy{
		Capabilities: tools.Capabilities{
			Filesystem: *e.cfg.CapFilesystem,
			Network:    *e.cfg.CapNetwork,
			Exec:       *e.cfg.CapExec,
			Memory:     *e.cfg.CapMemory,
			Browser:    *e.cfg.CapBrowser,
		},
		Platform: e.cfg.Platform,
	}
}

func NewEnvironment(ctx context.Context, cfg *config.Config) Environment {
	registry := tools.NewInMemoryRegistry()
	traceFactory := trace.NoopFactory()

	if cfg != nil && cfg.DebugTraces {
		traceDir := cfg.DebugTraceDir
		if traceDir == "" {
			traceDir = datadir.DebugTraceDir()
		}
		traceFactory = trace.NewFactory(traceDir, guardrails.NewRedactor())
	}

	return &environment{
		ctx:          ctx,
		cfg:          cfg,
		instructions: instructions.Instructions{},
		tools:        registry,
		traces:       traceFactory,
	}
}

func (e *environment) Prepare() error {
	if err := e.prepareTools(); err != nil {
		return err
	}
	return e.prepareInstructions()
}

func (e *environment) prepareTools() error {
	if e.runtime == nil {
		e.runtime = NewRuntime(e.fileRoots(), e.commandPolicy())
	}

	if err := e.tools.RegisterGroup(tools.Group{Name: "core"}); err != nil {
		return err
	}

	definitions := []tools.Definition{
		nativetools.TimeDefinition(),
		nativetools.ListFilesDefinition(e.runtime),
		nativetools.ReadFileDefinition(e.runtime),
		nativetools.SearchFilesDefinition(e.runtime),
		nativetools.WriteFileDefinition(e.runtime),
		nativetools.PatchDefinition(e.runtime),
		nativetools.TodoDefinition(e.runtime),
		nativetools.RunCommandDefinition(e.runtime),
	}

	for _, definition := range definitions {
		if err := e.tools.Register(definition); err != nil {
			return err
		}
	}

	return nil
}

func (e *environment) prepareInstructions() error {
	for _, instruction := range instructions.BuildBase(e.cfg.Name) {
		e.addInstruction(instruction)
	}

	personalityOverlay, err := loadPersonality()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load personality overlays")
	} else if personalityOverlay.Found {
		e.addInstruction(instructions.Instruction{Value: personalityOverlay.Content})
	}

	workspaceRules, err := loadWorkspaceRules(e.cfg.RulesFiles...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load workspace rules")
		return nil
	}

	if workspaceRules.Found {
		e.addInstruction(instructions.Instruction{Value: workspaceRules.Content})
	}

	if e.cfg != nil && e.cfg.Instruct != "" {
		e.setInstruction(instructions.Instruction{Name: configInstructInstructionName, Value: e.cfg.Instruct})
	}

	return nil
}

func (e *environment) Instructions() instructions.Instructions {
	if e == nil {
		return nil
	}
	copied := make(instructions.Instructions, len(e.instructions))
	copy(copied, e.instructions)
	return copied
}

func (e *environment) Tools() ToolRegistry {
	return e.tools
}

func (e *environment) NewIterationBudget() IterationBudget {
	if e == nil || e.cfg == nil || e.cfg.MaxIterations <= 0 {
		return NewIterationBudget(config.DefaultMaxIterations)
	}
	return NewIterationBudget(e.cfg.MaxIterations)
}

func (e *environment) NewTraceSession() trace.Session {
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

func (e *environment) fileRoots() []string {
	if e == nil || e.cfg == nil || len(e.cfg.FSRoots) == 0 {
		return guardrails.NormalizeRoots(nil)
	}
	return guardrails.NormalizeRoots(e.cfg.FSRoots)
}

func (e *environment) commandPolicy() guardrails.CommandPolicy {
	if e == nil || e.cfg == nil {
		return guardrails.CommandPolicy{}
	}

	return guardrails.CommandPolicy{
		Allow: e.cfg.ExecAllow,
		Ask:   e.cfg.ExecAsk,
		Deny:  e.cfg.ExecDeny,
	}.Normalize()
}

func (e *environment) addInstruction(instruction instructions.Instruction) {
	e.instructions = append(e.instructions, instruction)
}

func (e *environment) setInstruction(instruction instructions.Instruction) {
	instruction.Name = strings.TrimSpace(instruction.Name)
	instruction.Value = strings.TrimSpace(instruction.Value)

	if instruction.Name == "" {
		e.addInstruction(instruction)
		return
	}

	for idx, existing := range e.instructions {
		if existing.Name != instruction.Name {
			continue
		}

		if instruction.Value == "" {
			e.instructions = append(e.instructions[:idx], e.instructions[idx+1:]...)
			return
		}

		e.instructions[idx] = instruction
		return
	}

	if instruction.Value != "" {
		e.instructions = append(e.instructions, instruction)
	}
}

var _ Environment = (*environment)(nil)
