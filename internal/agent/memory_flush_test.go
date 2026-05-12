package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/trace"
)

func TestTurn_RunFlushesMemoryBeforeCompaction(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{
		{
			RequiresToolCalls: true,
			ToolCalls: []models.ToolCall{{
				ID:    "call-memory",
				Name:  "memory_extract",
				Input: `{"session_id":"default","offset_start":0,"offset_end":3}`,
			}},
		},
		{OutputText: `{
			"session_summary": "Older work",
			"current_task": "Flush before compaction",
			"discoveries": ["memory flushed"],
			"open_questions": [],
			"next_actions": []
		}`},
		{OutputText: "reply"},
	}}
	registry, calls := memoryFlushTestRegistry(t)
	turn, manager := newTestTurnHarness(t, nil, registry, client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}
	enabled := true
	turn.cfg = testSessionConfig(&config.Config{
		Name:       "Test Agent",
		Models:     config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model", ContextLength: 100}},
		Compaction: config.CompactionConfig{Enabled: &enabled, TriggerPercent: 0.5, WarnPercent: 0.8},
		Memory: config.MemoryConfig{
			Enabled: &enabled,
			Flush: config.FlushMemoryConfig{
				Enabled:         &enabled,
				MaxCalls:        1,
				MaxOutputTokens: 128,
				Timeout:         time.Second,
			},
		},
	})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, memoryFlushTestHistory(10)))

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Len(t, client.Requests, 3)
	require.Contains(t, client.Requests[0].Instructions, "# Pre-Context-Loss Memory Flush")
	require.Contains(t, client.Requests[0].Instructions, "compression")
	require.Equal(t, int64(128), client.Requests[0].MaxOutputTokens)
	require.Equal(t, []string{"memory_extract"}, memoryFlushRequestToolNames(client.Requests[0]))
	require.Len(t, *calls, 1)
	require.Equal(t, "memory_extract", (*calls)[0].Name)
	require.Equal(t, "model", (*calls)[0].Source)
	require.Contains(t, (*calls)[0].Input, `"session_id":"default"`)
	require.Contains(t, (*calls)[0].Input, `"offset_start":0`)

	summary, ok, err := manager.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Older work", summary.SessionSummary)

	eventTypes := memoryFlushTraceEventTypes(traceSession)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushStarted)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushModelRequested)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushWriteRequested)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushCompleted)
	require.Contains(t, eventTypes, trace.EvtContextCompactionRunning)
}

func TestAgent_CompactSessionFlushesMemoryBeforeSummary(t *testing.T) {
	enabled := true
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{
		{
			RequiresToolCalls: true,
			ToolCalls: []models.ToolCall{{
				ID:    "call-memory",
				Name:  "memory_extract",
				Input: `{"session_id":"ses_Z4VxN3E3h5cQH1sYq2k8a","offset_start":0,"offset_end":10}`,
			}},
		},
		{
			OutputText: `{"session_summary":"Manual compact summary","current_task":"t","discoveries":[],"open_questions":[],"next_actions":[]}`,
		},
	}}
	agent := newSessionOpsAgent(t, &config.Config{
		Name: "Test Agent",
		Models: config.ModelsConfig{
			Main: config.MainModelConfig{Name: "test-model", ContextLength: 128000},
		},
		Memory: config.MemoryConfig{
			Enabled: &enabled,
			Flush:   config.FlushMemoryConfig{Enabled: &enabled, MaxCalls: 1, Timeout: time.Second},
		},
	}, client, traceSession)
	registry, calls := memoryFlushTestRegistry(t)
	envStub := agent.env.(*mocks.EnvironmentStub)
	envStub.ToolRegistry = registry
	envStub.Policy = tools.Policy{Capabilities: tools.Capabilities{Memory: true}}

	session, err := agent.CreateSession(context.Background(), "ses_Z4VxN3E3h5cQH1sYq2k8a")
	require.NoError(t, err)
	appendUserMessages(t, agent, session.ID, 10, "message")

	result, err := agent.CompactSession(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, result.SessionID)
	summary, ok, err := agent.stateMgr.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Manual compact summary", summary.SessionSummary)
	require.Len(t, client.Requests, 2)
	require.Contains(t, client.Requests[0].Instructions, "# Pre-Context-Loss Memory Flush")
	require.Contains(t, client.Requests[0].Instructions, "compression")
	require.Len(t, *calls, 1)
	require.Equal(t, "memory_extract", (*calls)[0].Name)

	eventTypes := memoryFlushTraceEventTypes(traceSession)
	flushCompletedIndex := memoryFlushTraceEventIndex(eventTypes, trace.EvtMemoryFlushCompleted)
	summaryRequestedIndex := memoryFlushTraceEventIndex(eventTypes, trace.EvtSummaryRequested)
	require.NotEqual(t, -1, flushCompletedIndex)
	require.NotEqual(t, -1, summaryRequestedIndex)
	require.Less(t, flushCompletedIndex, summaryRequestedIndex)
}

func TestAgent_CloseFlushesCurrentSessionBeforeControlledExit(t *testing.T) {
	enabled := true
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{
		RequiresToolCalls: true,
		ToolCalls: []models.ToolCall{{
			ID:    "call-memory",
			Name:  "memory_extract",
			Input: `{"session_id":"default","offset_start":0,"offset_end":10}`,
		}},
	}}}
	agent := newSessionOpsAgent(t, &config.Config{
		Name: "Test Agent",
		Models: config.ModelsConfig{
			Main: config.MainModelConfig{Name: "test-model", ContextLength: 128000},
		},
		Memory: config.MemoryConfig{
			Enabled: &enabled,
			Flush:   config.FlushMemoryConfig{Enabled: &enabled, MaxCalls: 1, Timeout: time.Second},
		},
	}, client, traceSession)
	registry, calls := memoryFlushTestRegistry(t)
	envStub := agent.env.(*mocks.EnvironmentStub)
	envStub.ToolRegistry = registry
	envStub.Policy = tools.Policy{Capabilities: tools.Capabilities{Memory: true}}

	session, err := agent.stateMgr.Resolve(context.Background(), "")
	require.NoError(t, err)
	appendUserMessages(t, agent, session.ID, 10, "message")

	require.NoError(t, agent.Close())
	require.Len(t, client.Requests, 1)
	require.Contains(t, client.Requests[0].Instructions, "controlled exit")
	require.Len(t, *calls, 1)
	require.Equal(t, "memory_extract", (*calls)[0].Name)
}

func TestAgent_UseSessionFlushesPreviousSessionBeforeReset(t *testing.T) {
	enabled := true
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{
		RequiresToolCalls: true,
		ToolCalls: []models.ToolCall{{
			ID:    "call-memory",
			Name:  "memory_extract",
			Input: `{"session_id":"default","offset_start":0,"offset_end":10}`,
		}},
	}}}
	agent := newSessionOpsAgent(t, &config.Config{
		Name: "Test Agent",
		Models: config.ModelsConfig{
			Main: config.MainModelConfig{Name: "test-model", ContextLength: 128000},
		},
		Memory: config.MemoryConfig{
			Enabled: &enabled,
			Flush:   config.FlushMemoryConfig{Enabled: &enabled, MaxCalls: 1, Timeout: time.Second},
		},
	}, client, traceSession)
	registry, calls := memoryFlushTestRegistry(t)
	envStub := agent.env.(*mocks.EnvironmentStub)
	envStub.ToolRegistry = registry
	envStub.Policy = tools.Policy{Capabilities: tools.Capabilities{Memory: true}}

	current, err := agent.stateMgr.Resolve(context.Background(), "")
	require.NoError(t, err)
	appendUserMessages(t, agent, current.ID, 10, "message")
	target, err := agent.CreateSession(context.Background(), "ses_Z4VxN3E3h5cQH1sYq2k8b")
	require.NoError(t, err)

	require.NoError(t, agent.UseSession(context.Background(), target.ID))
	require.Len(t, client.Requests, 1)
	require.Contains(t, client.Requests[0].Instructions, "session reset")
	require.Len(t, *calls, 1)
	require.Equal(t, "memory_extract", (*calls)[0].Name)
}

func TestTurn_RunContinuesCompactionWhenMemoryFlushFails(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{
		Errors: []error{context.DeadlineExceeded},
		Responses: []*models.Response{
			{},
			{OutputText: `{
				"session_summary": "Older work",
				"current_task": "Continue after flush timeout",
				"discoveries": [],
				"open_questions": [],
				"next_actions": []
			}`},
			{OutputText: "reply"},
		},
	}
	registry, calls := memoryFlushTestRegistry(t)
	turn, manager := newTestTurnHarness(t, nil, registry, client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}
	enabled := true
	turn.cfg = testSessionConfig(&config.Config{
		Name:       "Test Agent",
		Models:     config.ModelsConfig{Main: config.MainModelConfig{Name: "test-model", ContextLength: 100}},
		Compaction: config.CompactionConfig{Enabled: &enabled, TriggerPercent: 0.5, WarnPercent: 0.8},
		Memory: config.MemoryConfig{
			Enabled: &enabled,
			Flush:   config.FlushMemoryConfig{Enabled: &enabled, MaxCalls: 1, Timeout: time.Second},
		},
	})

	session, err := manager.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NoError(t, manager.AppendMessages(context.Background(), session.ID, memoryFlushTestHistory(10)))

	reply, err := turn.Run(context.Background(), "hello", RespondOptions{})
	require.NoError(t, err)
	require.Equal(t, "reply", reply)
	require.Empty(t, *calls)

	summary, ok, err := manager.GetSummary(context.Background(), session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Older work", summary.SessionSummary)

	eventTypes := memoryFlushTraceEventTypes(traceSession)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushTimeout)
	require.Contains(t, eventTypes, trace.EvtContextCompactionSucceeded)
}

func TestTurn_MaybeFlushMemoryBeforeCompactionSkipsWhenGateIsClosed(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	turn := &Turn{
		cfg: testSessionConfig(&config.Config{
			Memory: config.MemoryConfig{Enabled: new(false)},
		}),
		modelClient: &mocks.ModelClientStub{},
		env: &mocks.EnvironmentStub{
			ToolRegistry: tools.NewInMemoryRegistry(),
		},
	}

	turn.maybeFlushMemoryBeforeCompaction(context.Background(), models.Request{Instructions: strings.Repeat("a", 300)}, traceSession)

	require.Empty(t, traceSession.Events)
}

func TestTurn_ShouldFlushMemoryBeforeCompactionHonorsConfigAndThreshold(t *testing.T) {
	enabled := true
	disabled := false
	turn := &Turn{
		cfg: testSessionConfig(&config.Config{
			Models:     config.ModelsConfig{Main: config.MainModelConfig{ContextLength: 100}},
			Compaction: config.CompactionConfig{Enabled: &enabled, TriggerPercent: 0.5},
			Memory: config.MemoryConfig{
				Enabled: &enabled,
				Flush:   config.FlushMemoryConfig{Enabled: &enabled},
			},
		}),
		modelClient: &mocks.ModelClientStub{},
		env: &mocks.EnvironmentStub{
			ToolRegistry: tools.NewInMemoryRegistry(),
		},
	}

	request := models.Request{Instructions: strings.Repeat("a", 300)}
	require.True(t, turn.shouldFlushMemoryBeforeCompaction(request))

	turn.cfg.Memory.Flush.Enabled = &disabled
	require.False(t, turn.shouldFlushMemoryBeforeCompaction(request))

	turn.cfg.Memory.Flush.Enabled = &enabled
	turn.cfg.Memory.Enabled = &disabled
	require.False(t, turn.shouldFlushMemoryBeforeCompaction(request))

	turn.cfg.Memory.Enabled = &enabled
	request.Instructions = "small"
	require.False(t, turn.shouldFlushMemoryBeforeCompaction(request))

	require.False(t, (*Turn)(nil).shouldFlushMemoryBeforeCompaction(request))
	turn.modelClient = nil
	request.Instructions = strings.Repeat("a", 300)
	require.False(t, turn.shouldFlushMemoryBeforeCompaction(request))
	turn.modelClient = &mocks.ModelClientStub{}
	turn.env = &mocks.EnvironmentStub{}
	require.False(t, turn.shouldFlushMemoryBeforeCompaction(request))
}

func TestAgent_ShouldFlushMemoryBeforeContextLossHonorsDependenciesAndConfig(t *testing.T) {
	enabled := true
	disabled := false
	agent := &Agent{
		cfg:         testSessionConfig(&config.Config{Memory: config.MemoryConfig{Enabled: &enabled, Flush: config.FlushMemoryConfig{Enabled: &enabled}}}),
		modelClient: &mocks.ModelClientStub{},
		stateMgr:    mustNewStateManager(t),
		env:         &mocks.EnvironmentStub{ToolRegistry: tools.NewInMemoryRegistry()},
		initialized: true,
	}

	require.True(t, agent.shouldFlushMemoryBeforeContextLoss())
	require.False(t, (*Agent)(nil).shouldFlushMemoryBeforeContextLoss())

	agent.cfg.Memory.Flush.Enabled = &disabled
	require.False(t, agent.shouldFlushMemoryBeforeContextLoss())

	agent.cfg.Memory.Flush.Enabled = &enabled
	agent.modelClient = nil
	require.False(t, agent.shouldFlushMemoryBeforeContextLoss())

	agent.modelClient = &mocks.ModelClientStub{}
	agent.env = nil
	require.False(t, agent.shouldFlushMemoryBeforeContextLoss())
}

func TestAgent_MaybeFlushMemoryBeforeContextLossRecordsLoadAndFlushFailures(t *testing.T) {
	enabled := true
	traceSession := &mocks.TraceSessionStub{}
	agent := &Agent{
		cfg:         testSessionConfig(&config.Config{Memory: config.MemoryConfig{Enabled: &enabled, Flush: config.FlushMemoryConfig{Enabled: &enabled}}}),
		modelClient: &mocks.ModelClientStub{},
		stateMgr:    mustNewStateManager(t),
		env: &mocks.EnvironmentStub{
			ToolRegistry: tools.NewInMemoryRegistry(),
		},
		initialized: true,
	}

	agent.maybeFlushMemoryBeforeContextLoss(context.Background(), "invalid-session", "reset", traceSession)
	require.Equal(t, []string{trace.EvtMemoryFlushFailed}, memoryFlushTraceEventTypes(traceSession))

	traceSession = &mocks.TraceSessionStub{}
	agent.env = &mocks.EnvironmentStub{
		ToolRegistry: &mocks.ToolRegistryStub{ResolveErr: errors.New("resolve failed")},
	}
	agent.maybeFlushMemoryBeforeContextLoss(context.Background(), storage.DefaultSessionID, "reset", traceSession)
	require.Equal(t, []string{trace.EvtMemoryFlushFailed}, memoryFlushTraceEventTypes(traceSession))
}

func TestAgent_MaybeFlushMemoryBeforeContextLossSkipsWhenDisabled(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	agent := &Agent{}

	agent.maybeFlushMemoryBeforeContextLoss(context.Background(), storage.DefaultSessionID, "reset", traceSession)

	require.Empty(t, traceSession.Events)
}

func TestTurn_FlushMemoryBeforeContextLossSkipsWhenNoSupportedTools(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	turn := &Turn{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent"}),
		modelClient: &mocks.ModelClientStub{},
		env: &mocks.EnvironmentStub{
			ToolRegistry: tools.NewInMemoryRegistry(),
		},
	}

	err := turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.NoError(t, err)
	require.Equal(t, []string{trace.EvtMemoryFlushSkipped}, memoryFlushTraceEventTypes(traceSession))
}

func TestTurn_FlushMemoryBeforeContextLossReturnsRegistryErrors(t *testing.T) {
	turn := &Turn{
		cfg:         testSessionConfig(&config.Config{Name: "Test Agent"}),
		modelClient: &mocks.ModelClientStub{},
		env: &mocks.EnvironmentStub{
			ToolRegistry: &mocks.ToolRegistryStub{ResolveErr: errors.New("resolve failed")},
		},
	}

	err := turn.flushMemoryBeforeContextLoss(context.Background(), "reset", trace.NoopSession())
	require.EqualError(t, err, "resolve failed")
}

func TestTurn_FlushMemoryBeforeContextLossValidatesReceiverContextAndResponse(t *testing.T) {
	err := (*Turn)(nil).flushMemoryBeforeContextLoss(context.Background(), "reset", trace.NoopSession())
	require.EqualError(t, err, "turn is required")

	traceSession := &mocks.TraceSessionStub{}
	registry, _ := memoryFlushTestRegistry(t)
	turn, _ := newTestTurnHarness(t, nil, registry, &mocks.ModelClientStub{Responses: []*models.Response{nil}})
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}
	err = turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.EqualError(t, err, "model response is required")

	turn.modelClient = &mocks.ModelClientStub{Responses: []*models.Response{{RequiresToolCalls: true}}}
	err = turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.EqualError(t, err, "memory flush requested tool execution without tool calls")
}

func TestTurn_FlushMemoryBeforeContextLossHandlesCanceledContext(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	registry, _ := memoryFlushTestRegistry(t)
	turn, _ := newTestTurnHarness(t, nil, registry, &mocks.ModelClientStub{})
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := turn.flushMemoryBeforeContextLoss(ctx, "reset", traceSession)

	require.ErrorIs(t, err, context.Canceled)
}

func TestTurn_FlushMemoryBeforeContextLossHandlesUnsupportedAndBoundedToolCalls(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	registry, calls := memoryFlushTestRegistry(t)
	turn, _ := newTestTurnHarness(t, nil, registry, &mocks.ModelClientStub{Responses: []*models.Response{{
		RequiresToolCalls: true,
		ToolCalls: []models.ToolCall{
			{ID: "call-time", Name: "time", Input: "{}"},
			{ID: "call-memory", Name: "memory_extract", Input: "{}"},
		},
	}}})
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}
	turn.cfg.Memory.Flush.MaxCalls = 1

	err := turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.NoError(t, err)
	require.Empty(t, *calls)

	eventTypes := memoryFlushTraceEventTypes(traceSession)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushSkipped)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushCompleted)
	completed := traceSession.Events[memoryFlushTraceEventIndex(eventTypes, trace.EvtMemoryFlushCompleted)]
	require.Equal(t, "bounded", completed.Payload.(map[string]any)["status"])
}

func TestTurn_FlushMemoryBeforeContextLossReturnsAssistantAndToolMessageNormalizationErrors(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	registry, _ := memoryFlushTestRegistry(t)
	turn, _ := newTestTurnHarness(t, nil, registry, &mocks.ModelClientStub{Responses: []*models.Response{{
		RequiresToolCalls: true,
		ToolCalls:         []models.ToolCall{{Name: "memory_extract", Input: "{}"}},
	}}})
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}

	err := turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.EqualError(t, err, "tool call id is required")

	turn.modelClient = &mocks.ModelClientStub{Responses: []*models.Response{{
		RequiresToolCalls: true,
		ToolCalls:         []models.ToolCall{{ID: "call-memory", Name: "memory_extract", Input: "{}"}},
	}}}
	turn.invokeToolFn = func(context.Context, environment.Environment, models.ToolCall) handmsg.Message {
		return handmsg.Message{Role: handmsg.RoleTool, Name: "memory_extract", Content: "{}"}
	}
	err = turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.EqualError(t, err, "tool call id is required")
}

func TestTurn_FlushMemoryBeforeContextLossRecordsNoOpCompletion(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	client := &mocks.ModelClientStub{Responses: []*models.Response{{
		OutputText: "no durable memory to flush",
	}}}
	registry, calls := memoryFlushTestRegistry(t)
	turn, _ := newTestTurnHarness(t, nil, registry, client)
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}

	err := turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.NoError(t, err)
	require.Empty(t, *calls)

	eventTypes := memoryFlushTraceEventTypes(traceSession)
	require.Contains(t, eventTypes, trace.EvtMemoryFlushCompleted)
	completed := traceSession.Events[memoryFlushTraceEventIndex(eventTypes, trace.EvtMemoryFlushCompleted)]
	require.Equal(t, "no_op", completed.Payload.(map[string]any)["status"])
}

func TestTurn_FlushMemoryBeforeContextLossUsesDefaultConfigWhenMissing(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}
	registry, calls := memoryFlushTestRegistry(t)
	turn, _ := newTestTurnHarness(t, nil, registry, &mocks.ModelClientStub{Responses: []*models.Response{{
		OutputText: "no durable memory to flush",
	}}})
	turn.cfg = nil
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		TraceSession: traceSession,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}

	err := turn.flushMemoryBeforeContextLoss(context.Background(), "reset", traceSession)
	require.NoError(t, err)
	require.Empty(t, *calls)
}

func TestTurn_InvokeFlushToolHandlesCustomInvokerAndToolFailures(t *testing.T) {
	custom := handmsg.Message{Role: handmsg.RoleTool, Name: "memory_extract", ToolCallID: "call-memory", Content: "{}"}
	turn := &Turn{
		invokeToolFn: func(context.Context, environment.Environment, models.ToolCall) handmsg.Message {
			return custom
		},
	}
	require.Equal(t, custom, turn.invokeFlushTool(context.Background(), models.ToolCall{ID: "call-memory", Name: "memory_extract"}))

	turn = &Turn{}
	message := turn.invokeFlushTool(context.Background(), models.ToolCall{ID: "call-memory", Name: "memory_extract"})
	require.Contains(t, message.Content, "tool registry is required")

	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: &mocks.ToolRegistryStub{Err: errors.New("invoke failed")},
	}
	message = turn.invokeFlushTool(context.Background(), models.ToolCall{ID: "call-memory", Name: "memory_extract"})
	require.Contains(t, message.Content, "invoke failed")

	registry := tools.NewInMemoryRegistry()
	require.NoError(t, registry.Register(tools.Definition{
		Name:     "memory_extract",
		Requires: tools.Capabilities{Memory: true},
		Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
			return tools.Result{Output: `{"ok":true}`, Error: "tool_error: denied"}, errors.New("invoke failed")
		}),
	}))
	turn.env = &mocks.EnvironmentStub{
		ToolRegistry: registry,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}
	message = turn.invokeFlushTool(context.Background(), models.ToolCall{ID: "call-memory", Name: "memory_extract"})
	require.Contains(t, message.Content, "tool_invocation_failed")
	require.Contains(t, message.Content, `"output"`)
}

func TestTurn_InvokeFlushToolHandlesMarshalError(t *testing.T) {
	previous := jsonMarshal
	t.Cleanup(func() {
		jsonMarshal = previous
	})
	jsonMarshal = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	registry, _ := memoryFlushTestRegistry(t)
	turn := &Turn{env: &mocks.EnvironmentStub{
		ToolRegistry: registry,
		Policy:       tools.Policy{Capabilities: tools.Capabilities{Memory: true}},
	}}

	message := turn.invokeFlushTool(context.Background(), models.ToolCall{ID: "call-memory", Name: "memory_extract"})

	require.Contains(t, message.Content, "marshal failed")
}

func TestTurn_AvailableMemoryFlushToolDefinitionsHandlesMissingRuntime(t *testing.T) {
	definitions, err := (*Turn)(nil).availableMemoryFlushToolDefinitions()
	require.NoError(t, err)
	require.Nil(t, definitions)

	turn := &Turn{}
	definitions, err = turn.availableMemoryFlushToolDefinitions()
	require.NoError(t, err)
	require.Nil(t, definitions)
}

func TestRecordMemoryFlushFailureSkipsNilError(t *testing.T) {
	traceSession := &mocks.TraceSessionStub{}

	recordMemoryFlushFailure(traceSession, "reset", nil)

	require.Empty(t, traceSession.Events)
}

func memoryFlushTestRegistry(t *testing.T) (*tools.InMemoryRegistry, *[]tools.Call) {
	t.Helper()

	calls := make([]tools.Call, 0)
	registry := tools.NewInMemoryRegistry()
	require.NoError(t, registry.Register(tools.Definition{
		Name:        "memory_extract",
		Description: "Extract memory.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Memory: true},
		InputSchema: map[string]any{"type": "object"},
		Handler: tools.HandlerFunc(func(_ context.Context, call tools.Call) (tools.Result, error) {
			calls = append(calls, call)
			return tools.Result{Output: `{"write_count":1}`}, nil
		}),
	}))
	require.NoError(t, registry.Register(tools.Definition{
		Name:        "time",
		Description: "Ignored non-memory tool.",
		Groups:      []string{"core"},
		InputSchema: map[string]any{"type": "object"},
		Handler: tools.HandlerFunc(func(context.Context, tools.Call) (tools.Result, error) {
			return tools.Result{Output: `{}`}, nil
		}),
	}))

	return registry, &calls
}

func memoryFlushTestHistory(count int) []handmsg.Message {
	history := make([]handmsg.Message, 0, count)
	for range count {
		history = append(history, handmsg.Message{
			Role:      handmsg.RoleUser,
			Content:   strings.Repeat("a", 40),
			CreatedAt: time.Now().UTC(),
		})
	}
	return history
}

func memoryFlushRequestToolNames(request models.Request) []string {
	names := make([]string, 0, len(request.Tools))
	for _, tool := range request.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func memoryFlushTraceEventTypes(traceSession *mocks.TraceSessionStub) []string {
	types := make([]string, 0, len(traceSession.Events))
	for _, event := range traceSession.Events {
		types = append(types, event.Type)
	}
	return types
}

func memoryFlushTraceEventIndex(types []string, eventType string) int {
	for idx, current := range types {
		if current == eventType {
			return idx
		}
	}

	return -1
}
