package environment

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/environment/budget"
	"github.com/wandxy/hand/internal/environment/planstore"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/memory"
	memguardrails "github.com/wandxy/hand/internal/memory/guardrails"
	"github.com/wandxy/hand/internal/personality"
	webprovider "github.com/wandxy/hand/internal/providers/web"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/listfiles"
	"github.com/wandxy/hand/internal/tools/memorysearch"
	"github.com/wandxy/hand/internal/tools/patch"
	"github.com/wandxy/hand/internal/tools/plan"
	"github.com/wandxy/hand/internal/tools/process"
	"github.com/wandxy/hand/internal/tools/readfile"
	"github.com/wandxy/hand/internal/tools/runcommand"
	"github.com/wandxy/hand/internal/tools/searchfiles"
	"github.com/wandxy/hand/internal/tools/sessionmessages"
	"github.com/wandxy/hand/internal/tools/sessionsearch"
	"github.com/wandxy/hand/internal/tools/time"
	"github.com/wandxy/hand/internal/tools/webextract"
	"github.com/wandxy/hand/internal/tools/websearch"
	"github.com/wandxy/hand/internal/tools/writefile"
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
	NewIterationBudget() budget.IterationBudget

	// NewTraceSession opens a trace sink for the given storage session when debug tracing is enabled.
	NewTraceSession(sessionID string) trace.Session

	// MemoryProvider returns the configured durable memory provider, when enabled.
	MemoryProvider() memory.Provider

	// CurrentPlan returns the in-memory plan state for the given session.
	CurrentPlan(sessionID string) planstore.Plan

	// HydratePlan seeds the in-memory plan state for the given session.
	HydratePlan(sessionID string, plan planstore.Plan)

	// SetStateManager wires state-backed features into the environment runtime.
	SetStateManager(*statemanager.Manager)
}

type environment struct {
	ctx          context.Context
	cfg          *config.Config
	instructions instructions.Instructions
	workspace    workspace.Result
	tools        tools.Registry
	traces       trace.Factory
	memory       memory.Provider
	runtime      *Runtime
	stateMgr     *statemanager.Manager
}

type ToolRegistry interface {
	GetGroup(string) (tools.Group, bool)
	List() tools.Definitions
	ListGroups() []tools.Group
	Resolve(tools.Policy) (tools.Definitions, error)
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
			Filesystem: *e.cfg.Cap.Filesystem,
			Network:    *e.cfg.Cap.Network,
			Exec:       *e.cfg.Cap.Exec,
			Memory:     *e.cfg.Cap.Memory,
			Browser:    *e.cfg.Cap.Browser,
		},
		Platform: e.cfg.Platform,
	}
}

func (e *environment) MemoryProvider() memory.Provider {
	if e == nil {
		return nil
	}
	return e.memory
}

func NewEnvironment(ctx context.Context, cfg *config.Config) Environment {
	registry := tools.NewInMemoryRegistry()
	traceFactory := trace.NoopFactory()

	if cfg != nil && cfg.Debug.Traces {
		traceDir := cfg.Debug.TraceDir
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
	if e == nil {
		return errors.New("environment is required")
	}

	if e.cfg == nil {
		return errors.New("config is required")
	}

	e.cfg.Normalize()

	e.prepareInstructions()

	if err := e.prepareMemory(); err != nil {
		return err
	}

	return e.prepareTools()
}

func (e *environment) prepareMemory() error {
	if e == nil || e.cfg == nil || !e.cfg.MemoryEnabled() {
		e.memory = nil
		return nil
	}

	provider, err := memory.NewProvider(e.cfg.Memory.Provider, memory.Options{
		Guardrails: memguardrails.New(guardrails.NewRedactor()),
	})
	if err != nil {
		return err
	}

	e.memory = provider
	return nil
}

func (e *environment) prepareTools() error {
	if e.stateMgr == nil {
		return errors.New("state manager is required")
	}

	if e.runtime == nil {
		e.runtime = NewRuntime(e.fileRoots(), e.commandPolicy(), e.stateMgr)
	}
	e.runtime.memory = e.memory

	if err := e.tools.RegisterGroup(tools.Group{Name: "core"}); err != nil {
		return err
	}

	definitions := tools.Definitions{
		time.Definition(),
		listfiles.Definition(e.runtime),
		readfile.Definition(e.runtime),
		searchfiles.Definition(e.runtime),
		writefile.Definition(e.runtime),
		patch.Definition(e.runtime),
		plan.Definition(e.runtime),
		process.Definition(e.runtime),
		runcommand.Definition(e.runtime),
		sessionsearch.Definition(e.runtime),
		sessionmessages.Definition(e.runtime),
	}

	if definition, ok, err := e.memorySearchDefinition(); err != nil {
		return err
	} else if ok {
		definitions = append(definitions, definition)
	}

	webProvider, err := webprovider.NewProvider(e.cfg)

	switch {
	case errors.Is(err, webprovider.ErrProviderNotConfigured):
	case err != nil:
		return err
	default:
		websitePolicy := guardrails.NewWebsitePolicy(
			e.cfg.Web.BlockedDomainsEnabled,
			e.cfg.Web.BlockedDomains,
			e.cfg.Web.BlockedDomainFiles,
		)

		if e.cfg.Web.CacheTTL > 0 {
			webProvider = webprovider.NewCachedProvider(
				webProvider,
				webprovider.CacheOptions{
					ProviderName: e.cfg.Web.Provider,
					TTL:          e.cfg.Web.CacheTTL,
				},
			)
		}

		definitions = append(definitions,
			webextract.Definition(
				webProvider,
				webextract.Options{
					MaxExtractCharPerResult:        e.cfg.Web.MaxExtractCharPerResult,
					MinSummarizeChars:              e.cfg.Web.ExtractMinSummarizeChars,
					MaxSummaryChars:                e.cfg.Web.ExtractMaxSummaryChars,
					MaxSummaryChunkChars:           e.cfg.Web.ExtractMaxSummaryChunkChars,
					SummarizeRefusalThresholdChars: e.cfg.Web.ExtractRefusalThresholdChars,
					WebsitePolicy:                  websitePolicy,
				},
			),
		)

		if e.cfg.Web.Provider != webprovider.ProviderNative {
			definitions = append(definitions,
				websearch.Definition(
					webProvider,
					websearch.Options{
						WebsitePolicy: websitePolicy,
					},
				),
			)
		}
	}

	for _, definition := range definitions {
		if err := e.tools.Register(definition); err != nil {
			return err
		}

		e.addInstruction(definition.UsageInstruction)
	}

	return nil
}

func (e *environment) memorySearchDefinition() (tools.Definition, bool, error) {
	if e == nil || e.runtime == nil {
		return tools.Definition{}, false, nil
	}
	ok, err := e.runtime.SupportsMemorySearch(e.ctx)
	if err != nil {
		return tools.Definition{}, false, err
	}
	if !ok {
		return tools.Definition{}, false, nil
	}

	return memorysearch.Definition(e.runtime), true, nil
}

func (e *environment) prepareInstructions() {
	e.addInstruction(instructions.BuildPlanningPolicy())

	for _, instruction := range instructions.BuildBase(e.cfg.Name) {
		e.addInstruction(instruction)
	}

	personalityOverlay, err := loadPersonality()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load personality overlays")
	} else if personalityOverlay.Found {
		e.addInstruction(instructions.Instruction{Value: personalityOverlay.Content})
	}

	workspaceRules, err := loadWorkspaceRules(e.cfg.Rules.Files...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load workspace rules")
		return
	}

	if workspaceRules.Found {
		e.workspace = workspaceRules
		e.addInstruction(instructions.Instruction{Value: workspaceRules.Content})
	}

	if e.cfg != nil && e.cfg.Session.Instruct != "" {
		e.setInstruction(instructions.Instruction{Name: configInstructInstructionName, Value: e.cfg.Session.Instruct})
	}
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

func (e *environment) NewIterationBudget() budget.IterationBudget {
	if e == nil || e.cfg == nil || e.cfg.Session.MaxIterations <= 0 {
		return budget.New(config.DefaultMaxIterations)
	}
	return budget.New(e.cfg.Session.MaxIterations)
}

func (e *environment) NewTraceSession(sessionID string) trace.Session {
	if e == nil || e.traces == nil {
		return trace.NoopSession()
	}

	metadata := trace.Metadata{Source: "agent"}
	if e.cfg != nil {
		metadata.AgentName = e.cfg.Name
		metadata.Model = e.cfg.Models.Main.Name
		metadata.APIMode = e.cfg.Models.Main.APIMode
	}

	session := e.traces.OpenSession(e.ctx, sessionID, metadata)
	if e.workspace.Truncated {
		session.Record(trace.EvtWorkspaceRulesTruncated, map[string]any{
			"original_length":    e.workspace.OriginalLength,
			"truncated_length":   e.workspace.TruncatedLength,
			"max_content_length": e.workspace.MaxContentLength,
			"marker":             e.workspace.TruncationMarker,
		})
	}

	return session
}

func (e *environment) CurrentPlan(sessionID string) planstore.Plan {
	if e == nil || e.runtime == nil {
		return planstore.Plan{}
	}
	return e.runtime.GetPlan(sessionID)
}

func (e *environment) HydratePlan(sessionID string, plan planstore.Plan) {
	if e == nil || e.runtime == nil {
		return
	}
	e.runtime.HydratePlan(sessionID, plan)
}

func (e *environment) SetStateManager(manager *statemanager.Manager) {
	if e == nil {
		return
	}
	e.stateMgr = manager
	if e.runtime != nil {
		e.runtime.stateMgr = manager
	}
}

func (e *environment) fileRoots() []string {
	if e == nil || e.cfg == nil || len(e.cfg.FS.Roots) == 0 {
		return guardrails.NormalizeRoots(nil)
	}
	return guardrails.NormalizeRoots(e.cfg.FS.Roots)
}

func (e *environment) commandPolicy() guardrails.CommandPolicy {
	if e == nil || e.cfg == nil {
		return guardrails.CommandPolicy{}
	}

	return guardrails.CommandPolicy{
		Allow: e.cfg.Exec.Allow,
		Ask:   e.cfg.Exec.Ask,
		Deny:  e.cfg.Exec.Deny,
	}.Normalize()
}

func (e *environment) addInstruction(instruction instructions.Instruction) {
	if strings.TrimSpace(instruction.Value) == "" {
		return
	}

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
