package environment

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	envbudget "github.com/wandxy/hand/internal/environment/budget"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/personality"
	webintegration "github.com/wandxy/hand/internal/providers/web"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/tools"
	listfiles "github.com/wandxy/hand/internal/tools/listfiles"
	"github.com/wandxy/hand/internal/tools/patch"
	"github.com/wandxy/hand/internal/tools/plan"
	"github.com/wandxy/hand/internal/tools/process"
	readfile "github.com/wandxy/hand/internal/tools/readfile"
	runcommand "github.com/wandxy/hand/internal/tools/runcommand"
	searchfiles "github.com/wandxy/hand/internal/tools/searchfiles"
	sessionsearch "github.com/wandxy/hand/internal/tools/sessionsearch"
	"github.com/wandxy/hand/internal/tools/time"
	webextract "github.com/wandxy/hand/internal/tools/webextract"
	websearch "github.com/wandxy/hand/internal/tools/websearch"
	writefile "github.com/wandxy/hand/internal/tools/writefile"
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
	NewIterationBudget() envbudget.IterationBudget

	// NewTraceSession opens a trace sink for the given storage session when debug tracing is enabled.
	NewTraceSession(sessionID string) trace.Session

	// CurrentPlan returns the in-memory plan state for the given session.
	CurrentPlan(sessionID string) envtypes.Plan

	// HydratePlan seeds the in-memory plan state for the given session.
	HydratePlan(sessionID string, plan envtypes.Plan)

	// SetSessionManager wires session-backed features into the environment runtime.
	SetSessionManager(*sessionstore.Manager)
}

type environment struct {
	ctx          context.Context
	cfg          *config.Config
	instructions instructions.Instructions
	workspace    workspace.Result
	tools        tools.Registry
	traces       trace.Factory
	runtime      *Runtime
	sessionMgr   *sessionstore.Manager
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
	if e == nil {
		return errors.New("environment is required")
	}
	if e.cfg == nil {
		return errors.New("config is required")
	}

	e.cfg.Normalize()

	if err := e.prepareTools(); err != nil {
		return err
	}
	return e.prepareInstructions()
}

func (e *environment) prepareTools() error {
	if e.runtime == nil {
		e.runtime = NewRuntime(e.fileRoots(), e.commandPolicy(), e.sessionMgr)
	}

	if err := e.tools.RegisterGroup(tools.Group{Name: "core"}); err != nil {
		return err
	}

	definitions := []tools.Definition{
		time.Definition(),
		listfiles.Definition(e.runtime),
		readfile.Definition(e.runtime),
		searchfiles.Definition(e.runtime),
		writefile.Definition(e.runtime),
		patch.Definition(e.runtime),
		plan.Definition(e.runtime),
		process.Definition(e.runtime),
		runcommand.Definition(e.runtime),
	}
	if e.sessionMgr != nil {
		definitions = append(definitions, sessionsearch.Definition(e.runtime))
	}

	webProvider, err := webintegration.NewProvider(e.cfg)

	switch {
	case errors.Is(err, webintegration.ErrProviderNotConfigured):
	case err != nil:
		return err
	default:
		websitePolicy := guardrails.NewWebsitePolicy(
			e.cfg.WebBlockedDomainsEnabled,
			e.cfg.WebBlockedDomains,
			e.cfg.WebBlockedDomainFiles,
		)

		if e.cfg.WebCacheTTL > 0 {
			webProvider = webintegration.NewCachedProvider(
				webProvider,
				webintegration.CacheOptions{
					ProviderName: e.cfg.WebProvider,
					TTL:          e.cfg.WebCacheTTL,
				},
			)
		}

		definitions = append(definitions,
			webextract.Definition(
				webProvider,
				webextract.Options{
					MaxExtractCharPerResult:        e.cfg.WebMaxExtractCharPerResult,
					MinSummarizeChars:              e.cfg.WebExtractMinSummarizeChars,
					MaxSummaryChars:                e.cfg.WebExtractMaxSummaryChars,
					MaxSummaryChunkChars:           e.cfg.WebExtractMaxSummaryChunkChars,
					SummarizeRefusalThresholdChars: e.cfg.WebExtractRefusalThresholdChars,
					WebsitePolicy:                  websitePolicy,
				},
			),
		)

		if e.cfg.WebProvider != webintegration.ProviderNative {
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
	}

	return nil
}

func (e *environment) prepareInstructions() error {
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

	workspaceRules, err := loadWorkspaceRules(e.cfg.RulesFiles...)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load workspace rules")
		return nil
	}

	if workspaceRules.Found {
		e.workspace = workspaceRules
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

func (e *environment) NewIterationBudget() envbudget.IterationBudget {
	if e == nil || e.cfg == nil || e.cfg.MaxIterations <= 0 {
		return envbudget.New(config.DefaultMaxIterations)
	}
	return envbudget.New(e.cfg.MaxIterations)
}

func (e *environment) NewTraceSession(sessionID string) trace.Session {
	if e == nil || e.traces == nil {
		return trace.NoopSession()
	}

	metadata := trace.Metadata{Source: "agent"}
	if e.cfg != nil {
		metadata.AgentName = e.cfg.Name
		metadata.Model = e.cfg.Model
		metadata.APIMode = e.cfg.ModelAPIMode
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

func (e *environment) CurrentPlan(sessionID string) envtypes.Plan {
	if e == nil || e.runtime == nil {
		return envtypes.Plan{}
	}
	return e.runtime.GetPlan(sessionID)
}

func (e *environment) HydratePlan(sessionID string, plan envtypes.Plan) {
	if e == nil || e.runtime == nil {
		return
	}
	e.runtime.HydratePlan(sessionID, plan)
}

func (e *environment) SetSessionManager(manager *sessionstore.Manager) {
	if e == nil {
		return
	}
	e.sessionMgr = manager
	if e.runtime != nil {
		e.runtime.sessionMgr = manager
	}
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
