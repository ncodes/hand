package agent

import (
	"context"

	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/host"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

func (a *Agent) newTurn(
	runtimeEnv environment.Environment,
	invokeToolFn func(context.Context, environment.Environment, models.ToolCall) handmsg.Message,
) *Turn {
	if invokeToolFn == nil {
		invokeToolFn = a.invokeToolWithEnvironment
	}

	sessionStore := host.NewSessionStore(a.stateMgr)
	toolRegistry := host.NewToolRegistry(runtimeEnv, invokeToolFn)

	return NewTurnWithSessionStore(
		a.cfg,
		a.modelClient,
		a.summaryClient,
		a.stateMgr,
		sessionStore,
		sessionStore,
		toolRegistry,
		host.ToolPolicyFromEnvironment(runtimeEnv),
		nil,
		runtimeEnv,
	)
}
