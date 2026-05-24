package agent

import (
	"context"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/host"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/state/manager"
	agentprompt "github.com/wandxy/hand/pkg/agent/prompt"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
	agenttool "github.com/wandxy/hand/pkg/agent/tool"
)

// NewTurn keeps the existing Hand constructor while the turn loop moves behind reusable boundaries.
func NewTurn(
	cfg *config.Config,
	modelClient models.Client,
	summaryClient models.Client,
	stateMgr *manager.Manager,
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message,
	runtimeEnv environment.Environment,
) *Turn {
	if summaryClient == nil {
		summaryClient = modelClient
	}
	if invokeToolFn == nil && runtimeEnv != nil {
		invokeToolFn = func(ctx context.Context, env environment.Environment, toolCall models.ToolCall) handmsg.Message {
			return invokeToolWithEnvironment(ctx, env, toolCall, summaryClient, cfg)
		}
	}

	var sessionStore agentsession.Store
	var traceRecorder agentsession.TraceRecorder
	if stateMgr != nil {
		store := host.NewSessionStore(stateMgr)
		sessionStore = store
		traceRecorder = store
	}
	var toolRegistry agenttool.Registry
	var promptProvider agentprompt.Provider
	var traceSessions traceSessionFactory
	var safetyEvents safetyTraceEventSource
	var memoryProviders memoryProviderSource
	var iterationBudgets iterationBudgetFactory
	var plans planStateStore
	var legacyRuntime any
	var toolInvoker any
	if runtimeEnv != nil {
		toolRegistry = host.NewToolRegistry(runtimeEnv, invokeToolFn)
		promptProvider = host.NewPromptProvider(runtimeEnv)
		traceSessions = runtimeEnv
		safetyEvents = runtimeEnv
		memoryProviders = runtimeEnv
		iterationBudgets = runtimeEnv
		plans = runtimeEnv
		legacyRuntime = runtimeEnv
	}
	if invokeToolFn != nil {
		toolInvoker = func(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
			return invokeToolFn(ctx, runtimeEnv, toolCall)
		}
	}

	return NewTurnWithSessionStore(
		cfg,
		modelClient,
		summaryClient,
		stateMgr,
		sessionStore,
		traceRecorder,
		toolRegistry,
		host.ToolPolicyFromEnvironment(runtimeEnv),
		promptProvider,
		traceSessions,
		safetyEvents,
		memoryProviders,
		iterationBudgets,
		plans,
		legacyRuntime,
		toolInvoker,
	)
}
