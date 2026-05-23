package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	storagemock "github.com/wandxy/hand/internal/state/mock"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

func TestTurn_LoadHydratesPlanUsingFilteredToolQueries(t *testing.T) {
	var capturedGetOpts []storage.MessageQueryOptions

	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			capturedGetOpts = append(capturedGetOpts, opts)
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				return []handmsg.Message{{
					Role:    handmsg.RoleTool,
					Name:    "plan_tool",
					Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`,
				}}, nil
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{
		InstructionsList: nil,
		ToolRegistry:     tools.NewInMemoryRegistry(),
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Len(t, capturedGetOpts, 2)
	require.Equal(t, storage.MessageQueryOptions{Offset: 0}, capturedGetOpts[0])
	require.Equal(t, storage.MessageQueryOptions{Role: handmsg.RoleTool, Name: "plan_tool", Order: storage.MessageOrderDesc, Limit: planHydrationPageSize, Offset: 0}, capturedGetOpts[1])
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "Implement feature", Status: envtypes.PlanStatusInProgress}},
		Explanation: "current plan",
	}, env.Plan)
}

func TestTurn_LoadHydratesPlanFromLaterValidResultOnSamePage(t *testing.T) {
	var capturedGetOpts []storage.MessageQueryOptions

	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			capturedGetOpts = append(capturedGetOpts, opts)
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				return []handmsg.Message{
					{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"broken\",\"content\":\"\",\"status\":\"pending\"}]}"}`},
					{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"step-2\",\"content\":\"Continue work\",\"status\":\"in_progress\"}],\"explanation\":\"fallback\"}"}`},
				}, nil
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-2", Content: "Continue work", Status: envtypes.PlanStatusInProgress}},
		Explanation: "fallback",
	}, env.Plan)
	require.Equal(t, storage.MessageQueryOptions{Role: handmsg.RoleTool, Name: "plan_tool", Order: storage.MessageOrderDesc, Limit: planHydrationPageSize, Offset: 0}, capturedGetOpts[1])
}

func TestTurn_LoadHydratesNewestValidPlanOnSamePage(t *testing.T) {
	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				return []handmsg.Message{
					{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"newest\",\"content\":\"Newest valid plan\",\"status\":\"in_progress\"}],\"explanation\":\"newest\"}"}`},
					{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"older\",\"content\":\"Older valid plan\",\"status\":\"in_progress\"}],\"explanation\":\"older\"}"}`},
				}, nil
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "newest", Content: "Newest valid plan", Status: envtypes.PlanStatusInProgress}},
		Explanation: "newest",
	}, env.Plan)
}

func TestTurn_LoadHydratesPlanFromLaterPageWhenEarlierPageIsInvalid(t *testing.T) {
	var capturedGetOpts []storage.MessageQueryOptions

	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			capturedGetOpts = append(capturedGetOpts, opts)
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				if opts.Offset == 0 {
					return []handmsg.Message{
						{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"broken-1\",\"content\":\"\",\"status\":\"pending\"}]}"}`},
						{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"broken-2\",\"content\":\"\",\"status\":\"pending\"}]}"}`},
					}, nil
				}
				if opts.Offset == 2 {
					return []handmsg.Message{
						{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"step-3\",\"content\":\"Recover state\",\"status\":\"in_progress\"}],\"explanation\":\"second page\"}"}`},
					}, nil
				}
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-3", Content: "Recover state", Status: envtypes.PlanStatusInProgress}},
		Explanation: "second page",
	}, env.Plan)
	require.Equal(t, storage.MessageQueryOptions{Role: handmsg.RoleTool, Name: "plan_tool", Order: storage.MessageOrderDesc, Limit: planHydrationPageSize, Offset: 0}, capturedGetOpts[1])
	require.Equal(t, storage.MessageQueryOptions{Role: handmsg.RoleTool, Name: "plan_tool", Order: storage.MessageOrderDesc, Limit: planHydrationPageSize, Offset: 2}, capturedGetOpts[2])
}

func TestTurn_LoadHydratesEmptyPlanWhenNoValidHistoricalPlanExists(t *testing.T) {
	var capturedGetOpts []storage.MessageQueryOptions

	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			capturedGetOpts = append(capturedGetOpts, opts)
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				if opts.Offset == 0 {
					return []handmsg.Message{
						{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"broken\",\"content\":\"\",\"status\":\"pending\"}]}"}`},
					}, nil
				}
				return nil, nil
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.False(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{}, env.Plan)
	require.Equal(t, storage.MessageQueryOptions{Role: handmsg.RoleTool, Name: "plan_tool", Order: storage.MessageOrderDesc, Limit: planHydrationPageSize, Offset: 1}, capturedGetOpts[2])
}

func TestTurn_LoadReturnsHydrationErrorFromLaterPageFetch(t *testing.T) {
	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				if opts.Offset == 0 {
					return []handmsg.Message{
						{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"output":"{\"steps\":[{\"id\":\"broken\",\"content\":\"\",\"status\":\"pending\"}]}"}`},
					}, nil
				}
				return nil, errors.New("later hydration fetch failed")
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()},
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "later hydration fetch failed")
}

func TestTurn_LoadHydratesLatestValidPlanFromHistory(t *testing.T) {
	manager := mustNewStateManager(t)
	env := &mocks.EnvironmentStub{
		InstructionsList: instructions.New("base"),
		ToolRegistry:     tools.NewInMemoryRegistry(),
		TraceSession:     &mocks.TraceSessionStub{},
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, []handmsg.Message{
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"old\",\"content\":\"Old\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"old\",\"explanation\":\"old plan\"}"}`},
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`},
	}))

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "Implement feature", Status: envtypes.PlanStatusInProgress}},
		Explanation: "current plan",
	}, env.Plan)
}

func TestTurn_LoadIgnoresMalformedPlanHistory(t *testing.T) {
	manager := mustNewStateManager(t)
	env := &mocks.EnvironmentStub{
		InstructionsList: instructions.New("base"),
		ToolRegistry:     tools.NewInMemoryRegistry(),
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, []handmsg.Message{
		{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"not-json"}`},
	}))

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.False(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{}, env.Plan)
}

func TestTurn_LoadHydratesPlanFromHistoryBeforeSummaryOffset(t *testing.T) {
	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetSummaryFunc: func(context.Context, string) (storage.SessionSummary, bool, error) {
			return storage.SessionSummary{
				SessionID:       storage.DefaultSessionID,
				SourceEndOffset: 3,
				SessionSummary:  "Older work",
			}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" && opts.Order == "desc" {
				return []handmsg.Message{
					{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`},
				}, nil
			}
			all := []handmsg.Message{
				{Role: handmsg.RoleTool, Name: "plan_tool", Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Implement feature\",\"status\":\"in_progress\"}],\"summary\":{\"total\":1,\"pending\":0,\"in_progress\":1,\"completed\":0,\"cancelled\":0},\"active_step_id\":\"step-1\",\"explanation\":\"current plan\"}"}`},
				{Role: handmsg.RoleAssistant, Content: "older-1"},
				{Role: handmsg.RoleAssistant, Content: "older-2"},
				{Role: handmsg.RoleAssistant, Content: "tail"},
			}
			if opts.Offset > 0 {
				return all[opts.Offset:], nil
			}
			return all, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{
		InstructionsList: instructions.New("base"),
		ToolRegistry:     tools.NewInMemoryRegistry(),
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		env,
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.NoError(t, err)
	require.True(t, turn.planHydrated)
	require.Equal(t, envtypes.Plan{
		Steps:       []envtypes.PlanStep{{ID: "step-1", Content: "Implement feature", Status: envtypes.PlanStatusInProgress}},
		Explanation: "current plan",
	}, env.Plan)
	require.Equal(t, []handmsg.Message{{Role: handmsg.RoleAssistant, Content: "tail"}}, turn.sessionHistory)
}

func TestTurn_LoadReturnsHydratePlanLookupError(t *testing.T) {
	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" {
				return nil, errors.New("hydrate lookup failed")
			}
			return nil, nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		&mocks.ModelClientStub{},
		nil,
		manager,
		nil,
		&mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()},
	)

	err = turn.load(context.Background(), RespondOptions{})
	require.EqualError(t, err, "hydrate lookup failed")
}

func TestTurn_RunRecordsHydratedPlanTrace(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "reply"}}}
	manager, err := statemanager.NewManager(&storagemock.Store{
		GetFunc: func(context.Context, string) (storage.Session, bool, error) {
			return storage.Session{ID: storage.DefaultSessionID, UpdatedAt: time.Now().UTC()}, true, nil
		},
		GetMessagesFunc: func(_ context.Context, _ string, opts storage.MessageQueryOptions) ([]handmsg.Message, error) {
			if opts.Role == handmsg.RoleTool && opts.Name == "plan_tool" && opts.Order == "desc" {
				return []handmsg.Message{{
					Role:    handmsg.RoleTool,
					Name:    "plan_tool",
					Content: `{"name":"plan_tool","output":"{\"steps\":[{\"id\":\"step-1\",\"content\":\"Do first\",\"status\":\"pending\"},{\"id\":\"step-2\",\"content\":\"Do now\",\"status\":\"in_progress\"},{\"id\":\"step-3\",\"content\":\"Done\",\"status\":\"completed\"},{\"id\":\"step-4\",\"content\":\"Skip\",\"status\":\"cancelled\"}],\"summary\":{\"total\":4,\"pending\":1,\"in_progress\":1,\"completed\":1,\"cancelled\":1},\"active_step_id\":\"step-2\",\"explanation\":\"hydrate\"}"}`,
				}}, nil
			}
			return nil, nil
		},
		AppendMessagesFunc: func(context.Context, string, []handmsg.Message) error {
			return nil
		},
	}, time.Hour, 24*time.Hour)
	require.NoError(t, err)

	env := &mocks.EnvironmentStub{
		ToolRegistry: tools.NewInMemoryRegistry(),
		TraceSession: traceSession,
	}
	turn := NewTurn(
		testSessionConfig(&config.Config{Name: "Test Agent", Models: config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model"}}}),
		client,
		nil,
		manager,
		nil,
		env,
	)

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{Stream: new(false)})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)

	var found bool
	for _, event := range traceSession.Events {
		if event.Type != trace.EvtPlanHydrated {
			continue
		}
		found = true
		payload, ok := event.Payload.(trace.PlanEventPayload)
		require.True(t, ok)
		require.Equal(t, storage.DefaultSessionID, payload.SessionID)
		require.Equal(t, "step-2", payload.ActiveStepID)
		require.Equal(t, "hydrate", payload.Explanation)
		require.Equal(t, trace.PlanSummaryPayload{
			Total:      4,
			Pending:    1,
			InProgress: 1,
			Completed:  1,
			Cancelled:  1,
		}, payload.Summary)
	}
	require.True(t, found)
}

func TestTurn_HydratePlanFromMessagesHandlesNilTurn(t *testing.T) {
	var turn *Turn
	require.False(t, turn.hydratePlanFromMessages([]handmsg.Message{{Role: handmsg.RoleTool, Name: "plan_tool"}}))
}

func TestTurn_HydratePlanFromMessagesHydratesEmptyPlanWhenNoValidToolMessageExists(t *testing.T) {
	env := &mocks.EnvironmentStub{}
	turn := &Turn{env: env, sessionID: storage.DefaultSessionID}

	ok := turn.hydratePlanFromMessages([]handmsg.Message{{Role: handmsg.RoleAssistant, Content: "hello"}})

	require.False(t, ok)
	require.Equal(t, envtypes.Plan{}, env.Plan)
}

func TestTurn_DecodeHydratedPlanPayloadRejectsInvalidStepsEncoding(t *testing.T) {
	plan, ok := decodeHydratedPlanPayload(`{"steps":"bad"}`)

	require.False(t, ok)
	require.Equal(t, envtypes.Plan{}, plan)
}

func TestTurn_DecodeHydratedPlanPayloadRejectsInvalidPlanState(t *testing.T) {
	plan, ok := decodeHydratedPlanPayload(`{"steps":[{"id":"step-1","content":"Work","status":"pending"}]}`)

	require.False(t, ok)
	require.Equal(t, envtypes.Plan{}, plan)
}

func TestTurn_ActiveHydratedPlanStepIDReturnsEmptyWhenNoActiveStepExists(t *testing.T) {
	require.Empty(t, getActiveHydratedPlanStepID(envtypes.Plan{
		Steps: []envtypes.PlanStep{{ID: "step-1", Content: "Done", Status: envtypes.PlanStatusCompleted}},
	}))
}
