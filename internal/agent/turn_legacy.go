package agent

import (
	"context"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/host"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/state/manager"
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
	if invokeToolFn == nil {
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
	return NewTurnWithSessionStore(
		cfg,
		modelClient,
		summaryClient,
		stateMgr,
		sessionStore,
		traceRecorder,
		nil,
		agenttool.Policy{},
		invokeToolFn,
		runtimeEnv,
	)
}
