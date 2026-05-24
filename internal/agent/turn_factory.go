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
	sessionStore := host.NewSessionStore(a.stateMgr)

	return NewTurnWithSessionStore(
		a.cfg,
		a.modelClient,
		a.summaryClient,
		a.stateMgr,
		sessionStore,
		sessionStore,
		invokeToolFn,
		runtimeEnv,
	)
}
