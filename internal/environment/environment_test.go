package environment

import (
	gctx "context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	envbudget "github.com/wandxy/hand/internal/environment/budget"
	envplanstore "github.com/wandxy/hand/internal/environment/planstore"
	envsessionmessages "github.com/wandxy/hand/internal/environment/sessionmessages"
	envsessionsearch "github.com/wandxy/hand/internal/environment/sessionsearch"
	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/personality"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	memorystore "github.com/wandxy/hand/internal/state/storememory"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/internal/workspace"
	"github.com/wandxy/hand/pkg/nanoid"
)

func TestNewEnvironment_InitializesDependencies(t *testing.T) {
	baseCtx := gctx.WithValue(gctx.Background(), "requestID", "req-123")
	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}}

	env := NewEnvironment(baseCtx, cfg)
	h := env.(*environment)

	require.Same(t, baseCtx, h.ctx)
	require.Same(t, cfg, h.cfg)
	require.Empty(t, env.Instructions())
}

func prepareTestEnvironment(t *testing.T, env Environment) {
	t.Helper()

	env.SetStateManager(newTestStateManager(t))
	require.NoError(t, env.Prepare())
}

func newTestStateManager(t *testing.T) *statemanager.Manager {
	t.Helper()

	manager, err := statemanager.NewManager(memorystore.NewStore(), time.Hour, 24*time.Hour)
	require.NoError(t, err)

	return manager
}

func preparedToolGuidance() instruct.Instructions {
	return instruct.Instructions{
		instruct.BuildSessionSearchGuidance(),
		instruct.BuildSessionMessagesGuidance(),
	}
}

func expectedPreparedInstructions(name string, extras ...instruct.Instruction) instruct.Instructions {
	expected := append(
		instruct.Instructions{instruct.BuildPlanningPolicy()},
		instruct.BuildBase(name)...,
	)
	expected = append(expected, extras...)
	expected = append(expected, preparedToolGuidance()...)
	return expected
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
	cfg := &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)
	require.Equal(t, expectedPreparedInstructions(cfg.Name), env.Instructions())
}

func TestEnvironment_PrepareRequiresConfig(t *testing.T) {
	env := NewEnvironment(gctx.Background(), nil)

	err := env.Prepare()

	require.EqualError(t, err, "config is required")
}

func TestEnvironment_PrepareRequiresEnvironment(t *testing.T) {
	var env *environment

	err := env.Prepare()

	require.EqualError(t, err, "environment is required")
}

func TestEnvironment_PrepareRequiresStateManager(t *testing.T) {
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

	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})

	err := env.Prepare()

	require.EqualError(t, err, "state manager is required")
}

func TestEnvironment_PrepareNormalizesConfig(t *testing.T) {
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

	cfg := &config.Config{Name: " Test Agent ", Debug: config.DebugConfig{TraceDir: t.TempDir()}}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, config.DefaultWebMaxExtractCharPerResult, cfg.Web.MaxExtractCharPerResult)
	require.NotNil(t, cfg.Cap.Network)
	require.True(t, *cfg.Cap.Network)
}

func TestEnvironment_PrepareConfiguresMemoryProviderWhenEnabled(t *testing.T) {
	enabled := true
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:   "Test Agent",
		Memory: config.MemoryConfig{Enabled: &enabled, Provider: memory.ProviderDefaultMemory},
		Debug:  config.DebugConfig{TraceDir: t.TempDir()},
	})
	env.SetStateManager(newTestStateManager(t))

	require.NoError(t, env.Prepare())
	require.IsType(t, &memory.MemoryProvider{}, env.MemoryProvider())
}

func TestEnvironment_PrepareConfiguresDefaultMemoryProviderWithStateStore(t *testing.T) {
	store := memorystore.NewStore()
	manager, err := statemanager.NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	enabled := true
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:   "Test Agent",
		Memory: config.MemoryConfig{Enabled: &enabled, Provider: memory.ProviderDefaultMemory},
		Debug:  config.DebugConfig{TraceDir: t.TempDir()},
	})
	env.SetStateManager(manager)

	require.NoError(t, env.Prepare())
	provider := env.MemoryProvider()
	require.IsType(t, &memory.MemoryProvider{}, provider)
	writer := provider.(memory.WriteProvider)
	_, err = writer.Upsert(gctx.Background(), memory.MemoryItem{Status: memory.StatusActive, Text: "state owned store"})
	require.NoError(t, err)
	result, err := store.SearchMemory(gctx.Background(), memory.SearchQuery{Text: "state owned"})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
	require.NoError(t, provider.Close())
}

func TestEnvironment_PrepareConfiguresPinnedMemoryOptions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pinned.md")
	require.NoError(t, os.WriteFile(file, []byte("Always use pnpm"), 0o600))
	store := memorystore.NewStore()
	manager, err := statemanager.NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	enabled := true
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name: "Test Agent",
		Memory: config.MemoryConfig{
			Enabled:  &enabled,
			Provider: memory.ProviderDefaultMemory,
			Pinned: config.PinnedMemoryConfig{
				Files:        []string{file},
				MaxChars:     100,
				MaxItemChars: 40,
			},
		},
		Debug: config.DebugConfig{TraceDir: t.TempDir()},
	})
	env.SetStateManager(manager)

	require.NoError(t, env.Prepare())
	pinned, ok := env.MemoryProvider().(memory.PinnedProvider)
	require.True(t, ok)

	items, err := pinned.LoadPinned(gctx.Background(), memory.SearchQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "pinned.md", items[0].Title)
	require.Equal(t, "Always use pnpm", items[0].Text)
}

func TestEnvironment_PrepareConfiguresDefaultMemoryProviderWithMemoryBackend(t *testing.T) {
	store := memorystore.NewStore()
	manager, err := statemanager.NewManager(store, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	enabled := true
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:    "Test Agent",
		Storage: config.StorageConfig{Backend: "memory"},
		Memory:  config.MemoryConfig{Enabled: &enabled, Provider: memory.ProviderDefaultMemory},
		Debug:   config.DebugConfig{TraceDir: t.TempDir()},
	})
	env.SetStateManager(manager)

	require.NoError(t, env.Prepare())
	provider := env.MemoryProvider()
	require.IsType(t, &memory.MemoryProvider{}, provider)

	writer := provider.(memory.WriteProvider)
	_, err = writer.Upsert(gctx.Background(), memory.MemoryItem{Status: memory.StatusActive, Text: "state owned memory"})
	require.NoError(t, err)

	result, err := store.SearchMemory(gctx.Background(), memory.SearchQuery{Text: "state owned"})
	require.NoError(t, err)
	require.Len(t, result.Hits, 1)
}

func TestEnvironment_PrepareConfiguresDefaultMemoryProviderByDefault(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})
	env.SetStateManager(newTestStateManager(t))

	require.NoError(t, env.Prepare())
	require.IsType(t, &memory.MemoryProvider{}, env.MemoryProvider())
}

func TestEnvironment_PrepareLeavesMemoryProviderDisabledWhenConfigured(t *testing.T) {
	enabled := false
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:   "Test Agent",
		Memory: config.MemoryConfig{Enabled: &enabled, Provider: memory.ProviderDefaultMemory},
		Debug:  config.DebugConfig{TraceDir: t.TempDir()},
	})
	env.SetStateManager(newTestStateManager(t))

	require.NoError(t, env.Prepare())
	require.Nil(t, env.MemoryProvider())
}

func TestEnvironment_PrepareReturnsMemoryProviderErrors(t *testing.T) {
	enabled := true
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:   "Test Agent",
		Memory: config.MemoryConfig{Enabled: &enabled, Provider: "missing"},
		Debug:  config.DebugConfig{TraceDir: t.TempDir()},
	})
	env.SetStateManager(&statemanager.Manager{})

	err := env.Prepare()
	require.ErrorIs(t, err, memory.ErrUnknownProvider)
}

func TestEnvironment_MemoryProviderReturnsNilForNilEnvironment(t *testing.T) {
	var env *environment
	require.Nil(t, env.MemoryProvider())
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
	cfg := &config.Config{
		Name:  "Test Agent",
		Debug: config.DebugConfig{TraceDir: dir},
		Rules: config.RulesConfig{Files: []string{"hand.md"}},
	}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)

	instructions := env.Instructions()
	require.Len(t, instructions, len(instruct.BuildBase(cfg.Name))+4)
	require.Equal(t, "## AGENTS.md\nrepo rules", instructions[len(instructions)-3].Value)
	require.Equal(t, instruct.BuildSessionSearchGuidance(), instructions[len(instructions)-2])
	require.Equal(t, instruct.BuildSessionMessagesGuidance(), instructions[len(instructions)-1])
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
	cfg := &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)

	instructions := env.Instructions()
	require.Len(t, instructions, len(instruct.BuildBase(cfg.Name))+5)
	require.Equal(t, "## SOUL.md\npersona", instructions[len(instructions)-4].Value)
	require.Equal(t, "## AGENTS.md\nrepo rules", instructions[len(instructions)-3].Value)
	require.Equal(t, instruct.BuildSessionSearchGuidance(), instructions[len(instructions)-2])
	require.Equal(t, instruct.BuildSessionMessagesGuidance(), instructions[len(instructions)-1])
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

	cfg := &config.Config{
		Name:    "Test Agent",
		Debug:   config.DebugConfig{TraceDir: t.TempDir()},
		Session: config.SessionConfig{Instruct: "be terse"},
	}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)

	instructions := env.Instructions()
	require.Equal(t, "## AGENTS.md\nrepo rules", instructions[len(instructions)-4].Value)
	require.Equal(t, "be terse", instructions[len(instructions)-3].Value)
	require.Equal(t, instruct.BuildSessionSearchGuidance(), instructions[len(instructions)-2])
	require.Equal(t, instruct.BuildSessionMessagesGuidance(), instructions[len(instructions)-1])
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
	cfg := &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)
	require.Equal(t, expectedPreparedInstructions(cfg.Name), env.Instructions())
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
	cfg := &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)
	require.Equal(t, expectedPreparedInstructions(cfg.Name), env.Instructions())
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
	cfg := &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)

	instructions := env.Instructions()
	require.Equal(t, instruct.PlanningPolicyInstructionName, instructions[0].Name)
	require.Contains(t, instructions[0].Value, "Use plan_tool for tasks with 3 or more meaningful steps")
	require.Contains(t, instructions[1].Value, "Test Agent is the user's personal agent")
	require.Contains(t, instructions[1].Value, "Use tools when they materially improve correctness or allow real action")
}

func TestEnvironment_SetStateManager(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})
	h := env.(*environment)

	require.Nil(t, h.stateMgr)

	manager := &statemanager.Manager{}
	h.SetStateManager(manager)

	require.Same(t, manager, h.stateMgr)
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
	cfg := &config.Config{Debug: config.DebugConfig{TraceDir: dir}}
	env := NewEnvironment(gctx.Background(), cfg)

	prepareTestEnvironment(t, env)

	instructions := env.Instructions()
	require.Equal(t, instruct.PlanningPolicyInstructionName, instructions[0].Name)
	require.Contains(t, instructions[1].Value, "Hand is the user's personal agent")
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
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}})

	prepareTestEnvironment(t, env)

	tools := env.Tools()
	require.NotNil(t, tools)

	definitions := tools.List()
	require.Len(t, definitions, 12)
	require.Equal(t, []string{"list_files", "memory_search", "patch", "plan_tool", "process", "read_file", "run_command", "search_files", "session_messages", "session_search", "time", "write_file"}, definitions.Names())
	for _, definition := range definitions {
		require.Equal(t, []string{"core"}, definition.Groups)
	}
	groups := tools.ListGroups()
	require.Len(t, groups, 1)
	require.Equal(t, "core", groups[0].Name)
}

func TestEnvironment_PrepareAppendsLoadedToolUsageInstructionsAfterBaseInstructions(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})
	prepareTestEnvironment(t, env)

	rendered := env.Instructions().String()
	require.True(t, strings.Index(rendered, "Test Agent is the user's personal agent") < strings.Index(rendered, "# Session Search Guidance"))
	require.True(t, strings.Index(rendered, "# Session Search Guidance") < strings.Index(rendered, "# Session Messages Guidance"))
	require.Contains(t, rendered, "Use session_search when the user references prior work")
	require.Contains(t, rendered, "Use session_messages when you need exact stored transcript content")
}

func TestEnvironment_PrepareRegistersSessionTools(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})
	prepareTestEnvironment(t, env)

	definitions, err := env.Tools().Resolve(tools.Policy{
		GroupNames:   []string{"core"},
		Capabilities: tools.Capabilities{Filesystem: true, Exec: true, Memory: true},
	})
	require.NoError(t, err)
	require.True(t, definitions.Has("session_search"))
	require.True(t, definitions.Has("session_messages"))
}

func TestEnvironment_PrepareRegistersMemorySearchWhenProviderSupportsSearch(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	enabled := true
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:   "Test Agent",
		Debug:  config.DebugConfig{TraceDir: t.TempDir()},
		Memory: config.MemoryConfig{Enabled: &enabled, Provider: memory.ProviderDefaultMemory},
	})
	prepareTestEnvironment(t, env)

	definitions, err := env.Tools().Resolve(tools.Policy{
		GroupNames:   []string{"core"},
		Capabilities: tools.Capabilities{Filesystem: true, Exec: true, Memory: true},
	})
	require.NoError(t, err)
	require.True(t, definitions.Has("memory_search"))
}

func TestEnvironment_PrepareToolsReturnsMemorySearchCapabilityError(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})
	h := env.(*environment)
	h.memory = &memorySearchProviderStub{capsErr: errors.New("capability failed")}
	h.SetStateManager(&statemanager.Manager{})

	require.EqualError(t, h.prepareTools(), "capability failed")
}

func TestEnvironment_MemorySearchDefinitionSkipsUnsupportedProviders(t *testing.T) {
	tests := []struct {
		name   string
		env    *environment
		err    string
		expect bool
	}{
		{
			name: "nil environment",
		},
		{
			name: "nil provider",
			env:  &environment{},
		},
		{
			name: "provider without search",
			env: &environment{
				memory: memoryProviderWithoutSearch{},
			},
		},
		{
			name: "search capability disabled",
			env: &environment{
				memory: &memorySearchProviderStub{
					caps: memory.Capabilities{},
				},
			},
		},
		{
			name: "capability error",
			env: &environment{
				memory: &memorySearchProviderStub{
					capsErr: errors.New("capability failed"),
				},
			},
			err: "capability failed",
		},
		{
			name: "search supported",
			env: &environment{
				memory: &memorySearchProviderStub{
					caps: memory.Capabilities{SupportsSearch: true},
				},
			},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var env *environment
			if tt.env != nil {
				env = tt.env
			}
			if env != nil && env.memory != nil {
				env.runtime = NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, nil)
				env.runtime.memory = env.memory
			}

			definition, ok, err := env.memorySearchDefinition()
			if tt.err != "" {
				require.EqualError(t, err, tt.err)
				require.False(t, ok)
				require.Empty(t, definition)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expect, ok)
			if tt.expect {
				require.Equal(t, "memory_search", definition.Name)
			} else {
				require.Empty(t, definition)
			}
		})
	}
}

func TestEnvironment_SessionSearchThenSessionMessagesWorkflow(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	store := memorystore.NewStore()
	manager, err := statemanager.NewManager(store, time.Minute, time.Hour)
	require.NoError(t, err)

	currentSessionID := nanoid.MustFromSeed(storage.SessionIDPrefix, "phase5-current", "EnvironmentPhase5TestSeed")
	priorSessionID := nanoid.MustFromSeed(storage.SessionIDPrefix, "phase5-prior", "EnvironmentPhase5TestSeed")
	require.NoError(t, manager.Save(gctx.Background(), memorystore.Session{ID: currentSessionID}))
	require.NoError(t, manager.Save(gctx.Background(), memorystore.Session{ID: priorSessionID}))
	require.NoError(t, manager.UseSession(gctx.Background(), currentSessionID))

	now := time.Now().UTC()
	require.NoError(t, manager.AppendMessages(gctx.Background(), currentSessionID, []messages.Message{
		{ID: 1, Role: messages.RoleUser, Content: "current session needle context", CreatedAt: now},
	}))
	require.NoError(t, manager.AppendMessages(gctx.Background(), priorSessionID, []messages.Message{
		{ID: 11, Role: messages.RoleUser, Content: "before context", CreatedAt: now.Add(time.Second)},
		{ID: 12, Role: messages.RoleAssistant, Content: "needle exact details", CreatedAt: now.Add(2 * time.Second)},
		{ID: 13, Role: messages.RoleAssistant, Content: "after context", CreatedAt: now.Add(3 * time.Second)},
	}))

	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})
	env.SetStateManager(manager)
	require.NoError(t, env.Prepare())

	searchResult, err := env.Tools().Invoke(tools.WithSessionID(gctx.Background(), currentSessionID), tools.Call{
		Name:  "session_search",
		Input: `{"query":"needle"}`,
	})
	require.NoError(t, err)
	require.Empty(t, searchResult.Error)

	var searchPayload struct {
		Results []envsessionsearch.SessionSearchResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(searchResult.Output), &searchPayload))
	require.Len(t, searchPayload.Results, 1)
	require.Equal(t, priorSessionID, searchPayload.Results[0].SessionID)
	require.Len(t, searchPayload.Results[0].Messages, 1)
	searchHit := searchPayload.Results[0].Messages[0]
	require.Equal(t, uint(12), searchHit.MessageID)
	require.Equal(t, "needle exact details", searchHit.Snippet)

	fetchInput := `{"session_id":"` + priorSessionID + `","anchor_message_id":12,"before":1,"after":1}`
	fetchResult, err := env.Tools().Invoke(gctx.Background(), tools.Call{
		Name:  "session_messages",
		Input: fetchInput,
	})
	require.NoError(t, err)
	require.Empty(t, fetchResult.Error)

	var fetchPayload envsessionmessages.SessionMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(fetchResult.Output), &fetchPayload))
	require.Equal(t, priorSessionID, fetchPayload.SessionID)
	require.False(t, fetchPayload.Truncated)
	require.Len(t, fetchPayload.Messages, 3)
	require.Equal(t, []uint{11, 12, 13}, []uint{
		fetchPayload.Messages[0].MessageID,
		fetchPayload.Messages[1].MessageID,
		fetchPayload.Messages[2].MessageID,
	})
	require.Equal(t, []int{0, 1, 2}, []int{
		fetchPayload.Messages[0].Offset,
		fetchPayload.Messages[1].Offset,
		fetchPayload.Messages[2].Offset,
	})
	require.Equal(t, []string{"before context", "needle exact details", "after context"}, []string{
		fetchPayload.Messages[0].Content,
		fetchPayload.Messages[1].Content,
		fetchPayload.Messages[2].Content,
	})
}

func TestEnvironment_PrepareRegistersWebSearchWhenProviderConfigured(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:  "Test Agent",
		Debug: config.DebugConfig{TraceDir: t.TempDir()},
		Web:   config.WebConfig{Provider: "exa", APIKey: "exa-key"},
	})

	prepareTestEnvironment(t, env)

	definitions := env.Tools().List()
	require.Len(t, definitions, 14)
	require.Equal(t, []string{
		"list_files",
		"memory_search",
		"patch",
		"plan_tool",
		"process",
		"read_file",
		"run_command",
		"search_files",
		"session_messages",
		"session_search",
		"time",
		"web_extract",
		"web_search",
		"write_file",
	}, definitions.Names())
}

func TestEnvironment_PrepareWrapsWebProviderWithCacheWhenConfigured(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Cached", "url": "https://example.com", "highlights": []string{"hit"}},
			},
		}))
	}))
	defer server.Close()

	env := NewEnvironment(gctx.Background(), &config.Config{
		Name: "Test Agent",
		Web: config.WebConfig{
			Provider: "exa",
			APIKey:   "exa-key",
			BaseURL:  server.URL,
			CacheTTL: time.Minute,
		},
	})

	prepareTestEnvironment(t, env)
	for range 2 {
		result, err := env.Tools().Invoke(gctx.Background(), tools.Call{
			Name:  "web_search",
			Input: `{"query":"golang","count":1}`,
		})
		require.NoError(t, err)
		require.Empty(t, result.Error)
	}
	require.Equal(t, 1, requests)
}

func TestEnvironment_PrepareLeavesWebProviderUncachedWhenDisabled(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Uncached", "url": "https://example.com", "highlights": []string{"hit"}},
			},
		}))
	}))
	defer server.Close()

	env := NewEnvironment(gctx.Background(), &config.Config{
		Name: "Test Agent",
		Web: config.WebConfig{
			Provider: "exa",
			APIKey:   "exa-key",
			BaseURL:  server.URL,
		},
	})

	prepareTestEnvironment(t, env)
	for range 2 {
		result, err := env.Tools().Invoke(gctx.Background(), tools.Call{
			Name:  "web_search",
			Input: `{"query":"golang","count":1}`,
		})
		require.NoError(t, err)
		require.Empty(t, result.Error)
	}
	require.Equal(t, 2, requests)
}

func TestEnvironment_PrepareAppliesWebsitePolicyToWebTools(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Blocked", "url": "https://blocked.example", "highlights": []string{"hit"}},
			},
		}))
	}))
	defer server.Close()

	env := NewEnvironment(gctx.Background(), &config.Config{
		Name: "Test Agent",
		Web: config.WebConfig{
			Provider:                "exa",
			APIKey:                  "exa-key",
			BaseURL:                 server.URL,
			BlockedDomainsEnabled:   true,
			BlockedDomains:          []string{"blocked.example"},
			MaxExtractCharPerResult: config.DefaultWebMaxExtractCharPerResult,
		},
	})

	prepareTestEnvironment(t, env)
	result, err := env.Tools().Invoke(gctx.Background(), tools.Call{
		Name:  "web_search",
		Input: `{"query":"golang","count":1}`,
	})
	require.NoError(t, err)
	require.Empty(t, result.Error)
	require.Contains(t, result.Output, `"results":[]`)
}

func TestEnvironment_PrepareRegistersOnlyWebExtractForNativeProvider(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:  "Test Agent",
		Debug: config.DebugConfig{TraceDir: t.TempDir()},
		Web:   config.WebConfig{Provider: "native"},
	})

	prepareTestEnvironment(t, env)

	definitions := env.Tools().List()
	require.True(t, definitions.Has("web_extract"))
	require.False(t, definitions.Has("web_search"))
}

func TestEnvironment_PrepareSkipsWebSearchWhenProviderNotConfigured(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:  "Test Agent",
		Debug: config.DebugConfig{TraceDir: t.TempDir()},
	})

	prepareTestEnvironment(t, env)

	definitions := env.Tools().List()
	require.Len(t, definitions, 12)
	require.False(t, definitions.Has("web_search"))
	require.False(t, definitions.Has("web_extract"))
}

func TestEnvironment_PrepareReturnsWebProviderErrors(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:  "Test Agent",
		Debug: config.DebugConfig{TraceDir: t.TempDir()},
		Web:   config.WebConfig{Provider: "parallel"},
	})
	env.SetStateManager(newTestStateManager(t))

	err := env.Prepare()
	require.EqualError(t, err, "parallel requires web API key")
}

func TestEnvironment_WebToolsResolveOnlyWithNetworkCapability(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:  "Test Agent",
		Debug: config.DebugConfig{TraceDir: t.TempDir()},
		Web:   config.WebConfig{Provider: "exa", APIKey: "exa-key"},
	})

	prepareTestEnvironment(t, env)

	withNetwork, err := env.Tools().Resolve(tools.Policy{GroupNames: []string{"core"}, Capabilities: tools.Capabilities{Filesystem: true, Exec: true, Memory: true, Network: true}})
	require.NoError(t, err)
	require.True(t, withNetwork.Has("web_extract"))
	require.True(t, withNetwork.Has("web_search"))

	withoutNetwork, err := env.Tools().Resolve(tools.Policy{GroupNames: []string{"core"}, Capabilities: tools.Capabilities{Filesystem: true, Exec: true, Memory: true}})
	require.NoError(t, err)
	require.False(t, withoutNetwork.Has("web_search"))
	require.False(t, withoutNetwork.Has("web_extract"))
}

func TestEnvironment_CurrentPlanAndHydratePlanHandleNilReceiver(t *testing.T) {
	var env *environment

	require.Equal(t, envplanstore.Plan{}, env.CurrentPlan("session-1"))
	env.HydratePlan("session-1", envplanstore.Plan{
		Steps: []envplanstore.PlanStep{{ID: "step-1", Content: "First", Status: envplanstore.PlanStatusInProgress}},
	})
	require.Equal(t, envplanstore.Plan{}, env.CurrentPlan("session-1"))
}

func TestEnvironment_CurrentPlanAndHydratePlanUseRuntimeStore(t *testing.T) {
	env := &environment{runtime: NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, nil)}

	env.HydratePlan("session-1", envplanstore.Plan{
		Steps:       []envplanstore.PlanStep{{ID: "step-1", Content: "First", Status: envplanstore.PlanStatusInProgress}},
		Explanation: "restored",
	})

	require.Equal(t, envplanstore.Plan{
		Steps:       []envplanstore.PlanStep{{ID: "step-1", Content: "First", Status: envplanstore.PlanStatusInProgress}},
		Explanation: "restored",
	}, env.CurrentPlan("session-1"))
}

func TestRuntime_SearchMemoryHandlesUnavailableProviders(t *testing.T) {
	tests := []struct {
		name    string
		runtime *Runtime
		message string
	}{
		{name: "nil runtime", message: "memory search is not configured"},
		{name: "nil provider", runtime: &Runtime{}, message: "memory search is not configured"},
		{name: "provider without search", runtime: &Runtime{memory: memoryProviderWithoutSearch{}}, message: "memory search is not supported by provider"},
		{name: "search capability disabled", runtime: &Runtime{memory: &memorySearchProviderStub{}}, message: "memory search is not supported by provider"},
		{name: "capability error", runtime: &Runtime{memory: &memorySearchProviderStub{capsErr: errors.New("capability failed")}}, message: "capability failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.runtime.SearchMemory(gctx.Background(), memory.SearchQuery{Text: "hello"})

			require.EqualError(t, err, tt.message)
			require.Empty(t, result)
		})
	}
}

func TestRuntime_SearchMemorySearchesProvider(t *testing.T) {
	provider := &memorySearchProviderStub{
		caps: memory.Capabilities{SupportsSearch: true},
		searchResult: memory.SearchResult{Hits: []memory.SearchHit{{
			Item: memory.MemoryItem{ID: "mem_123", Status: memory.StatusActive, Text: "hello"},
		}}},
	}
	runtime := &Runtime{memory: provider}
	query := memory.SearchQuery{Text: "hello", Limit: 3}

	result, err := runtime.SearchMemory(gctx.Background(), query)

	require.NoError(t, err)
	require.Equal(t, query, provider.searchQuery)
	require.Equal(t, provider.searchResult, result)
}

func TestEnvironment_PrepareReturnsToolRegistrationError(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}})
	env.(*environment).tools = failingRegistry{err: errors.New("register failed")}
	env.SetStateManager(newTestStateManager(t))
	err := env.Prepare()
	require.EqualError(t, err, "register failed")
	require.Equal(t, append(
		instruct.Instructions{instruct.BuildPlanningPolicy()},
		instruct.BuildBase("Test Agent")...,
	), env.Instructions())
}

func TestEnvironment_PrepareReturnsToolGroupRegistrationError(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{}, nil
	}

	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}})
	env.(*environment).tools = failingGroupRegistry{err: errors.New("group failed")}
	env.SetStateManager(newTestStateManager(t))
	err := env.Prepare()
	require.EqualError(t, err, "group failed")
	require.Equal(t, append(
		instruct.Instructions{instruct.BuildPlanningPolicy()},
		instruct.BuildBase("Test Agent")...,
	), env.Instructions())
}

func TestEnvironment_PrepareToolsPreservesExistingRuntime(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: t.TempDir()}})
	h := env.(*environment)
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{}, nil)
	h.runtime = runtime
	h.SetStateManager(&statemanager.Manager{})
	require.NoError(t, h.prepareTools())
	require.Same(t, runtime, h.runtime)
}

func TestEnvironment_InstructionsReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}})
	h := env.(*environment)
	h.instructions = append(h.instructions, instruct.Instruction{Value: "hello"})
	instructions := env.Instructions()
	require.Len(t, instructions, 1)
	instructions[0].Value = "changed"
	require.Equal(t, "hello", env.Instructions()[0].Value)
}

func TestEnvironment_InstructionsReturnsNilForNilEnvironment(t *testing.T) {
	var env *environment
	require.Nil(t, env.Instructions())
}

func TestEnvironment_SetInstructionAddsUnnamedInstruction(t *testing.T) {
	env := &environment{}
	env.setInstruction(instruct.Instruction{Value: "  hello  "})
	require.Equal(t, instruct.Instructions{{Value: "hello"}}, env.instructions)
}

func TestEnvironment_SetStateManagerSkipsNilEnvironment(t *testing.T) {
	var env *environment
	env.SetStateManager(&statemanager.Manager{})
}

func TestEnvironment_SetInstructionUpdatesExistingNamedInstruction(t *testing.T) {
	env := &environment{
		instructions: instruct.Instructions{{Name: configInstructInstructionName, Value: "old"}},
	}
	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "  new  "})
	require.Equal(t, instruct.Instructions{{Name: configInstructInstructionName, Value: "new"}}, env.instructions)
}

func TestEnvironment_SetInstructionRemovesExistingNamedInstructionWhenEmpty(t *testing.T) {
	env := &environment{
		instructions: instruct.Instructions{
			{Value: "base"},
			{Name: configInstructInstructionName, Value: "old"},
		},
	}

	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "   "})

	require.Equal(t, instruct.Instructions{{Value: "base"}}, env.instructions)
}

func TestEnvironment_SetInstructionAppendsNewNamedInstructionWhenMissing(t *testing.T) {
	env := &environment{
		instructions: instruct.Instructions{{Value: "base"}},
	}

	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "  new  "})

	require.Equal(t, instruct.Instructions{
		{Value: "base"},
		{Name: configInstructInstructionName, Value: "new"},
	}, env.instructions)
}

func TestEnvironment_SetInstructionSkipsEmptyNewNamedInstruction(t *testing.T) {
	env := &environment{
		instructions: instruct.Instructions{{Value: "base"}},
	}
	env.setInstruction(instruct.Instruction{Name: configInstructInstructionName, Value: "   "})
	require.Equal(t, instruct.Instructions{{Value: "base"}}, env.instructions)
}

func TestEnvironment_NewIterationBudgetUsesConfigValue(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{
		Session: config.SessionConfig{MaxIterations: 12},
		Debug:   config.DebugConfig{TraceDir: dir},
	})
	require.Equal(t, 12, env.NewIterationBudget().Remaining())
	require.IsType(t, envbudget.IterationBudget{}, env.NewIterationBudget())
}

func TestEnvironment_NewIterationBudgetUsesDefaultWhenUnset(t *testing.T) {
	require.Equal(t, config.DefaultMaxIterations, (&environment{}).NewIterationBudget().Remaining())
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
	var env *environment

	opts := env.ToolPolicy()

	require.Equal(t, "cli", opts.Platform)
	require.True(t, opts.Capabilities.Filesystem)
	require.True(t, opts.Capabilities.Network)
	require.True(t, opts.Capabilities.Exec)
	require.True(t, opts.Capabilities.Memory)
	require.False(t, opts.Capabilities.Browser)
}

func TestEnvironment_ToolPolicyUsesDefaultsForNilConfig(t *testing.T) {
	env := &environment{}

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
		Platform: "desktop",
		Cap: config.CapConfig{
			Filesystem: new(false),
			Network:    new(false),
			Exec:       new(true),
			Memory:     new(false),
			Browser:    new(true),
		},
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
	var env *environment
	require.Equal(t, guardrails.NormalizeRoots(nil), env.fileRoots())
}

func TestEnvironment_FileRootsUsesDefaultsForNilConfig(t *testing.T) {
	env := &environment{}
	require.Equal(t, guardrails.NormalizeRoots(nil), env.fileRoots())
}

func TestEnvironment_FileRootsUsesConfiguredRoots(t *testing.T) {
	root := t.TempDir()
	env := &environment{cfg: &config.Config{FS: config.FSConfig{Roots: []string{root, filepath.Join(root, ".")}}}}
	require.Equal(t, []string{root}, env.fileRoots())
}

func TestEnvironment_CommandPolicyUsesDefaultsForNilEnvironment(t *testing.T) {
	var env *environment
	require.Equal(t, guardrails.CommandPolicy{}, env.commandPolicy())
}

func TestEnvironment_CommandPolicyUsesDefaultsForNilConfig(t *testing.T) {
	env := &environment{}
	require.Equal(t, guardrails.CommandPolicy{}, env.commandPolicy())
}

func TestNewEnvironment_ConfiguresTraceFactoryWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:   "Test Agent",
		Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "gpt-5.1", APIMode: "responses"}},
		Debug:  config.DebugConfig{Traces: true, TraceDir: dir},
	})
	const traceSessionID = "ses_test123"
	session := env.NewTraceSession(traceSessionID)
	require.Equal(t, traceSessionID, session.ID())
	session.Close()
}

func TestEnvironment_NewTraceSessionRecordsWorkspaceRuleTruncation(t *testing.T) {
	previousPersonality := loadPersonality
	previousWorkspace := loadWorkspaceRules
	t.Cleanup(func() {
		loadPersonality = previousPersonality
		loadWorkspaceRules = previousWorkspace
	})
	loadPersonality = func() (personality.Result, error) {
		return personality.Result{}, nil
	}
	loadWorkspaceRules = func(...string) (workspace.Result, error) {
		return workspace.Result{
			Found:            true,
			Content:          "## AGENTS.md\nrepo rules\n\n[... workspace rules truncated ...]\n\n## pkg/hand.md\nmore",
			Truncated:        true,
			MaxContentLength: 15000,
			OriginalLength:   24000,
			TruncatedLength:  15000,
			TruncationMarker: "[... workspace rules truncated ...]",
		}, nil
	}

	dir := t.TempDir()
	cfg := &config.Config{
		Name:   "Test Agent",
		Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "gpt-5.1", APIMode: "responses"}},
		Debug:  config.DebugConfig{Traces: true, TraceDir: dir},
	}
	env := NewEnvironment(gctx.Background(), cfg)
	prepareTestEnvironment(t, env)

	const traceSessionID = "ses_rules"
	session := env.NewTraceSession(traceSessionID)
	session.Close()

	tracePath, err := trace.ResolveTraceFilePath(dir, traceSessionID)
	require.NoError(t, err)

	lines := readJSONLines(t, tracePath)
	require.Len(t, lines, 2)
	require.Equal(t, trace.EvtChatStarted, lines[0].Type)
	require.Equal(t, trace.EvtWorkspaceRulesTruncated, lines[1].Type)

	payload, ok := lines[1].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(24000), payload["original_length"])
	require.Equal(t, float64(15000), payload["truncated_length"])
	require.Equal(t, float64(15000), payload["max_content_length"])
	require.Equal(t, "[... workspace rules truncated ...]", payload["marker"])
}

func TestNewEnvironment_ReturnsNoopTraceSessionWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{TraceDir: dir}})
	session := env.NewTraceSession("ses_test123")
	require.Equal(t, "", session.ID())
}

func TestEnvironment_NewTraceSessionNilEnvironment(t *testing.T) {
	var env *environment
	session := env.NewTraceSession("ses_test123")
	require.Equal(t, "", session.ID())
}

func TestEnvironment_NewTraceSessionNilTraceFactory(t *testing.T) {
	env := &environment{}
	session := env.NewTraceSession("ses_test123")
	require.Equal(t, "", session.ID())
}

func TestNewEnvironment_UsesDefaultTraceDirWhenEnabledWithoutConfiguredDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HAND_HOME", home)
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Debug: config.DebugConfig{Traces: true}})

	const traceSessionID = "ses_test123"
	session := env.NewTraceSession(traceSessionID)
	require.Equal(t, traceSessionID, session.ID())
	session.Close()
	matches, err := filepath.Glob(filepath.Join(datadir.DebugTraceDir(), "*"+traceSessionID+".jsonl"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.FileExists(t, matches[0])
}

func readJSONLines(t *testing.T, path string) []trace.Event {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := make([]trace.Event, 0)
	for _, raw := range splitLines(data) {
		if len(raw) == 0 {
			continue
		}
		var event trace.Event
		require.NoError(t, json.Unmarshal(raw, &event))
		lines = append(lines, event)
	}

	return lines
}

func splitLines(data []byte) [][]byte {
	lines := make([][]byte, 0)
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		lines = append(lines, data[start:i])
		start = i + 1
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}

	return lines
}
