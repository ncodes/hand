package environment

import (
	gctx "context"
	stdctx "context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/personality"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/workspace"
)

func TestNewEnvironment_InitializesDependencies(t *testing.T) {
	baseCtx := gctx.WithValue(gctx.Background(), "requestID", "req-123")
	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}

	env := NewEnvironment(baseCtx, cfg)

	require.Same(t, baseCtx, env.ctx)
	require.Same(t, cfg, env.cfg)
	require.Empty(t, env.Instructions())
}

func TestEnvironment_PrepareAddsFullBaseInstructionStack(t *testing.T) {
	previousPersonality := loadPersonality
	previous := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previous
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}
	env := NewEnvironment(gctx.Background(), cfg)

	err := env.Prepare()

	require.NoError(t, err)
	require.Equal(t, instruct.BuildBase(cfg.Name), env.Instructions())
}

func TestEnvironment_PrepareAppendsWorkspaceRules(t *testing.T) {
	previousPersonality := loadPersonality
	previous := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previous
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(files ...string) (workspace.Result, error) {
		require.Equal(t, []string{"hand.md"}, files)
		return workspace.Result{
			Found:   true,
			Content: "## AGENTS.md\nrepo rules",
		}, nil
	}

	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir, RulesFiles: []string{"hand.md"}}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())

	instructions := env.Instructions()
	require.Len(t, instructions, len(instruct.BuildBase(cfg.Name))+1)
	require.Equal(t, "## AGENTS.md\nrepo rules", instructions[len(instructions)-1].Value)
}

func TestEnvironment_PrepareAppendsPersonalityBeforeWorkspaceRules(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{Found: true, Content: "## SOUL.md\npersona"}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{Found: true, Content: "## AGENTS.md\nrepo rules"}, nil
	}

	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())

	instructions := env.Instructions()
	require.Len(t, instructions, len(instruct.BuildBase(cfg.Name))+2)
	require.Equal(t, "## SOUL.md\npersona", instructions[len(instructions)-2].Value)
	require.Equal(t, "## AGENTS.md\nrepo rules", instructions[len(instructions)-1].Value)
}

func TestEnvironment_PrepareAppendsInstructAfterWorkspaceRules(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{Found: true, Content: "## SOUL.md\npersona"}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{Found: true, Content: "## AGENTS.md\nrepo rules"}, nil
	}

	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: t.TempDir(), Instruct: "be terse"}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())

	instructions := env.Instructions()
	require.Equal(t, "## AGENTS.md\nrepo rules", instructions[len(instructions)-2].Value)
	require.Equal(t, "be terse", instructions[len(instructions)-1].Value)
}

func TestEnvironment_PrepareIgnoresPersonalityLoadError(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, errors.New("personality failed")
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())
	require.Equal(t, instruct.BuildBase(cfg.Name), env.Instructions())
}

func TestEnvironment_PrepareIgnoresWorkspaceRuleLoadError(t *testing.T) {
	previousPersonality := loadPersonality
	previous := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previous
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, errors.New("cwd failed")
	}

	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())
	require.Equal(t, instruct.BuildBase(cfg.Name), env.Instructions())
}

func TestEnvironment_PrepareIncludesConfiguredNameAndToolGuidance(t *testing.T) {
	previousPersonality := loadPersonality
	previous := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previous
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())

	instructions := env.Instructions()
	require.Contains(t, instructions[0].Value, "Test Agent is the user's personal agent")
	require.Contains(t, instructions[2].Value, "Use tools when they materially improve correctness or allow real action")
}

func TestEnvironment_PrepareUsesDefaultIdentityWhenNameIsEmpty(t *testing.T) {
	previousPersonality := loadPersonality
	previous := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previous
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	dir := t.TempDir()
	cfg := &config.Config{DebugTraceDir: dir}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())

	instructions := env.Instructions()
	require.Contains(t, instructions[0].Value, "Hand is the user's personal agent")
}

func TestEnvironment_PrepareRegistersNativeTools(t *testing.T) {
	previousPersonality := loadPersonality
	previous := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previous
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})

	require.NoError(t, env.Prepare())

	tools := env.Tools()
	require.NotNil(t, tools)

	definitions := tools.List()
	require.Len(t, definitions, 8)
	require.Equal(t, []string{"list_files", "patch", "read_file", "run_command", "search_files", "time", "todo", "write_file"}, []string{
		definitions[0].Name,
		definitions[1].Name,
		definitions[2].Name,
		definitions[3].Name,
		definitions[4].Name,
		definitions[5].Name,
		definitions[6].Name,
		definitions[7].Name,
	})
	for _, definition := range definitions {
		require.Equal(t, []string{"core"}, definition.Groups)
	}
	groups := tools.ListGroups()
	require.Len(t, groups, 1)
	require.Equal(t, "core", groups[0].Name)
}

type failingRegistry struct {
	err error
}

func (r failingRegistry) Register(tools.Definition) error {
	return r.err
}

func (failingRegistry) Get(string) (tools.Definition, bool) {
	return tools.Definition{}, false
}

func (failingRegistry) RegisterGroup(tools.Group) error {
	return nil
}

func (failingRegistry) GetGroup(string) (tools.Group, bool) {
	return tools.Group{}, false
}

func (failingRegistry) List() []tools.Definition {
	return nil
}

func (failingRegistry) ListGroups() []tools.Group {
	return nil
}

func (failingRegistry) Resolve(tools.Policy) ([]tools.Definition, error) {
	return nil, nil
}

func (failingRegistry) Invoke(stdctx.Context, tools.Call) (tools.Result, error) {
	return tools.Result{}, nil
}

type failingGroupRegistry struct {
	err error
}

func (failingGroupRegistry) Register(tools.Definition) error {
	return nil
}

func (failingGroupRegistry) Get(string) (tools.Definition, bool) {
	return tools.Definition{}, false
}

func (r failingGroupRegistry) RegisterGroup(tools.Group) error {
	return r.err
}

func (failingGroupRegistry) GetGroup(string) (tools.Group, bool) {
	return tools.Group{}, false
}

func (failingGroupRegistry) List() []tools.Definition {
	return nil
}

func (failingGroupRegistry) ListGroups() []tools.Group {
	return nil
}

func (failingGroupRegistry) Resolve(tools.Policy) ([]tools.Definition, error) {
	return nil, nil
}

func (failingGroupRegistry) Invoke(stdctx.Context, tools.Call) (tools.Result, error) {
	return tools.Result{}, nil
}

func TestEnvironment_PrepareReturnsToolRegistrationError(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
	env.tools = failingRegistry{err: errors.New("register failed")}

	err := env.Prepare()

	require.EqualError(t, err, "register failed")
	require.Empty(t, env.Instructions())
}

func TestEnvironment_PrepareReturnsToolGroupRegistrationError(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
	env.tools = failingGroupRegistry{err: errors.New("group failed")}

	err := env.Prepare()

	require.EqualError(t, err, "group failed")
	require.Empty(t, env.Instructions())
}

func TestEnvironment_PrepareToolsPreservesExistingRuntime(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: t.TempDir()})
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{})
	env.runtime = runtime

	require.NoError(t, env.prepareTools())
	require.Same(t, runtime, env.runtime)
}

func TestEnvironment_InstructionsReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
	env.instructions = append(env.instructions, instruct.Instruction{Value: "hello"})
	instructions := env.Instructions()
	require.Len(t, instructions, 1)
	instructions[0].Value = "changed"
	require.Equal(t, "hello", env.Instructions()[0].Value)
}

func TestEnvironment_InstructionsReturnsNilForNilEnvironment(t *testing.T) {
	var env *Environment
	require.Nil(t, env.Instructions())
}

func TestEnvironment_SetInstructionAddsUnnamedInstruction(t *testing.T) {
	env := &Environment{}

	env.setInstruction(instruct.Instruction{Value: "  hello  "})

	require.Equal(t, instruct.Instructions{{Value: "hello"}}, env.instructions)
}

func TestEnvironment_SetInstructionUpdatesExistingNamedInstruction(t *testing.T) {
	env := &Environment{
		instructions: instruct.Instructions{{Name: configInstructInstructionName, Value: "old"}},
	}

	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "  new  "})

	require.Equal(t, instruct.Instructions{{Name: configInstructInstructionName, Value: "new"}}, env.instructions)
}

func TestEnvironment_SetInstructionRemovesExistingNamedInstructionWhenEmpty(t *testing.T) {
	env := &Environment{
		instructions: instruct.Instructions{
			{Value: "base"},
			{Name: configInstructInstructionName, Value: "old"},
		},
	}

	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "   "})

	require.Equal(t, instruct.Instructions{{Value: "base"}}, env.instructions)
}

func TestEnvironment_SetInstructionAppendsNewNamedInstructionWhenMissing(t *testing.T) {
	env := &Environment{
		instructions: instruct.Instructions{{Value: "base"}},
	}

	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "  new  "})

	require.Equal(t, instruct.Instructions{
		{Value: "base"},
		{Name: configInstructInstructionName, Value: "new"},
	}, env.instructions)
}

func TestEnvironment_SetInstructionSkipsEmptyNewNamedInstruction(t *testing.T) {
	env := &Environment{
		instructions: instruct.Instructions{{Value: "base"}},
	}

	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "   "})

	require.Equal(t, instruct.Instructions{{Value: "base"}}, env.instructions)
}

func TestEnvironment_NewIterationBudgetUsesConfigValue(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{MaxIterations: 12, DebugTraceDir: dir})
	require.Equal(t, 12, env.NewIterationBudget().Remaining())
}

func TestEnvironment_NewIterationBudgetUsesDefaultWhenUnset(t *testing.T) {
	require.Equal(t, config.DefaultMaxIterations, (&Environment{}).NewIterationBudget().Remaining())
}

func TestEnvironment_ToolPolicyUsesCLIPlatformAndLocalCapabilities(t *testing.T) {
	cfg := &config.Config{}
	cfg.Normalize()
	env := NewEnvironment(gctx.Background(), cfg)

	opts := env.ToolPolicy()

	require.Equal(t, "cli", opts.Platform)
	require.True(t, opts.Capabilities.Filesystem)
	require.True(t, opts.Capabilities.Network)
	require.True(t, opts.Capabilities.Exec)
	require.True(t, opts.Capabilities.Memory)
	require.False(t, opts.Capabilities.Browser)
}

func TestEnvironment_ToolPolicyUsesDefaultsForNilEnvironment(t *testing.T) {
	var env *Environment

	opts := env.ToolPolicy()

	require.Equal(t, "cli", opts.Platform)
	require.True(t, opts.Capabilities.Filesystem)
	require.True(t, opts.Capabilities.Network)
	require.True(t, opts.Capabilities.Exec)
	require.True(t, opts.Capabilities.Memory)
	require.False(t, opts.Capabilities.Browser)
}

func TestEnvironment_ToolPolicyUsesDefaultsForNilConfig(t *testing.T) {
	env := &Environment{}

	opts := env.ToolPolicy()

	require.Equal(t, "cli", opts.Platform)
	require.True(t, opts.Capabilities.Filesystem)
	require.True(t, opts.Capabilities.Network)
	require.True(t, opts.Capabilities.Exec)
	require.True(t, opts.Capabilities.Memory)
	require.False(t, opts.Capabilities.Browser)
}

func TestEnvironment_ToolPolicyUsesConfigValues(t *testing.T) {
	cfg := &config.Config{
		Platform:      "desktop",
		CapFilesystem: new(false),
		CapNetwork:    new(false),
		CapExec:       new(true),
		CapMemory:     new(false),
		CapBrowser:    new(true),
	}
	cfg.Normalize()
	env := NewEnvironment(gctx.Background(), cfg)

	opts := env.ToolPolicy()

	require.Equal(t, "desktop", opts.Platform)
	require.False(t, opts.Capabilities.Filesystem)
	require.False(t, opts.Capabilities.Network)
	require.True(t, opts.Capabilities.Exec)
	require.False(t, opts.Capabilities.Memory)
	require.True(t, opts.Capabilities.Browser)
}

func TestEnvironment_FileRootsUsesDefaultsForNilEnvironment(t *testing.T) {
	var env *Environment

	require.Equal(t, guardrails.NormalizeRoots(nil), env.fileRoots())
}

func TestEnvironment_FileRootsUsesDefaultsForNilConfig(t *testing.T) {
	env := &Environment{}

	require.Equal(t, guardrails.NormalizeRoots(nil), env.fileRoots())
}

func TestEnvironment_FileRootsUsesConfiguredRoots(t *testing.T) {
	root := t.TempDir()
	env := &Environment{cfg: &config.Config{FSRoots: []string{root, filepath.Join(root, ".")}}}

	require.Equal(t, []string{root}, env.fileRoots())
}

func TestEnvironment_CommandPolicyUsesDefaultsForNilEnvironment(t *testing.T) {
	var env *Environment

	require.Equal(t, guardrails.CommandPolicy{}, env.commandPolicy())
}

func TestEnvironment_CommandPolicyUsesDefaultsForNilConfig(t *testing.T) {
	env := &Environment{}

	require.Equal(t, guardrails.CommandPolicy{}, env.commandPolicy())
}

func TestNewEnvironment_ConfiguresTraceFactoryWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Model: "gpt-5.1", ModelAPIMode: "responses", DebugTraces: true, DebugTraceDir: dir})

	session := env.NewTraceSession()
	require.NotEmpty(t, session.ID())
	session.Close()
}

func TestNewEnvironment_ReturnsNoopTraceSessionWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})

	session := env.NewTraceSession()
	require.Equal(t, "", session.ID())
}

func TestEnvironment_NewTraceSessionNilEnvironment(t *testing.T) {
	var env *Environment

	session := env.NewTraceSession()

	require.Equal(t, "", session.ID())
}

func TestEnvironment_NewTraceSessionNilTraceFactory(t *testing.T) {
	env := &Environment{}

	session := env.NewTraceSession()

	require.Equal(t, "", session.ID())
}

func TestNewEnvironment_UsesDefaultTraceDirWhenEnabledWithoutConfiguredDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HAND_HOME", home)
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraces: true})

	session := env.NewTraceSession()
	require.NotEmpty(t, session.ID())
	session.Close()
	require.FileExists(t, filepath.Join(datadir.DebugTraceDir(), session.ID()+".jsonl"))
}
