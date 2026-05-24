package agent

import (
	"context"

	"github.com/wandxy/hand/internal/environment"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	agenttool "github.com/wandxy/hand/pkg/agent/tool"
)

func (a *Agent) newTurn(
	runtimeEnv environment.Environment,
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message,
) *Turn {
	if invokeToolFn == nil {
		invokeToolFn = a.invokeToolWithEnvironment
	}

	sessionStore := legacySessionStore{manager: a.stateMgr}

	return NewTurnWithSessionStore(
		a.cfg,
		a.modelClient,
		a.summaryClient,
		a.stateMgr,
		sessionStore,
		sessionStore,
		nil,
		agenttool.Policy{},
		legacyPromptProvider{env: runtimeEnv},
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		runtimeEnv,
		func(ctx context.Context, toolCall models.ToolCall) handmsg.Message {
			return invokeToolFn(ctx, runtimeEnv, toolCall)
		},
	)
}
