package environment

import (
	gctx "context"
	stdctx "context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	instruct "github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/personality"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
	"github.com/wandxy/hand/internal/workspace"
)

func TestNewEnvironment_InitializesDependencies(t *testing.T) {
	baseCtx := gctx.WithValue(gctx.Background(), "requestID", "req-123")
	dir := t.TempDir()
	cfg := &config.Config{Name: "Test Agent", DebugTraceDir: dir}

	env := NewEnvironment(baseCtx, cfg)
	h := env.(*environment)

	require.Same(t, baseCtx, h.ctx)
	require.Same(t, cfg, h.cfg)
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
	require.Equal(t, append(
		instruct.Instructions{instruct.BuildPlanningPolicy()},
		instruct.BuildBase(cfg.Name)...,
	), env.Instructions())
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

	cfg := &config.Config{Name: " Test Agent ", DebugTraceDir: t.TempDir()}
	env := NewEnvironment(gctx.Background(), cfg)

	require.NoError(t, env.Prepare())
	require.Equal(t, "Test Agent", cfg.Name)
	require.Equal(t, config.DefaultWebMaxExtractCharPerResult, cfg.WebMaxExtractCharPerResult)
	require.NotNil(t, cfg.CapNetwork)
	require.True(t, *cfg.CapNetwork)
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
	require.Len(t, instructions, len(instruct.BuildBase(cfg.Name))+2)
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
	require.Len(t, instructions, len(instruct.BuildBase(cfg.Name))+3)
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
	require.Equal(t, append(
		instruct.Instructions{instruct.BuildPlanningPolicy()},
		instruct.BuildBase(cfg.Name)...,
	), env.Instructions())
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
	require.Equal(t, append(
		instruct.Instructions{instruct.BuildPlanningPolicy()},
		instruct.BuildBase(cfg.Name)...,
	), env.Instructions())
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
	require.Equal(t, instruct.PlanningPolicyInstructionName, instructions[0].Name)
	require.Contains(t, instructions[0].Value, "Use plan_tool for tasks with 3 or more meaningful steps")
	require.Contains(t, instructions[1].Value, "Test Agent is the user's personal agent")
	require.Contains(t, instructions[1].Value, "Use tools when they materially improve correctness or allow real action")
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
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})

	require.NoError(t, env.Prepare())

	tools := env.Tools()
	require.NotNil(t, tools)

	definitions := tools.List()
	require.Len(t, definitions, 9)
	require.Equal(t, []string{"list_files", "patch", "plan_tool", "process", "read_file", "run_command", "search_files", "time", "write_file"}, []string{
		definitions[0].Name,
		definitions[1].Name,
		definitions[2].Name,
		definitions[3].Name,
		definitions[4].Name,
		definitions[5].Name,
		definitions[6].Name,
		definitions[7].Name,
		definitions[8].Name,
	})
	for _, definition := range definitions {
		require.Equal(t, []string{"core"}, definition.Groups)
	}
	groups := tools.ListGroups()
	require.Len(t, groups, 1)
	require.Equal(t, "core", groups[0].Name)
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
		Name:          "Test Agent",
		DebugTraceDir: t.TempDir(),
		WebProvider:   "exa",
		WebAPIKey:     "exa-key",
	})

	require.NoError(t, env.Prepare())

	definitions := env.Tools().List()
	require.Len(t, definitions, 11)
	require.Equal(t, []string{
		"list_files",
		"patch",
		"plan_tool",
		"process",
		"read_file",
		"run_command",
		"search_files",
		"time",
		"web_extract",
		"web_search",
		"write_file",
	}, []string{
		definitions[0].Name,
		definitions[1].Name,
		definitions[2].Name,
		definitions[3].Name,
		definitions[4].Name,
		definitions[5].Name,
		definitions[6].Name,
		definitions[7].Name,
		definitions[8].Name,
		definitions[9].Name,
		definitions[10].Name,
	})
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
		Name:        "Test Agent",
		WebProvider: "exa",
		WebAPIKey:   "exa-key",
		WebBaseURL:  server.URL,
		WebCacheTTL: time.Minute,
	})

	require.NoError(t, env.Prepare())
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
		Name:        "Test Agent",
		WebProvider: "exa",
		WebAPIKey:   "exa-key",
		WebBaseURL:  server.URL,
	})

	require.NoError(t, env.Prepare())
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
		Name:                       "Test Agent",
		WebProvider:                "exa",
		WebAPIKey:                  "exa-key",
		WebBaseURL:                 server.URL,
		WebBlockedDomainsEnabled:   true,
		WebBlockedDomains:          []string{"blocked.example"},
		WebMaxExtractCharPerResult: config.DefaultWebMaxExtractCharPerResult,
	})

	require.NoError(t, env.Prepare())
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
		Name:          "Test Agent",
		DebugTraceDir: t.TempDir(),
		WebProvider:   "native",
	})

	require.NoError(t, env.Prepare())

	names := make([]string, 0, len(env.Tools().List()))
	for _, definition := range env.Tools().List() {
		names = append(names, definition.Name)
	}
	require.Contains(t, names, "web_extract")
	require.NotContains(t, names, "web_search")
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
		Name:          "Test Agent",
		DebugTraceDir: t.TempDir(),
	})

	require.NoError(t, env.Prepare())

	definitions := env.Tools().List()
	require.Len(t, definitions, 9)
	for _, definition := range definitions {
		require.NotEqual(t, "web_search", definition.Name)
		require.NotEqual(t, "web_extract", definition.Name)
	}
}

func TestEnvironment_PrepareReturnsWebProviderErrors(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{
		Name:          "Test Agent",
		DebugTraceDir: t.TempDir(),
		WebProvider:   "parallel",
	})

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
		Name:          "Test Agent",
		DebugTraceDir: t.TempDir(),
		WebProvider:   "exa",
		WebAPIKey:     "exa-key",
	})

	require.NoError(t, env.Prepare())

	withNetwork, err := env.Tools().Resolve(tools.Policy{GroupNames: []string{"core"}, Capabilities: tools.Capabilities{Filesystem: true, Exec: true, Memory: true, Network: true}})
	require.NoError(t, err)
	withNetworkNames := make([]string, 0, len(withNetwork))
	for _, definition := range withNetwork {
		withNetworkNames = append(withNetworkNames, definition.Name)
	}
	require.Contains(t, withNetworkNames, "web_extract")
	require.Contains(t, withNetworkNames, "web_search")

	withoutNetwork, err := env.Tools().Resolve(tools.Policy{GroupNames: []string{"core"}, Capabilities: tools.Capabilities{Filesystem: true, Exec: true, Memory: true}})
	require.NoError(t, err)
	for _, definition := range withoutNetwork {
		require.NotEqual(t, "web_search", definition.Name)
		require.NotEqual(t, "web_extract", definition.Name)
	}
}

func TestEnvironment_CurrentPlanAndHydratePlanHandleNilReceiver(t *testing.T) {
	var env *environment

	require.Equal(t, envtypes.Plan{}, env.CurrentPlan("session-1"))
	env.HydratePlan("session-1", envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
	})
	require.Equal(t, envtypes.Plan{}, env.CurrentPlan("session-1"))
}

func TestEnvironment_CurrentPlanAndHydratePlanUseRuntimeStore(t *testing.T) {
	env := &environment{runtime: NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{})}

	env.HydratePlan("session-1", envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
		Explanation: "restored",
	})

	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "First", Status: envtypes.PlanStatusInProgress}},
		Explanation: "restored",
	}, env.CurrentPlan("session-1"))
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
	env.(*environment).tools = failingRegistry{err: errors.New("register failed")}
	err := env.Prepare()
	require.EqualError(t, err, "register failed")
	require.Empty(t, env.Instructions())
}

func TestEnvironment_PrepareReturnsToolGroupRegistrationError(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
	env.(*environment).tools = failingGroupRegistry{err: errors.New("group failed")}
	err := env.Prepare()
	require.EqualError(t, err, "group failed")
	require.Empty(t, env.Instructions())
}

func TestEnvironment_PrepareToolsPreservesExistingRuntime(t *testing.T) {
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: t.TempDir()})
	h := env.(*environment)
	runtime := NewRuntime([]string{t.TempDir()}, guardrails.CommandPolicy{})
	h.runtime = runtime
	require.NoError(t, h.prepareTools())
	require.Same(t, runtime, h.runtime)
}

func TestEnvironment_InstructionsReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
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
	env := NewEnvironment(gctx.Background(), &config.Config{MaxIterations: 12, DebugTraceDir: dir})
	require.Equal(t, 12, env.NewIterationBudget().Remaining())
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
	var env *environment
	require.Equal(t, guardrails.NormalizeRoots(nil), env.fileRoots())
}

func TestEnvironment_FileRootsUsesDefaultsForNilConfig(t *testing.T) {
	env := &environment{}
	require.Equal(t, guardrails.NormalizeRoots(nil), env.fileRoots())
}

func TestEnvironment_FileRootsUsesConfiguredRoots(t *testing.T) {
	root := t.TempDir()
	env := &environment{cfg: &config.Config{FSRoots: []string{root, filepath.Join(root, ".")}}}
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
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", Model: "gpt-5.1",
		ModelAPIMode: "responses", DebugTraces: true, DebugTraceDir: dir})
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
		Name:          "Test Agent",
		Model:         "gpt-5.1",
		ModelAPIMode:  "responses",
		DebugTraces:   true,
		DebugTraceDir: dir,
	}
	env := NewEnvironment(gctx.Background(), cfg)
	require.NoError(t, env.Prepare())

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
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraceDir: dir})
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
	env := NewEnvironment(gctx.Background(), &config.Config{Name: "Test Agent", DebugTraces: true})

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
